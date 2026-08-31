// Command server runs the public HTTP API.
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

	"github.com/palak-kasoundhan/linkedin-profile-api/internal/api"
	"github.com/palak-kasoundhan/linkedin-profile-api/internal/config"
	"github.com/palak-kasoundhan/linkedin-profile-api/internal/linkedin"
	"github.com/palak-kasoundhan/linkedin-profile-api/internal/scrape"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}

	client := linkedin.New(cfg.Cookie, cfg.CSRFToken(), cfg.UserAgent)
	scraper := scrape.New(client, cfg.CacheTTL)
	handler := api.New(scraper, cfg.APIKey)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      profileTimeout + 15*time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		slog.Info("listening", "addr", srv.Addr, "cache_ttl", cfg.CacheTTL.String(), "auth", cfg.APIKey != "")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("listen", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	slog.Info("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("shutdown", "err", err)
	}
}

// profileTimeout mirrors api.profileRequestTimeout for sizing the write timeout.
const profileTimeout = 120 * time.Second
