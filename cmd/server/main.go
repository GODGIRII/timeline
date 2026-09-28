package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/GODGIRII/timeline/internal/storage"
	"github.com/GODGIRII/timeline/internal/transport"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "listen address")
	dbPath := flag.String("db", "data/timeline.db", "database path")
	origin := flag.String("origin", "https://localhost:8080", "exact browser origin")
	dev := flag.Bool("dev", false, "allow insecure cookies for HTTP localhost only")
	cert := flag.String("tls-cert", "", "TLS certificate; omit only behind an HTTPS reverse proxy or in dev mode")
	key := flag.String("tls-key", "", "TLS private key")
	webDir := flag.String("web-dir", "web/dist", "built frontend directory")
	flag.Parse()
	if (*cert == "") != (*key == "") {
		log.Fatal("tls-cert and tls-key must be supplied together")
	}
	if err := os.MkdirAll(filepath.Dir(*dbPath), 0700); err != nil {
		log.Fatal(err)
	}
	store, err := storage.Open(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()
	app, err := transport.New(store, transport.Config{Origin: *origin, InsecureCookies: *dev, WebDir: *webDir})
	if err != nil {
		log.Fatal(err)
	}
	defer app.Close()
	server := &http.Server{Addr: *addr, Handler: app, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log.Printf("timeline listening on %s; browser origin %s", *addr, *origin)
	serveErr := make(chan error, 1)
	go func() {
		if *cert != "" {
			serveErr <- server.ListenAndServeTLS(*cert, *key)
		} else {
			serveErr <- server.ListenAndServe()
		}
	}()
	select {
	case err = <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Printf("server failed: %v", err)
		}
	case <-ctx.Done():
		app.Close()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			log.Print(err)
			_ = server.Close()
		}
	}
}
