package tempo_real

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/coder/websocket"
)

const (
	tamanhoFilaEnvio = 128              // mensagens esperando para ir ao navegador
	intervaloPing    = 25 * time.Second // mantém a conexão viva atrás de proxies
	prazoEscrita     = 10 * time.Second
	limiteLeitura    = 64 << 10 // 64 KB por mensagem (ofertas WebRTC cabem com folga)
)

// Cliente é uma conexão WebSocket (uma aba do navegador).
type Cliente struct {
	ID        string
	UsuarioID int64
	Nome      string

	envio     chan []byte
	fim       chan struct{}
	fecharUma sync.Once

	// Usados só pela goroutine da sala:
	ultimosChats []time.Time
	naChamada    bool
	midia        Participante
}

func novoCliente(usuarioID int64, nome string) *Cliente {
	id := make([]byte, 8)
	_, _ = rand.Read(id)
	return &Cliente{
		ID:        hex.EncodeToString(id),
		UsuarioID: usuarioID,
		Nome:      nome,
		envio:     make(chan []byte, tamanhoFilaEnvio),
		fim:       make(chan struct{}),
	}
}

// enviar põe a mensagem na fila sem bloquear a sala. Se a fila encheu (conexão lenta ou
// travada), o cliente é derrubado: o navegador reconecta e recebe o estado inteiro de novo.
func (c *Cliente) enviar(s Saida) {
	dados, err := json.Marshal(s)
	if err != nil {
		slog.Error("não foi possível montar a mensagem", "erro", err)
		return
	}
	select {
	case c.envio <- dados:
	default:
		c.fechar()
	}
}

func (c *Cliente) fechar() {
	c.fecharUma.Do(func() { close(c.fim) })
}

// escrever leva a fila de envio para o WebSocket e manda pings periódicos.
func (c *Cliente) escrever(ctx context.Context, conn *websocket.Conn) {
	ping := time.NewTicker(intervaloPing)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.fim:
			conn.Close(websocket.StatusPolicyViolation, "conexão lenta demais")
			return
		case dados := <-c.envio:
			prazo, cancelar := context.WithTimeout(ctx, prazoEscrita)
			err := conn.Write(prazo, websocket.MessageText, dados)
			cancelar()
			if err != nil {
				c.fechar()
				return
			}
		case <-ping.C:
			prazo, cancelar := context.WithTimeout(ctx, prazoEscrita)
			err := conn.Ping(prazo)
			cancelar()
			if err != nil {
				c.fechar()
				return
			}
		}
	}
}

// ler recebe as mensagens do navegador e entrega para a sala até a conexão cair.
func (c *Cliente) ler(ctx context.Context, conn *websocket.Conn, sala *Sala) {
	for {
		tipo, dados, err := conn.Read(ctx)
		if err != nil {
			status := websocket.CloseStatus(err)
			if status == -1 && !errors.Is(err, context.Canceled) {
				slog.Debug("conexão caiu", "cliente", c.ID, "erro", err)
			}
			return
		}
		if tipo != websocket.MessageText {
			continue
		}
		var e Entrada
		if err := json.Unmarshal(dados, &e); err != nil {
			c.enviar(Saida{Tipo: "erro", Erro: "Mensagem inválida."})
			continue
		}
		select {
		case sala.recebido <- recebido{cliente: c, entrada: e}:
		case <-ctx.Done():
			return
		case <-c.fim:
			return
		case <-sala.parada:
			return
		}
	}
}
