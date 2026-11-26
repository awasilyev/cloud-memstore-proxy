package discovery

import (
	"encoding/json"
	"strings"
	"testing"
)

const (
	testIntermediate = "-----BEGIN CERTIFICATE-----\nINTERMEDIATE\n-----END CERTIFICATE-----\n"
	testRoot         = "-----BEGIN CERTIFICATE-----\nROOT\n-----END CERTIFICATE-----\n"
)

// Response shape of GET v1/{instance}/certificateAuthority for a GOOGLE_MANAGED_PER_INSTANCE_CA instance
const testCertificateAuthorityResponse = `{
  "name": "projects/p/locations/europe-west1/instances/valkey/certificateAuthority",
  "managedServerCa": {
    "caCerts": [
      {"certificates": [` + "\"-----BEGIN CERTIFICATE-----\\nINTERMEDIATE\\n-----END CERTIFICATE-----\\n\", \"-----BEGIN CERTIFICATE-----\\nROOT\\n-----END CERTIFICATE-----\\n\"" + `]}
    ]
  }
}`

func TestCertificateAuthorityBundle(t *testing.T) {
	var ca CertificateAuthority
	if err := json.Unmarshal([]byte(testCertificateAuthorityResponse), &ca); err != nil {
		t.Fatal(err)
	}

	bundle, err := ca.bundle()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(bundle, "INTERMEDIATE") || !strings.Contains(bundle, "ROOT") {
		t.Fatalf("bundle must contain every certificate, got:\n%s", bundle)
	}
	if got := strings.Count(bundle, "BEGIN CERTIFICATE"); got != 2 {
		t.Fatalf("expected 2 certificates, got %d", got)
	}
}

func TestCertificateAuthorityBundle_Empty(t *testing.T) {
	var ca CertificateAuthority
	if err := json.Unmarshal([]byte(`{"managedServerCa":{}}`), &ca); err != nil {
		t.Fatal(err)
	}
	if _, err := ca.bundle(); err == nil {
		t.Fatal("expected error for response without certificates")
	}
}

func TestJoinCerts(t *testing.T) {
	got := joinCerts([]CertInfo{{Cert: testIntermediate}, {Cert: testRoot}})
	if want := testIntermediate + testRoot; got != want {
		t.Fatalf("joinCerts() = %q, want %q", got, want)
	}
}

func TestValkeyInstanceServerCaMode(t *testing.T) {
	var inst ValKeyInstance
	if err := json.Unmarshal([]byte(`{"transitEncryptionMode":"SERVER_AUTHENTICATION","serverCaMode":"GOOGLE_MANAGED_PER_INSTANCE_CA"}`), &inst); err != nil {
		t.Fatal(err)
	}
	if inst.ServerCaMode != perInstanceCAMode {
		t.Fatalf("ServerCaMode = %q", inst.ServerCaMode)
	}
}
