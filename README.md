# ps4-package-installer-queue

Aplicativo de terminal (TUI) em Go que monta uma **fila de instalação de pacotes `.pkg` no PS4**, enviando um por vez (ou N) para o [ezRemote Client](https://github.com/cy33hc/ps4-ezremote-client) e liberando o próximo assim que o download anterior termina.

```
PS4 PKG Queue v0.1.0                                         ● NAS conectado  ·  PS4 192.168.0.104
╭──────────────────────────────────────────────────────────────────────────────────────────────────╮
│ Pasta http://192.168.0.121:8080/                                                                 │
╰──────────────────────────────────────────────────────────────────────────────────────────────────╯
╭───────────────────────────────────────────────╮╭───────────────────────────────────────────────╮
│ Arquivos  /  1 marcado(s)                     ││ Fila  1 ativo · 1 pendente(s) · 0 concluído(s)│
│   ▸ GOD OF WAR 1/                             ││ › ↓ Mega Man X Collection.pkg        baixando │
│   [ ] Mega Man X Collection.pkg     baixando  ││     ████░░░░░░  41% 1.1 GB/2.7 GB · 03:12     │
│ › [x] Tekken 5.pkg                    3.9 GB  ││   ○ Tekken 5.pkg                           #1 │
│   [ ] Resident Evil 4.pkg             3.8 GB  ││     3.9 GB · aguardando a vez                 │
╰───────────────────────────────────────────────╯╰───────────────────────────────────────────────╯
```

- **Topo:** URL da pasta HTTP com os `.pkg` (listagem do Apache).
- **Esquerda:** navegue pelas pastas e marque os arquivos.
- **Direita:** o que está sendo instalado e a fila. Dá para enfileirar mais itens a qualquer momento, mesmo com downloads em andamento.

## Como funciona

```
          (1) POST /__local__/install_url
  PC ───────────────────────────────────────▶ PS4 (ezRemote, porta 8080)
  │                                              │
  │                                              │ (2) baixa o .pkg via HTTP
  │                                              ▼
  │  (3) SSH: tail do access log          NAS (Apache, porta 8080)
  └──────────  + /proc/net/tcp  ─────────────────┘
```

1. O app pede ao ezRemote que instale a URL do pacote com o *Remote Package Installer*. O PS4 baixa **direto do NAS**.
2. O ezRemote não informa o progresso por HTTP. Por isso o app acompanha o NAS **via SSH**:
   - lê o *access log* do Apache para somar os bytes servidos ao IP do PS4;
   - consulta `/proc/net/tcp` para saber se o PS4 ainda tem conexões abertas.
3. Um item é **concluído** quando o arquivo inteiro já foi servido e o PS4 fica `idle_done` sem conexões. Ele **falha** se o PS4 não começar em `start_timeout` ou se parar por `stall_timeout`. Aí o próximo da fila é enviado.

> O Apache só grava a linha no log quando cada requisição termina, então a barra de progresso pode avançar aos saltos. O indicador "transferindo" mostra que há conexões ativas.

## Requisitos

- **PS4** com GoldHEN e **ezRemote Client** aberto (com o *ezRemote Server* ativo).
- **NAS** servindo os `.pkg` via HTTP com o Apache, com acesso SSH e o access log no formato abaixo. Veja o passo a passo em [docs/nas-setup.md](docs/nas-setup.md).
- **Go 1.23+** para compilar.

## Primeiros passos

Os comandos abaixo são para o PowerShell no Windows.

**1. Confira o Go.** A versão precisa ser 1.23 ou mais nova. Se o comando não existir, instale pelo [go.dev/dl](https://go.dev/dl/) e abra um novo PowerShell.

```powershell
go version
```

**2. Baixe o projeto e as dependências.** O `go mod tidy` baixa as bibliotecas e cria o `go.sum`.

```powershell
git clone https://github.com/paulorcv/ps4-package-installer-queue.git
cd ps4-package-installer-queue
go mod tidy
```

**3. Rode os testes (opcional).** Todas as linhas devem terminar com `ok`.

```powershell
go test ./...
```

**4. Compile.**

```powershell
go build -o ps4pkgq.exe ./cmd/ps4pkgq
```

Ou instale direto no `GOPATH/bin`:

```powershell
go install github.com/paulorcv/ps4-package-installer-queue/cmd/ps4pkgq@latest
```

**5. Crie a configuração.** O `-init` grava o arquivo em `%AppData%\ps4pkgq\config.yaml` e mostra o caminho. Confira os IPs do PS4 e do NAS:

```powershell
.\ps4pkgq.exe -init
notepad $env:APPDATA\ps4pkgq\config.yaml
```

**6. Defina a senha do SSH.** Assim ela não fica gravada no arquivo (veja [Senha do SSH](#senha-do-ssh)).

```powershell
$env:PS4PKGQ_SSH_PASSWORD = "sua-senha"
```

**7. Prepare o PS4.** Carregue o GoldHEN e abra o ezRemote Client com o ezRemote Server ativo.

**8. Rode o app.** Em alguns segundos, o topo deve mostrar **● NAS conectado** e a esquerda lista as pastas da `source_url`.

```powershell
.\ps4pkgq.exe
```

Para usar: navegue com as setas, abra pastas com `enter`, marque com `espaço` e aperte `e` para enfileirar. O `tab` troca de painel, o `?` mostra todos os atalhos e o `q` sai.

### No macOS

O `make` e o `git` vêm com as Command Line Tools da Apple. O Go pode ser instalado pelo Homebrew ou pelo instalador do [go.dev/dl](https://go.dev/dl/).

```bash
xcode-select --install        # make e git (uma vez só)
brew install go               # ou o instalador do go.dev

git clone https://github.com/paulorcv/ps4-package-installer-queue.git
cd ps4-package-installer-queue
make test                     # testes
make build                    # gera dist/ps4pkgq para o seu Mac

./dist/ps4pkgq -init
open -e ~/Library/Application\ Support/ps4pkgq/config.yaml

export PS4PKGQ_SSH_PASSWORD="sua-senha"
./dist/ps4pkgq
```

Registre o NAS no `~/.ssh/known_hosts` do Mac uma vez (veja [Configuração](#configuração)). Para não exportar a senha a cada sessão, prefira uma chave SSH em `nas.ssh.key_file`.

## Configuração

> **Não renomeie nem mova** o [`internal/config/config.example.yaml`](internal/config/config.example.yaml). Ele é embutido no binário e serve de modelo para o `-init`. O seu `config.yaml` fica em `%AppData%\ps4pkgq\` no Windows, `~/Library/Application Support/ps4pkgq/` no macOS e `~/.config/ps4pkgq/` no Linux, ou onde você indicar com `-config`.

Para manter o config em outro lugar, como na pasta do projeto, use `.\ps4pkgq.exe -config .\config.yaml`. O `.gitignore` impede que um `config.yaml` vá para o repositório.

O NAS precisa estar no seu `~/.ssh/known_hosts`. Basta conectar uma vez pelo `ssh` e aceitar a chave. Firmwares antigos, como o do WD My Cloud, exigem `ssh -o HostKeyAlgorithms=+ssh-rsa -o PubkeyAcceptedAlgorithms=+ssh-rsa root@IP_DO_NAS`. O app já faz isso por conta própria.

### Senha do SSH

O app aceita, nesta ordem de preferência:

1. **Variável `PS4PKGQ_SSH_PASSWORD`**, que tem prioridade sobre o arquivo e não grava a senha em lugar nenhum.
2. **`nas.ssh.password` no `config.yaml`**, que funciona, mas deixa a senha em texto puro.
3. **Chave SSH em `nas.ssh.key_file`**, com a chave pública cadastrada no `authorized_keys` do NAS. Assim você não usa senha.

Todas as opções estão comentadas em [`internal/config/config.example.yaml`](internal/config/config.example.yaml).

| Opção | Padrão | Descrição |
|---|---|---|
| `source_url` | | Pasta HTTP aberta ao iniciar |
| `ps4.host` | | IP do PS4 |
| `ps4.ezremote_port` | `8080` | Porta da interface web do ezRemote |
| `nas.ssh.*` | | Host, porta, usuário e senha/chave do NAS |
| `nas.access_log` | `/var/log/apache2/pkg-access.log` | Log lido via `tail -F` |
| `nas.http_port` | `8080` | Porta do Apache, para contar conexões |
| `queue.max_concurrent` | `1` | Downloads simultâneos |
| `queue.start_timeout` | `3m` | Tempo máximo até o PS4 começar |
| `queue.idle_done` | `60s` | Silêncio após servir tudo para concluir |
| `queue.stall_timeout` | `15m` | Silêncio sem terminar para falhar |
| `queue.poll_interval` | `5s` | Intervalo de verificação no NAS |

### Flags

```
-config string     arquivo de configuração (padrão %AppData%\ps4pkgq\config.yaml)
-init              cria o arquivo de configuração de exemplo e sai
-url string        pasta HTTP com os .pkg (sobrepõe source_url)
-debug-log string  grava mensagens de depuração neste arquivo
-version           mostra a versão
```

## Atalhos

| Onde | Tecla | Ação |
|---|---|---|
| Geral | `tab` / `shift+tab` | Troca de painel |
| | `u` | Editar a URL |
| | `?` | Ajuda completa |
| | `q` / `ctrl+c` | Sair |
| URL | `enter` | Carregar a pasta |
| | `esc` | Cancelar |
| Arquivos | `↑↓` `pgup` `pgdn` `g` `G` | Navegar |
| | `espaço` | Marcar/desmarcar |
| | `a` | Marcar/desmarcar todos da pasta |
| | `enter` | Abrir pasta |
| | `backspace` | Pasta acima |
| | `e` | Enfileirar marcados (ou o item sob o cursor) |
| | `r` | Recarregar |
| | `esc` | Limpar seleção |
| Fila | `K` / `J` | Mover item pendente para cima/baixo |
| | `d` | Remover (pendente, concluído ou com falha) |
| | `r` | Tentar de novo |
| | `m` | Marcar como concluído manualmente |
| | `c` | Limpar concluídos |

A marcação é mantida ao navegar entre pastas, então dá para marcar arquivos de várias pastas e enfileirar tudo de uma vez.

## Solução de problemas

| Sintoma | O que fazer |
|---|---|
| `configuração não encontrada` | Rode `.\ps4pkgq.exe -init` ou aponte o arquivo com `-config` |
| `informe nas.ssh.password, nas.ssh.key_file ou a variável PS4PKGQ_SSH_PASSWORD` | Defina a senha (passo 6) |
| Topo mostra erro de `known_hosts` ou `key mismatch` | Conecte uma vez com `ssh` e aceite a chave do NAS |
| Topo fica em "conectando ao NAS…" | Confira o IP, a porta e o usuário em `nas.ssh` e se o SSH está ativo no NAS |
| `erro ao listar` na esquerda | Abra a `source_url` no navegador; o Apache precisa estar rodando |
| Item falha com "o PS4 não começou a baixar" | Confira se o ezRemote está aberto no PS4 e se o `ps4.host` está certo |
| Item fica "baixando" com 0% | Confira se o access log está configurado ([docs/nas-setup.md](docs/nas-setup.md#2-configuração-do-apache)) |

Para investigar, rode com `.\ps4pkgq.exe -debug-log debug.log` e veja o arquivo gerado.

## Desenvolvimento

```powershell
go test ./...        # testes
go vet ./...
go run ./cmd/ps4pkgq -debug-log debug.log
```

Ou com `make` (macOS, Linux ou WSL): `make test`, `make build` (binário para a máquina atual) e `make build-windows` (gera o `.exe` a partir do Mac).

### Estrutura

```
cmd/ps4pkgq/        ponto de entrada e flags
internal/config/    configuração YAML + exemplo embutido
internal/listing/   leitura da listagem HTTP (Apache autoindex)
internal/ezremote/  cliente da API do ezRemote
internal/monitor/   SSH no NAS: tail do access log e /proc/net/tcp
internal/queue/     motor da fila e regras de conclusão
internal/ui/        interface Bubble Tea
docs/               configuração do NAS
```

## Limitações conhecidas

- Com `max_concurrent` maior que 1, as conexões abertas não podem ser atribuídas a um arquivo específico. Cada download ativo vê o total.
- A fila fica em memória e se perde ao fechar o app.
- Se o log não contabilizar todos os bytes (por exemplo, quando o PS4 retoma um download parcial), o item termina como falha por inatividade. Use `m` para marcá-lo como concluído.
