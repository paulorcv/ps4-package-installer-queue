package monitor

import (
	"encoding/binary"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Event is one finished HTTP request read from the Apache access log.
type Event struct {
	Host   string
	Method string
	Path   string // URL path, percent-decoded, without query string
	Status int
	Bytes  int64
	Time   time.Time // when the line was received
}

// logLineRE accepts both the compact format used by this project
//
//	%h %t "%r" %>s %B
//
// and Apache's common/combined formats
//
//	%h %l %u %t "%r" %>s %b ...
var logLineRE = regexp.MustCompile(`^(\S+) (?:\S+ \S+ )?\[[^\]]*\] "(\S+) (\S+)[^"]*" (\d{3}) (\S+)`)

// ParseLine parses one access log line. ok is false for lines that do not
// look like a request.
func ParseLine(line string, now time.Time) (Event, bool) {
	m := logLineRE.FindStringSubmatch(line)
	if m == nil {
		return Event{}, false
	}
	status, err := strconv.Atoi(m[4])
	if err != nil {
		return Event{}, false
	}
	var n int64
	if m[5] != "-" {
		n, err = strconv.ParseInt(m[5], 10, 64)
		if err != nil {
			return Event{}, false
		}
	}
	p := m[3]
	if i := strings.IndexByte(p, '?'); i >= 0 {
		p = p[:i]
	}
	if dec, err := url.PathUnescape(p); err == nil {
		p = dec
	}
	return Event{Host: m[1], Method: m[2], Path: p, Status: status, Bytes: n, Time: now}, true
}

// tcpEstablished is the state code for ESTABLISHED in /proc/net/tcp.
const tcpEstablished = "01"

// CountConnections counts ESTABLISHED IPv4 connections in the contents of
// /proc/net/tcp whose local port is localPort and remote address is remoteIP.
func CountConnections(procNetTCP string, remoteIP string, localPort int) (int, error) {
	ip := net.ParseIP(remoteIP).To4()
	if ip == nil {
		return 0, fmt.Errorf("IP IPv4 inválido: %q", remoteIP)
	}
	// /proc/net/tcp prints the address as a little-endian 32-bit hex number.
	want := strings.ToUpper(fmt.Sprintf("%08X", binary.LittleEndian.Uint32(ip)))
	wantPort := strings.ToUpper(fmt.Sprintf("%04X", localPort))

	count := 0
	for i, line := range strings.Split(procNetTCP, "\n") {
		if i == 0 {
			continue // header
		}
		f := strings.Fields(line)
		if len(f) < 4 {
			continue
		}
		local, remote, state := f[1], f[2], f[3]
		lp := strings.SplitN(local, ":", 2)
		rp := strings.SplitN(remote, ":", 2)
		if len(lp) != 2 || len(rp) != 2 {
			continue
		}
		if strings.ToUpper(lp[1]) == wantPort && strings.ToUpper(rp[0]) == want && state == tcpEstablished {
			count++
		}
	}
	return count, nil
}

// shellQuote quotes s for a POSIX shell.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
