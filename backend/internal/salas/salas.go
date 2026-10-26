// Package salas guarda as salas de estudo, as tarefas e as mensagens no banco, e tem as
// rotas REST de listar, criar e buscar salas.
package salas

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/lib/pq"

	"github.com/Cauan-Silva/estuda-junto/internal/api"
	"github.com/Cauan-Silva/estuda-junto/internal/pomodoro"
)

type Sala struct {
	ID             int64     `json:"id"`
	Codigo         string    `json:"codigo"`
	Nome           string    `json:"nome"`
	Descricao      string    `json:"descricao"`
	Publica        bool      `json:"publica"`
	DonoID         int64     `json:"donoId"`
	DonoNome       string    `json:"donoNome"`
	FocoMin        int       `json:"focoMin"`
	PausaMin       int       `json:"pausaMin"`
	PausaLongaMin  int       `json:"pausaLongaMin"`
	CiclosAteLonga int       `json:"ciclosAteLonga"`
	TodosControlam bool      `json:"todosControlam"`
	CriadaEm       time.Time `json:"criadaEm"`
}

// Pomodoro devolve as durações da sala no formato do relógio.
func (s Sala) Pomodoro() pomodoro.Config {
	return pomodoro.Config{
		Foco:           time.Duration(s.FocoMin) * time.Minute,
		Pausa:          time.Duration(s.PausaMin) * time.Minute,
		PausaLonga:     time.Duration(s.PausaLongaMin) * time.Minute,
		CiclosAteLonga: s.CiclosAteLonga,
	}
}

type Tarefa struct {
	ID        int64     `json:"id"`
	Texto     string    `json:"texto"`
	Feita     bool      `json:"feita"`
	AutorID   int64     `json:"autorId"`
	AutorNome string    `json:"autorNome"`
	FeitaPor  string    `json:"feitaPor,omitempty"`
	CriadaEm  time.Time `json:"criadaEm"`
}

type Mensagem struct {
	ID        int64     `json:"id"`
	AutorID   int64     `json:"autorId"`
	AutorNome string    `json:"autorNome"`
	Texto     string    `json:"texto"`
	CriadaEm  time.Time `json:"criadaEm"`
}

var ErrNaoEncontrada = api.NovoErro(http.StatusNotFound, "Sala não encontrada. Confira o código.")

// Repositorio fala com o PostgreSQL.
type Repositorio struct {
	db *sql.DB
}

func NovoRepositorio(db *sql.DB) *Repositorio { return &Repositorio{db: db} }

const colunasSala = `s.id, s.codigo, s.nome, s.descricao, s.publica, s.dono_id, u.nome,
	s.foco_min, s.pausa_min, s.pausa_longa_min, s.ciclos_ate_longa, s.todos_controlam, s.criada_em`

func lerSala(linha interface{ Scan(...any) error }) (Sala, error) {
	var s Sala
	err := linha.Scan(&s.ID, &s.Codigo, &s.Nome, &s.Descricao, &s.Publica, &s.DonoID, &s.DonoNome,
		&s.FocoMin, &s.PausaMin, &s.PausaLongaMin, &s.CiclosAteLonga, &s.TodosControlam, &s.CriadaEm)
	return s, err
}

// PorCodigo busca a sala pelo código de 6 letras (maiúsculas ou minúsculas).
func (r *Repositorio) PorCodigo(ctx context.Context, codigo string) (Sala, error) {
	s, err := lerSala(r.db.QueryRowContext(ctx,
		`SELECT `+colunasSala+` FROM salas s JOIN usuarios u ON u.id = s.dono_id WHERE s.codigo = $1`,
		strings.ToUpper(strings.TrimSpace(codigo))))
	if errors.Is(err, sql.ErrNoRows) {
		return s, ErrNaoEncontrada
	}
	return s, err
}

// Publicas lista as salas públicas mais recentes.
func (r *Repositorio) Publicas(ctx context.Context, limite int) ([]Sala, error) {
	linhas, err := r.db.QueryContext(ctx,
		`SELECT `+colunasSala+` FROM salas s JOIN usuarios u ON u.id = s.dono_id
		 WHERE s.publica ORDER BY s.criada_em DESC LIMIT $1`, limite)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	lista := []Sala{}
	for linhas.Next() {
		s, err := lerSala(linhas)
		if err != nil {
			return nil, err
		}
		lista = append(lista, s)
	}
	return lista, linhas.Err()
}

// DoDono lista as salas criadas pela pessoa (inclusive as privadas).
func (r *Repositorio) DoDono(ctx context.Context, donoID int64) ([]Sala, error) {
	linhas, err := r.db.QueryContext(ctx,
		`SELECT `+colunasSala+` FROM salas s JOIN usuarios u ON u.id = s.dono_id
		 WHERE s.dono_id = $1 ORDER BY s.criada_em DESC`, donoID)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	lista := []Sala{}
	for linhas.Next() {
		s, err := lerSala(linhas)
		if err != nil {
			return nil, err
		}
		lista = append(lista, s)
	}
	return lista, linhas.Err()
}

// Letras do código: sem 0/O e 1/I/L, para não confundir quem digita.
const letrasCodigo = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

func NovoCodigo() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = letrasCodigo[int(b[i])%len(letrasCodigo)]
	}
	return string(b)
}

