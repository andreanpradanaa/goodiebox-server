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

	migrations "github.com/andreanpradana/goodiebox-server"
	"github.com/andreanpradana/goodiebox-server/internal/api"
	"github.com/andreanpradana/goodiebox-server/internal/config"
	"github.com/andreanpradana/goodiebox-server/internal/db"
	"github.com/andreanpradana/goodiebox-server/internal/midtrans"
	"github.com/andreanpradana/goodiebox-server/internal/orders"
	"github.com/andreanpradana/goodiebox-server/internal/payments"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("konfigurasi tidak valid", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("database gagal", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := db.Migrate(cfg.DatabaseURL, migrations.FS); err != nil {
		slog.Error("migrasi gagal", "err", err)
		os.Exit(1)
	}
	slog.Info("migrasi database selesai")

	mtClient := midtrans.New(cfg.MidtransServerKey, cfg.MidtransClientKey, cfg.MidtransProduction)
	env := "sandbox"
	if cfg.MidtransProduction {
		env = "production"
	}
	slog.Info("midtrans aktif", "env", env)

	orderRepo := orders.NewRepo(pool)
	payRepo := payments.NewRepo(pool)
	orderSvc := orders.NewService(orderRepo, payRepo, mtClient, cfg.BoxPriceIDR)
	paySvc := payments.NewService(payRepo, mtClient)

	router := api.NewRouter(api.Deps{
		Cfg:      cfg,
		Midtrans: mtClient,
		Orders:   orderSvc,
		Payments: paySvc,
	})

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		slog.Info("server berjalan", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server berhenti dengan error", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	slog.Info("sinyal shutdown diterima, menutup server...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown tidak bersih", "err", err)
	}
	slog.Info("server berhenti")
}
