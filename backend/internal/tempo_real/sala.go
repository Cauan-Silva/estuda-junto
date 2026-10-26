package tempo_real

import (
	"context"
	"log/slog"
	"sort"
	"sync/atomic"
	"time"

	"github.com/Cauan-Silva/estuda-junto/internal/pomodoro"
	"github.com/Cauan-Silva/estuda-junto/internal/salas"
)

const (
	LimiteChamada      = 6 // pessoas na chamada de vídeo (cada uma manda vídeo para todas as outras)
	mensagensGuardadas = 50
	maxTarefas         = 100
	chatsPorJanela     = 5               // até 5 mensagens...
	janelaChat         = 5 * time.Second // ...a cada 5 segundos por conexão
	tempoVazia         = 5 * time.Minute // sala sem ninguém por esse tempo sai da memória
)

type recebido struct {
	cliente *Cliente
	entrada Entrada
}

// Resumo é o que a lista de salas mostra sem incomodar a goroutine da sala.
type Resumo struct {
	Online  int
	Fase    pomodoro.Fase
	Rodando bool
}

// Sala é o ator: só a goroutine de rodar() mexe nos campos abaixo de "estado".
type Sala struct {
	codigo   string
	gerente  *Gerente
	entrar   chan *Cliente
	sair     chan *Cliente
	recebido chan recebido
	encerrar chan struct{} // pedido de desligamento (servidor parando)
	parada   chan struct{} // fechado quando a goroutine da sala termina
	ocupacao atomic.Int32  // conexões registradas (controlado com a trava do gerente)
	resumo   atomic.Pointer[Resumo]

	// estado
	info       salas.Sala
	relogio    *pomodoro.Relogio
	clientes   map[*Cliente]bool
	membros    map[int64]*Membro
	focoDesde  map[int64]time.Time // desde quando cada pessoa está acumulando foco
	tarefas    []salas.Tarefa
	mensagens  []salas.Mensagem
	vaziaDesde time.Time
}

func novaSala(g *Gerente, info salas.Sala, tarefas []salas.Tarefa, mensagens []salas.Mensagem) *Sala {
	s := &Sala{
		codigo:    info.Codigo,
		gerente:   g,
		entrar:    make(chan *Cliente),
		sair:      make(chan *Cliente, 16),
		recebido:  make(chan recebido, 256),
		encerrar:  make(chan struct{}),
		parada:    make(chan struct{}),
		info:      info,
		relogio:   pomodoro.Novo(info.Pomodoro()),
		clientes:  map[*Cliente]bool{},
		membros:   map[int64]*Membro{},
		focoDesde: map[int64]time.Time{},
		tarefas:   tarefas,
		mensagens: mensagens,
	}
	s.resumo.Store(&Resumo{Fase: pomodoro.Foco})
	return s
}

func (s *Sala) rodar() {
	relogio := time.NewTicker(s.gerente.intervaloTick)
	defer relogio.Stop()
	defer close(s.parada)
	s.vaziaDesde = s.gerente.agora()

	for {
		select {
		case c := <-s.entrar:
			s.aoEntrar(c)
		case c := <-s.sair:
			s.aoSair(c)
		case r := <-s.recebido:
			if s.clientes[r.cliente] {
				s.tratar(r.cliente, r.entrada)
			}
		case <-s.encerrar:
			// Servidor desligando: guarda o foco de quem estava estudando e fecha as conexões
			// (os navegadores reconectam sozinhos quando o servidor voltar).
			agora := s.gerente.agora()
			for id, desde := range s.focoDesde {
				s.gerente.registrarEstudo(id, s.info.ID, desde, agora.Sub(desde))
			}
			for c := range s.clientes {
				c.fechar()
			}
			return
		case <-relogio.C:
			agora := s.gerente.agora()
			if transicoes := s.relogio.Avancar(agora); len(transicoes) > 0 {
				for _, t := range transicoes {
					s.contarFoco(agora)
					s.paraTodos(Saida{Tipo: "fase", Terminou: t.Terminou, Comecou: t.Comecou})
				}
				s.enviarTimer()
				s.enviarMembros()
			}
			if len(s.clientes) == 0 && agora.Sub(s.vaziaDesde) >= tempoVazia {
				if s.gerente.tentarEncerrar(s) {
					return
				}
			}
		}
	}
}

// ---- entrar e sair ----

