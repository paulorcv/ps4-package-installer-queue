# 0005. Regras de conclusão, falha e pausa da fila

- **Status:** Aceita
- **Data:** 2026-11-02

## Contexto

Sem progresso vindo do PS4, a fila decide o estado de cada item a partir do que o NAS observa ([ADR 0002](0002-deteccao-de-fim-via-nas.md)). As regras precisam evitar dois erros: liberar o próximo cedo demais, competindo pelo disco do PS4, ou travar a fila esperando para sempre.

## Decisão

Estados: `pendente → enviando → baixando → concluído | falhou`. O padrão é **um download por vez** (`max_concurrent: 1`), porque downloads em paralelo disputam a escrita no HD do PS4 e tendem a demorar mais no total.

Para um item `baixando`, a cada `poll_interval`:

| Regra | Condição | Padrão |
|---|---|---|
| **Concluído** | Arquivo inteiro servido **e** nenhuma conexão do PS4 por `idle_done` | 60s |
| **Falhou: não começou** | Nenhum sinal do PS4 (bytes ou conexão) por `start_timeout` após o envio | 3min |
| **Falhou: travou** | Nenhuma conexão por `stall_timeout`, sem completar | 15min |
| **Monitor indisponível** | Não decide nada; continua esperando | — |

Só contam requisições `GET` com status 200 ou 206 vindas do IP do PS4. As requisições que o ezRemote faz ao ler o cabeçalho do pacote, ainda no estado `enviando`, também contam.

### ezRemote sem resposta

Erros de rede ou tempo esgotado ao enviar **não falham o item**. O item volta ao topo como `pendente` e a fila **pausa**, tentando de novo após 15s, 30s, 60s… até 2min por tentativa. A interface mostra "⚠ ezRemote não respondeu · nova tentativa em…". Uma resposta do ezRemote recusando a instalação continua marcando o item como falha.

Essa regra veio de um incidente real: com o ezRemote suspenso por alguns segundos, cada item falhava e o próximo era enviado na hora, derrubando a fila inteira. O tempo limite do pedido ao ezRemote também subiu de 30s para 2min, porque ele só responde depois de ler o cabeçalho do pacote no NAS.

### Edição da fila

- Itens podem ser adicionados a qualquer momento. URLs que já estão na fila e não terminaram são ignoradas.
- Itens pendentes podem ser reordenados (`K`/`J`) e removidos (`d`). Itens em andamento não podem ser removidos.
- Itens com falha podem ser tentados de novo (`r`), o que zera os bytes contados.
- Itens baixando ou com falha podem ser marcados como concluídos à mão (`m`).

## Alternativas consideradas

- **Falhar o item em qualquer erro de envio:** era o comportamento inicial e causou o incidente acima.
- **Nova tentativa automática para itens que travaram:** descartada por enquanto, porque um download que travou pode indicar um problema que precisa de atenção.

## Consequências

- Com o arquivo inteiro servido, o item leva pelo menos `idle_done` para ser concluído.
- A fila não avança enquanto o ezRemote estiver fora, mas também não perde itens.
- Toda regra nova precisa de teste em `internal/queue/engine_test.go`, usando o relógio falso do harness.
