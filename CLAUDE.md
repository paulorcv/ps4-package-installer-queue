# CLAUDE.md

Instruções para sessões do Claude Code neste repositório.

## Leia antes de mudar algo

1. **[`docs/context.md`](docs/context.md)**: estado atual, ambiente real (PS4, NAS, IPs), limitações, próximos passos e questões em aberto.
2. **[`docs/adr/`](docs/adr/README.md)**: as decisões técnicas e o porquê de cada uma. Não reverta uma decisão aceita sem uma ADR nova que a substitua.
3. **[`docs/nas-setup.md`](docs/nas-setup.md)**: como o NAS foi configurado.

## Mantenha a memória do projeto

Ao terminar uma mudança, no mesmo commit:

- **Estado mudou** (funcionalidade nova, bug corrigido, limitação resolvida): atualize `docs/context.md`, incluindo a tabela "Estado atual", o "Histórico" e a data de "Última atualização".
- **Decisão técnica nova ou revista**: crie uma ADR seguindo [`docs/adr/README.md`](docs/adr/README.md). Proponha com status **Proposta** e deixe o dono do projeto aceitar.
- **Comportamento visível mudou** (teclas, flags, config): atualize o `README.md`.
- Diga ao usuário o que foi registrado, para ele revisar no diff.

## O que é

TUI em Go (Bubble Tea) que enfileira instalações de `.pkg` no PS4 pela API web do ezRemote Client (`POST /__local__/install_url`, `enable_rpi: true`). O ezRemote não expõe progresso, então o fim de cada download é detectado observando o NAS via SSH: `tail -F` do access log do Apache (bytes servidos ao IP do PS4) e `/proc/net/tcp` (conexões abertas do PS4).

## Comandos

- `go test -race ./...`: testes, sem rede nem hardware
- `go vet ./...` e `gofmt -l .`
- `go run ./cmd/ps4pkgq -debug-log debug.log`
- `GOOS=windows go build ./cmd/ps4pkgq`: o dono do projeto usa Windows e, no futuro, macOS

## Arquitetura

- `internal/queue`: fonte da verdade da fila. `Engine` é thread-safe; a UI só lê via `Snapshot()`/`Status()` e é avisada por `Changes()`. Regras de conclusão em `Poll`; pausa com backoff em erros de rede no envio ([ADR 0005](docs/adr/0005-regras-da-fila.md)).
- `internal/monitor`: `Watcher` mantém o SSH com reconexão. Precisa continuar aceitando `ssh-rsa` e `diffie-hellman-group14-sha1` ([ADR 0006](docs/adr/0006-configuracao-e-credenciais.md)).
- `internal/ui`: três focos (URL, arquivos, fila). Layout em `view.go`; `model_test.go` garante que nada passa da largura e da altura.
- `internal/listing`: parser do autoindex do Apache (só filhos diretos, pastas e `.pkg`).
- `internal/ezremote`: cliente do `install_url`. Erros de rede precisam continuar reconhecíveis como `net.Error`, porque a fila decide entre pausar e falhar com base nisso.

## Convenções

Resumo da [ADR 0008](docs/adr/0008-convencoes.md):
- Código e comentários em inglês; interface, erros para o usuário e docs em português (pt-BR).
- Testes com fakes, `httptest` e relógio injetado; sem `time.Sleep` para avançar regras.
- Sem dependências novas sem necessidade; preferir a biblioteca padrão.
- Toda regra nova da fila precisa de teste em `engine_test.go`.