func (s *Sala) aoEntrar(c *Cliente) {
	s.clientes[c] = true
	m := s.membros[c.UsuarioID]
	if m == nil {
		m = &Membro{UsuarioID: c.UsuarioID, Nome: c.Nome}
		s.membros[c.UsuarioID] = m
	}
	m.conexoes++
	s.contarFoco(s.gerente.agora())

	agora := s.gerente.agora()
	timer := s.relogio.Estado(agora)
	c.enviar(Saida{
		Tipo:          "estado",
		Sala:          s.infoSala(),
		Voce:          s.voce(c),
		Timer:         &timer,
		Membros:       s.listaMembros(),
		Tarefas:       s.tarefas,
		Mensagens:     s.mensagens,
		Chamada:       s.listaChamada(),
		LimiteChamada: LimiteChamada,
	})
	s.enviarMembros()
	s.atualizarResumo()
}

func (s *Sala) aoSair(c *Cliente) {
	if !s.clientes[c] {
		return
	}
	delete(s.clientes, c)
	s.gerente.liberar(s)
	estavaNaChamada := c.naChamada
	c.naChamada = false

	if m := s.membros[c.UsuarioID]; m != nil {
		m.conexoes--
		if m.conexoes <= 0 {
			delete(s.membros, c.UsuarioID)
		}
	}
	s.contarFoco(s.gerente.agora())
	if estavaNaChamada {
		s.atualizarNaChamada(c.UsuarioID)
		s.enviarChamada()
	}
	s.enviarMembros()
	if len(s.clientes) == 0 {
		s.vaziaDesde = s.gerente.agora()
	}
	s.atualizarResumo()
}

// ---- mensagens do navegador ----

func (s *Sala) tratar(c *Cliente, e Entrada) {
	agora := s.gerente.agora()
	switch e.Tipo {
	case "chat":
		s.chat(c, e.Texto, agora)

	case "timer":
		if !s.podeControlar(c) {
			c.enviar(Saida{Tipo: "erro", Erro: "Só quem criou a sala controla o relógio."})
			return
		}
		mudou := false
		switch e.Acao {
		case "iniciar":
			mudou = s.relogio.Iniciar(agora)
		case "pausar":
			mudou = s.relogio.Pausar(agora)
		case "pular":
			t := s.relogio.Pular(agora)
			s.contarFoco(agora)
			s.paraTodos(Saida{Tipo: "fase", Terminou: t.Terminou, Comecou: t.Comecou})
			mudou = true
		case "reiniciar":
			s.relogio.Reiniciar()
			mudou = true
		}
		if mudou {
			s.contarFoco(agora)
			s.enviarTimer()
			s.enviarMembros()
			s.atualizarResumo()
		}

	case "config":
		s.mudarConfig(c, e.Config)

	case "status":
		texto, ok := salas.LimparTexto(e.Texto, 60)
		if !ok && texto != "" {
			c.enviar(Saida{Tipo: "erro", Erro: "O status pode ter até 60 caracteres."})
			return
		}
		if m := s.membros[c.UsuarioID]; m != nil && m.Status != texto {
			m.Status = texto
			s.enviarMembros()
		}

	case "tarefa_nova":
		texto, ok := salas.LimparTexto(e.Texto, 200)
		if !ok {
			c.enviar(Saida{Tipo: "erro", Erro: "A tarefa precisa ter de 1 a 200 caracteres."})
			return
		}
		if len(s.tarefas) >= maxTarefas {
			c.enviar(Saida{Tipo: "erro", Erro: "O quadro já tem 100 tarefas. Apague as concluídas."})
			return
		}
		s.mexerNasTarefas(c, func(ctx context.Context) error {
			return s.gerente.armazem.CriarTarefa(ctx, s.info.ID, c.UsuarioID, texto)
		})

	case "tarefa_marcar":
		s.mexerNasTarefas(c, func(ctx context.Context) error {
			return s.gerente.armazem.MarcarTarefa(ctx, s.info.ID, e.ID, c.UsuarioID, e.Feita)
		})

	case "tarefa_apagar":
		s.mexerNasTarefas(c, func(ctx context.Context) error {
			return s.gerente.armazem.ApagarTarefa(ctx, s.info.ID, e.ID)
		})

	case "chamada_entrar":
		s.entrarNaChamada(c, e)

	case "chamada_sair":
		if c.naChamada {
			c.naChamada = false
			s.atualizarNaChamada(c.UsuarioID)
			s.enviarChamada()
			s.enviarMembros()
		}

	case "midia":
		if c.naChamada {
			c.midia.Camera, c.midia.Microfone, c.midia.Tela = e.Camera, e.Microfone, e.Tela
			s.enviarChamada()
		}

	case "sinal":
		s.repassarSinal(c, e)

	default:
		c.enviar(Saida{Tipo: "erro", Erro: "Tipo de mensagem desconhecido."})
	}
}

