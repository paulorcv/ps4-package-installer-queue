# Contexto do projeto

Memória viva do projeto: o estado atual, o ambiente em que ele roda e o que vem a seguir. As decisões técnicas, com o porquê de cada uma, ficam em [`docs/adr/`](adr/README.md).

> **Última atualização:** 2026-11-02
> Atualize este arquivo no mesmo commit de qualquer mudança que altere o que está descrito aqui.

## Objetivo

Instalar vários `.pkg` no PS4 em sequência, sem acompanhar cada download. O usuário marca os pacotes num app de terminal no PC, e o app envia um por vez ao PS4, liberando o próximo quando o anterior termina.

## Estado atual

**Versão:** pré-lançamento, sem tag. O binário mostra `dev`.

| Área | Situação |
|---|---|
| Listagem de pastas HTTP (Apache autoindex) | Funcionando e testada; usada com o NAS real |
| Envio ao ezRemote (`install_url`) | Funcionando; downloads reais confirmados no PS4 |
| Monitor SSH no NAS (log + conexões) | Funcionando contra o My Cloud real; testes emulam o servidor SSH legado |
| Fila com concorrência configurável | Funcionando; itens podem ser adicionados com downloads em andamento |
| Pausa e nova tentativa quando o ezRemote não responde | Implementado e testado; **ainda não confirmado em uso real** ([ADR 0005](adr/0005-regras-da-fila.md)) |
| Interface (Bubble Tea) | Funcionando no Windows Terminal |
| Build no macOS | Compila (arm64 e amd64); **não testado em um Mac real** |
| CI (GitHub Actions) | Configurado para Linux, Windows e macOS |
| Persistência da fila | **Não existe**: a fila se perde ao fechar o app ([ADR 0007](adr/0007-fila-em-memoria.md)) |

### Problemas conhecidos

- **A barra de progresso avança aos saltos.** O Apache só grava a linha no log quando cada requisição termina ([ADR 0002](adr/0002-deteccao-de-fim-via-nas.md)).
- **Com `max_concurrent` maior que 1**, as conexões abertas não podem ser atribuídas a um arquivo específico; cada download ativo vê o total.
- **O PS4 suspende o ezRemote** quando ele sai da tela (jogo aberto ou modo repouso). A fila pausa e tenta de novo, mas não avança até o ezRemote voltar.
- **O exploit do firmware 13.52 é instável**: às vezes o GoldHEN não carrega e o console desliga. Isso não depende do app.

## Ambiente

O app foi feito para esta instalação específica. Os IPs ficam fixos no roteador.

| Componente | Detalhes |
|---|---|
| **PS4** | Firmware 13.52, GoldHEN 2.4b18.x (browser exploit do raw13g), IP `192.168.0.104` |
| **ezRemote Client** | v2.08 (cy33hc), interface web na porta `8080` do PS4, com o ezRemote Server ativo |
| **NAS** | WD My Cloud single bay de 1ª geração (WDBCTL0020HWT): Debian 7.1, ARMv7, sem Docker, IP `192.168.0.121` |
| **Servidor HTTP** | Segunda instância do Apache 2.4.10 na porta `8080`, servindo `/shares/paulo/ps4` ([ADR 0003](adr/0003-servidor-http-no-nas.md)) |
| **SSH do NAS** | OpenSSH antigo: só oferece chave `ssh-rsa`/`ssh-dss`; usuário `root` com senha |
| **PC** | Windows (`paulim-desktop`), cabeado, `192.168.0.103`; uso futuro também no macOS |

A configuração do NAS está passo a passo em [`docs/nas-setup.md`](nas-setup.md).

## Como funciona

```
          (1) POST /__local__/install_url   {url, enable_rpi: true}
  PC ───────────────────────────────────────▶ PS4 (ezRemote, porta 8080)
  │                                              │
  │                                              │ (2) baixa o .pkg via HTTP com Range
  │                                              ▼
  │  (3) SSH: tail -F do access log        NAS (Apache, porta 8080)
  └──────────  + cat /proc/net/tcp  ─────────────┘
```

1. O app pede ao ezRemote que instale a URL. O ezRemote só **dispara** a instalação e não expõe progresso ([ADR 0001](adr/0001-instalacao-via-ezremote.md)).
2. O PS4 baixa direto do NAS.
3. O app acompanha o NAS via SSH: soma os bytes servidos ao IP do PS4 e conta as conexões abertas. Com isso decide quando o item terminou ([ADR 0002](adr/0002-deteccao-de-fim-via-nas.md), [ADR 0005](adr/0005-regras-da-fila.md)).

### Mapa do código

| Pacote | Responsabilidade |
|---|---|
| `cmd/ps4pkgq` | Flags, carga da configuração e ligação dos componentes |
| `internal/config` | Config YAML, valores padrão, validação, exemplo embutido |
| `internal/listing` | Leitura da listagem HTTP e tamanhos via `HEAD` |
| `internal/ezremote` | Cliente da API web do ezRemote |
| `internal/monitor` | SSH com reconexão; parser do log e do `/proc/net/tcp` |
| `internal/queue` | Fonte da verdade da fila; regras de conclusão, falha e pausa |
| `internal/ui` | Modelo Bubble Tea, teclas e layout |

## Histórico

| Data | Marco |
|---|---|
| 2026-10-30 | Instalações manuais via FTP e DPI; descoberta de que os `.pkg` copiados para o HD interno ocupavam espaço invisível |
| 2026-11-02 | Apache no My Cloud servindo os pkgs com Range; DPI e ezRemote apontando direto para o NAS |
| 2026-11-02 | Protótipo em Python 2 rodando no NAS (`ps4queue.py`): fila em lote, sem interface. Substituído por este projeto |
| 2026-11-02 | Projeto em Go criado: TUI, monitor SSH, fila, testes, CI e docs |
| 2026-11-02 | Primeiro uso real: fila com 10 itens, downloads concluindo |
| 2026-11-02 | Correção: o ezRemote fora do ar derrubava a fila inteira; agora a fila pausa e tenta de novo |

## Próximos passos

Ideias levantadas e ainda não decididas, em ordem aproximada de valor:

1. **Confirmar em uso real** a pausa com nova tentativa quando o ezRemote some.
2. **Persistir a fila** em disco para sobreviver ao fechamento do app ([ADR 0007](adr/0007-fila-em-memoria.md)).
3. **Pedir a senha do SSH ao iniciar**, com digitação oculta, ou guardá-la no Gerenciador de Credenciais do Windows / Keychain do macOS.
4. **Releases com binários** prontos (por exemplo, GoReleaser acionado por tag).
5. **Licença**: o repositório é público e ainda não tem uma.
6. **Progresso contínuo**, em vez de aos saltos, se surgir uma fonte melhor que o access log.

## Questões em aberto

- Os downloads em segundo plano do ezRemote dependem do ezRemote Server ficar ativo até o fim? O código sugere que sim, mas não foi confirmado.
- O que acontece com a fila quando o PS4 entra em modo repouso no meio de um download?
