package monitor

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/paulorcv/ps4-package-installer-queue/internal/config"
)

// legacyNAS emulates the SSH server of an old WD My Cloud (Debian 7,
// OpenSSH 6.0): it only offers an ssh-rsa host key, only the
// diffie-hellman-group14-sha1 key exchange, and only accepts ssh-rsa
// signatures for public key auth. Modern OpenSSH clients need
// "-o HostKeyAlgorithms=+ssh-rsa -o PubkeyAcceptedAlgorithms=+ssh-rsa" for it.
type legacyNAS struct {
	addr    string
	port    int
	hostKey ssh.PublicKey
	logLine string
}

func startLegacyNAS(t *testing.T, password string, authorized ssh.PublicKey) *legacyNAS {
	t.Helper()
	hostSigner := rsaSigner(t)
	legacyHost, err := ssh.NewSignerWithAlgorithms(hostSigner.(ssh.AlgorithmSigner), []string{ssh.KeyAlgoRSA})
	if err != nil {
		t.Fatal(err)
	}

	cfg := &ssh.ServerConfig{
		Config:                  ssh.Config{KeyExchanges: []string{"diffie-hellman-group14-sha1"}},
		PublicKeyAuthAlgorithms: []string{ssh.KeyAlgoRSA},
		PasswordCallback: func(c ssh.ConnMetadata, pw []byte) (*ssh.Permissions, error) {
			if password != "" && c.User() == "root" && string(pw) == password {
				return nil, nil
			}
			return nil, io.EOF
		},
		PublicKeyCallback: func(c ssh.ConnMetadata, k ssh.PublicKey) (*ssh.Permissions, error) {
			if authorized != nil && bytes.Equal(k.Marshal(), authorized.Marshal()) {
				return nil, nil
			}
			return nil, io.EOF
		},
	}
	cfg.AddHostKey(legacyHost)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })

	nas := &legacyNAS{
		addr:    ln.Addr().String(),
		port:    ln.Addr().(*net.TCPAddr).Port,
		hostKey: legacyHost.PublicKey(),
		logLine: `192.168.0.104 [02/Nov/2026:10:30:01 -0300] "GET /GOD%20OF%20WAR/game.pkg HTTP/1.1" 206 4096`,
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go nas.serve(conn, cfg)
		}
	}()
	return nas
}

func (n *legacyNAS) serve(conn net.Conn, cfg *ssh.ServerConfig) {
	_, chans, reqs, err := ssh.NewServerConn(conn, cfg)
	if err != nil {
		conn.Close()
		return
	}
	go ssh.DiscardRequests(reqs)
	for nc := range chans {
		if nc.ChannelType() != "session" {
			nc.Reject(ssh.UnknownChannelType, "only sessions")
			continue
		}
		ch, chReqs, err := nc.Accept()
		if err != nil {
			continue
		}
		go n.session(ch, chReqs)
	}
}

func (n *legacyNAS) session(ch ssh.Channel, reqs <-chan *ssh.Request) {
	defer ch.Close()
	for req := range reqs {
		if req.Type != "exec" {
			req.Reply(false, nil)
			continue
		}
		var exec struct{ Command string }
		ssh.Unmarshal(req.Payload, &exec)
		req.Reply(true, nil)

		switch {
		case strings.HasPrefix(exec.Command, "tail -n 0 -F '/var/log/apache2/pkg-access.log'"):
			io.WriteString(ch, "tail: arquivo truncado\n"+n.logLine+"\n")
			// Like the real `tail -F`, ignore stdin EOF (the Go client closes
			// stdin right away) and keep following until the channel closes.
			for range reqs {
			}
		case exec.Command == "cat /proc/net/tcp":
			io.WriteString(ch, procTCP)
			ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
		default:
			ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{127}))
		}
		return
	}
}

func rsaSigner(t *testing.T) ssh.Signer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	s, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func writeKnownHosts(t *testing.T, addr string, key ssh.PublicKey) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "known_hosts")
	line := knownhosts.Line([]string{knownhosts.Normalize(addr)}, key) + "\n"
	if err := os.WriteFile(p, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func waitConnected(t *testing.T, w *Watcher) {
	t.Helper()
	timeout := time.After(10 * time.Second)
	for {
		select {
		case s := <-w.Status():
			if s.Connected {
				return
			}
		case <-timeout:
			t.Fatal("watcher never connected")
		}
	}
}

func checkWatcher(t *testing.T, w *Watcher) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)
	waitConnected(t, w)

	select {
	case ev := <-w.Events():
		if ev.Host != "192.168.0.104" || ev.Path != "/GOD OF WAR/game.pkg" || ev.Bytes != 4096 {
			t.Errorf("event = %+v", ev)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no log event received")
	}

	n, err := w.Connections(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("connections = %d, want 1", n)
	}
}

func TestWatcherLegacyNASWithPassword(t *testing.T) {
	nas := startLegacyNAS(t, "secret", nil)
	w := New(config.SSH{
		Host: "127.0.0.1", Port: nas.port, User: "root", Password: "secret",
		KnownHosts: writeKnownHosts(t, nas.addr, nas.hostKey),
	}, "/var/log/apache2/pkg-access.log", 8080, "192.168.0.104")
	checkWatcher(t, w)
}

func TestWatcherLegacyNASWithRSAKey(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := ssh.NewPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	keyFile := filepath.Join(t.TempDir(), "id_rsa")
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if err := os.WriteFile(keyFile, pemBytes, 0o600); err != nil {
		t.Fatal(err)
	}

	nas := startLegacyNAS(t, "", pub)
	w := New(config.SSH{
		Host: "127.0.0.1", Port: nas.port, User: "root", KeyFile: keyFile,
		KnownHosts: writeKnownHosts(t, nas.addr, nas.hostKey),
	}, "/var/log/apache2/pkg-access.log", 8080, "192.168.0.104")
	checkWatcher(t, w)
}

func TestWatcherRejectsUnknownHostKey(t *testing.T) {
	nas := startLegacyNAS(t, "secret", nil)
	other := rsaSigner(t).PublicKey() // known_hosts has a different key
	w := New(config.SSH{
		Host: "127.0.0.1", Port: nas.port, User: "root", Password: "secret",
		KnownHosts: writeKnownHosts(t, nas.addr, other),
	}, "/var/log/apache2/pkg-access.log", 8080, "192.168.0.104")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	timeout := time.After(10 * time.Second)
	for {
		select {
		case s := <-w.Status():
			if s.Connected {
				t.Fatal("connected despite a host key mismatch")
			}
			if strings.Contains(s.Message, "key mismatch") {
				return
			}
		case <-timeout:
			t.Fatal("expected a host key mismatch error")
		}
	}
}

// Port numbers in known_hosts use the [host]:port form.
func TestKnownHostsNormalize(t *testing.T) {
	if got := knownhosts.Normalize(net.JoinHostPort("192.168.0.121", strconv.Itoa(22))); got != "192.168.0.121" {
		t.Errorf("port 22 should be bare host, got %q", got)
	}
}
