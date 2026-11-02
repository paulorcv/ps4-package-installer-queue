package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadAppliesDefaultsAndEnv(t *testing.T) {
	t.Setenv(PasswordEnvVar, "from-env")
	p := writeConfig(t, `
source_url: http://192.168.0.121:8080/
ps4:
  host: 192.168.0.104
nas:
  ssh:
    host: 192.168.0.121
    known_hosts: /tmp/kh
queue:
  idle_done: 90s
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.NAS.SSH.Password != "from-env" {
		t.Errorf("password = %q", cfg.NAS.SSH.Password)
	}
	if cfg.PS4.EzRemotePort != 8080 || cfg.NAS.SSH.Port != 22 || cfg.NAS.SSH.User != "root" {
		t.Errorf("defaults not applied: %+v", cfg)
	}
	if cfg.Queue.IdleDone != 90*time.Second || cfg.Queue.MaxConcurrent != 1 {
		t.Errorf("queue = %+v", cfg.Queue)
	}
}

func TestLoadValidation(t *testing.T) {
	t.Setenv(PasswordEnvVar, "")
	p := writeConfig(t, `
queue:
  max_concurrent: 0
`)
	_, err := Load(p)
	if err == nil {
		t.Fatal("expected validation error")
	}
	for _, want := range []string{"ps4.host", "nas.ssh.host", "max_concurrent", PasswordEnvVar} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestExampleIsValid(t *testing.T) {
	t.Setenv(PasswordEnvVar, "x")
	p := writeConfig(t, string(Example))
	if _, err := Load(p); err != nil {
		t.Fatalf("example config does not load: %v", err)
	}
}
