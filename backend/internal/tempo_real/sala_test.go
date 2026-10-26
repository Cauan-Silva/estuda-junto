package tempo_real

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/Cauan-Silva/estuda-junto/internal/auth"
	"github.com/Cauan-Silva/estuda-junto/internal/pomodoro"
	"github.com/Cauan-Silva/estuda-junto/internal/salas"
)

// ---- banco falso ----

type armazemFalso struct {
	mu        sync.Mutex
	sala      salas.Sala
	tarefas   []salas.Tarefa
	mensagens []salas.Mensagem
	proximoID int64
}

func (a *armazemFalso) PorCodigo(_ context.Context, codigo string) (salas.Sala, error) {
	if codigo != a.sala.Codigo {
		return salas.Sala{}, salas.ErrNaoEncontrada
	}
	return a.sala, nil
}
func (a *armazemFalso) Tarefas(context.Context, int64) ([]salas.Tarefa, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]salas.Tarefa{}, a.tarefas...), nil
}
func (a *armazemFalso) CriarTarefa(_ context.Context, _ int64, autor int64, texto string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.proximoID++
	a.tarefas = append(a.tarefas, salas.Tarefa{ID: a.proximoID, Texto: texto, AutorID: autor})
	return nil
}
func (a *armazemFalso) MarcarTarefa(_ context.Context, _, id, _ int64, feita bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.tarefas {
		if a.tarefas[i].ID == id {
			a.tarefas[i].Feita = feita
		}
	}
	return nil
}
func (a *armazemFalso) ApagarTarefa(_ context.Context, _, id int64) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.tarefas {
		if a.tarefas[i].ID == id {
			a.tarefas = append(a.tarefas[:i], a.tarefas[i+1:]...)
			break
		}
	}
	return nil
}
func (a *armazemFalso) MensagensRecentes(context.Context, int64, int) ([]salas.Mensagem, error) {
	return nil, nil
}
func (a *armazemFalso) CriarMensagem(_ context.Context, _, autor int64, nome, texto string) (salas.Mensagem, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.proximoID++
	return salas.Mensagem{ID: a.proximoID, AutorID: autor, AutorNome: nome, Texto: texto}, nil
}
func (a *armazemFalso) SalvarConfig(_ context.Context, _ int64, cfg pomodoro.Config, todos bool) error {
	return nil
}

type registroFalso struct {
	mu       sync.Mutex
	sessoes  map[int64]time.Duration
	gravados chan struct{}
}

func (r *registroFalso) Registrar(_ context.Context, usuarioID, _ int64, _ time.Time, d time.Duration) error {
	r.mu.Lock()
	r.sessoes[usuarioID] += d
	r.mu.Unlock()
	r.gravados <- struct{}{}
	return nil
}

// ---- relógio controlado ----

type relogioFalso struct {
	mu    sync.Mutex
	agora time.Time
}

func (r *relogioFalso) Agora() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.agora
}
func (r *relogioFalso) Avancar(d time.Duration) {
	r.mu.Lock()
	r.agora = r.agora.Add(d)
	r.mu.Unlock()
}

// ---- montagem ----

const (
	donoID      = 1
	convidadoID = 2
)

type ambiente struct {
	t        *testing.T
	servidor *httptest.Server
	emissor  *auth.Emissor
	registro *registroFalso
	relogio  *relogioFalso
	gerente  *Gerente
}

func novoAmbiente(t *testing.T) *ambiente {
	t.Helper()
	sala := salas.Sala{ID: 10, Codigo: "ABC234", Nome: "Cálculo", DonoID: donoID, DonoNome: "Dona",
		FocoMin: 25, PausaMin: 5, PausaLongaMin: 15, CiclosAteLonga: 4}
	emissor := auth.NovoEmissor([]byte("um-segredo-de-teste-com-32-caracteres!"))
	reg := &registroFalso{sessoes: map[int64]time.Duration{}, gravados: make(chan struct{}, 10)}
	rel := &relogioFalso{agora: time.Date(2026, 10, 21, 15, 0, 0, 0, time.UTC)}
	g := NovoGerente(&armazemFalso{sala: sala}, reg, emissor, nil)
	g.agora = rel.Agora
	g.intervaloTick = 5 * time.Millisecond

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ws/salas/{codigo}", g.RotaWS)
	srv := httptest.NewServer(mux)
	t.Cleanup(func() {
		srv.Close()
		g.EncerrarTodas()
	})
	return &ambiente{t: t, servidor: srv, emissor: emissor, registro: reg, relogio: rel, gerente: g}
}

type conexao struct {
	t    *testing.T
	conn *websocket.Conn
	ctx  context.Context
}

