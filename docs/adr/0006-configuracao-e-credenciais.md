# 0006. Configuração em YAML e credenciais do SSH

- **Status:** Aceita
- **Data:** 2026-11-02

## Contexto

O app precisa dos IPs do PS4 e do NAS, das credenciais do SSH e dos tempos da fila. O repositório é público, então nada disso pode ir para o Git. O NAS usa um OpenSSH antigo, que só oferece chave de servidor `ssh-rsa`, algo que os clientes modernos desativam por padrão.

## Decisão

**Configuração**
- Arquivo YAML no diretório de configuração do usuário: `%AppData%\ps4pkgq\config.yaml` no Windows, `~/Library/Application Support/ps4pkgq/` no macOS e `~/.config/ps4pkgq/` no Linux. Outro caminho pode ser passado com `-config`.
- `ps4pkgq -init` cria o arquivo a partir de um exemplo comentado **embutido no binário** (`internal/config/config.example.yaml`, via `go:embed`). O arquivo é criado com permissão só para o usuário e nunca sobrescreve um existente.
- A validação junta todos os erros numa mensagem só.

**Senha do SSH**, em ordem de preferência:
1. Variável `PS4PKGQ_SSH_PASSWORD`, com prioridade sobre o arquivo.
2. `nas.ssh.password` no YAML (texto puro).
3. Chave em `nas.ssh.key_file`.

A senha é enviada pelos métodos `password` e `keyboard-interactive`, porque alguns servidores Debian antigos só aceitam o segundo.

**Identidade do servidor**
- A chave do NAS é validada pelo `~/.ssh/known_hosts`. Basta conectar uma vez pelo `ssh` e aceitar a chave.
- `insecure_ignore_host_key` existe, mas desligado por padrão.

**Algoritmos legados**, liberados no cliente sem configuração:
- Chave de servidor `ssh-rsa` e `ssh-dss`, além das modernas.
- Troca de chaves `diffie-hellman-group14-sha1` e `diffie-hellman-group1-sha1`.
- No login por chave RSA, a biblioteca assina com `ssh-rsa` quando o servidor não anuncia `server-sig-algs`. Isso equivale a `PubkeyAcceptedAlgorithms=+ssh-rsa`.

## Alternativas consideradas

- **Variáveis de ambiente para tudo:** pouco prático para tantos campos.
- **Pedir a senha ao iniciar** ou **guardar no cofre do sistema operacional:** são melhorias candidatas, listadas em [docs/context.md](../context.md#próximos-passos).

## Consequências

- O `.gitignore` bloqueia `config.yaml` e `*.log`.
- Aceitar `ssh-rsa` e SHA-1 enfraquece a conexão com o NAS. Isso é aceitável numa rede local, porque o NAS não oferece nada melhor.
- `internal/monitor/watcher_test.go` sobe um servidor SSH limitado aos mesmos algoritmos do My Cloud, para que uma atualização da biblioteca que remova esse suporte seja detectada.
