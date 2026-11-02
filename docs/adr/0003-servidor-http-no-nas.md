# 0003. Servir os pkgs com uma instância própria do Apache no NAS

- **Status:** Aceita
- **Data:** 2026-11-02

## Contexto

O instalador do PS4 só baixa por **HTTP**, não entende SMB (`\\192.168.0.121\…`). Ele também baixa por faixas, então o servidor precisa responder a requisições **Range** com `206 Partial Content`.

O NAS é um WD My Cloud de 1ª geração: Debian 7.1, ARMv7, sistema fechado pela WD, **sem suporte a Docker** e sem `busybox`. Ele tem Apache 2.4.10, Python 2.7, Perl e PHP.

## Decisão

Subir uma **segunda instância do Apache** com um arquivo de configuração próprio (`/root/pkg-httpd.conf`), na porta 8080, servindo só a pasta `/shares/paulo/ps4`. A instância da WD, que serve o painel, não é alterada.

A inicialização no boot usa um script (`/root/start-pkgserver.sh`) chamado pelo `rc.local`. O script espera o volume de dados montar antes de subir o Apache, e o `rc.local` precisou ser habilitado com `update-rc.d`, porque o firmware o deixa desativado.

## Alternativas consideradas

- **`SimpleHTTPServer` do Python 2:** não suporta Range.
- **`busybox httpd`:** não existe nesse firmware.
- **nginx em Docker em outra máquina** montando o compartilhamento SMB: robusto, mas exige outra máquina sempre ligada.
- **Servir pelo DPI no PC:** coloca o PC no caminho dos dados e exige o DPI aberto.
- **Mexer na configuração da WD:** pode quebrar o painel e ser sobrescrita pelo firmware.

## Consequências

- O servidor **não tem autenticação**. Por isso ele serve só a pasta dos pacotes, nunca a pasta pessoal inteira.
- **Atualizações de firmware podem apagar** o `/root` e o `rc.local`. Há uma cópia dos arquivos em `/shares/paulo`, e a atualização automática deve ficar desligada.
- O `/var/log` fica em memória e é recriado a cada boot. O script cria a pasta de log antes de subir o Apache.
- A configuração usa diretivas do Apache 2.4 (`Require all granted`, MPM carregado como módulo).
