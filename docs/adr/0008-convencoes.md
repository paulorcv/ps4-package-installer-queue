# 0008. Convenções de código, testes e dependências

- **Status:** Aceita
- **Data:** 2026-11-02

## Contexto

O projeto é pessoal, mas público, e vai evoluir com a ajuda de assistentes de código. Convenções explícitas evitam que cada mudança siga um estilo diferente.

## Decisão

**Idioma**
- Identificadores e comentários de código em **inglês**.
- Textos da interface, mensagens de erro para o usuário e documentação em **português (pt-BR)**.

**Estrutura**
- Código da aplicação em `internal/`, um pacote por responsabilidade. Ponto de entrada em `cmd/ps4pkgq`.
- `internal/queue` é a fonte da verdade. A interface só lê via `Snapshot()` e `Status()` e é avisada por `Changes()`, um canal coalescido.

**Testes**
- Sem rede nem hardware: `httptest`, fakes e um servidor SSH em processo.
- A lógica de tempo usa um relógio injetado (`Options.Now`), nunca `time.Sleep` para avançar regras.
- Os testes rodam com `-race`.
- Toda regra nova da fila ganha teste.

**Dependências**
- Versões fixadas no `go.mod`; o `go.sum` é gerado com `go mod tidy` na máquina do desenvolvedor.
- Dependência nova só com motivo claro; a preferência é a biblioteca padrão.
- Versão mínima do Go: 1.23.

**CI**
- GitHub Actions em Linux, Windows e macOS: `gofmt`, `go vet`, testes com `-race` e build.

## Consequências

- O `CLAUDE.md` resume estas regras para sessões do Claude Code.
- Mudanças de comportamento visível atualizam o README; mudanças de estado ou de decisão atualizam o `docs/context.md` e os ADRs.
