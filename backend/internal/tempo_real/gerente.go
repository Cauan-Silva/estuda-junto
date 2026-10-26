package tempo_real

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/Cauan-Silva/estuda-junto/internal/api"
	"github.com/Cauan-Silva/estuda-junto/internal/auth"
	"github.com/Cauan-Silva/estuda-junto/internal/pomodoro"
	"github.com/Cauan-Silva/estuda-junto/internal/salas"
)

// Armazem é o que a sala precisa do banco (o *salas.Repositorio cumpre; os testes usam um falso).
type Armazem interface {
	PorCodigo(ctx context.Context, codigo string) (salas.Sala, error)
	Tarefas(ctx context.Context, salaID int64) ([]salas.Tarefa, error)
	CriarTarefa(ctx context.Context, salaID, autorID int64, texto string) error
	MarcarTarefa(ctx context.Context, salaID, tarefaID, porID int64, feita bool) error
	ApagarTarefa(ctx context.Context, salaID, tarefaID int64) error
	MensagensRecentes(ctx context.Context, salaID int64, limite int) ([]salas.Mensagem, error)
	CriarMensagem(ctx context.Context, salaID, autorID int64, autorNome, texto string) (salas.Mensagem, error)
	SalvarConfig(ctx context.Context, salaID int64, cfg pomodoro.Config, todosControlam bool) error
}

// Registro guarda o tempo de foco (o *estudo.Servico cumpre).
type Registro interface {
	Registrar(ctx context.Context, usuarioID, salaID int64, inicio time.Time, duracao time.Duration) error
}

// Gerente sabe quais salas estão abertas na memória e cria a goroutine de cada uma.
type Gerente struct {
	armazem  Armazem
	registro Registro
	emissor  *auth.Emissor
	origens  []string

	mu    sync.Mutex
	salas map[string]*Sala

	agora         func() time.Time
	intervaloTick time.Duration
	gravacoes     sync.WaitGroup
}

func NovoGerente(armazem Armazem, registro Registro, emissor *auth.Emissor, origensWS []string) *Gerente {
	return &Gerente{
		armazem:       armazem,
		registro:      registro,
		emissor:       emissor,
		origens:       origensWS,
		salas:         map[string]*Sala{},
		agora:         time.Now,
		intervaloTick: time.Second,
	}
}

// Resumo devolve quantos estão online e a fase do relógio de uma sala aberta.
func (g *Gerente) Resumo(codigo string) (Resumo, bool) {
	codigo = strings.ToUpper(strings.TrimSpace(codigo))
	g.mu.Lock()
	s := g.salas[codigo]
	g.mu.Unlock()
	if s == nil {
		return Resumo{}, false
	}
	return *s.resumo.Load(), true
}

// abrir devolve a sala da memória ou carrega do banco e liga a goroutine dela.
// A ocupação é reservada já aqui, com a trava, para a sala não fechar no meio do caminho.
func (g *Gerente) abrir(ctx context.Context, codigo string) (*Sala, error) {
	codigo = strings.ToUpper(strings.TrimSpace(codigo))
	g.mu.Lock()
	if s := g.salas[codigo]; s != nil {
		s.ocupacao.Add(1)
		g.mu.Unlock()
		return s, nil
	}
	g.mu.Unlock()

	// Carrega fora da trava para uma sala lenta não atrasar as outras.
	info, err := g.armazem.PorCodigo(ctx, codigo)
	if err != nil {
		return nil, err
	}
	tarefas, err := g.armazem.Tarefas(ctx, info.ID)
	if err != nil {
		return nil, err
	}
	mensagens, err := g.armazem.MensagensRecentes(ctx, info.ID, mensagensGuardadas)
	if err != nil {
		return nil, err
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	s := g.salas[info.Codigo]
	if s == nil { // ninguém abriu enquanto carregávamos
		s = novaSala(g, info, tarefas, mensagens)
		g.salas[info.Codigo] = s
		go s.rodar()
	}
	s.ocupacao.Add(1)
	return s, nil
}

// liberar devolve a vaga reservada em abrir (chamado pela própria sala quando alguém sai).
func (g *Gerente) liberar(s *Sala) { s.ocupacao.Add(-1) }

// tentarEncerrar tira a sala da memória se ninguém estiver entrando nela.
func (g *Gerente) tentarEncerrar(s *Sala) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if s.ocupacao.Load() > 0 {
		return false
	}
	delete(g.salas, s.codigo)
	slog.Debug("sala saiu da memória", "sala", s.codigo)
	return true
}

func (g *Gerente) registrarEstudo(usuarioID, salaID int64, inicio time.Time, duracao time.Duration) {
	if g.registro == nil || duracao < time.Minute {
		return
	}
	g.gravacoes.Add(1)
	go func() {
		defer g.gravacoes.Done()
		ctx, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelar()
		if err := g.registro.Registrar(ctx, usuarioID, salaID, inicio, duracao); err != nil {
			slog.Error("não foi possível guardar o tempo de foco", "usuario", usuarioID, "erro", err)
		}
	}()
}

// EncerrarTodas fecha todas as salas abertas (ao desligar o servidor).
func (g *Gerente) EncerrarTodas() {
	g.mu.Lock()
	abertas := make([]*Sala, 0, len(g.salas))
	for codigo, s := range g.salas {
		abertas = append(abertas, s)
		delete(g.salas, codigo)
	}
	g.mu.Unlock()
	for _, s := range abertas {
		select {
		case s.encerrar <- struct{}{}:
			<-s.parada
		case <-s.parada:
		}
	}
}

// AguardarGravacoes espera os tempos de foco pendentes irem para o banco (ao desligar).
func (g *Gerente) AguardarGravacoes() { g.gravacoes.Wait() }

// RotaWS é GET /ws/salas/{codigo}?token=... O token vai na URL porque o navegador não deixa
// mandar o cabeçalho Authorization no WebSocket.
func (g *Gerente) RotaWS(w http.ResponseWriter, r *http.Request) {
	id, err := g.emissor.Conferir(r.URL.Query().Get("token"))
	if err != nil {
		api.Responder(w, http.StatusUnauthorized, map[string]string{"erro": "Entre na sua conta."})
		return
	}

	sala, err := g.abrir(r.Context(), r.PathValue("codigo"))
	if err != nil {
		var e *api.Erro
		if !errors.As(err, &e) {
			slog.Error("não foi possível abrir a sala", "erro", err)
		}
		api.Falhar(w, r, err)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: g.origens})
	if err != nil {
		g.liberar(sala)
		return // o Accept já respondeu ao navegador
	}
	conn.SetReadLimit(limiteLeitura)

	ctx, cancelar := context.WithCancel(r.Context())
	defer cancelar()

	c := novoCliente(id.UsuarioID, id.Nome)
	select {
	case sala.entrar <- c:
	case <-sala.parada:
		conn.Close(websocket.StatusTryAgainLater, "sala fechando")
		return
	}
	go c.escrever(ctx, conn)
	c.ler(ctx, conn, sala)

	select {
	case sala.sair <- c:
	case <-sala.parada:
	}
	cancelar()
	conn.Close(websocket.StatusNormalClosure, "")
}
