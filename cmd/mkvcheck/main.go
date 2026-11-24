package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"mkvcheck/internal/abuse"
	"mkvcheck/internal/server"
	"mkvcheck/internal/store"
	"mkvcheck/web"
)

const (
	reportRetention    = 48 * time.Hour
	purgeInterval      = 10 * time.Minute
	readHeaderTimeout  = 5 * time.Second
	readTimeout        = 15 * time.Second
	writeTimeout       = 30 * time.Second
	idleTimeout        = 60 * time.Second
	shutdownTimeout    = 15 * time.Second
	healthcheckTimeout = 3 * time.Second
	healthcheckSubcmd  = "healthcheck"
	defaultListenAddr  = ":8080"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == healthcheckSubcmd {
		os.Exit(runHealthcheck())
	}
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := server.ConfigFromEnv()
	if err != nil {
		return err
	}
	reports, err := store.Open(cfg.DataDir, cfg.MaxStoreBytes, reportRetention, time.Now)
	if err != nil {
		return err
	}
	staticFiles, err := web.Static()
	if err != nil {
		return err
	}
	guard := abuse.New(abuse.Config{
		HMACKey:       cfg.HMACKey,
		Difficulty:    cfg.Difficulty,
		RatePerMinute: cfg.RatePerMinute,
		Burst:         cfg.Burst,
		ClientIP:      abuse.ClientIPFromHeader(cfg.ClientIPHeader),
	}, time.Now)
	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           server.New(cfg, reports, guard, staticFiles),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}
	return serveUntilSignal(httpServer, reports)
}

func serveUntilSignal(httpServer *http.Server, reports *store.Store) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go reports.RunPurger(ctx, purgeInterval)
	failures := make(chan error, 1)
	go func() { failures <- httpServer.ListenAndServe() }()
	select {
	case err := <-failures:
		return err
	case <-ctx.Done():
	}
	shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := httpServer.Shutdown(shutdownContext); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func runHealthcheck() int {
	addr := os.Getenv("MKVCHECK_ADDR")
	if addr == "" {
		addr = defaultListenAddr
	}
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	client := http.Client{Timeout: healthcheckTimeout}
	response, err := client.Get("http://" + net.JoinHostPort("127.0.0.1", port) + "/healthz")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
