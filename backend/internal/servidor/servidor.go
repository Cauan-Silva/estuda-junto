// Package servidor junta as peças: rotas HTTP, WebSocket e telas.
package servidor

import (
	"database/sql"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Cauan-Silva/estuda-junto/internal/api"
	"github.com/Cauan-Silva/estuda-junto/internal/auth"
	"github.com/Cauan-Silva/estuda-junto/internal/config"
	"github.com/Cauan-Silva/estuda-junto/internal/estudo"
	"github.com/Cauan-Silva/estuda-junto/internal/salas"
	"github.com/Cauan-Silva/estuda-junto/internal/tempo_real"
	"github.com/Cauan-Silva/estuda-junto/internal/usuarios"
	"github.com/Cauan-Silva/estuda-junto/internal/web"
)

type Servidor struct {
	cfg      config.Config
	emissor  *auth.Emissor
	usuarios *usuarios.Servico
	salas    *salas.Repositorio
	estudo   *estudo.Servico
	Gerente  *tempo_real.Gerente
}

func Novo(cfg config.Config, db *sql.DB) *Servidor {
	emissor := auth.NovoEmissor(cfg.SegredoJWT)
	repo := salas.NovoRepositorio(db)
	est := estudo.NovoServico(db)
	return &Servidor{
		cfg:      cfg,
		emissor:  emissor,
		usuarios: usuarios.NovoServico(db, emissor),
		salas:    repo,
		estudo:   est,
		Gerente:  tempo_real.NovoGerente(repo, est, emissor, cfg.OrigensWS),
	}
}

// Rotas monta o roteador (padrões do net/http do Go 1.22+: "MÉTODO /caminho/{variavel}").
func (s *Servidor) Rotas() http.Handler {
	mux := http.NewServeMux()
	logado := func(m api.Manipulador) http.Handler { return api.ExigirLogin(s.emissor, m) }

	mux.Handle("GET /api/saude", api.Manipulador(func(w http.ResponseWriter, r *http.Request) error {
		api.Responder(w, http.StatusOK, map[string]string{"status": "ok"})
		return nil
	}))
	mux.Handle("POST /api/auth/cadastro", limitarPorIP(api.Manipulador(s.usuarios.Cadastrar)))
	mux.Handle("POST /api/auth/login", limitarPorIP(api.Manipulador(s.usuarios.Entrar)))

	mux.Handle("GET /api/eu", logado(s.eu))
	mux.Handle("GET /api/rtc", logado(s.rtc))
	mux.Handle("GET /api/ranking", logado(s.estudo.RotaRanking))

	mux.Handle("GET /api/salas", logado(s.listarSalas))
	mux.Handle("POST /api/salas", logado(s.criarSala))
	mux.Handle("GET /api/salas/{codigo}", logado(s.buscarSala))
	mux.Handle("GET /api/salas/{codigo}/ranking", logado(s.rankingDaSala))

	mux.HandleFunc("GET /ws/salas/{codigo}", s.Gerente.RotaWS)

	// Qualquer /api desconhecida é 404 em JSON; o resto são as telas.
	mux.Handle("/api/", api.Manipulador(func(w http.ResponseWriter, r *http.Request) error {
		return api.NovoErro(http.StatusNotFound, "Rota não encontrada.")
	}))
	mux.Handle("/", web.Telas())

	return registrar(cabecalhosDeSeguranca(mux))
}

// ---- eu ----

func (s *Servidor) eu(w http.ResponseWriter, r *http.Request) error {
	u, err := s.usuarios.Buscar(r.Context(), api.Quem(r).UsuarioID)
	if err != nil {
		return err
	}
	est, err := s.estudo.DoUsuario(r.Context(), u.ID)
	if err != nil {
		return err
	}
	api.Responder(w, http.StatusOK, map[string]any{"usuario": u, "estatisticas": est})
	return nil
}

// rtc devolve os servidores que o navegador usa para achar o caminho até os outros na chamada.
// STUN (público e gratuito) descobre o endereço; TURN (opcional) retransmite quando a
// conexão direta é bloqueada.
func (s *Servidor) rtc(w http.ResponseWriter, r *http.Request) error {
	servidores := []map[string]any{
		{"urls": []string{"stun:stun.l.google.com:19302", "stun:stun1.l.google.com:19302"}},
	}
	if len(s.cfg.TurnURLs) > 0 {
		servidores = append(servidores, map[string]any{
			"urls": s.cfg.TurnURLs, "username": s.cfg.TurnUsuario, "credential": s.cfg.TurnSenha,
		})
	}
	api.Responder(w, http.StatusOK, map[string]any{
		"iceServers":    servidores,
		"limiteChamada": tempo_real.LimiteChamada,
	})
	return nil
}

