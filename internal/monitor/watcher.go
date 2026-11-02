// Package monitor watches the NAS over SSH: it follows the Apache access log
// and counts the PS4's open connections to the HTTP server, which together
// tell when the console has finished downloading a package.
package monitor

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/paulorcv/ps4-package-installer-queue/internal/config"
)

// Status reports the state of the SSH connection.
type Status struct {
	Connected bool
	Message   string
}

// Watcher keeps an SSH connection to the NAS and streams log events.
type Watcher struct {
	ssh      config.SSH
	logPath  string
	httpPort int
	ps4IP    string

	events chan Event
	status chan Status

	mu     sync.Mutex
	client *ssh.Client
}

// New creates a watcher. Call Run to start it.
func New(sshCfg config.SSH, accessLog string, httpPort int, ps4IP string) *Watcher {
	return &Watcher{
		ssh:      sshCfg,
		logPath:  accessLog,
		httpPort: httpPort,
		ps4IP:    ps4IP,
		events:   make(chan Event, 256),
		status:   make(chan Status, 16),
	}
}

// Events delivers parsed access log lines.
func (w *Watcher) Events() <-chan Event { return w.events }

// Status delivers connection state changes.
func (w *Watcher) Status() <-chan Status { return w.status }

// Run connects and follows the log until ctx is cancelled, reconnecting with
// backoff whenever the connection drops.
func (w *Watcher) Run(ctx context.Context) {
	backoff := 2 * time.Second
	for {
		w.emit(Status{Message: "conectando ao NAS…"})
		err := w.session(ctx)
		if ctx.Err() != nil {
			w.closeClient()
			return
		}
		w.emit(Status{Message: fmt.Sprintf("SSH: %v (nova tentativa em %s)", err, backoff)})

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < time.Minute {
			backoff *= 2
		}
	}
}

// session runs one connection lifetime: dial, tail the log, keep alive.
func (w *Watcher) session(ctx context.Context) error {
	client, err := w.dial(ctx)
	if err != nil {
		return err
	}
	w.mu.Lock()
	w.client = client
	w.mu.Unlock()
	defer w.closeClient()

	sess, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("abrindo sessão: %w", err)
	}
	defer sess.Close()

	out, err := sess.StdoutPipe()
	if err != nil {
		return err
	}
	cmd := "tail -n 0 -F " + shellQuote(w.logPath) + " 2>/dev/null"
	if err := sess.Start(cmd); err != nil {
		return fmt.Errorf("executando tail: %w", err)
	}
	w.emit(Status{Connected: true, Message: "conectado ao NAS, acompanhando " + w.logPath})

	// Keepalives detect a dead link; closing the client unblocks the scanner.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ctx.Done():
				client.Close()
				return
			case <-t.C:
				if _, _, err := client.SendRequest("keepalive@openssh.com", true, nil); err != nil {
					client.Close()
					return
				}
			}
		}
	}()

	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		ev, ok := ParseLine(sc.Text(), time.Now())
		if !ok {
			continue
		}
		select {
		case w.events <- ev:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("lendo log: %w", err)
	}
	return errors.New("conexão encerrada")
}

// Connections returns how many connections the PS4 has open to the NAS
// HTTP server right now.
func (w *Watcher) Connections(ctx context.Context) (int, error) {
	w.mu.Lock()
	client := w.client
	w.mu.Unlock()
	if client == nil {
		return 0, errors.New("sem conexão SSH com o NAS")
	}

	type result struct {
		out []byte
		err error
	}
	ch := make(chan result, 1)
	go func() {
		sess, err := client.NewSession()
		if err != nil {
			ch <- result{err: err}
			return
		}
		defer sess.Close()
		out, err := sess.Output("cat /proc/net/tcp")
		ch <- result{out, err}
	}()

	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case r := <-ch:
		if r.err != nil {
			return 0, fmt.Errorf("lendo conexões: %w", r.err)
		}
		return CountConnections(string(r.out), w.ps4IP, w.httpPort)
	}
}

func (w *Watcher) dial(ctx context.Context) (*ssh.Client, error) {
	auth, err := authMethods(w.ssh)
	if err != nil {
		return nil, err
	}
	hostKey, err := hostKeyCallback(w.ssh)
	if err != nil {
		return nil, err
	}

	cfg := &ssh.ClientConfig{
		User:            w.ssh.User,
		Auth:            auth,
		HostKeyCallback: hostKey,
		Timeout:         15 * time.Second,
		// Old NAS firmwares (e.g. WD My Cloud on Debian 7) only offer ssh-rsa
		// and ssh-dss host keys, which modern clients disable by default.
		HostKeyAlgorithms: []string{
			ssh.KeyAlgoED25519,
			ssh.KeyAlgoECDSA256,
			ssh.KeyAlgoRSASHA512,
			ssh.KeyAlgoRSASHA256,
			ssh.KeyAlgoRSA,
			ssh.KeyAlgoDSA, //nolint:staticcheck // needed for legacy NAS firmware
		},
		Config: ssh.Config{
			KeyExchanges: []string{
				"curve25519-sha256",
				"curve25519-sha256@libssh.org",
				"ecdh-sha2-nistp256",
				"ecdh-sha2-nistp384",
				"ecdh-sha2-nistp521",
				"diffie-hellman-group14-sha256",
				"diffie-hellman-group14-sha1",
				"diffie-hellman-group1-sha1",
			},
		},
	}

	addr := net.JoinHostPort(w.ssh.Host, strconv.Itoa(w.ssh.Port))
	d := net.Dialer{Timeout: cfg.Timeout}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	c, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return ssh.NewClient(c, chans, reqs), nil
}

func authMethods(c config.SSH) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod
	if c.KeyFile != "" {
		key, err := os.ReadFile(c.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("lendo chave %s: %w", c.KeyFile, err)
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("chave %s: %w", c.KeyFile, err)
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}
	if c.Password != "" {
		pw := c.Password
		methods = append(methods,
			ssh.Password(pw),
			ssh.KeyboardInteractive(func(_, _ string, questions []string, _ []bool) ([]string, error) {
				answers := make([]string, len(questions))
				for i := range answers {
					answers[i] = pw
				}
				return answers, nil
			}),
		)
	}
	if len(methods) == 0 {
		return nil, errors.New("nenhum método de autenticação SSH configurado")
	}
	return methods, nil
}

func hostKeyCallback(c config.SSH) (ssh.HostKeyCallback, error) {
	if c.InsecureIgnoreHostKey {
		return ssh.InsecureIgnoreHostKey(), nil //nolint:gosec // opt-in via config
	}
	cb, err := knownhosts.New(c.KnownHosts)
	if err != nil {
		return nil, fmt.Errorf("known_hosts %s: %w (conecte uma vez com o ssh para registrar o NAS)", c.KnownHosts, err)
	}
	return cb, nil
}

func (w *Watcher) closeClient() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.client != nil {
		w.client.Close()
		w.client = nil
	}
}

func (w *Watcher) emit(s Status) {
	select {
	case w.status <- s:
	default: // drop if the UI is not keeping up; the next status supersedes it
	}
}