func (a *ambiente) conectar(usuarioID int64, nome, codigo string) (*conexao, error) {
	token, _ := a.emissor.Emitir(auth.Identidade{UsuarioID: usuarioID, Nome: nome})
	url := "ws" + strings.TrimPrefix(a.servidor.URL, "http") + "/ws/salas/" + codigo + "?token=" + token
	ctx, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
	a.t.Cleanup(cancelar)
	conn, resp, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		if resp != nil {
			return nil, fmt.Errorf("status %d: %w", resp.StatusCode, err)
		}
		return nil, err
	}
	a.t.Cleanup(func() { conn.CloseNow() })
	return &conexao{t: a.t, conn: conn, ctx: ctx}, nil
}

func (a *ambiente) entrar(usuarioID int64, nome string) *conexao {
	a.t.Helper()
	c, err := a.conectar(usuarioID, nome, "ABC234")
	if err != nil {
		a.t.Fatal(err)
	}
	c.esperar("estado")
	return c
}

func (c *conexao) mandar(e Entrada) {
	c.t.Helper()
	dados, _ := json.Marshal(e)
	if err := c.conn.Write(c.ctx, websocket.MessageText, dados); err != nil {
		c.t.Fatal(err)
	}
}

// esperar lê mensagens até chegar uma do tipo pedido (e que passe no filtro, se houver).
func (c *conexao) esperar(tipo string, filtro ...func(Saida) bool) Saida {
	c.t.Helper()
	for {
		_, dados, err := c.conn.Read(c.ctx)
		if err != nil {
			c.t.Fatalf("esperando %q: %v", tipo, err)
		}
		var s Saida
		if err := json.Unmarshal(dados, &s); err != nil {
			c.t.Fatal(err)
		}
		if s.Tipo == tipo && (len(filtro) == 0 || filtro[0](s)) {
			return s
		}
	}
}

// ---- testes ----

func TestEntrarRecebeOEstadoCompleto(t *testing.T) {
	a := novoAmbiente(t)
	c, err := a.conectar(donoID, "Dona", "abc234") // código em minúsculas também vale
	if err != nil {
		t.Fatal(err)
	}
	e := c.esperar("estado")
	if e.Sala == nil || e.Sala.Nome != "Cálculo" || e.Voce == nil || !e.Voce.Dono || !e.Voce.PodeControlar {
		t.Fatalf("estado incompleto: %+v", e)
	}
	if e.Timer == nil || e.Timer.Fase != pomodoro.Foco || e.Timer.RestanteMs != (25*time.Minute).Milliseconds() {
		t.Errorf("relógio errado: %+v", e.Timer)
	}
	if len(e.Membros) != 1 || e.Membros[0].Nome != "Dona" || e.LimiteChamada != LimiteChamada {
		t.Errorf("membros errados: %+v", e.Membros)
	}
}

func TestSalaInexistenteENaoLogado(t *testing.T) {
	a := novoAmbiente(t)
	if _, err := a.conectar(donoID, "Dona", "ZZZ999"); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("esperava 404, veio %v", err)
	}
	url := "ws" + strings.TrimPrefix(a.servidor.URL, "http") + "/ws/salas/ABC234?token=falso"
	_, resp, err := websocket.Dial(context.Background(), url, nil)
	if err == nil || resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("esperava 401, veio %v", err)
	}
}

func TestChatChegaParaTodos(t *testing.T) {
	a := novoAmbiente(t)
	dona := a.entrar(donoID, "Dona")
	conv := a.entrar(convidadoID, "Convidado")

	conv.mandar(Entrada{Tipo: "chat", Texto: "  bora estudar!  "})
	for _, c := range []*conexao{dona, conv} {
		m := c.esperar("chat")
		if m.Mensagem == nil || m.Mensagem.Texto != "bora estudar!" || m.Mensagem.AutorNome != "Convidado" {
			t.Errorf("mensagem errada: %+v", m.Mensagem)
		}
	}
}

func TestChatTemLimiteDeVelocidade(t *testing.T) {
	a := novoAmbiente(t)
	c := a.entrar(donoID, "Dona")
	for i := 0; i < chatsPorJanela; i++ {
		c.mandar(Entrada{Tipo: "chat", Texto: "oi"})
		c.esperar("chat")
	}
	c.mandar(Entrada{Tipo: "chat", Texto: "mais uma"})
	if e := c.esperar("erro"); !strings.Contains(e.Erro, "Espere") {
		t.Errorf("esperava aviso de limite, veio %q", e.Erro)
	}
}

