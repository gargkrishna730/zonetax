package conntrack

import (
	"strings"
	"testing"
)

func TestParseLine_TCPWithAccounting(t *testing.T) {
	line := `ipv4     2 tcp      6 108 ESTABLISHED src=10.42.1.5 dst=10.42.2.9 sport=45678 dport=8080 packets=15 bytes=2340 src=10.42.2.9 dst=10.42.1.5 sport=8080 dport=45678 packets=12 bytes=18900 [ASSURED] mark=0 secctx=system_u:object_r:unlabeled_t:s0 zone=0 use=2`

	f, ok := ParseLine(line)
	if !ok {
		t.Fatalf("ParseLine() ok = false, want true")
	}
	if f.Protocol != "tcp" {
		t.Errorf("Protocol = %q, want tcp", f.Protocol)
	}
	if f.OrigSrcIP != "10.42.1.5" || f.OrigDstIP != "10.42.2.9" {
		t.Errorf("orig src/dst = %s/%s, want 10.42.1.5/10.42.2.9", f.OrigSrcIP, f.OrigDstIP)
	}
	if f.ReplySrcIP != "10.42.2.9" || f.ReplyDstIP != "10.42.1.5" {
		t.Errorf("reply src/dst = %s/%s, want 10.42.2.9/10.42.1.5", f.ReplySrcIP, f.ReplyDstIP)
	}
	if f.OrigSrcPort != 45678 || f.OrigDstPort != 8080 {
		t.Errorf("orig ports = %d/%d, want 45678/8080", f.OrigSrcPort, f.OrigDstPort)
	}
	if f.OrigBytes != 2340 || f.ReplyBytes != 18900 {
		t.Errorf("bytes = %d/%d, want 2340/18900", f.OrigBytes, f.ReplyBytes)
	}
	if f.OrigPackets != 15 || f.ReplyPackets != 12 {
		t.Errorf("packets = %d/%d, want 15/12", f.OrigPackets, f.ReplyPackets)
	}
	if !f.HasByteAccounting() {
		t.Error("HasByteAccounting() = false, want true")
	}
}

func TestParseLine_UDPWithoutAccounting(t *testing.T) {
	// UDP entries have no TCP state field, and here accounting is disabled (no packets=/bytes=).
	line := `ipv4     2 udp      17 29 src=10.42.1.5 dst=10.42.3.7 sport=53124 dport=53 src=10.42.3.7 dst=10.42.1.5 sport=53 dport=53124 mark=0 use=1`

	f, ok := ParseLine(line)
	if !ok {
		t.Fatalf("ParseLine() ok = false, want true")
	}
	if f.Protocol != "udp" {
		t.Errorf("Protocol = %q, want udp", f.Protocol)
	}
	if f.OrigBytes != -1 || f.ReplyBytes != -1 {
		t.Errorf("bytes = %d/%d, want -1/-1 (unknown)", f.OrigBytes, f.ReplyBytes)
	}
	if f.HasByteAccounting() {
		t.Error("HasByteAccounting() = true, want false")
	}
}

func TestParseLine_Invalid(t *testing.T) {
	tests := []string{
		"",
		"   ",
		"some garbage line with no proto or src/dst",
		"ipv4     2 tcp      6 108 ESTABLISHED mark=0 use=2", // protocol but no src/dst
	}
	for _, line := range tests {
		if _, ok := ParseLine(line); ok {
			t.Errorf("ParseLine(%q) ok = true, want false", line)
		}
	}
}

func TestParseReader(t *testing.T) {
	data := strings.Join([]string{
		`ipv4     2 tcp      6 108 ESTABLISHED src=10.42.1.5 dst=10.42.2.9 sport=1 dport=2 packets=1 bytes=100 src=10.42.2.9 dst=10.42.1.5 sport=2 dport=1 packets=1 bytes=90 [ASSURED] mark=0 use=2`,
		``, // blank line should be skipped, not error
		`ipv4     2 udp      17 29 src=10.42.1.5 dst=10.42.3.7 sport=3 dport=4 src=10.42.3.7 dst=10.42.1.5 sport=4 dport=3 mark=0 use=1`,
		`not a valid conntrack line`,
	}, "\n")

	flows, err := ParseReader(strings.NewReader(data))
	if err != nil {
		t.Fatalf("ParseReader() error = %v", err)
	}
	if len(flows) != 2 {
		t.Fatalf("ParseReader() returned %d flows, want 2", len(flows))
	}
	if flows[0].Protocol != "tcp" || flows[1].Protocol != "udp" {
		t.Errorf("unexpected protocols: %q, %q", flows[0].Protocol, flows[1].Protocol)
	}
}
