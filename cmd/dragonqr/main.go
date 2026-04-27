package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"dragonqr/internal/game"
	"dragonqr/internal/server"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "dragonqr: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var addr string
	var questPath string
	var dataPath string
	var baseURL string

	flag.StringVar(&addr, "addr", ":8080", "HTTP listen address")
	flag.StringVar(&questPath, "quest", "quest.yaml", "quest YAML file")
	flag.StringVar(&dataPath, "data", "data/players.json", "player JSON data file")
	flag.StringVar(&baseURL, "base-url", "", "public base URL for printed QR codes")
	flag.Parse()

	q, err := game.LoadQuest(questPath)
	if err != nil {
		return err
	}
	store, err := game.NewStore(dataPath)
	if err != nil {
		return err
	}
	app, err := server.New(q, store, server.Config{
		Addr:              addr,
		BaseURL:           baseURL,
		OrganizerPassword: os.Getenv("ORGANIZER_PASSWORD"),
	})
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           app.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	fmt.Printf("Dragon QR listening on %s\n", listenerURL(addr))
	err = srv.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func listenerURL(addr string) string {
	if len(addr) > 0 && addr[0] == ':' {
		return "http://localhost" + addr
	}
	return "http://" + addr
}
