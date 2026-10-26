package servidor

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Cauan-Silva/estuda-junto/internal/api"
)

// limitarPorIP segura tentativas de login e cadastro em massa: no máximo 10 por minuto
// por endereço IP (um balde que enche de novo aos poucos).
func limitarPorIP(proximo http.Handler) http.Handler {
	l := &limitador{baldes: map[string]*balde{}, capacidade: 10, recarga: 6 * time.Second, agora: time.Now}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.permitir(ipDe(r)) {
			api.Responder(w, http.StatusTooManyRequests, map[string]string{"erro": "Muitas tentativas. Espere um minuto e tente de novo."})
			return
		}
		proximo.ServeHTTP(w, r)
	})
}

type balde struct {
	fichas float64
	visto  time.Time
}

type limitador struct {
	mu         sync.Mutex
	baldes     map[string]*balde
	capacidade float64
	recarga    time.Duration // tempo para ganhar uma ficha
	agora      func() time.Time
}

func (l *limitador) permitir(chave string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	agora := l.agora()
	b := l.baldes[chave]
	if b == nil {
		if len(l.baldes) > 10_000 { // não deixa o mapa crescer sem fim
			l.baldes = map[string]*balde{}
		}
		b = &balde{fichas: l.capacidade, visto: agora}
		l.baldes[chave] = b
	}
	b.fichas = min(l.capacidade, b.fichas+agora.Sub(b.visto).Seconds()/l.recarga.Seconds())
	b.visto = agora
	if b.fichas < 1 {
		return false
	}
	b.fichas--
	return true
}

// ipDe usa o primeiro endereço do X-Forwarded-For (o Render fica na frente do servidor).
func ipDe(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