func TestSoODonoControlaORelogioAteLiberar(t *testing.T) {
	a := novoAmbiente(t)
	dona := a.entrar(donoID, "Dona")
	conv := a.entrar(convidadoID, "Convidado")

	conv.mandar(Entrada{Tipo: "timer", Acao: "iniciar"})
	if e := conv.esperar("erro"); !strings.Contains(e.Erro, "relógio") {
		t.Errorf("convidado não deveria controlar: %q", e.Erro)
	}

	dona.mandar(Entrada{Tipo: "config", Config: &ConfigSala{FocoMin: 50, PausaMin: 10, PausaLongaMin: 20, CiclosAteLonga: 3, TodosControlam: true}})
	s := conv.esperar("sala")
	if !s.Voce.PodeControlar || !s.Sala.TodosControlam {
		t.Fatalf("depois de liberar, o convidado deveria poder controlar: %+v", s.Voce)
	}
	conv.mandar(Entrada{Tipo: "timer", Acao: "iniciar"})
	tm := dona.esperar("timer", func(s Saida) bool { return s.Timer.Rodando })
	if tm.Timer.FocoMin != 50 || tm.Timer.RestanteMs != (50*time.Minute).Milliseconds() {
		t.Errorf("relógio deveria rodar com 50 min: %+v", tm.Timer)
	}
}

func TestConvidadoNaoMudaConfig(t *testing.T) {
	a := novoAmbiente(t)
	a.entrar(donoID, "Dona")
	conv := a.entrar(convidadoID, "Convidado")
	conv.mandar(Entrada{Tipo: "config", Config: &ConfigSala{FocoMin: 1, PausaMin: 1, PausaLongaMin: 1, CiclosAteLonga: 1}})
	if e := conv.esperar("erro"); !strings.Contains(e.Erro, "configuração") {
		t.Errorf("esperava recusa, veio %q", e.Erro)
	}
}

func TestFocoFeitoVaiParaORanking(t *testing.T) {
	a := novoAmbiente(t)
	dona := a.entrar(donoID, "Dona")
	conv := a.entrar(convidadoID, "Convidado")

	dona.mandar(Entrada{Tipo: "timer", Acao: "iniciar"})
	dona.esperar("timer", func(s Saida) bool { return s.Timer.Rodando })

	a.relogio.Avancar(25*time.Minute + time.Second) // o foco termina
	f := conv.esperar("fase")
	if f.Terminou != pomodoro.Foco || f.Comecou != pomodoro.Pausa {
		t.Errorf("esperava foco → pausa: %+v", f)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-a.registro.gravados:
		case <-time.After(3 * time.Second):
			t.Fatal("o foco não foi registrado")
		}
	}
	a.registro.mu.Lock()
	defer a.registro.mu.Unlock()
	for _, id := range []int64{donoID, convidadoID} {
		if d := a.registro.sessoes[id]; d < 25*time.Minute || d > 26*time.Minute {
			t.Errorf("usuário %d deveria ter ~25 min, tem %v", id, d)
		}
	}
}

func TestQuemSaiNoMeioDoFocoLevaOTempoQueFicou(t *testing.T) {
	a := novoAmbiente(t)
	dona := a.entrar(donoID, "Dona")
	conv := a.entrar(convidadoID, "Convidado")
	dona.mandar(Entrada{Tipo: "timer", Acao: "iniciar"})
	dona.esperar("timer", func(s Saida) bool { return s.Timer.Rodando })

	a.relogio.Avancar(10 * time.Minute)
	conv.conn.Close(websocket.StatusNormalClosure, "")
	dona.esperar("membros", func(s Saida) bool { return len(s.Membros) == 1 })
	select {
	case <-a.registro.gravados:
	case <-time.After(3 * time.Second):
		t.Fatal("o foco de quem saiu não foi registrado")
	}
	a.registro.mu.Lock()
	defer a.registro.mu.Unlock()
	if d := a.registro.sessoes[convidadoID]; d != 10*time.Minute {
		t.Errorf("esperava 10 min, veio %v", d)
	}
}

func TestDuasAbasDaMesmaPessoaContamComoUmMembro(t *testing.T) {
	a := novoAmbiente(t)
	aba1 := a.entrar(donoID, "Dona")
	aba2 := a.entrar(donoID, "Dona")
	m := aba1.esperar("membros", func(s Saida) bool { return true })
	if len(m.Membros) != 1 {
		t.Errorf("esperava 1 membro, vieram %d", len(m.Membros))
	}
	aba2.conn.Close(websocket.StatusNormalClosure, "")
	m = aba1.esperar("membros")
	if len(m.Membros) != 1 {
		t.Errorf("fechar uma aba não tira a pessoa da sala: %+v", m.Membros)
	}
}