func (s *Sala) chat(c *Cliente, texto string, agora time.Time) {
	texto, ok := salas.LimparTexto(texto, 500)
	if !ok {
		c.enviar(Saida{Tipo: "erro", Erro: "A mensagem precisa ter de 1 a 500 caracteres."})
		return
	}
	// Limite simples: no máximo 5 mensagens em 5 segundos por conexão.
	recentes := c.ultimosChats[:0]
	for _, t := range c.ultimosChats {
		if agora.Sub(t) < janelaChat {
			recentes = append(recentes, t)
		}
	}
	c.ultimosChats = recentes
	if len(recentes) >= chatsPorJanela {
		c.enviar(Saida{Tipo: "erro", Erro: "Calma! Espere alguns segundos para mandar outra mensagem."})
		return
	}
	c.ultimosChats = append(c.ultimosChats, agora)

	ctx, cancelar := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelar()
	m, err := s.gerente.armazem.CriarMensagem(ctx, s.info.ID, c.UsuarioID, c.Nome, texto)
	if err != nil {
		slog.Error("não foi possível salvar a mensagem", "sala", s.codigo, "erro", err)
		c.enviar(Saida{Tipo: "erro", Erro: "Não foi possível enviar a mensagem."})
		return
	}
	s.mensagens = append(s.mensagens, m)
	if len(s.mensagens) > mensagensGuardadas {
		s.mensagens = s.mensagens[len(s.mensagens)-mensagensGuardadas:]
	}
	s.paraTodos(Saida{Tipo: "chat", Mensagem: &m})
}

func (s *Sala) mexerNasTarefas(c *Cliente, mudar func(context.Context) error) {
	ctx, cancelar := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelar()
	if err := mudar(ctx); err != nil {
		slog.Error("não foi possível alterar a tarefa", "sala", s.codigo, "erro", err)
		c.enviar(Saida{Tipo: "erro", Erro: "Não foi possível salvar a tarefa."})
		return
	}
	tarefas, err := s.gerente.armazem.Tarefas(ctx, s.info.ID)
	if err != nil {
		slog.Error("não foi possível ler as tarefas", "sala", s.codigo, "erro", err)
		return
	}
	s.tarefas = tarefas
	s.paraTodos(Saida{Tipo: "tarefas", Tarefas: s.tarefas})
}

func (s *Sala) mudarConfig(c *Cliente, cfg *ConfigSala) {
	if c.UsuarioID != s.info.DonoID {
		c.enviar(Saida{Tipo: "erro", Erro: "Só quem criou a sala muda a configuração."})
		return
	}
	if cfg == nil {
		return
	}
	novo := cfg.Pomodoro()
	if err := novo.Validar(); err != nil {
		c.enviar(Saida{Tipo: "erro", Erro: "Configuração inválida: " + err.Error() + "."})
		return
	}
	ctx, cancelar := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelar()
	if err := s.gerente.armazem.SalvarConfig(ctx, s.info.ID, novo, cfg.TodosControlam); err != nil {
		slog.Error("não foi possível salvar a configuração", "sala", s.codigo, "erro", err)
		c.enviar(Saida{Tipo: "erro", Erro: "Não foi possível salvar a configuração."})
		return
	}
	s.info.FocoMin, s.info.PausaMin, s.info.PausaLongaMin = cfg.FocoMin, cfg.PausaMin, cfg.PausaLongaMin
	s.info.CiclosAteLonga, s.info.TodosControlam = cfg.CiclosAteLonga, cfg.TodosControlam
	s.relogio.MudarConfig(novo)
	s.enviarTimer()
	for cl := range s.clientes {
		cl.enviar(Saida{Tipo: "sala", Sala: s.infoSala(), Voce: s.voce(cl)})
	}
}

// ---- chamada de vídeo (WebRTC) ----
//
// O servidor não toca no vídeo: ele só repassa os "sinais" (oferta, resposta e candidatos
// de conexão) entre as abas que estão na chamada. Quem entra manda a oferta para cada
// participante que já estava lá; o vídeo vai direto de navegador para navegador.

