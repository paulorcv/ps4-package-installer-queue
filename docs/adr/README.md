# Registros de decisão de arquitetura (ADR)

Cada arquivo registra **uma** decisão técnica: o problema, o que foi escolhido, as alternativas descartadas e as consequências. O objetivo é que, meses depois, dê para entender por que o projeto é como é antes de mudar algo.

| # | Decisão | Status |
|---|---|---|
| [0001](0001-instalacao-via-ezremote.md) | Instalar pela API web do ezRemote | Aceita |
| [0002](0002-deteccao-de-fim-via-nas.md) | Detectar o fim do download observando o NAS via SSH | Aceita |
| [0003](0003-servidor-http-no-nas.md) | Servir os pkgs com uma instância própria do Apache no NAS | Aceita |
| [0004](0004-go-e-bubble-tea.md) | App em Go com interface de terminal (Bubble Tea) | Aceita |
| [0005](0005-regras-da-fila.md) | Regras de conclusão, falha e pausa da fila | Aceita |
| [0006](0006-configuracao-e-credenciais.md) | Configuração em YAML e credenciais do SSH | Aceita |
| [0007](0007-fila-em-memoria.md) | Fila somente em memória, por enquanto | Aceita (provisória) |
| [0008](0008-convencoes.md) | Convenções de código, testes e dependências | Aceita |

## Como registrar uma decisão nova

1. Copie o modelo abaixo para `NNNN-titulo-curto.md`, com o próximo número.
2. Preencha com status **Proposta** e abra um commit ou PR para revisão.
3. Ao aceitar, mude o status para **Aceita** e inclua a linha na tabela acima.
4. Para mudar uma decisão antiga, **não edite** o conteúdo dela. Crie uma nova e marque a antiga como **Substituída por NNNN**.
5. Atualize o [`docs/context.md`](../context.md) se o estado do projeto mudou.

## Modelo

```markdown
# NNNN. Título

- **Status:** Proposta | Aceita | Substituída por NNNN
- **Data:** AAAA-MM-DD

## Contexto
Qual problema ou restrição levou à decisão.

## Decisão
O que foi escolhido.

## Alternativas consideradas
- Opção: por que foi descartada.

## Consequências
O que fica mais fácil, o que fica mais difícil e o que precisa ser vigiado.
```
