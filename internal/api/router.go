// Package api menyatukan routing HTTP, middleware, dan handler.
package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/andreanpradana/goodiebox-server/internal/config"
	"github.com/andreanpradana/goodiebox-server/internal/midtrans"
	"github.com/andreanpradana/goodiebox-server/internal/orders"
	"github.com/andreanpradana/goodiebox-server/internal/payments"
)

// Deps adalah kumpulan service yang dibutuhkan router.
type Deps struct {
	Cfg      *config.Config
	Midtrans *midtrans.Client
	Orders   *orders.Service
	Payments *payments.Service
}

// NewRouter merakit seluruh endpoint API v1.
func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   d.Cfg.AllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Content-Type", "Idempotency-Key"},
		ExposedHeaders:   []string{"Content-Type"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	ordersH := NewOrdersHandler(d.Orders, d.Cfg.MidtransClientKey, d.Cfg.MidtransProduction)
	paymentsH := NewPaymentsHandler(d.Payments)
	giftsH := NewGiftsHandler(d.Orders)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Route("/v1", func(r chi.Router) {
		r.Post("/orders", ordersH.Create)
		r.Get("/orders/{orderCode}", ordersH.GetStatus)
		r.Post("/payments/notifications", paymentsH.Notification)
		r.Get("/gifts/{publicSlug}", giftsH.Get)
	})

	return r
}
