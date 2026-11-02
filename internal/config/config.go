// Package config loads and validates the application configuration.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// PasswordEnvVar overrides nas.ssh.password when set, so the password does
// not need to live in the config file.
const PasswordEnvVar = "PS4PKGQ_SSH_PASSWORD"

// Config is the root configuration.
type Config struct {
	// SourceURL is the HTTP directory listing (Apache autoindex) shown on start.
	SourceURL string `yaml:"source_url"`
	PS4       PS4    `yaml:"ps4"`
	NAS       NAS    `yaml:"nas"`
	Queue     Queue  `yaml:"queue"`
}

// PS4 describes the console running ezRemote Client.
type PS4 struct {
	Host         string `yaml:"host"`
	EzRemotePort int    `yaml:"ezremote_port"`
}

// NAS describes the HTTP server that hosts the packages and how to watch it.
type NAS struct {
	SSH SSH `yaml:"ssh"`
	// AccessLog is the Apache access log path on the NAS.
	AccessLog string `yaml:"access_log"`
	// HTTPPort is the port Apache listens on (used to count PS4 connections).
	HTTPPort int `yaml:"http_port"`
}

// SSH holds the connection settings used to watch the NAS.
type SSH struct {
	Host                  string `yaml:"host"`
	Port                  int    `yaml:"port"`
	User                  string `yaml:"user"`
	Password              string `yaml:"password"`
	KeyFile               string `yaml:"key_file"`
	KnownHosts            string `yaml:"known_hosts"`
	InsecureIgnoreHostKey bool   `yaml:"insecure_ignore_host_key"`
}

// Queue tunes how downloads are scheduled and when they are considered done.
type Queue struct {
	MaxConcurrent int           `yaml:"max_concurrent"`
	StartTimeout  time.Duration `yaml:"start_timeout"`
	IdleDone      time.Duration `yaml:"idle_done"`
	StallTimeout  time.Duration `yaml:"stall_timeout"`
	PollInterval  time.Duration `yaml:"poll_interval"`
}

// Default returns a configuration with sensible defaults.
func Default() Config {
	return Config{
		PS4: PS4{EzRemotePort: 8080},
		NAS: NAS{
			SSH:       SSH{Port: 22, User: "root"},
			AccessLog: "/var/log/apache2/pkg-access.log",
			HTTPPort:  8080,
		},
		Queue: Queue{
			MaxConcurrent: 1,
			StartTimeout:  3 * time.Minute,
			IdleDone:      60 * time.Second,
			StallTimeout:  15 * time.Minute,
			PollInterval:  5 * time.Second,
		},
	}
}

// DefaultPath returns the per-user config file location, e.g.
// %AppData%\ps4pkgq\config.yaml on Windows.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "ps4pkgq", "config.yaml"), nil
}

// Load reads the YAML file at path on top of the defaults and applies
// environment overrides.
func Load(path string) (Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("lendo configuração %s: %w", path, err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("interpretando configuração %s: %w", path, err)
	}

	if pw := os.Getenv(PasswordEnvVar); pw != "" {
		cfg.NAS.SSH.Password = pw
	}
	if cfg.NAS.SSH.KnownHosts == "" {
		if home, err := os.UserHomeDir(); err == nil {
			cfg.NAS.SSH.KnownHosts = filepath.Join(home, ".ssh", "known_hosts")
		}
	}
	cfg.NAS.SSH.KeyFile = expandHome(cfg.NAS.SSH.KeyFile)
	cfg.NAS.SSH.KnownHosts = expandHome(cfg.NAS.SSH.KnownHosts)

	return cfg, cfg.Validate()
}

// Validate reports every problem found, joined.
func (c Config) Validate() error {
	var errs []error
	if c.PS4.Host == "" {
		errs = append(errs, errors.New("ps4.host é obrigatório"))
	}
	if c.PS4.EzRemotePort <= 0 {
		errs = append(errs, errors.New("ps4.ezremote_port deve ser positivo"))
	}
	if c.NAS.SSH.Host == "" {
		errs = append(errs, errors.New("nas.ssh.host é obrigatório"))
	}
	if c.NAS.SSH.User == "" {
		errs = append(errs, errors.New("nas.ssh.user é obrigatório"))
	}
	if c.NAS.SSH.Password == "" && c.NAS.SSH.KeyFile == "" {
		errs = append(errs, fmt.Errorf("informe nas.ssh.password, nas.ssh.key_file ou a variável %s", PasswordEnvVar))
	}
	if c.NAS.AccessLog == "" {
		errs = append(errs, errors.New("nas.access_log é obrigatório"))
	}
	if c.NAS.HTTPPort <= 0 {
		errs = append(errs, errors.New("nas.http_port deve ser positivo"))
	}
	if c.Queue.MaxConcurrent < 1 {
		errs = append(errs, errors.New("queue.max_concurrent deve ser pelo menos 1"))
	}
	for name, d := range map[string]time.Duration{
		"queue.start_timeout": c.Queue.StartTimeout,
		"queue.idle_done":     c.Queue.IdleDone,
		"queue.stall_timeout": c.Queue.StallTimeout,
		"queue.poll_interval": c.Queue.PollInterval,
	} {
		if d <= 0 {
			errs = append(errs, fmt.Errorf("%s deve ser maior que zero", name))
		}
	}
	return errors.Join(errs...)
}

func expandHome(p string) string {
	if len(p) < 2 || p[0] != '~' || (p[1] != '/' && p[1] != '\\') {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	return filepath.Join(home, p[2:])
}
