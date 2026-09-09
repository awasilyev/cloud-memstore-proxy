package proxy

import (
	"strings"
	"testing"
)

// sampleNodeMap mirrors what the Manager builds during cluster discovery:
// backend client "ip:port" -> local proxy listener "ip:port".
func sampleNodeMap() map[string]string {
	return map[string]string{
		"10.0.0.1:6379": "127.0.0.1:6380",
		"10.0.0.2:6379": "127.0.0.1:6379",
		"10.0.0.3:6379": "127.0.0.1:6381",
	}
}

func TestLooksLikeClusterNodesReply(t *testing.T) {
	id := strings.Repeat("a", 40)
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"valid node line", id + " 10.0.0.1:6379@16379 master - 0 0 1 connected", true},
		{"uppercase hex id", strings.Repeat("A", 40) + " 10.0.0.1:6379@16379 master", true},
		{"too short", "abc", false},
		{"40 chars but not hex", strings.Repeat("z", 40) + " x", false},
		{"40 hex but no trailing space", id + "x10.0.0.1", false},
		{"binary-ish payload", "\x00\x05hello world value payload", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := looksLikeClusterNodesReply(tt.in); got != tt.want {
				t.Errorf("looksLikeClusterNodesReply(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestSplitNodeAddress(t *testing.T) {
	tests := []struct {
		name           string
		token          string
		wantClientAddr string
		wantBusPort    string
	}{
		{"ip port bus", "10.0.0.1:6379@16379", "10.0.0.1:6379", "16379"},
		{"with hostname", "10.0.0.1:6379@16379,node1.internal", "10.0.0.1:6379", "16379"},
		{"no bus port", "10.0.0.1:6379", "10.0.0.1:6379", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotAddr, gotBus := splitNodeAddress(tt.token)
			if gotAddr != tt.wantClientAddr || gotBus != tt.wantBusPort {
				t.Errorf("splitNodeAddress(%q) = (%q, %q), want (%q, %q)",
					tt.token, gotAddr, gotBus, tt.wantClientAddr, tt.wantBusPort)
			}
		})
	}
}

func TestRewriteClusterNodes(t *testing.T) {
	// Realistic CLUSTER NODES reply: 3 masters, mixed flags, one with an
	// advertised hostname, each line \n-terminated (including the last).
	reply := "" +
		"07c37dfeb235213a872192d90877d0cd55635b91 10.0.0.1:6379@16379,node1.internal master - 0 1426238317239 2 connected 5461-10922\n" +
		"67ed2db8d677e59ec4a4cefb06858cf2a1a89fa1 10.0.0.2:6379@16379 myself,master - 0 0 1 connected 0-5460\n" +
		"6ec23923021cf3ffec47632106199cb7f496ce01 10.0.0.3:6379@16379 master - 0 1426238316232 4 connected 10923-16383\n"

	v := &RESPValue{Type: BulkString, Str: reply}
	changed := v.RewriteClusterNodes(sampleNodeMap())
	if !changed {
		t.Fatal("expected RewriteClusterNodes to report a change")
	}

	// Backend client addresses must be gone; local listeners must be present.
	for _, backend := range []string{"10.0.0.1:6379", "10.0.0.2:6379", "10.0.0.3:6379"} {
		if strings.Contains(v.Str, backend) {
			t.Errorf("rewritten reply still contains backend address %q:\n%s", backend, v.Str)
		}
	}
	for _, local := range []string{"127.0.0.1:6380@16379", "127.0.0.1:6379@16379", "127.0.0.1:6381@16379"} {
		if !strings.Contains(v.Str, local) {
			t.Errorf("rewritten reply missing local address %q:\n%s", local, v.Str)
		}
	}

	// Advertised hostname must be dropped so clients can't route around the proxy.
	if strings.Contains(v.Str, "node1.internal") {
		t.Errorf("rewritten reply should not contain advertised hostname:\n%s", v.Str)
	}

	// Structure must be preserved: flags, slot ranges, node IDs, trailing newline.
	for _, keep := range []string{
		"07c37dfeb235213a872192d90877d0cd55635b91",
		"myself,master",
		"5461-10922", "0-5460", "10923-16383",
	} {
		if !strings.Contains(v.Str, keep) {
			t.Errorf("rewritten reply lost expected token %q:\n%s", keep, v.Str)
		}
	}
	if !strings.HasSuffix(v.Str, "connected 10923-16383\n") {
		t.Errorf("trailing newline / final line not preserved:\n%q", v.Str)
	}
	if lines := strings.Count(v.Str, "\n"); lines != 3 {
		t.Errorf("expected 3 newline-terminated lines, got %d:\n%s", lines, v.Str)
	}
}

func TestRewriteClusterNodesUnmappedNodeUnchanged(t *testing.T) {
	// A node absent from nodeMap (e.g. discovery missed it) must be left as-is
	// rather than corrupted.
	reply := "07c37dfeb235213a872192d90877d0cd55635b91 10.9.9.9:6379@16379 master - 0 0 2 connected 0-16383\n"
	v := &RESPValue{Type: BulkString, Str: reply}

	if v.RewriteClusterNodes(sampleNodeMap()) {
		t.Error("expected no change for an unmapped node")
	}
	if v.Str != reply {
		t.Errorf("unmapped reply was modified:\ngot  %q\nwant %q", v.Str, reply)
	}
}

func TestRewriteClusterNodesIgnoresNonClusterBulkStrings(t *testing.T) {
	// Ordinary bulk-string replies (GET values, DUMP payloads) must never be
	// treated as CLUSTER NODES output, even if a nodeMap is present.
	cases := []string{
		"OK",
		"some user value that happens to mention 10.0.0.1:6379",
		"\x00\x0eRedisDumpBinary\xff\x00\x01",
	}
	for _, s := range cases {
		v := &RESPValue{Type: BulkString, Str: s}
		if v.RewriteClusterNodes(sampleNodeMap()) {
			t.Errorf("RewriteClusterNodes should ignore non-cluster bulk string %q", s)
		}
		if v.Str != s {
			t.Errorf("non-cluster bulk string was modified: got %q want %q", v.Str, s)
		}
	}
}

func TestRewriteClusterNodesWrongType(t *testing.T) {
	// Non-bulk-string values (arrays, simple strings, nulls) must be no-ops.
	values := []*RESPValue{
		{Type: Array, Array: []RESPValue{{Type: BulkString, Str: "x"}}},
		{Type: SimpleString, Str: "OK"},
		{Type: BulkString, Null: true},
	}
	for i, v := range values {
		if v.RewriteClusterNodes(sampleNodeMap()) {
			t.Errorf("case %d: expected no rewrite for type %c (null=%v)", i, byte(v.Type), v.Null)
		}
	}
}

// TestRewriteClusterNodesWireRoundTrip verifies the rewritten value survives a
// full parse -> rewrite -> serialize -> parse cycle, i.e. it stays a valid RESP
// bulk string that a client can read back.
func TestRewriteClusterNodesWireRoundTrip(t *testing.T) {
	reply := "07c37dfeb235213a872192d90877d0cd55635b91 10.0.0.1:6379@16379 master - 0 0 2 connected 0-16383\n"
	wire := (&RESPValue{Type: BulkString, Str: reply}).Serialize()

	parsed, err := NewRESPReader(strings.NewReader(string(wire))).ReadValue()
	if err != nil {
		t.Fatalf("failed to parse serialized reply: %v", err)
	}
	if !parsed.RewriteClusterNodes(sampleNodeMap()) {
		t.Fatal("expected rewrite on parsed reply")
	}

	reparsed, err := NewRESPReader(strings.NewReader(string(parsed.Serialize()))).ReadValue()
	if err != nil {
		t.Fatalf("failed to re-parse rewritten reply: %v", err)
	}
	if reparsed.Type != BulkString {
		t.Fatalf("expected bulk string after round trip, got type %c", byte(reparsed.Type))
	}
	if !strings.Contains(reparsed.Str, "127.0.0.1:6380@16379") {
		t.Errorf("round-tripped reply missing rewritten address:\n%s", reparsed.Str)
	}
	if strings.Contains(reparsed.Str, "10.0.0.1:6379") {
		t.Errorf("round-tripped reply still contains backend address:\n%s", reparsed.Str)
	}
}
