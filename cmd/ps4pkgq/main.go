// Command ps4pkgq is a terminal app that queues PS4 package installs through
// ezRemote Client, one (or N) at a time, watching the NAS to know when each
// download has finished.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/paulorcv/ps4-package-installer-queue/internal/config"
	"github.com/paulorcv/ps4-package-installer-queue/internal/ezremote"
	"github.com/paulorcv/ps4-package-installer-queue/internal/listing"
	"github.com/paulorcv/ps4-package-installer-queue/internal/monitor"
	"github.com/paulorcv/ps4-package-installer-queue/internal/queue"
	"github.com/paulorcv/ps4-package-installer-queue/internal/ui"
)

// version is set at build time with -ldflags "-X main.version=v1.2.3".
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
}

func run() error {
	defaultPath, err := config.DefaultPath()
	if err != nil {
		return err
	}

	var (
		cfgPath  = flag.String("config", defaultPath, "arquivo de configuração")
		initCfg  = flag.Bool("init", false, "cria um arquivo de configuração de exemplo e sai")
		urlFlag  = flag.String("url", "", "pasta HTTP com os .pkg (sobrepõe source_url)")
		showVer  = flag.Bool("version", false, "mostra a versão e sai")
		debugLog = flag.String("debug-log", "", "grava mensagens de depuração neste arquivo")
	)
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "ps4pkgq %s: fila de instalação de PKGs no PS4 via ezRemote\n\nUso:\n  ps4pkgq [opções]\n\nOpções:\n", version)
		flag.PrintDefaults()
	}
	flag.Parse()

	if *showVer {
		fmt.Println("ps4pkgq", version)
		return nil
	}
	if *initCfg {
		if err := config.WriteExample(*cfgPath); err != nil {
			return err
		}
		fmt.Printf("configuração criada em %s\nedite os IPs e a senha (ou defina %s) e rode ps4pkgq novamente.\n", *cfgPath, config.PasswordEnvVar)
		return nil
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("configuração não encontrada em %s\nrode \"ps4pkgq -init\" para criar uma", *cfgPath)
		}
		return err
	}
	if *urlFlag != "" {
		cfg.SourceURL = *urlFlag
	}

	if *debugLog != "" {
		f, err := tea.LogToFile(*debugLog, "ps4pkgq")
		if err != nil {
			return err
		}
		defer f.Close()
	} else {
		log.SetOutput(discard{})
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	httpClient := &http.Client{Timeout: 30 * time.Second}
	// ezRemote only answers after reading the package header from the NAS,
	// which can take a while for large packages.
	ezClient := &http.Client{Timeout: 2 * time.Minute}

	watcher := monitor.New(cfg.NAS.SSH, cfg.NAS.AccessLog, cfg.NAS.HTTPPort, cfg.PS4.Host)
	go watcher.Run(ctx)

	engine := queue.New(queue.Options{
		PS4Host:       cfg.PS4.Host,
		MaxConcurrent: cfg.Queue.MaxConcurrent,
		StartTimeout:  cfg.Queue.StartTimeout,
		IdleDone:      cfg.Queue.IdleDone,
		StallTimeout:  cfg.Queue.StallTimeout,
		PollInterval:  cfg.Queue.PollInterval,
		SendTimeout:   2 * time.Minute,
	}, ezremote.New(cfg.PS4.Host, cfg.PS4.EzRemotePort, ezClient), watcher)
	go engine.Run(ctx, watcher.Events())

	model := ui.New(ui.Deps{
		Lister:        listing.New(httpClient),
		Engine:        engine,
		MonitorStatus: watcher.Status(),
		StartURL:      cfg.SourceURL,
		PS4Host:       cfg.PS4.Host,
		Version:       version,
	})

	_, err = tea.NewProgram(model, tea.WithAltScreen()).Run()
	return err
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }
