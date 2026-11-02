# 0002. Detectar o fim do download observando o NAS via SSH

- **Status:** Aceita
- **Data:** 2026-11-02

## Contexto

O ezRemote não informa quando um download termina ([ADR 0001](0001-instalacao-via-ezremote.md)). Para enviar o próximo item da fila, o app precisa de outra fonte que diga quando o PS4 parou de baixar.

## Decisão

Manter o download **direto do NAS para o PS4** e observar o NAS por SSH, com duas fontes:

1. **`tail -n 0 -F` do access log do Apache**, no formato `%h %t "%r" %>s %B`. Cada linha com o IP do PS4, método `GET`, status 200/206 e o caminho do pacote soma bytes ao item.
2. **`cat /proc/net/tcp`** a cada `poll_interval`, para contar as conexões `ESTABLISHED` do IP do PS4 na porta do Apache.

Com bytes servidos e conexões ativas, as regras da [ADR 0005](0005-regras-da-fila.md) decidem quando o item terminou. O parser também aceita os formatos *common* e *combined* do Apache.

A decisão foi do dono do projeto, entre esta opção e a do proxy abaixo.

## Alternativas consideradas

- **O app como proxy HTTP entre o PS4 e o NAS:** daria progresso exato, em bytes e em tempo real, com menos configuração. Foi descartado para manter o tráfego direto NAS → PS4 e não depender do PC no meio do caminho dos dados.
- **Esperar um tempo estimado pelo tamanho:** impreciso, sem saber se o download falhou.

## Consequências

- **O progresso avança aos saltos**, porque o Apache só grava a linha quando cada requisição termina. O indicador "transferindo" vem das conexões abertas.
- O NAS precisa de **SSH ativo** e do **access log configurado** ([docs/nas-setup.md](../nas-setup.md)).
- Com mais de um download simultâneo, as conexões não podem ser atribuídas a um arquivo específico.
- Bytes que o log não contabiliza (por exemplo, um download retomado) deixam o item sem completar. Por isso existe a ação manual de marcar como concluído (`m`).
- O `tail -F` precisa ignorar o fim do stdin. A biblioteca SSH do Go fecha o stdin logo de início, e o `tail` real ignora isso. Os testes emulam esse comportamento.
