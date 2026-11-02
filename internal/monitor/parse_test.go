package monitor

import (
	"testing"
	"time"
)

func TestParseLine(t *testing.T) {
	now := time.Unix(0, 0)
	cases := []struct {
		name string
		line string
		want Event
		ok   bool
	}{
		{
			name: "compact format",
			line: `192.168.0.104 [02/Nov/2026:10:30:01 -0300] "GET /GOD%20OF%20WAR%201/game.pkg HTTP/1.1" 206 1048576`,
			want: Event{Host: "192.168.0.104", Method: "GET", Path: "/GOD OF WAR 1/game.pkg", Status: 206, Bytes: 1048576},
			ok:   true,
		},
		{
			name: "common log format with query and dash bytes",
			line: `192.168.0.103 - - [02/Nov/2026:10:30:01 -0300] "HEAD /a.pkg?x=1 HTTP/1.1" 200 -`,
			want: Event{Host: "192.168.0.103", Method: "HEAD", Path: "/a.pkg", Status: 200, Bytes: 0},
			ok:   true,
		},
		{
			name: "combined format",
			line: `10.0.0.5 - bob [02/Nov/2026:10:30:01 -0300] "GET /b.pkg HTTP/1.1" 200 512 "-" "Mozilla"`,
			want: Event{Host: "10.0.0.5", Method: "GET", Path: "/b.pkg", Status: 200, Bytes: 512},
			ok:   true,
		},
		{name: "garbage", line: "tail: file truncated", ok: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := ParseLine(c.line, now)
			if ok != c.ok {
				t.Fatalf("ok = %v, want %v", ok, c.ok)
			}
			if !ok {
				return
			}
			c.want.Time = now
			if got != c.want {
				t.Errorf("got %+v\nwant %+v", got, c.want)
			}
		})
	}
}

// procTCP has the PS4 (192.168.0.104 = 6800A8C0) with two connections to
// port 8080 (1F90): one ESTABLISHED, one TIME_WAIT, plus unrelated sockets.
const procTCP = `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 1 1 0 100 0 0 10 0
   1: 7900A8C0:1F90 6800A8C0:C350 01 00000000:00000000 00:00000000 00000000    33        0 2 1 0 20 4 30 10 -1
   2: 7900A8C0:1F90 6800A8C0:C351 06 00000000:00000000 03:00000F3E 00000000     0        0 0 3 0
   3: 7900A8C0:0016 6700A8C0:D431 01 00000000:00000000 02:0009F1AE 00000000     0        0 3 4 0 21 4 18 10 -1
   4: 7900A8C0:1F90 6700A8C0:D432 01 00000000:00000000 00:00000000 00000000    33        0 4 1 0 20 4 30 10 -1
`

func TestCountConnections(t *testing.T) {
	n, err := CountConnections(procTCP, "192.168.0.104", 8080)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("PS4 connections = %d, want 1", n)
	}

	n, _ = CountConnections(procTCP, "192.168.0.103", 8080)
	if n != 1 {
		t.Errorf("PC connections = %d, want 1", n)
	}

	if _, err := CountConnections(procTCP, "not-an-ip", 8080); err == nil {
		t.Error("expected error for invalid IP")
	}
}

func TestShellQuote(t *testing.T) {
	if got := shellQuote("/var/log/it's.log"); got != `'/var/log/it'\''s.log'` {
		t.Errorf("got %s", got)
	}
}
