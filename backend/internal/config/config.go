// Package config lê a configuração do servidor das variáveis de ambiente.
package config

import (
	"errors"
	"os"
	"strings"
)

// Config reúne tudo o que o servidor precisa para subir.
type Config struct {
	Porta           string   // PORT (o Render define sozinho)
	BancoURL        string   // DATABASE_URL, ex.: postgres://usuario:senha@host/banco?sslmode=require
	SegredoJWT      []byte   // JWT_SEGREDO, pelo menos 32 caracteres
	OrigensWS       []string // ORIGENS_WS: hosts extras aceitos no WebSocket (ex.: localhost:5173 do Vite)
	Desenvolvimento bool     // AMBIENTE=desenvolvimento libera um segredo padrão

	// Chamada de vídeo: servidor TURN opcional, para redes que bloqueiam a conexão direta.
	TurnURLs    []string // TURN_URLS, separados por vírgula (ex.: turn:servidor:3478,turns:servidor:5349)
	TurnUsuario string   // TURN_USUARIO
	TurnSenha   string   // TURN_SENHA
}

const segredoDeDesenvolvimento = "segredo-de-desenvolvimento-troque-em-producao!!"

// Ler monta a Config e devolve erro se faltar algo obrigatório.
func Ler() (Config, error) {
	c := Config{
		Porta:           valor("PORT", "8080"),
		BancoURL:        valor("DATABASE_URL", "postgres://estuda:estuda@localhost:5432/estuda?sslmode=disable"),
		Desenvolvimento: valor("AMBIENTE", "producao") == "desenvolvimento",
	}

	segredo := os.Getenv("JWT_SEGREDO")
	if segredo == "" && c.Desenvolvimento {
		segredo = segredoDeDesenvolvimento
	}
	if len(segredo) < 32 {
		return c, errors.New("JWT_SEGREDO precisa ter pelo menos 32 caracteres (em desenvolvimento, use AMBIENTE=desenvolvimento)")
	}
	c.SegredoJWT = []byte(segredo)

	for _, o := range strings.Split(valor("ORIGENS_WS", ""), ",") {
		if o = strings.TrimSpace(o); o != "" {
			c.OrigensWS = append(c.OrigensWS, o)
		}
	}
	for _, u := range strings.Split(valor("TURN_URLS", ""), ",") {
		if u = strings.TrimSpace(u); u != "" {
			c.TurnURLs = append(c.TurnURLs, u)
		}
	}
	c.TurnUsuario = valor("TURN_USUARIO", "")
	c.TurnSenha = valor("TURN_SENHA", "")

	if c.Desenvolvimento && len(c.OrigensWS) == 0 {
		c.OrigensWS = []string{"localhost:5173", "127.0.0.1:5173"}
	}
	return c, nil
}

func valor(nome, padrao string) string {
	if v := strings.TrimSpace(os.Getenv(nome)); v != "" {
		return v
	}
	return padrao
}