func (s *Sala) entrarNaChamada(c *Cliente, e Entrada) {
	if c.naChamada {
		return
	}
	if len(s.participantes()) >= LimiteChamada {
		c.enviar(Saida{Tipo: "erro", Erro: "A chamada está cheia (máximo de 6 pessoas)."})
		return
	}
	c.naChamada = true
	c.midia = Participante{ID: c.ID, UsuarioID: c.UsuarioID, Nome: c.Nome, Camera: e.Camera, Microfone: e.Microfone}
	s.atualizarNaChamada(c.UsuarioID)
	s.enviarChamada()
	s.enviarMembros()
}

func (s *Sala) repassarSinal(c *Cliente, e Entrada) {
	if !c.naChamada || len(e.Dados) == 0 {
		return
	}
	for destino := range s.clientes {
		if destino.ID == e.Para && destino.naChamada && destino != c {
			destino.enviar(Saida{Tipo: "sinal", De: c.ID, Dados: e.Dados})
			return
		}
	}
}

func (s *Sala) participantes() []*Cliente {
	lista := []*Cliente{}
	for c := range s.clientes {
		if c.naChamada {
			lista = append(lista, c)
		}
	}
	return lista
}

func (s *Sala) listaChamada() []Participante {
	lista := []Participante{}
	for _, c := range s.participantes() {
		lista = append(lista, c.midia)
	}
	sort.Slice(lista, func(i, j int) bool { return lista[i].ID < lista[j].ID })
	return lista
}

func (s *Sala) atualizarNaChamada(usuarioID int64) {
	m := s.membros[usuarioID]
	if m == nil {
		return
	}
	m.NaChamada = false
	for c := range s.clientes {
		if c.UsuarioID == usuarioID && c.naChamada {
			m.NaChamada = true
		}
	}
}

// ---- tempo de foco ----

// contarFoco liga ou desliga a contagem de cada pessoa. Conta quem está na sala enquanto
// o relógio roda na fase de foco; ao parar, o trecho vai para o banco (em segundo plano,
// para a sala não esperar o banco).
func (s *Sala) contarFoco(agora time.Time) {
	emFoco := s.relogio.EmFoco()
	for id, m := range s.membros {
		_, contando := s.focoDesde[id]
		if emFoco && !contando {
			s.focoDesde[id] = agora
		}
		m.Focando = emFoco
	}
	for id, desde := range s.focoDesde {
		if _, presente := s.membros[id]; emFoco && presente {
			continue
		}
		delete(s.focoDesde, id)
		s.gerente.registrarEstudo(id, s.info.ID, desde, agora.Sub(desde))
	}
}

// ---- envio ----

func (s *Sala) podeControlar(c *Cliente) bool {
	return c.UsuarioID == s.info.DonoID || s.info.TodosControlam
}

func (s *Sala) voce(c *Cliente) *Voce {
	return &Voce{ConexaoID: c.ID, UsuarioID: c.UsuarioID, Dono: c.UsuarioID == s.info.DonoID, PodeControlar: s.podeControlar(c)}
}

func (s *Sala) infoSala() *InfoSala {
	return &InfoSala{
		Codigo: s.info.Codigo, Nome: s.info.Nome, Descricao: s.info.Descricao, Publica: s.info.Publica,
		DonoID: s.info.DonoID, DonoNome: s.info.DonoNome, TodosControlam: s.info.TodosControlam,
	}
}

func (s *Sala) listaMembros() []Membro {
	lista := make([]Membro, 0, len(s.membros))
	for _, m := range s.membros {
		lista = append(lista, *m)
	}
	sort.Slice(lista, func(i, j int) bool { return lista[i].Nome < lista[j].Nome })
	return lista
}

func (s *Sala) paraTodos(msg Saida) {
	for c := range s.clientes {
		c.enviar(msg)
	}
}

func (s *Sala) enviarTimer() {
	e := s.relogio.Estado(s.gerente.agora())
	s.paraTodos(Saida{Tipo: "timer", Timer: &e})
}

func (s *Sala) enviarMembros() {
	s.paraTodos(Saida{Tipo: "membros", Membros: s.listaMembros()})
}

func (s *Sala) enviarChamada() {
	s.paraTodos(Saida{Tipo: "chamada", Chamada: s.listaChamada()})
}

func (s *Sala) atualizarResumo() {
	s.resumo.Store(&Resumo{Online: len(s.membros), Fase: s.relogio.Fase(), Rodando: s.relogio.Rodando()})
}
