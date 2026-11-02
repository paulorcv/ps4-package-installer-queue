# 0001. Instalar pela API web do ezRemote

- **Status:** Aceita
- **Data:** 2026-11-02

## Contexto

O app precisa pedir ao PS4 que baixe e instale um `.pkg` a partir de uma URL, de forma programática. O console está no firmware 13.52 com GoldHEN, cujo suporte é recente e ainda instável para homebrews.

## Decisão

Usar o endpoint `POST /__local__/install_url` da interface web do **ezRemote Client** (porta 8080 do PS4), com `enable_rpi: true`, para que o PS4 baixe direto da URL pelo *Remote Package Installer*.

O formato foi obtido capturando a requisição da própria interface web do ezRemote e confirmado no código-fonte dele:

- O corpo é `{"action":"installUrl","url":"…","enable_rpi":true, …}`.
- A resposta é sempre HTTP 200, com `{"result":{"success":bool,"error":"…"}}`. É preciso ler o corpo para saber se deu erro.
- O endpoint só **dispara** a instalação. A tarefa fica registrada no instalador do PS4, e **não há endpoint de progresso**.

## Alternativas consideradas

- **API do Remote Package Installer do flatz (porta 12800):** documentada e com progresso por tarefa, mas a porta não respondeu no console com GoldHEN 13.52. O app dedicado, que abriria a porta, não tem `.pkg` oficial publicado.
- **DirectPackageInstaller (DPI):** funciona via payload na porta 9090, mas é um app de interface no PC, sem API para automatizar. Também há relatos de falha ao enviar vários pacotes seguidos.
- **Package Installer do GoldHEN com pendrive ou HD exFAT:** é manual, exige copiar os arquivos antes e não permite fila.

## Consequências

- O ezRemote Client precisa estar **aberto durante toda a fila**, porque cada item novo é enviado ao servidor web dele. O PS4 suspende o app quando ele sai da tela.
- Sem progresso vindo do PS4, o fim do download precisa ser detectado de outro jeito ([ADR 0002](0002-deteccao-de-fim-via-nas.md)).
- O endpoint não é uma API pública e pode mudar entre versões do ezRemote. A versão validada é a 2.08.
