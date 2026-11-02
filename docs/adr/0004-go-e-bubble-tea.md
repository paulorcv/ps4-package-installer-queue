# 0004. App em Go com interface de terminal (Bubble Tea)

- **Status:** Aceita
- **Data:** 2026-11-02

## Contexto

O protótipo era um script em Python 2 rodando no próprio NAS: enviava os pacotes de uma pasta em lote, sem interface e sem permitir adicionar itens depois de começar. O dono do projeto pediu um app local, em Go, com interface de terminal, versionado no GitHub.

Requisitos da interface:
- URL da pasta HTTP no topo, editável.
- Painel esquerdo para navegar e marcar arquivos.
- Painel direito com o que está sendo enviado e o que vem na fila.
- Poder enfileirar mais itens a qualquer momento, mesmo com envios em andamento.

## Decisão

- **Go**, com um único binário por plataforma e cross-compile para Windows e macOS.
- **Bubble Tea** para a interface, com **Bubbles** (`textinput`, `spinner`, `progress`, `help`, `key`) e **Lip Gloss** para o layout.
- `golang.org/x/crypto/ssh` para o SSH e `gopkg.in/yaml.v3` para a configuração. Todo o resto usa a biblioteca padrão.
- Layout: cabeçalho com o status do NAS e o IP do PS4; URL em uma caixa; dois painéis lado a lado; quatro linhas de log; barra de ajuda que muda conforme o painel em foco.
- A marcação de arquivos é mantida ao navegar entre pastas, para enfileirar de várias pastas de uma vez.

## Alternativas consideradas

- **Continuar com o script no NAS:** limitado ao Python 2.7 do Debian 7 e sem interface.
- **Interface web ou desktop:** mais pesada de manter, para um uso pessoal em rede local.

## Consequências

- O app precisa ficar aberto no PC enquanto a fila roda.
- A interface é testável sem terminal real: `internal/ui/model_test.go` renderiza a tela e confere que nada passa da largura e da altura.
- Os símbolos usados (`●`, `↓`, `✓`, `✗`, `▸`) dependem de um terminal com Unicode, como o Windows Terminal.