// ---- salas ----

type salaComResumo struct {
	salas.Sala
	Online  int    `json:"online"`
	Fase    string `json:"fase,omitempty"`
	Rodando bool   `json:"rodando"`
}

func (s *Servidor) comResumo(lista []salas.Sala) []salaComResumo {
	saida := make([]salaComResumo, 0, len(lista))
	for _, sala := range lista {
		item := salaComResumo{Sala: sala}
		if r, ok := s.Gerente.Resumo(sala.Codigo); ok {
			item.Online, item.Fase, item.Rodando = r.Online, string(r.Fase), r.Rodando
		}
		saida = append(saida, item)
	}
	return saida
}

// listarSalas devolve as públicas (com quem está online primeiro) e as da própria pessoa.
func (s *Servidor) listarSalas(w http.ResponseWriter, r *http.Request) error {
	publicas, err := s.salas.Publicas(r.Context(), 50)
	if err != nil {
		return err
	}
	minhas, err := s.salas.DoDono(r.Context(), api.Quem(r).UsuarioID)
	if err != nil {
		return err
	}
	lista := s.comResumo(publicas)
	ordenarPorOnline(lista)
	api.Responder(w, http.StatusOK, map[string]any{"publicas": lista, "minhas": s.comResumo(minhas)})
	return nil
}

func ordenarPorOnline(lista []salaComResumo) {
	// Ordenação estável por número de pessoas online (as recém-criadas continuam na frente no empate).
	for i := 1; i < len(lista); i++ {
		for j := i; j > 0 && lista[j].Online > lista[j-1].Online; j-- {
			lista[j], lista[j-1] = lista[j-1], lista[j]
		}
	}
}

type pedidoSala struct {
	Nome      string `json:"nome"`
	Descricao string `json:"descricao"`
	Publica   bool   `json:"publica"`
}

func (s *Servidor) criarSala(w http.ResponseWriter, r *http.Request) error {
	var p pedidoSala
	if err := api.Ler(r, &p); err != nil {
		return err
	}
	nome, descricao, err := salas.ValidarSala(p.Nome, p.Descricao)
	if err != nil {
		return err
	}
	sala, err := s.salas.Criar(r.Context(), api.Quem(r).UsuarioID, nome, descricao, p.Publica)
	if err != nil {
		return err
	}
	api.Responder(w, http.StatusCreated, sala)
	return nil
}

func (s *Servidor) buscarSala(w http.ResponseWriter, r *http.Request) error {
	sala, err := s.salas.PorCodigo(r.Context(), r.PathValue("codigo"))
	if err != nil {
		return err
	}
	api.Responder(w, http.StatusOK, s.comResumo([]salas.Sala{sala})[0])
	return nil
}

func (s *Servidor) rankingDaSala(w http.ResponseWriter, r *http.Request) error {
	sala, err := s.salas.PorCodigo(r.Context(), r.PathValue("codigo"))
	if err != nil {
		return err
	}
	lista, periodo, err := s.estudo.Ranking(r.Context(), r.URL.Query().Get("periodo"), sala.ID)
	if err != nil {
		return err
	}
	api.Responder(w, http.StatusOK, map[string]any{"periodo": periodo, "posicoes": lista})
	return nil
}

// ---- middlewares ----

func cabecalhosDeSeguranca(proximo http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("X-Frame-Options", "DENY")
		// Câmera, microfone e tela só para o próprio site.
		h.Set("Permissions-Policy", "camera=(self), microphone=(self), display-capture=(self)")
		proximo.ServeHTTP(w, r)
	})
}

type gravadorStatus struct {
	http.ResponseWriter
	status int
}

func (g *gravadorStatus) WriteHeader(status int) {
	g.status = status
	g.ResponseWriter.WriteHeader(status)
}

// Unwrap deixa o WebSocket chegar na conexão original (o http.ResponseController usa isto).
func (g *gravadorStatus) Unwrap() http.ResponseWriter { return g.ResponseWriter }

// registrar escreve uma linha de log por pedido da API (sem a query, que pode ter o token).
func registrar(proximo http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			proximo.ServeHTTP(w, r)
			return
		}
		inicio := time.Now()
		g := &gravadorStatus{ResponseWriter: w, status: http.StatusOK}
		proximo.ServeHTTP(g, r)
		slog.Info("pedido", "metodo", r.Method, "caminho", r.URL.Path, "status", g.status,
			"ms", time.Since(inicio).Milliseconds())
	})
}
