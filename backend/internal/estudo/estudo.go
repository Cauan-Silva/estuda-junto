// Package estudo guarda o tempo de foco de cada pessoa e monta as estatísticas e os rankings.
package estudo

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/Cauan-Silva/estuda-junto/internal/api"
)

// Fuso usado para "hoje" e "esta semana" (o tzdata vai embutido no binário, veja main.go).
var Fuso = carregarFuso()

func carregarFuso() *time.Location {
	if l, err := time.LoadLocation("America/Sao_Paulo"); err == nil {
		return l
	}
	return time.FixedZone("BRT", -3*60*60)
}

type Servico struct {
	db    *sql.DB
	agora func() time.Time
}

func NovoServico(db *sql.DB) *Servico {
	return &Servico{db: db, agora: time.Now}
}

// Registrar guarda um trecho de foco. Trechos de menos de 1 minuto são ignorados
// (entrar e sair da sala não deve pontuar).
func (s *Servico) Registrar(ctx context.Context, usuarioID, salaID int64, inicio time.Time, duracao time.Duration) error {
	segundos := int(duracao / time.Second)
	if segundos < 60 {
		return nil
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessoes_estudo (usuario_id, sala_id, inicio, segundos) VALUES ($1, $2, $3, $4)`,
		usuarioID, salaID, inicio, segundos)
	return err
}

// InicioDaSemana é a segunda-feira 00:00 (no fuso de Brasília) da semana de t.
func InicioDaSemana(t time.Time) time.Time {
	t = t.In(Fuso)
	dias := (int(t.Weekday()) + 6) % 7 // segunda = 0
	y, m, d := t.Date()
	return time.Date(y, m, d-dias, 0, 0, 0, 0, Fuso)
}

// InicioDoDia é 00:00 (no fuso de Brasília) do dia de t.
func InicioDoDia(t time.Time) time.Time {
	y, m, d := t.In(Fuso).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, Fuso)
}

// InicioDoPeriodo traduz "semana", "mes" e "sempre".
func InicioDoPeriodo(periodo string, agora time.Time) (time.Time, string) {
	switch periodo {
	case "mes":
		y, m, _ := agora.In(Fuso).Date()
		return time.Date(y, m, 1, 0, 0, 0, 0, Fuso), "mes"
	case "sempre":
		return time.Date(2000, 1, 1, 0, 0, 0, 0, Fuso), "sempre"
	default:
		return InicioDaSemana(agora), "semana"
	}
}

// Dia é o total de um dia no gráfico dos últimos 7 dias.
type Dia struct {
	Data     string `json:"data"` // aaaa-mm-dd
	Segundos int64  `json:"segundos"`
}

type Estatisticas struct {
	HojeSegundos   int64 `json:"hojeSegundos"`
	SemanaSegundos int64 `json:"semanaSegundos"`
	TotalSegundos  int64 `json:"totalSegundos"`
	Sequencia      int   `json:"sequencia"` // dias seguidos com estudo (contando hoje ou ontem)
	UltimosDias    []Dia `json:"ultimosDias"`
}

// DoUsuario soma o tempo de hoje, da semana e de sempre, monta os últimos 7 dias e a sequência.
func (s *Servico) DoUsuario(ctx context.Context, usuarioID int64) (Estatisticas, error) {
	agora := s.agora()
	hoje := InicioDoDia(agora)
	e := Estatisticas{}

	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(segundos) FILTER (WHERE inicio >= $2), 0),
		       COALESCE(SUM(segundos) FILTER (WHERE inicio >= $3), 0),
		       COALESCE(SUM(segundos), 0)
		FROM sessoes_estudo WHERE usuario_id = $1`,
		usuarioID, hoje, InicioDaSemana(agora)).Scan(&e.HojeSegundos, &e.SemanaSegundos, &e.TotalSegundos)
	if err != nil {
		return e, err
	}

	// Totais por dia dos últimos 60 dias (para os 7 do gráfico e para a sequência).
	linhas, err := s.db.QueryContext(ctx, `
		SELECT to_char(inicio AT TIME ZONE 'America/Sao_Paulo', 'YYYY-MM-DD') AS dia, SUM(segundos)
		FROM sessoes_estudo
		WHERE usuario_id = $1 AND inicio >= $2
		GROUP BY dia`, usuarioID, hoje.AddDate(0, 0, -60))
	if err != nil {
		return e, err
	}
	defer linhas.Close()
	porDia := map[string]int64{}
	for linhas.Next() {
		var dia string
		var seg int64
		if err := linhas.Scan(&dia, &seg); err != nil {
			return e, err
		}
		porDia[dia] = seg
	}
	if err := linhas.Err(); err != nil {
		return e, err
	}

	e.UltimosDias, e.Sequencia = MontarDias(porDia, hoje)
	return e, nil
}

// MontarDias devolve os últimos 7 dias (do mais antigo para hoje) e a sequência de dias
// seguidos com estudo. Se hoje ainda não teve estudo, a sequência conta a partir de ontem.
func MontarDias(porDia map[string]int64, hoje time.Time) ([]Dia, int) {
	dias := make([]Dia, 0, 7)
	for i := 6; i >= 0; i-- {
		d := hoje.AddDate(0, 0, -i).Format("2006-01-02")
		dias = append(dias, Dia{Data: d, Segundos: porDia[d]})
	}
	sequencia := 0
	dia := hoje
	if porDia[dia.Format("2006-01-02")] == 0 {
		dia = dia.AddDate(0, 0, -1)
	}
	for porDia[dia.Format("2006-01-02")] > 0 {
		sequencia++
		dia = dia.AddDate(0, 0, -1)
	}
	return dias, sequencia
}

// Posicao é uma linha do ranking.
type Posicao struct {
	UsuarioID int64  `json:"usuarioId"`
	Nome      string `json:"nome"`
	Segundos  int64  `json:"segundos"`
}

// Ranking devolve as 20 pessoas que mais focaram no período (de todas as salas ou de uma).
func (s *Servico) Ranking(ctx context.Context, periodo string, salaID int64) ([]Posicao, string, error) {
	inicio, periodo := InicioDoPeriodo(periodo, s.agora())
	linhas, err := s.db.QueryContext(ctx, `
		SELECT u.id, u.nome, SUM(e.segundos) AS total
		FROM sessoes_estudo e JOIN usuarios u ON u.id = e.usuario_id
		WHERE e.inicio >= $1 AND ($2 = 0 OR e.sala_id = $2)
		GROUP BY u.id, u.nome
		ORDER BY total DESC, u.nome
		LIMIT 20`, inicio, salaID)
	if err != nil {
		return nil, periodo, err
	}
	defer linhas.Close()
	lista := []Posicao{}
	for linhas.Next() {
		var p Posicao
		if err := linhas.Scan(&p.UsuarioID, &p.Nome, &p.Segundos); err != nil {
			return nil, periodo, err
		}
		lista = append(lista, p)
	}
	return lista, periodo, linhas.Err()
}

// RotaRanking é GET /api/ranking?periodo=semana|mes|sempre.
func (s *Servico) RotaRanking(w http.ResponseWriter, r *http.Request) error {
	lista, periodo, err := s.Ranking(r.Context(), r.URL.Query().Get("periodo"), 0)
	if err != nil {
		return err
	}
	api.Responder(w, http.StatusOK, map[string]any{"periodo": periodo, "posicoes": lista})
	return nil
}
