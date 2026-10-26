// Package usuarios cuida do cadastro, do login e dos dados da pessoa logada.
package usuarios

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/lib/pq"

	"github.com/Cauan-Silva/estuda-junto/internal/api"
	"github.com/Cauan-Silva/estuda-junto/internal/auth"
)

type Usuario struct {
	ID       int64     `json:"id"`
	Nome     string    `json:"nome"`
	Email    string    `json:"email"`
	CriadoEm time.Time `json:"criadoEm"`
}

type Servico struct {
	db      *sql.DB
	emissor *auth.Emissor
	hash    func(string) (string, error)
}

func NovoServico(db *sql.DB, emissor *auth.Emissor) *Servico {
	return &Servico{db: db, emissor: emissor, hash: auth.HashSenha}
}

type pedidoCadastro struct {
	Nome  string `json:"nome"`
	Email string `json:"email"`
	Senha string `json:"senha"`
}

type pedidoLogin struct {
	Email string `json:"email"`
	Senha string `json:"senha"`
}

type respostaLogin struct {
	Token   string  `json:"token"`
	Usuario Usuario `json:"usuario"`
}

// ValidarCadastro confere nome, e-mail e senha e devolve os valores limpos.
func ValidarCadastro(nome, email, senha string) (string, string, error) {
	nome = strings.Join(strings.Fields(nome), " ")
	email = strings.ToLower(strings.TrimSpace(email))
	switch {
	case utf8.RuneCountInString(nome) < 2 || utf8.RuneCountInString(nome) > 60:
		return "", "", api.NovoErro(http.StatusBadRequest, "O nome precisa ter de 2 a 60 caracteres.")
	case len(email) > 160 || !emailValido(email):
		return "", "", api.NovoErro(http.StatusBadRequest, "Informe um e-mail válido.")
	case utf8.RuneCountInString(senha) < 8:
		return "", "", api.NovoErro(http.StatusBadRequest, "A senha precisa ter pelo menos 8 caracteres.")
	case len(senha) > 200:
		return "", "", api.NovoErro(http.StatusBadRequest, "A senha é longa demais.")
	}
	return nome, email, nil
}

func emailValido(email string) bool {
	endereco, err := mail.ParseAddress(email)
	return err == nil && endereco.Address == email && strings.Contains(email[strings.Index(email, "@"):], ".")
}

// Cadastrar cria a conta e já devolve o token de login.
func (s *Servico) Cadastrar(w http.ResponseWriter, r *http.Request) error {
	var p pedidoCadastro
	if err := api.Ler(r, &p); err != nil {
		return err
	}
	nome, email, err := ValidarCadastro(p.Nome, p.Email, p.Senha)
	if err != nil {
		return err
	}
	hash, err := s.hash(p.Senha)
	if err != nil {
		return err
	}
	var u Usuario
	err = s.db.QueryRowContext(r.Context(),
		`INSERT INTO usuarios (nome, email, senha_hash) VALUES ($1, $2, $3)
		 RETURNING id, nome, email, criado_em`, nome, email, hash).
		Scan(&u.ID, &u.Nome, &u.Email, &u.CriadoEm)
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && pqErr.Code == "23505" {
		return api.NovoErro(http.StatusConflict, "Já existe uma conta com esse e-mail.")
	}
	if err != nil {
		return err
	}
	return s.responderComToken(w, http.StatusCreated, u)
}

// Entrar confere e-mail e senha.
func (s *Servico) Entrar(w http.ResponseWriter, r *http.Request) error {
	var p pedidoLogin
	if err := api.Ler(r, &p); err != nil {
		return err
	}
	var u Usuario
	var hash string
	err := s.db.QueryRowContext(r.Context(),
		`SELECT id, nome, email, criado_em, senha_hash FROM usuarios WHERE lower(email) = lower($1)`,
		strings.TrimSpace(p.Email)).Scan(&u.ID, &u.Nome, &u.Email, &u.CriadoEm, &hash)
	recusa := api.NovoErro(http.StatusUnauthorized, "E-mail ou senha incorretos.")
	if errors.Is(err, sql.ErrNoRows) {
		return recusa
	}
	if err != nil {
		return err
	}
	if ok, err := auth.ConferirSenha(p.Senha, hash); err != nil || !ok {
		return recusa
	}
	return s.responderComToken(w, http.StatusOK, u)
}

func (s *Servico) responderComToken(w http.ResponseWriter, status int, u Usuario) error {
	token, err := s.emissor.Emitir(auth.Identidade{UsuarioID: u.ID, Nome: u.Nome})
	if err != nil {
		return err
	}
	api.Responder(w, status, respostaLogin{Token: token, Usuario: u})
	return nil
}

// Buscar devolve um usuário pelo id.
func (s *Servico) Buscar(ctx context.Context, id int64) (Usuario, error) {
	var u Usuario
	err := s.db.QueryRowContext(ctx, `SELECT id, nome, email, criado_em FROM usuarios WHERE id = $1`, id).
		Scan(&u.ID, &u.Nome, &u.Email, &u.CriadoEm)
	if errors.Is(err, sql.ErrNoRows) {
		return u, api.NovoErro(http.StatusUnauthorized, "Conta não encontrada. Entre de novo.")
	}
	return u, err
}
