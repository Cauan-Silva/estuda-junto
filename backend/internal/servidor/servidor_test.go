package servidor

// Testes de ponta a ponta com um PostgreSQL de verdade. Rodam quando a variável
// TEST_DATABASE_URL existe (no GitHub Actions ela aponta para um Postgres temporário):
//
//	TEST_DATABASE_URL=postgres://postgres@localhost:5432/estuda_teste?sslmode=disable go test ./...

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/Cauan-Silva/estuda-junto/internal/banco"
	"github.com/Cauan-Silva/estuda-junto/internal/config"
)

func abrirBancoDeTeste(t *testing.T) *sql.DB {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("defina TEST_DATABASE_URL para rodar os testes com o banco")
	}
	db, err := banco.Abrir(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`TRUNCATE usuarios, salas, tarefas, mensagens, sessoes_estudo RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

type cliente struct {
	t     *testing.T
	base  string
	token string
}

func (c *cliente) pedir(metodo, caminho string, corpo any, destino any) int {
	c.t.Helper()
	var leitor *bytes.Reader
	if corpo != nil {
		b, _ := json.Marshal(corpo)
		leitor = bytes.NewReader(b)
	} else {
		leitor = bytes.NewReader(nil)
	}
	req, _ := http.NewRequest(metodo, c.base+caminho, leitor)
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	if destino != nil {
		_ = json.NewDecoder(resp.Body).Decode(destino)
	}
	return resp.StatusCode
}

func subir(t *testing.T) (*httptest.Server, *sql.DB) {
	db := abrirBancoDeTeste(t)
	srv := Novo(config.Config{SegredoJWT: []byte("um-segredo-de-teste-com-32-caracteres!")}, db)
	ts := httptest.NewServer(srv.Rotas())
	t.Cleanup(func() {
		ts.Close()
		srv.Gerente.EncerrarTodas()
		srv.Gerente.AguardarGravacoes()
	})
	return ts, db
}

type respostaLogin struct {
	Token   string `json:"token"`
	Usuario struct {
		ID   int64  `json:"id"`
		Nome string `json:"nome"`
	} `json:"usuario"`
	Erro string `json:"erro"`
}

func cadastrar(t *testing.T, base, nome, email string) *cliente {
	t.Helper()
	c := &cliente{t: t, base: base}
	var r respostaLogin
	if st := c.pedir("POST", "/api/auth/cadastro", map[string]string{"nome": nome, "email": email, "senha": "senha-forte-123"}, &r); st != http.StatusCreated {
		t.Fatalf("cadastro: status %d (%s)", st, r.Erro)
	}
	c.token = r.Token
	return c
}

func TestCadastroLoginEEu(t *testing.T) {
	ts, _ := subir(t)
	cadastrar(t, ts.URL, "Cauan Silva", "Cauan@Exemplo.com")

	anonimo := &cliente{t: t, base: ts.URL}
	var r respostaLogin
	if st := anonimo.pedir("POST", "/api/auth/cadastro", map[string]string{"nome": "Outro", "email": "cauan@exemplo.com", "senha": "outra-senha-123"}, &r); st != http.StatusConflict {
		t.Errorf("e-mail repetido (maiúsculas diferentes) deveria dar 409, deu %d", st)
	}
	if st := anonimo.pedir("POST", "/api/auth/login", map[string]string{"email": "cauan@exemplo.com", "senha": "errada-123"}, &r); st != http.StatusUnauthorized {
		t.Errorf("senha errada deveria dar 401, deu %d", st)
	}
	if st := anonimo.pedir("POST", "/api/auth/login", map[string]string{"email": "CAUAN@exemplo.com", "senha": "senha-forte-123"}, &r); st != http.StatusOK || r.Token == "" {
		t.Fatalf("login deveria funcionar: %d", st)
	}

	logado := &cliente{t: t, base: ts.URL, token: r.Token}
	var eu struct {
		Usuario struct {
			Nome  string `json:"nome"`
			Email string `json:"email"`
		} `json:"usuario"`
		Estatisticas struct {
			UltimosDias []any `json:"ultimosDias"`
		} `json:"estatisticas"`
	}
	if st := logado.pedir("GET", "/api/eu", nil, &eu); st != http.StatusOK {
		t.Fatalf("/api/eu: %d", st)
	}
	if eu.Usuario.Nome != "Cauan Silva" || eu.Usuario.Email != "cauan@exemplo.com" || len(eu.Estatisticas.UltimosDias) != 7 {
		t.Errorf("dados errados: %+v", eu)
	}
	if st := anonimo.pedir("GET", "/api/eu", nil, nil); st != http.StatusUnauthorized {
		t.Errorf("sem token deveria dar 401, deu %d", st)
	}
}

func TestCadastroValida(t *testing.T) {
	ts, _ := subir(t)
	c := &cliente{t: t, base: ts.URL}
	casos := []map[string]string{
		{"nome": "A", "email": "a@b.com", "senha": "12345678"},
		{"nome": "Ana", "email": "sem-arroba", "senha": "12345678"},
		{"nome": "Ana", "email": "ana@exemplo.com", "senha": "curta"},
	}
	for _, caso := range casos {
		var r respostaLogin
		if st := c.pedir("POST", "/api/auth/cadastro", caso, &r); st != http.StatusBadRequest || r.Erro == "" {
			t.Errorf("%v deveria dar 400 com mensagem, deu %d", caso, st)
		}
	}
}

type salaResp struct {
	Codigo  string `json:"codigo"`
	Nome    string `json:"nome"`
	Publica bool   `json:"publica"`
	Online  int    `json:"online"`
	Erro    string `json:"erro"`
}

func TestSalasRankingEWebSocket(t *testing.T) {
	ts, db := subir(t)
	ana := cadastrar(t, ts.URL, "Ana", "ana@exemplo.com")
	bia := cadastrar(t, ts.URL, "Bia", "bia@exemplo.com")

	var publica, privada salaResp
	if st := ana.pedir("POST", "/api/salas", map[string]any{"nome": "  ENEM   2026 ", "descricao": "Revisão", "publica": true}, &publica); st != http.StatusCreated {
		t.Fatalf("criar sala: %d %s", st, publica.Erro)
	}
	if publica.Nome != "ENEM 2026" || len(publica.Codigo) != 6 {
		t.Errorf("sala criada errada: %+v", publica)
	}
	ana.pedir("POST", "/api/salas", map[string]any{"nome": "Só minha", "publica": false}, &privada)

	var lista struct {
		Publicas []salaResp `json:"publicas"`
		Minhas   []salaResp `json:"minhas"`
	}
	bia.pedir("GET", "/api/salas", nil, &lista)
	if len(lista.Publicas) != 1 || lista.Publicas[0].Codigo != publica.Codigo || len(lista.Minhas) != 0 {
		t.Errorf("a Bia deveria ver só a pública: %+v", lista)
	}
	ana.pedir("GET", "/api/salas", nil, &lista)
	if len(lista.Minhas) != 2 {
		t.Errorf("a Ana deveria ver as 2 dela: %+v", lista.Minhas)
	}

	// A sala privada abre pelo código (é assim que se convida alguém).
	var achada salaResp
	if st := bia.pedir("GET", "/api/salas/"+strings.ToLower(privada.Codigo), nil, &achada); st != http.StatusOK || achada.Nome != "Só minha" {
		t.Errorf("buscar pelo código: %d %+v", st, achada)
	}
	if st := bia.pedir("GET", "/api/salas/XXXXXX", nil, nil); st != http.StatusNotFound {
		t.Errorf("código inexistente deveria dar 404, deu %d", st)
	}

	// WebSocket de verdade: a Bia entra, manda uma mensagem e uma tarefa (vão para o banco).
	ctx, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelar()
	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws/salas/" + publica.Codigo + "?token=" + bia.token
	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	ler := func(tipo string) map[string]any {
		for {
			_, dados, err := conn.Read(ctx)
			if err != nil {
				t.Fatalf("esperando %s: %v", tipo, err)
			}
			var m map[string]any
			json.Unmarshal(dados, &m)
			if m["tipo"] == tipo {
				return m
			}
		}
	}
	ler("estado")
	conn.Write(ctx, websocket.MessageText, []byte(`{"tipo":"chat","texto":"oi, gente"}`))
	ler("chat")
	conn.Write(ctx, websocket.MessageText, []byte(`{"tipo":"tarefa_nova","texto":"Redação"}`))
	ler("tarefas")

	var mensagens, tarefas int
	db.QueryRow(`SELECT count(*) FROM mensagens`).Scan(&mensagens)
	db.QueryRow(`SELECT count(*) FROM tarefas`).Scan(&tarefas)
	if mensagens != 1 || tarefas != 1 {
		t.Errorf("esperava 1 mensagem e 1 tarefa no banco, há %d e %d", mensagens, tarefas)
	}

	bia.pedir("GET", "/api/salas", nil, &lista)
	if lista.Publicas[0].Online != 1 {
		t.Errorf("a lista deveria mostrar 1 pessoa online, mostra %d", lista.Publicas[0].Online)
	}

	// Ranking: tempo de foco gravado direto no banco.
	var salaID int64
	db.QueryRow(`SELECT id FROM salas WHERE codigo = $1`, publica.Codigo).Scan(&salaID)
	agora := time.Now()
	db.Exec(`INSERT INTO sessoes_estudo (usuario_id, sala_id, inicio, segundos) VALUES
		(1, $1, $2, 3000), (2, $1, $2, 1500), (2, NULL, $2, 600)`, salaID, agora.Add(-time.Hour))
	var ranking struct {
		Periodo  string `json:"periodo"`
		Posicoes []struct {
			Nome     string `json:"nome"`
			Segundos int64  `json:"segundos"`
		} `json:"posicoes"`
	}
	bia.pedir("GET", "/api/ranking", nil, &ranking)
	if ranking.Periodo != "semana" || len(ranking.Posicoes) != 2 || ranking.Posicoes[0].Nome != "Ana" || ranking.Posicoes[1].Segundos != 2100 {
		t.Errorf("ranking geral errado: %+v", ranking)
	}
	bia.pedir("GET", "/api/salas/"+publica.Codigo+"/ranking", nil, &ranking)
	if len(ranking.Posicoes) != 2 || ranking.Posicoes[1].Segundos != 1500 {
		t.Errorf("ranking da sala deveria contar só o tempo dela: %+v", ranking)
	}

	var eu struct {
		Estatisticas struct {
			HojeSegundos   int64 `json:"hojeSegundos"`
			SemanaSegundos int64 `json:"semanaSegundos"`
			Sequencia      int   `json:"sequencia"`
		} `json:"estatisticas"`
	}
	bia.pedir("GET", "/api/eu", nil, &eu)
	if eu.Estatisticas.SemanaSegundos != 2100 || eu.Estatisticas.Sequencia < 1 {
		t.Errorf("estatísticas erradas: %+v", eu.Estatisticas)
	}
}

func TestRotasDiversas(t *testing.T) {
	ts, _ := subir(t)
	c := &cliente{t: t, base: ts.URL}
	var saude map[string]string
	if st := c.pedir("GET", "/api/saude", nil, &saude); st != http.StatusOK || saude["status"] != "ok" {
		t.Errorf("saúde: %d %v", st, saude)
	}
	if st := c.pedir("GET", "/api/nao-existe", nil, nil); st != http.StatusNotFound {
		t.Errorf("rota desconhecida: %d", st)
	}
	resp, err := http.Get(ts.URL + "/sala/ABC234")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("X-Frame-Options") != "DENY" {
		t.Errorf("telas: %d %v", resp.StatusCode, resp.Header)
	}

	u := cadastrar(t, ts.URL, "Ana", "ana2@exemplo.com")
	var rtc struct {
		IceServers    []map[string]any `json:"iceServers"`
		LimiteChamada int              `json:"limiteChamada"`
	}
	u.pedir("GET", "/api/rtc", nil, &rtc)
	if len(rtc.IceServers) != 1 || rtc.LimiteChamada != 6 {
		t.Errorf("rtc sem TURN deveria ter só o STUN: %+v", rtc)
	}
}

func TestLimitePorIP(t *testing.T) {
	agora := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := &limitador{baldes: map[string]*balde{}, capacidade: 3, recarga: time.Minute, agora: func() time.Time { return agora }}
	for i := 0; i < 3; i++ {
		if !l.permitir("1.2.3.4") {
			t.Fatalf("tentativa %d deveria passar", i+1)
		}
	}
	if l.permitir("1.2.3.4") {
		t.Error("a 4ª tentativa seguida deveria ser barrada")
	}
	if !l.permitir("5.6.7.8") {
		t.Error("outro IP não é afetado")
	}
	agora = agora.Add(time.Minute)
	if !l.permitir("1.2.3.4") {
		t.Error("depois de 1 minuto ganha uma tentativa de volta")
	}
}
