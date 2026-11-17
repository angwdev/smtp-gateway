package http

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	nethttp "net/http"
	"time"
)

// Serve runs handler on addr until ctx is cancelled, then drains in-flight
// requests for up to five seconds.
func Serve(ctx context.Context, addr string, handler nethttp.Handler, log *slog.Logger) error {
	srv := &nethttp.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    32 << 10,
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	log.Info("panel listening", "addr", addr)

	select {
	case err := <-errc:
		return fmt.Errorf("panel listener: %w", err)
	case <-ctx.Done():
	}
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(sctx); err != nil {
		return fmt.Errorf("panel shutdown: %w", err)
	}
	if err := <-errc; err != nil && !errors.Is(err, nethttp.ErrServerClosed) {
		return fmt.Errorf("panel listener: %w", err)
	}
	return nil
}
