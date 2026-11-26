package proxy

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/awasilyev/cloud-memstore-proxy/pkg/config"
)

// testPKI mimics a Memorystore per-instance CA: root -> intermediate -> server.
type testPKI struct {
	caBundle string // intermediate + root, as returned by the certificateAuthority API
	server   tls.Certificate
}

func newTestCert(t *testing.T, tmpl, parent *x509.Certificate, parentKey *ecdsa.PrivateKey) (*x509.Certificate, *ecdsa.PrivateKey, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if parent == nil {
		parent, parentKey = tmpl, key
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, &key.PublicKey, parentKey)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert, key, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func newTestPKI(t *testing.T, serverIPs []net.IP) testPKI {
	t.Helper()
	now := time.Now()
	ca := func(serial int64, cn string, pathLen int) *x509.Certificate {
		return &x509.Certificate{
			SerialNumber:          big.NewInt(serial),
			Subject:               pkix.Name{CommonName: cn},
			NotBefore:             now.Add(-time.Hour),
			NotAfter:              now.Add(time.Hour),
			IsCA:                  true,
			BasicConstraintsValid: true,
			MaxPathLen:            pathLen,
			MaxPathLenZero:        pathLen == 0,
			KeyUsage:              x509.KeyUsageCertSign,
		}
	}
	root, rootKey, rootPEM := newTestCert(t, ca(1, "Google-Root-CA-test", 1), nil, nil)
	inter, interKey, interPEM := newTestCert(t, ca(2, "CAS-Managed CA-test", 0), root, rootKey)
	_, serverKey, serverPEM := newTestCert(t, &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: "memorystore"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  serverIPs,
	}, inter, interKey)

	keyDER, err := x509.MarshalECPrivateKey(serverKey)
	if err != nil {
		t.Fatal(err)
	}
	server, err := tls.X509KeyPair(append(serverPEM, interPEM...), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
	if err != nil {
		t.Fatal(err)
	}
	return testPKI{caBundle: string(interPEM) + string(rootPEM), server: server}
}

// startTLSServer accepts connections and completes the TLS handshake. Returns host:port.
func startTLSServer(t *testing.T, cert tls.Certificate) string {
	t.Helper()
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_ = conn.(*tls.Conn).Handshake()
			}()
		}
	}()
	return ln.Addr().String()
}

func TestTLSVerification(t *testing.T) {
	localhost := []net.IP{net.ParseIP("127.0.0.1")}

	tests := []struct {
		name          string
		serverIPs     []net.IP
		perInstanceCA bool
		otherCA       bool
		skipVerify    bool
		wantErr       bool
	}{
		{name: "per-instance CA, no IP SAN: chain only", perInstanceCA: true},
		{name: "per-instance CA, IP SAN", serverIPs: localhost, perInstanceCA: true},
		{name: "shared CA, IP SAN", serverIPs: localhost},
		{name: "shared CA, no IP SAN: rejected", wantErr: true},
		{name: "untrusted CA: rejected", perInstanceCA: true, otherCA: true, wantErr: true},
		{name: "untrusted CA with skip-verify", perInstanceCA: true, otherCA: true, skipVerify: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newTestPKI(t, tt.serverIPs)
			addr := startTLSServer(t, p.server)

			ca := p.caBundle
			if tt.otherCA {
				ca = newTestPKI(t, localhost).caBundle
			}

			m := NewManager(&config.Config{})
			if err := m.SetTLSConfig(ca, tt.perInstanceCA, tt.skipVerify); err != nil {
				t.Fatalf("SetTLSConfig: %v", err)
			}

			host, _, _ := net.SplitHostPort(addr)
			conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", addr, m.tlsConfigFor(host))
			if conn != nil {
				conn.Close()
			}
			if tt.wantErr && err == nil {
				t.Fatal("expected handshake error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected handshake error: %v", err)
			}
		})
	}
}

func TestSetTLSConfig_RequiresCA(t *testing.T) {
	m := NewManager(&config.Config{})

	if err := m.SetTLSConfig("", true, false); err == nil {
		t.Fatal("expected error without CA when verification is enabled")
	}
	if err := m.SetTLSConfig("not a certificate", true, false); err == nil {
		t.Fatal("expected error for unparsable CA")
	}
	if err := m.SetTLSConfig("", true, true); err != nil {
		t.Fatalf("skip-verify without CA should be allowed: %v", err)
	}
}
