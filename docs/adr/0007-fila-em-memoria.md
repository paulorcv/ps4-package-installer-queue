# 0007. Fila somente em memória, por enquanto

- **Status:** Aceita (provisória)
- **Data:** 2026-11-02

## Contexto

A primeira versão precisava validar o fluxo completo: listar, enfileirar, enviar e detectar o fim. Persistir a fila exige decidir o formato, o local e o que fazer com itens que estavam em andamento quando o app fechou.

## Decisão

A fila vive só na memória do processo. Ao fechar o app, ela se perde.

## Alternativas consideradas

- **Salvar em JSON** no diretório de configuração a cada mudança e recarregar ao iniciar.

## Consequências

- Fechar o app, ou ele travar, obriga a montar a fila de novo.
- Downloads já iniciados no PS4 continuam, porque o instalador do console não depende do app. Só o envio dos próximos itens para.

## Para revisitar

Quando a persistência for implementada, decidir:
- Itens `enviando` ou `baixando` ao reabrir: voltam para `pendente`, viram `falhou` ou o app tenta descobrir pelo log do NAS se terminaram?
- Itens concluídos devem ficar no histórico ou ser descartados?
