// Servidor do Estuda Junto: API REST, WebSocket das salas e as telas, num binário só.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "time/tzdata" // fuso de Brasília embutido (a imagem Docker mínima não tem o tzdata)

	"github.com/Cauan-Silva/estuda-junto/internal/banco"
	"github.com/Cauan-Silva/estuda-junto/internal/config"
	"github.com/Cauan-Silva/estuda-junto/internal/servidor"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	if err := rodar(); err != nil {
		slog.Error("o servidor parou", "erro", err)
		os.Exit(1)
	}
}

func rodar() error {
	cfg, err := config.Ler()
	if err != nil {
		return err
	}

	ctx, parar := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer parar()

	db, err := banco.Abrir(ctx, cfg.BancoURL)
	if err != nil {
		return err
	}
	defer db.Close()

	srv := servidor.Novo(cfg, db)
	servidorHTTP := &http.Server{
		Addr:              ":" + cfg.Porta,
		Handler:           srv.Rotas(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	erros := make(chan error, 1)
	go func() {
		slog.Info("servidor no ar", "porta", cfg.Porta)
		erros <- servidorHTTP.ListenAndServe()
	}()

	select {
	case err := <-erros:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		slog.Info("desligando...")
	}

	// Dá até 10 s para os pedidos em andamento terminarem e os tempos de foco irem ao banco.
	desligar, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelar()
	_ = servidorHTTP.Shutdown(desligar)
	srv.Gerente.EncerrarTodas() // fecha as salas guardando o foco de quem ainda estava estudando
	srv.Gerente.AguardarGravacoes()
	return nil
}
