package pomodoro

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 21, 15, 0, 0, 0, time.UTC)

func cfgTeste() Config {
	return Config{Foco: 25 * time.Minute, Pausa: 5 * time.Minute, PausaLonga: 15 * time.Minute, CiclosAteLonga: 2}
}

func TestComecaParadoNoFocoCompleto(t *testing.T) {
	r := Novo(cfgTeste())
	if r.Fase() != Foco || r.Rodando() || r.Restante(t0) != 25*time.Minute {
		t.Fatalf("estado inicial errado: %+v", r.Estado(t0))
	}
	if r.EmFoco() {
		t.Error("parado não conta como foco")
	}
}

func TestPausarECongelarORestante(t *testing.T) {
	r := Novo(cfgTeste())
	r.Iniciar(t0)
	if !r.EmFoco() {
		t.Error("rodando no foco deveria contar como foco")
	}
	r.Pausar(t0.Add(10 * time.Minute))
	if got := r.Restante(t0.Add(time.Hour)); got != 15*time.Minute {
		t.Errorf("parado deveria manter 15 min, ficou %v", got)
	}
	r.Iniciar(t0.Add(2 * time.Hour))
	if got := r.Restante(t0.Add(2*time.Hour + 5*time.Minute)); got != 10*time.Minute {
		t.Errorf("depois de continuar deveria faltar 10 min, falta %v", got)
	}
}

func TestIniciarEPausarRepetidosNaoFazemNada(t *testing.T) {
	r := Novo(cfgTeste())
	if !r.Iniciar(t0) || r.Iniciar(t0.Add(time.Minute)) {
		t.Error("o segundo Iniciar não deveria fazer nada")
	}
	if !r.Pausar(t0.Add(time.Minute)) || r.Pausar(t0.Add(2*time.Minute)) {
		t.Error("o segundo Pausar não deveria fazer nada")
	}
}

func TestCicloCompletoComPausaLonga(t *testing.T) {
	r := Novo(cfgTeste()) // pausa longa a cada 2 focos
	r.Iniciar(t0)

	tr := r.Avancar(t0.Add(25 * time.Minute))
	if len(tr) != 1 || tr[0].Terminou != Foco || tr[0].Comecou != Pausa {
		t.Fatalf("depois do 1º foco esperava pausa: %+v", tr)
	}
	if !r.Rodando() || r.Restante(t0.Add(25*time.Minute)) != 5*time.Minute {
		t.Error("a pausa deveria começar sozinha, com 5 min")
	}

	r.Avancar(t0.Add(30 * time.Minute)) // fim da pausa → foco
	tr = r.Avancar(t0.Add(55 * time.Minute))
	if len(tr) != 1 || tr[0].Comecou != PausaLonga {
		t.Fatalf("depois do 2º foco esperava pausa longa: %+v", tr)
	}
	if r.FocosConcluidos() != 2 {
		t.Errorf("esperava 2 focos, veio %d", r.FocosConcluidos())
	}
}

func TestServidorParadoMuitoTempoAvancaVariasFases(t *testing.T) {
	r := Novo(cfgTeste())
	r.Iniciar(t0)
	// 25 (foco) + 5 (pausa) + 25 (foco) = 55 min; aos 56 está na pausa longa há 1 min
	tr := r.Avancar(t0.Add(56 * time.Minute))
	if len(tr) != 3 {
		t.Fatalf("esperava 3 transições, vieram %d", len(tr))
	}
	if r.Fase() != PausaLonga || r.Restante(t0.Add(56*time.Minute)) != 14*time.Minute {
		t.Errorf("esperava pausa longa com 14 min, está em %s com %v", r.Fase(), r.Restante(t0.Add(56*time.Minute)))
	}
}

func TestPularFocoNaoContaComoConcluido(t *testing.T) {
	r := Novo(cfgTeste())
	tr := r.Pular(t0)
	if !tr.Pulou || tr.Comecou != Pausa || r.Rodando() {
		t.Errorf("pular parado deveria ir para a pausa, parado: %+v", tr)
	}
	if r.FocosConcluidos() != 0 {
		t.Error("foco pulado não conta")
	}
}

func TestReiniciarVoltaAoComecoDaFase(t *testing.T) {
	r := Novo(cfgTeste())
	r.Iniciar(t0)
	r.Reiniciar()
	if r.Rodando() || r.Restante(t0.Add(time.Hour)) != 25*time.Minute {
		t.Error("reiniciar deveria parar com a duração cheia")
	}
}

func TestMudarConfigNoComecoAplicaNaHora(t *testing.T) {
	r := Novo(cfgTeste())
	nova := cfgTeste()
	nova.Foco = 50 * time.Minute
	r.MudarConfig(nova)
	if r.Restante(t0) != 50*time.Minute {
		t.Errorf("parado no começo deveria virar 50 min, está %v", r.Restante(t0))
	}

	r.Iniciar(t0)
	nova.Foco = 10 * time.Minute
	r.MudarConfig(nova)
	if r.Restante(t0.Add(time.Minute)) != 49*time.Minute {
		t.Error("no meio do foco a mudança só vale para a próxima fase")
	}
}

func TestEstadoParaONavegador(t *testing.T) {
	r := Novo(cfgTeste())
	r.Iniciar(t0)
	e := r.Estado(t0.Add(time.Minute))
	if e.FimEm != t0.Add(25*time.Minute).UnixMilli() || e.RestanteMs != (24*time.Minute).Milliseconds() {
		t.Errorf("estado errado: %+v", e)
	}
	if e.Agora != t0.Add(time.Minute).UnixMilli() || e.FocoMin != 25 {
		t.Errorf("estado errado: %+v", e)
	}
}

func TestValidarConfig(t *testing.T) {
	if err := ConfigPadrao.Validar(); err != nil {
		t.Fatal(err)
	}
	ruim := ConfigPadrao
	ruim.Foco = 0
	if ruim.Validar() == nil {
		t.Error("foco zero deveria ser recusado")
	}
	ruim = ConfigPadrao
	ruim.CiclosAteLonga = 20
	if ruim.Validar() == nil {
		t.Error("20 ciclos deveriam ser recusados")
	}
}