// Criar grava uma sala nova com um código livre.
func (r *Repositorio) Criar(ctx context.Context, donoID int64, nome, descricao string, publica bool) (Sala, error) {
	for tentativa := 0; tentativa < 5; tentativa++ {
		var id int64
		err := r.db.QueryRowContext(ctx,
			`INSERT INTO salas (codigo, nome, descricao, publica, dono_id) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			NovoCodigo(), nome, descricao, publica, donoID).Scan(&id)
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			continue // código repetido: sorteia outro
		}
		if err != nil {
			return Sala{}, err
		}
		return lerSala(r.db.QueryRowContext(ctx,
			`SELECT `+colunasSala+` FROM salas s JOIN usuarios u ON u.id = s.dono_id WHERE s.id = $1`, id))
	}
	return Sala{}, errors.New("não foi possível gerar um código de sala livre")
}

// SalvarConfig grava as durações do pomodoro e quem pode controlar o relógio.
func (r *Repositorio) SalvarConfig(ctx context.Context, salaID int64, cfg pomodoro.Config, todosControlam bool) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE salas SET foco_min = $2, pausa_min = $3, pausa_longa_min = $4, ciclos_ate_longa = $5, todos_controlam = $6
		 WHERE id = $1`,
		salaID, int(cfg.Foco/time.Minute), int(cfg.Pausa/time.Minute), int(cfg.PausaLonga/time.Minute),
		cfg.CiclosAteLonga, todosControlam)
	return err
}

// ---- tarefas ----

func (r *Repositorio) Tarefas(ctx context.Context, salaID int64) ([]Tarefa, error) {
	linhas, err := r.db.QueryContext(ctx, `
		SELECT t.id, t.texto, t.feita, t.autor_id, a.nome, COALESCE(f.nome, ''), t.criada_em
		FROM tarefas t
		JOIN usuarios a ON a.id = t.autor_id
		LEFT JOIN usuarios f ON f.id = t.feita_por
		WHERE t.sala_id = $1 ORDER BY t.id LIMIT 200`, salaID)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	lista := []Tarefa{}
	for linhas.Next() {
		var t Tarefa
		if err := linhas.Scan(&t.ID, &t.Texto, &t.Feita, &t.AutorID, &t.AutorNome, &t.FeitaPor, &t.CriadaEm); err != nil {
			return nil, err
		}
		lista = append(lista, t)
	}
	return lista, linhas.Err()
}

func (r *Repositorio) CriarTarefa(ctx context.Context, salaID, autorID int64, texto string) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO tarefas (sala_id, autor_id, texto) VALUES ($1, $2, $3)`,
		salaID, autorID, texto)
	return err
}

func (r *Repositorio) MarcarTarefa(ctx context.Context, salaID, tarefaID, porID int64, feita bool) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE tarefas SET feita = $3, feita_por = CASE WHEN $3 THEN $4::BIGINT END WHERE sala_id = $1 AND id = $2`,
		salaID, tarefaID, feita, porID)
	return err
}

func (r *Repositorio) ApagarTarefa(ctx context.Context, salaID, tarefaID int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM tarefas WHERE sala_id = $1 AND id = $2`, salaID, tarefaID)
	return err
}

// ---- mensagens ----

// MensagensRecentes devolve as últimas mensagens, da mais antiga para a mais nova.
func (r *Repositorio) MensagensRecentes(ctx context.Context, salaID int64, limite int) ([]Mensagem, error) {
	linhas, err := r.db.QueryContext(ctx, `
		SELECT * FROM (
			SELECT m.id, m.autor_id, u.nome, m.texto, m.criada_em
			FROM mensagens m JOIN usuarios u ON u.id = m.autor_id
			WHERE m.sala_id = $1 ORDER BY m.id DESC LIMIT $2
		) recentes ORDER BY id`, salaID, limite)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	lista := []Mensagem{}
	for linhas.Next() {
		var m Mensagem
		if err := linhas.Scan(&m.ID, &m.AutorID, &m.AutorNome, &m.Texto, &m.CriadaEm); err != nil {
			return nil, err
		}
		lista = append(lista, m)
	}
	return lista, linhas.Err()
}

func (r *Repositorio) CriarMensagem(ctx context.Context, salaID, autorID int64, autorNome, texto string) (Mensagem, error) {
	m := Mensagem{AutorID: autorID, AutorNome: autorNome, Texto: texto}
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO mensagens (sala_id, autor_id, texto) VALUES ($1, $2, $3) RETURNING id, criada_em`,
		salaID, autorID, texto).Scan(&m.ID, &m.CriadaEm)
	return m, err
}

// ---- validação ----

// LimparTexto tira espaços sobrando e confere o tamanho (em caracteres).
func LimparTexto(texto string, maximo int) (string, bool) {
	texto = strings.TrimSpace(texto)
	n := utf8.RuneCountInString(texto)
	return texto, n > 0 && n <= maximo
}

// ValidarSala confere nome e descrição de uma sala nova.
func ValidarSala(nome, descricao string) (string, string, error) {
	nome = strings.Join(strings.Fields(nome), " ")
	if n := utf8.RuneCountInString(nome); n < 3 || n > 60 {
		return "", "", api.NovoErro(http.StatusBadRequest, "O nome da sala precisa ter de 3 a 60 caracteres.")
	}
	descricao = strings.TrimSpace(descricao)
	if utf8.RuneCountInString(descricao) > 200 {
		return "", "", api.NovoErro(http.StatusBadRequest, "A descrição pode ter até 200 caracteres.")
	}
	return nome, descricao, nil
}
