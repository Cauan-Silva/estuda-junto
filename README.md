# Estuda Junto

[![CI](https://github.com/Cauan-Silva/estuda-junto/actions/workflows/ci.yml/badge.svg)](https://github.com/Cauan-Silva/estuda-junto/actions/workflows/ci.yml)

Salas de estudo online em tempo real. Cada sala tem um relógio Pomodoro sincronizado para todo mundo, lista de quem está online e do que cada um está estudando, chat, quadro de tarefas da turma e chamada de vídeo com compartilhamento de tela. O tempo de foco vira estatística pessoal e ranking semanal.

**Ver online:** https://estuda-junto-0wi3.onrender.com (o plano gratuito pode levar ~1 minuto para acordar)

## O que tem aqui

**Na sala**
- **Pomodoro sincronizado:** foco, pausa e pausa longa iguais para todos, com som e notificação na troca de fase. Quem criou a sala ajusta os tempos e decide se todos podem controlar o relógio
- **Quem está estudando:** pessoas online, o que cada uma está estudando e quem está em foco agora
- **Chat** com limite de mensagens por segundo e **quadro de tarefas** compartilhado (quem criou, quem concluiu)
- **Chamada de vídeo** com câmera, microfone e **compartilhamento de tela**, para até 6 pessoas
- **Convite por link:** cada sala tem um código de 6 letras; salas privadas só abrem pelo código

**Fora da sala**
- Tempo de foco de hoje, da semana e de sempre, gráfico dos últimos 7 dias e **sequência de dias estudando**
- **Ranking** geral e por sala (semana, mês ou sempre)
- Lista de salas abertas com quantas pessoas estão em cada uma e em que fase o relógio está

## Arquitetura

```mermaid
flowchart LR
    subgraph Navegadores
        A["Telas (React 19)"]
        B["Telas (React 19)"]
    end
    subgraph Servidor["Servidor em Go (um binário)"]
        API["API REST + JWT"]
        G["Gerente de salas"]
        S1(["goroutine da sala ABC234"])
        S2(["goroutine da sala K7M2QX"])
        G --> S1
        G --> S2
    end
    DB[("PostgreSQL")]
    A -- "HTTPS" --> API
    A -- "WebSocket" --> S1
    B -- "WebSocket" --> S1
    A <-. "vídeo direto (WebRTC)" .-> B
    API --> DB
    S1 --> DB
```

**Cada sala é um ator.** Quando alguém entra numa sala, o servidor liga uma goroutine só para ela. Essa goroutine é a única dona do estado da sala (relógio, quem está online, tarefas, chat e chamada) e recebe tudo por canais: quem entrou, quem saiu, mensagens dos navegadores e o tique do relógio. Como nada mais mexe nesse estado, não há trava (mutex) nem condição de corrida, e os testes rodam com o detector de corrida do Go (`-race`). Depois de 5 minutos vazia, a sala sai da memória.

**O relógio não manda segundos.** O servidor envia "a fase termina às 15:24:58" e cada navegador calcula o restante com o próprio relógio, corrigido pela diferença para o relógio do servidor. Todos veem o mesmo número, e o servidor só fala quando algo muda.

**O tempo de foco é contado no servidor.** Enquanto o relógio roda na fase de foco, cada pessoa presente acumula tempo; ao sair, pausar ou acabar o foco, o trecho vai para o banco (trechos de menos de 1 minuto não contam). Ao desligar o servidor, as salas gravam o foco de quem ainda estava estudando.

**A chamada é de navegador para navegador.** O vídeo não passa pelo servidor: ele só repassa os sinais WebRTC (oferta, resposta e candidatos) entre quem está na chamada. Para não haver ofertas cruzadas, quem tem o id de conexão menor sempre oferta. Cada conexão nasce com um canal de áudio e um de vídeo, então ligar a câmera, desligar ou trocar pela tela é só trocar a faixa, sem renegociar.

## Tecnologias

- **Servidor:** Go 1.24 (biblioteca padrão para HTTP e rotas), [coder/websocket](https://github.com/coder/websocket), PostgreSQL com [lib/pq](https://github.com/lib/pq), migrações SQL embutidas no binário
- **Segurança:** senhas com PBKDF2-SHA256 (600 mil iterações) e JWT HS256 feitos com a biblioteca padrão, limite de tentativas de login por IP, cabeçalhos de segurança
- **Telas:** React 19 e Vite, sem outras dependências (roteador e WebRTC escritos à mão)
- **Infra:** Docker (imagem distroless, sem shell), Render, Neon e GitHub Actions
- **Testes:** 38 testes em Go, incluindo o ator da sala com WebSockets de verdade e testes de ponta a ponta com PostgreSQL

## Como rodar no seu computador

Precisa de [Go 1.24+](https://go.dev/dl/), [Node 22+](https://nodejs.org) e um PostgreSQL (ou Docker).

**Tudo com Docker:**

```bash
docker compose up --build
```

Abra http://localhost:8080.

**Para desenvolver (com recarga automática das telas):**

```bash
# 1. Banco
docker compose up -d banco

# 2. Servidor (porta 8080)
cd backend
AMBIENTE=desenvolvimento go run ./cmd/servidor

# 3. Telas (porta 5173, repassa /api e /ws para o servidor)
cd frontend
npm install
npm run dev
```

No Windows (PowerShell), troque a linha do servidor por `$env:AMBIENTE="desenvolvimento"; go run ./cmd/servidor`.

**Testes:**

```bash
cd backend
go test -race ./...                       # sem banco: os testes de ponta a ponta são pulados
TEST_DATABASE_URL="postgres://estuda:estuda@localhost:5432/estuda?sslmode=disable" go test -race ./...
```

## Variáveis de ambiente

| Variável | Para quê |
|---|---|
| `DATABASE_URL` | Conexão com o PostgreSQL, ex.: `postgres://usuario:senha@host/banco?sslmode=require` |
| `JWT_SEGREDO` | Segredo dos logins, com pelo menos 32 caracteres (em desenvolvimento há um padrão) |
| `PORT` | Porta do servidor (padrão 8080; o Render define sozinho) |
| `AMBIENTE` | `desenvolvimento` libera o segredo padrão e o WebSocket vindo do Vite |
| `ORIGENS_WS` | Endereços extras aceitos no WebSocket, separados por vírgula |
| `TURN_URLS`, `TURN_USUARIO`, `TURN_SENHA` | Opcionais: servidor TURN para a chamada de vídeo (veja abaixo) |

## Publicar de graça (Render + Neon)

1. **Banco no Neon:** crie um projeto em [neon.tech](https://neon.tech) na região AWS US East 1 e copie a connection string (`postgresql://...`). Tire o `-pooler` do endereço, se houver, e o `&channel_binding=require` do fim.
2. **Servidor no Render:** em [render.com](https://render.com), **New → Blueprint** e escolha este repositório. Cole a connection string em `DATABASE_URL`. O `JWT_SEGREDO` é gerado sozinho.
3. Espere o deploy e abra o endereço que o Render mostrar. As tabelas são criadas no primeiro start.

O plano gratuito do Render dorme depois de 15 minutos sem uso; o primeiro acesso leva cerca de 1 minuto, e o WebSocket reconecta sozinho.

### Chamada de vídeo em redes restritas (TURN)

Na maioria das redes de casa, a conexão direta entre navegadores funciona só com STUN (já configurado, gratuito). Algumas redes (4G de certas operadoras, redes de empresa e de universidade) bloqueiam a conexão direta; nelas, a chamada precisa de um servidor TURN, que retransmite o vídeo. Há opções gratuitas com limite mensal, como o [Open Relay da Metered](https://www.metered.ca/tools/openrelay/). Preencha `TURN_URLS` (ex.: `turn:servidor:3478,turns:servidor:5349`), `TURN_USUARIO` e `TURN_SENHA` no Render.

## API

| Método | Rota | Descrição |
|---|---|---|
| POST | `/api/auth/cadastro`, `/api/auth/login` | Cria a conta / entra (devolve o token) |
| GET | `/api/eu` | Dados da conta e estatísticas de foco |
| GET | `/api/salas` | Salas públicas (com quem está online) e as suas |
| POST | `/api/salas` | Cria uma sala |
| GET | `/api/salas/{codigo}` | Dados de uma sala |
| GET | `/api/salas/{codigo}/ranking?periodo=semana\|mes\|sempre` | Ranking da sala |
| GET | `/api/ranking?periodo=` | Ranking geral |
| GET | `/api/rtc` | Servidores STUN/TURN para a chamada |
| GET | `/api/saude` | Verificação de saúde (usada pelo Render) |
| WS | `/ws/salas/{codigo}?token=` | Tempo real da sala |

Todas as rotas, menos cadastro, login e saúde, pedem `Authorization: Bearer <token>`. O WebSocket recebe o token na URL porque o navegador não deixa mandar cabeçalhos nele.

**Mensagens do WebSocket** (JSON com um campo `tipo`):
- **Do navegador:** `chat`, `timer` (iniciar, pausar, pular, reiniciar), `config`, `status`, `tarefa_nova`, `tarefa_marcar`, `tarefa_apagar`, `chamada_entrar`, `chamada_sair`, `midia` e `sinal`.
- **Do servidor:** `estado` (tudo, ao entrar), `timer`, `fase`, `membros`, `chat`, `tarefas`, `sala`, `chamada`, `sinal` e `erro`.

## Estrutura

```
backend/
├── cmd/servidor/        # main: lê a configuração, abre o banco e sobe o HTTP
└── internal/
    ├── api/             # JSON, erros e o middleware de login
    ├── auth/            # PBKDF2 e JWT com a biblioteca padrão
    ├── banco/           # conexão e migrações SQL embutidas
    ├── config/          # variáveis de ambiente
    ├── estudo/          # tempo de foco, estatísticas e ranking
    ├── pomodoro/        # o relógio (lógica pura, sem goroutines)
    ├── salas/           # salas, tarefas e mensagens no PostgreSQL
    ├── servidor/        # rotas, limite de tentativas e testes de ponta a ponta
    ├── tempo_real/      # gerente de salas, o ator de cada sala e o WebSocket
    ├── usuarios/        # cadastro e login
    └── web/             # serve o React embutido no binário
frontend/src/
├── componentes/         # Timer, ChamadaVideo, Chat, Tarefas, Pessoas, Ranking...
├── paginas/             # Entrar, Início e Sala
└── servicos/            # API, WebSocket da sala (useSala), WebRTC (chamada.js), roteador
```

## Licença

[MIT](LICENSE)