func TestTarefasCompartilhadas(t *testing.T) {
	a := novoAmbiente(t)
	dona := a.entrar(donoID, "Dona")
	conv := a.entrar(convidadoID, "Convidado")

	dona.mandar(Entrada{Tipo: "tarefa_nova", Texto: "Lista 3 de derivadas"})
	lista := conv.esperar("tarefas")
	if len(lista.Tarefas) != 1 || lista.Tarefas[0].Texto != "Lista 3 de derivadas" {
		t.Fatalf("tarefa não chegou: %+v", lista.Tarefas)
	}
	conv.mandar(Entrada{Tipo: "tarefa_marcar", ID: lista.Tarefas[0].ID, Feita: true})
	if l := dona.esperar("tarefas", func(s Saida) bool { return len(s.Tarefas) == 1 && s.Tarefas[0].Feita }); l.Tarefas[0].ID != lista.Tarefas[0].ID {
		t.Error("tarefa errada marcada")
	}
	dona.mandar(Entrada{Tipo: "tarefa_apagar", ID: lista.Tarefas[0].ID})
	if l := conv.esperar("tarefas", func(s Saida) bool { return len(s.Tarefas) == 0 }); len(l.Tarefas) != 0 {
		t.Error("tarefa deveria sumir")
	}
}

func TestChamadaRepassaSinaisSoEntreParticipantes(t *testing.T) {
	a := novoAmbiente(t)
	dona := a.entrar(donoID, "Dona")
	conv := a.entrar(convidadoID, "Convidado")

	dona.mandar(Entrada{Tipo: "chamada_entrar", Camera: true, Microfone: true})
	ch := conv.esperar("chamada", func(s Saida) bool { return len(s.Chamada) == 1 })
	idDona := ch.Chamada[0].ID
	if !ch.Chamada[0].Camera || ch.Chamada[0].Nome != "Dona" {
		t.Errorf("participante errado: %+v", ch.Chamada[0])
	}

	// Fora da chamada, o sinal do convidado é ignorado.
	conv.mandar(Entrada{Tipo: "sinal", Para: idDona, Dados: json.RawMessage(`{"tipo":"ignorado"}`)})

	conv.mandar(Entrada{Tipo: "chamada_entrar", Camera: false, Microfone: true})
	ch = dona.esperar("chamada", func(s Saida) bool { return len(s.Chamada) == 2 })
	var idConv string
	for _, p := range ch.Chamada {
		if p.UsuarioID == convidadoID {
			idConv = p.ID
		}
	}
	conv.mandar(Entrada{Tipo: "sinal", Para: idDona, Dados: json.RawMessage(`{"descricao":{"type":"offer","sdp":"v=0"}}`)})
	s := dona.esperar("sinal")
	if s.De != idConv || !strings.Contains(string(s.Dados), `"offer"`) {
		t.Errorf("sinal errado: %+v", s)
	}

	conv.mandar(Entrada{Tipo: "midia", Camera: true, Microfone: false, Tela: true})
	ch = dona.esperar("chamada", func(s Saida) bool {
		for _, p := range s.Chamada {
			if p.ID == idConv && p.Tela {
				return true
			}
		}
		return false
	})

	conv.mandar(Entrada{Tipo: "chamada_sair"})
	dona.esperar("chamada", func(s Saida) bool { return len(s.Chamada) == 1 })
}

func TestChamadaTemLimiteDePessoas(t *testing.T) {
	a := novoAmbiente(t)
	var conexoes []*conexao
	for i := 0; i < LimiteChamada; i++ {
		c := a.entrar(int64(100+i), fmt.Sprintf("Pessoa %d", i))
		c.mandar(Entrada{Tipo: "chamada_entrar"})
		c.esperar("chamada", func(s Saida) bool { return len(s.Chamada) == i+1 })
		conexoes = append(conexoes, c) // mantém as conexões abertas até o fim do teste
	}
	extra := a.entrar(999, "Atrasada")
	extra.mandar(Entrada{Tipo: "chamada_entrar"})
	if e := extra.esperar("erro"); !strings.Contains(e.Erro, "cheia") {
		t.Errorf("esperava chamada cheia, veio %q", e.Erro)
	}
	_ = conexoes
}

func TestSalaVaziaSaiDaMemoria(t *testing.T) {
	a := novoAmbiente(t)
	c := a.entrar(donoID, "Dona")
	if _, ok := a.gerente.Resumo("ABC234"); !ok {
		t.Fatal("a sala deveria estar aberta")
	}
	c.conn.Close(websocket.StatusNormalClosure, "")
	time.Sleep(50 * time.Millisecond)
	a.relogio.Avancar(tempoVazia + time.Second)
	prazo := time.Now().Add(3 * time.Second)
	for time.Now().Before(prazo) {
		if _, ok := a.gerente.Resumo("ABC234"); !ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Error("a sala vazia deveria sair da memória")
}
