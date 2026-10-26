// Package pomodoro é o relógio da sala: foco, pausa curta e pausa longa.
//
// O servidor é a fonte da verdade. Em vez de mandar o tempo restante a cada segundo, ele
// manda "quando a fase termina" (FimEm); cada navegador calcula o restante com o próprio
// relógio, corrigido pela diferença para o relógio do servidor. Assim todos veem o mesmo
// número, mesmo com atraso de rede, e o servidor só fala quando algo muda.
package pomodoro

import (
	"errors"
	"time"
)

type Fase string

const (
	Foco       Fase = "foco"
	Pausa      Fase = "pausa"
	PausaLonga Fase = "pausa_longa"
)

// Config são as durações da sala.
type Config struct {
	Foco           time.Duration
	Pausa          time.Duration
	PausaLonga     time.Duration
	CiclosAteLonga int // focos até a pausa longa
}

var ConfigPadrao = Config{Foco: 25 * time.Minute, Pausa: 5 * time.Minute, PausaLonga: 15 * time.Minute, CiclosAteLonga: 4}

// Validar recusa durações fora do razoável.
func (c Config) Validar() error {
	switch {
	case c.Foco < time.Minute || c.Foco > 180*time.Minute:
		return errors.New("o foco precisa ter de 1 a 180 minutos")
	case c.Pausa < time.Minute || c.Pausa > 60*time.Minute:
		return errors.New("a pausa precisa ter de 1 a 60 minutos")
	case c.PausaLonga < time.Minute || c.PausaLonga > 90*time.Minute:
		return errors.New("a pausa longa precisa ter de 1 a 90 minutos")
	case c.CiclosAteLonga < 1 || c.CiclosAteLonga > 12:
		return errors.New("os ciclos até a pausa longa precisam ser de 1 a 12")
	}
	return nil
}

// Relogio guarda o estado do pomodoro. Não é seguro para uso concorrente: quem usa é só
// a goroutine da sala.
type Relogio struct {
	cfg      Config
	fase     Fase
	rodando  bool
	fimEm    time.Time     // quando rodando
	restante time.Duration // quando parado
	ciclos   int           // focos concluídos desde a última pausa longa
	focos    int           // focos concluídos no total (para mostrar)
}

func Novo(cfg Config) *Relogio {
	return &Relogio{cfg: cfg, fase: Foco, restante: cfg.Foco}
}

// Transicao conta o que aconteceu quando uma fase acabou.
type Transicao struct {
	Terminou Fase
	Comecou  Fase
	Pulou    bool // true quando alguém apertou "pular"
}

func (r *Relogio) duracao(f Fase) time.Duration {
	switch f {
	case Pausa:
		return r.cfg.Pausa
	case PausaLonga:
		return r.cfg.PausaLonga
	default:
		return r.cfg.Foco
	}
}

func (r *Relogio) Fase() Fase           { return r.fase }
func (r *Relogio) Rodando() bool        { return r.rodando }
func (r *Relogio) Config() Config       { return r.cfg }
func (r *Relogio) FocosConcluidos() int { return r.focos }

// EmFoco diz se agora conta como tempo de estudo.
func (r *Relogio) EmFoco() bool { return r.rodando && r.fase == Foco }

// Restante é quanto falta para a fase acabar.
func (r *Relogio) Restante(agora time.Time) time.Duration {
	if !r.rodando {
		return r.restante
	}
	if falta := r.fimEm.Sub(agora); falta > 0 {
		return falta
	}
	return 0
}

// Iniciar começa (ou continua) a contagem. Não faz nada se já estiver rodando.
func (r *Relogio) Iniciar(agora time.Time) bool {
	if r.rodando {
		return false
	}
	r.rodando = true
	r.fimEm = agora.Add(r.restante)
	return true
}

// Pausar congela o tempo restante.
func (r *Relogio) Pausar(agora time.Time) bool {
	if !r.rodando {
		return false
	}
	r.restante = r.Restante(agora)
	r.rodando = false
	return true
}

// Reiniciar volta a fase atual ao começo, parada.
func (r *Relogio) Reiniciar() {
	r.rodando = false
	r.restante = r.duracao(r.fase)
}

// Pular encerra a fase atual na hora e começa a próxima (mantendo rodando ou parado).
// Pular um foco não conta como foco concluído.
func (r *Relogio) Pular(agora time.Time) Transicao {
	return r.avancar(agora, true)
}

// Avancar confere se a fase acabou; se sim, passa para a próxima e já começa a contar.
// Devolve as transições que ocorreram (mais de uma se o servidor ficou muito tempo parado).
func (r *Relogio) Avancar(agora time.Time) []Transicao {
	var feitas []Transicao
	for r.rodando && !agora.Before(r.fimEm) && len(feitas) < 10 {
		fim := r.fimEm
		t := r.avancar(fim, false)
		feitas = append(feitas, t)
	}
	return feitas
}

func (r *Relogio) avancar(momento time.Time, pulou bool) Transicao {
	t := Transicao{Terminou: r.fase, Pulou: pulou}
	if r.fase == Foco {
		if !pulou {
			r.focos++
		}
		r.ciclos++
		if r.ciclos >= r.cfg.CiclosAteLonga {
			r.fase = PausaLonga
			r.ciclos = 0
		} else {
			r.fase = Pausa
		}
	} else {
		r.fase = Foco
	}
	t.Comecou = r.fase
	r.restante = r.duracao(r.fase)
	if r.rodando {
		r.fimEm = momento.Add(r.restante)
	}
	return t
}

// MudarConfig troca as durações. Se o relógio estiver parado no começo de uma fase, a
// fase passa a ter a duração nova; se estiver no meio, a nova duração vale a partir da próxima.
func (r *Relogio) MudarConfig(cfg Config) {
	noComeco := !r.rodando && r.restante == r.duracao(r.fase)
	r.cfg = cfg
	if noComeco {
		r.restante = r.duracao(r.fase)
	}
}

// Estado é o que vai para os navegadores.
type Estado struct {
	Fase           Fase  `json:"fase"`
	Rodando        bool  `json:"rodando"`
	FimEm          int64 `json:"fimEm,omitempty"` // ms desde 1970, quando rodando
	RestanteMs     int64 `json:"restanteMs"`      // quando parado (e, rodando, o valor no momento do envio)
	DuracaoMs      int64 `json:"duracaoMs"`       // duração total da fase atual
	Ciclo          int   `json:"ciclo"`           // focos feitos desde a última pausa longa
	CiclosAteLonga int   `json:"ciclosAteLonga"`
	Focos          int   `json:"focos"`
	FocoMin        int   `json:"focoMin"`
	PausaMin       int   `json:"pausaMin"`
	PausaLongaMin  int   `json:"pausaLongaMin"`
	Agora          int64 `json:"agora"` // relógio do servidor, para o navegador corrigir o dele
}

func (r *Relogio) Estado(agora time.Time) Estado {
	e := Estado{
		Fase:           r.fase,
		Rodando:        r.rodando,
		RestanteMs:     r.Restante(agora).Milliseconds(),
		DuracaoMs:      r.duracao(r.fase).Milliseconds(),
		Ciclo:          r.ciclos,
		CiclosAteLonga: r.cfg.CiclosAteLonga,
		Focos:          r.focos,
		FocoMin:        int(r.cfg.Foco / time.Minute),
		PausaMin:       int(r.cfg.Pausa / time.Minute),
		PausaLongaMin:  int(r.cfg.PausaLonga / time.Minute),
		Agora:          agora.UnixMilli(),
	}
	if r.rodando {
		e.FimEm = r.fimEm.UnixMilli()
	}
	return e
}
