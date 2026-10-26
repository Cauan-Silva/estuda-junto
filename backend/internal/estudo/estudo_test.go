package estudo

import (
	"testing"
	"time"
)

func TestInicioDaSemanaEhSegundaNoFusoDeBrasilia(t *testing.T) {
	// Quarta, 21/10/2026, 01:30 em UTC = terça 22:30 em Brasília
	t1 := time.Date(2026, 10, 21, 1, 30, 0, 0, time.UTC)
	got := InicioDaSemana(t1)
	want := time.Date(2026, 10, 19, 0, 0, 0, 0, Fuso)
	if !got.Equal(want) {
		t.Errorf("esperava %v, veio %v", want, got)
	}
	// Domingo pertence à semana que começou na segunda anterior
	dom := time.Date(2026, 10, 25, 20, 0, 0, 0, Fuso)
	if !InicioDaSemana(dom).Equal(want) {
		t.Errorf("domingo deveria ficar na semana de 19/10, veio %v", InicioDaSemana(dom))
	}
}

func TestInicioDoPeriodo(t *testing.T) {
	agora := time.Date(2026, 10, 21, 15, 0, 0, 0, Fuso)
	if ini, p := InicioDoPeriodo("mes", agora); p != "mes" || ini.Day() != 1 || ini.Month() != 10 {
		t.Errorf("mês errado: %v %s", ini, p)
	}
	if _, p := InicioDoPeriodo("qualquer", agora); p != "semana" {
		t.Errorf("padrão deveria ser semana, veio %s", p)
	}
}

func TestMontarDiasESequencia(t *testing.T) {
	hoje := time.Date(2026, 10, 21, 0, 0, 0, 0, Fuso)
	porDia := map[string]int64{
		"2026-10-21": 1500,
		"2026-10-20": 3000,
		"2026-10-19": 600,
		"2026-10-17": 900, // buraco no dia 18: a sequência para aqui
	}
	dias, seq := MontarDias(porDia, hoje)
	if len(dias) != 7 || dias[6].Data != "2026-10-21" || dias[0].Data != "2026-10-15" {
		t.Fatalf("dias errados: %+v", dias)
	}
	if dias[5].Segundos != 3000 || dias[3].Segundos != 0 {
		t.Errorf("totais errados: %+v", dias)
	}
	if seq != 3 {
		t.Errorf("esperava sequência 3, veio %d", seq)
	}
}

func TestSequenciaContaDesdeOntemSeHojeAindaNaoEstudou(t *testing.T) {
	hoje := time.Date(2026, 10, 21, 0, 0, 0, 0, Fuso)
	_, seq := MontarDias(map[string]int64{"2026-10-20": 60, "2026-10-19": 60}, hoje)
	if seq != 2 {
		t.Errorf("esperava 2, veio %d", seq)
	}
	_, seq = MontarDias(map[string]int64{"2026-10-18": 60}, hoje)
	if seq != 0 {
		t.Errorf("sem estudo ontem nem hoje a sequência é 0, veio %d", seq)
	}
}
