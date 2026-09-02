// Package conntrack parses Linux /proc/net/nf_conntrack entries into structured Flow records.
//
// The nf_conntrack text format is not strictly positional across protocols/kernel versions
// (e.g. UDP entries omit the TCP state field), so this parser scans whitespace-separated tokens
// looking for known key=value pairs (src=, dst=, sport=, dport=, packets=, bytes=) rather than
// relying on fixed field offsets. Each of those keys appears twice per line — once for the
// original direction, once for the reply direction — so the second occurrence of each key is
// captured separately.
//
// Byte/packet counts (packets=, bytes=) are only present in the proc output when the kernel has
// conntrack accounting enabled (sysctl net.netfilter.nf_conntrack_acct=1). When absent, the
// corresponding Flow fields are set to -1 so callers can distinguish "zero bytes" from "unknown".
package conntrack

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// knownProtocols are the l4 protocol names conntrack emits as a bare token (not a key=value
// pair). Restricting to this set avoids misclassifying other bare tokens (e.g. connection state
// words like ESTABLISHED, or the l3 protocol name "ipv4") as the protocol.
var knownProtocols = map[string]bool{
	"tcp":     true,
	"udp":     true,
	"udplite": true,
	"icmp":    true,
	"sctp":    true,
	"dccp":    true,
}

// Flow is a single parsed conntrack entry, describing one bidirectional connection.
type Flow struct {
	Protocol string

	OrigSrcIP   string
	OrigDstIP   string
	OrigSrcPort int
	OrigDstPort int

	ReplySrcIP   string
	ReplyDstIP   string
	ReplySrcPort int
	ReplyDstPort int

	// OrigBytes/ReplyBytes/OrigPackets/ReplyPackets are -1 when the kernel did not report
	// accounting data for this entry (conntrack accounting not enabled).
	OrigBytes    int64
	ReplyBytes   int64
	OrigPackets  int64
	ReplyPackets int64
}

// HasByteAccounting reports whether this flow carries usable byte-count data in either
// direction.
func (f Flow) HasByteAccounting() bool {
	return f.OrigBytes >= 0 || f.ReplyBytes >= 0
}

// ParseLine parses a single nf_conntrack line. ok is false for blank lines or lines that don't
// resemble a valid conntrack entry (missing protocol or original src/dst).
func ParseLine(line string) (f Flow, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return Flow{}, false
	}

	f = Flow{OrigBytes: -1, ReplyBytes: -1, OrigPackets: -1, ReplyPackets: -1}
	occurrence := make(map[string]int, 6)

	for _, tok := range strings.Fields(line) {
		if f.Protocol == "" && knownProtocols[tok] {
			f.Protocol = tok
			continue
		}

		eq := strings.IndexByte(tok, '=')
		if eq < 0 {
			continue // flags like [ASSURED], state words, l3proto name, etc.
		}
		key, val := tok[:eq], tok[eq+1:]
		n := occurrence[key]
		occurrence[key]++

		switch key {
		case "src":
			if n == 0 {
				f.OrigSrcIP = val
			} else {
				f.ReplySrcIP = val
			}
		case "dst":
			if n == 0 {
				f.OrigDstIP = val
			} else {
				f.ReplyDstIP = val
			}
		case "sport":
			if p, err := strconv.Atoi(val); err == nil {
				if n == 0 {
					f.OrigSrcPort = p
				} else {
					f.ReplySrcPort = p
				}
			}
		case "dport":
			if p, err := strconv.Atoi(val); err == nil {
				if n == 0 {
					f.OrigDstPort = p
				} else {
					f.ReplyDstPort = p
				}
			}
		case "bytes":
			if b, err := strconv.ParseInt(val, 10, 64); err == nil {
				if n == 0 {
					f.OrigBytes = b
				} else {
					f.ReplyBytes = b
				}
			}
		case "packets":
			if p, err := strconv.ParseInt(val, 10, 64); err == nil {
				if n == 0 {
					f.OrigPackets = p
				} else {
					f.ReplyPackets = p
				}
			}
		}
	}

	if f.Protocol == "" || f.OrigSrcIP == "" || f.OrigDstIP == "" {
		return Flow{}, false
	}
	return f, true
}

// ParseReader parses every line from r, skipping any that don't parse as valid conntrack
// entries. It does not fail the whole read on a single malformed line — the conntrack table can
// be large and partially inconsistent mid-read.
func ParseReader(r io.Reader) ([]Flow, error) {
	scanner := bufio.NewScanner(r)
	// Lines can be long (secctx=... labels etc.); grow the buffer beyond bufio's 64KB default.
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	var flows []Flow
	for scanner.Scan() {
		if f, ok := ParseLine(scanner.Text()); ok {
			flows = append(flows, f)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("conntrack: scan: %w", err)
	}
	return flows, nil
}
