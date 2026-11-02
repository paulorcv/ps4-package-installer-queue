# CLAUDE.md

Contexto para sessões do Claude Code neste repositório.

## O que é

TUI em Go (Bubble Tea) que enfileira instalações de `.pkg` no PS4 via a API web do ezRemote Client (`POST /__local__/install_url`, `enable_rpi: true`). O ezRemote não expõe progresso, então o fim de cada download é detectado observando o NAS via SSH: `tail -F` do access log do Apache (bytes servidos ao IP do PS4) e `/proc/net/tcp` (conexões abertas do PS4 na porta HTTP).

## Comandos

- `go test -race ./...`: testes (sem rede; tudo com fakes/httptest)
- `go vet ./...`
- `go run ./cmd/ps4pkgq -debug-log debug.log`
- `GOOS=windows go build ./cmd/ps4pkgq`: o usuário roda no Windows

## Arquitetura

- `internal/queue`: fonte da verdade da fila. `Engine` é thread-safe; a UI só lê via `Snapshot()` e é avisada por `Changes()` (canal coalescido). Regras de conclusão em `Poll`.
- `internal/monitor`: `Watcher` mantém a conexão SSH com reconexão e backoff. Suporta firmwares antigos (`ssh-rsa`, `diffie-hellman-group14-sha1`).
- `internal/ui`: `Model` com três focos (URL, arquivos, fila). Layout em `view.go`; `model_test.go` verifica que nada passa da largura/altura do terminal.
- `internal/listing`: parser de autoindex do Apache por regex (mantém só filhos diretos de pastas e arquivos `.pkg`).

## Convenções

- Identificadores e comentários de código em inglês; textos de interface, mensagens de erro e docs em português (pt-BR).
- Sem dependências novas sem necessidade; preferir a stdlib.
- Toda regra nova da fila precisa de teste em `engine_test.go` usando o relógio falso do harness.
