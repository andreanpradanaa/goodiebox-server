package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/andreanpradana/goodiebox-server/internal/midtrans"
	"github.com/andreanpradana/goodiebox-server/internal/orders"
	"github.com/andreanpradana/goodiebox-server/internal/payments"
)

// OrdersHandler menangani endpoint order.
type OrdersHandler struct {
	svc          *orders.Service
	clientKey    string
	isProduction bool
}

func NewOrdersHandler(svc *orders.Service, clientKey string, isProduction bool) *OrdersHandler {
	return &OrdersHandler{svc: svc, clientKey: clientKey, isProduction: isProduction}
}

type createOrderResponse struct {
	OrderCode       string `json:"order_code"`
	PublicSlug      string `json:"public_slug"`
	Status          string `json:"status"`
	Amount          int64  `json:"amount"`
	Currency        string `json:"currency"`
	SnapToken       string `json:"snap_token"`
	SnapRedirectURL string `json:"snap_redirect_url"`
	ClientKey       string `json:"client_key"`
	IsProduction    bool   `json:"is_production"`
	CreatedAt       string `json:"created_at"`
}

// Create: POST /v1/orders
func (h *OrdersHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req orders.CreateRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}

	// Idempotency-Key opsional (materi payment design): retry client
	// dengan key sama tidak membuat order ganda.
	result, err := h.svc.Create(r.Context(), req, r.Header.Get("Idempotency-Key"))
	if err != nil {
		var verr *orders.ValidationError
		switch {
		case errors.As(err, &verr):
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", verr.Error())
		case errors.Is(err, orders.ErrMidtransUnavailable):
			writeError(w, http.StatusBadGateway, "MIDTRANS_UNAVAILABLE",
				"payment provider sedang tidak tersedia, coba lagi")
		default:
			writeError(w, http.StatusInternalServerError, "INTERNAL", "gagal membuat order")
		}
		return
	}

	writeJSON(w, http.StatusCreated, createOrderResponse{
		OrderCode:       result.Order.OrderCode,
		PublicSlug:      result.Order.PublicSlug,
		Status:          result.Order.Status,
		Amount:          result.Order.Amount,
		Currency:        result.Order.Currency,
		SnapToken:       result.SnapToken,
		SnapRedirectURL: result.SnapRedirectURL,
		ClientKey:       h.clientKey,
		IsProduction:    h.isProduction,
		CreatedAt:       result.Order.CreatedAt.Format(time.RFC3339),
	})
}

type orderStatusResponse struct {
	OrderCode      string `json:"order_code"`
	PublicSlug     string `json:"public_slug"`
	Status         string `json:"status"`
	Amount         int64  `json:"amount"`
	Currency       string `json:"currency"`
	PaymentStatus  string `json:"payment_status"`
	PaymentType    string `json:"payment_type,omitempty"`
	PaidAt         string `json:"paid_at,omitempty"`
	GiftUnlockPath string `json:"gift_unlock_path"`
}

// GetStatus: GET /v1/orders/{orderCode} — polling status pembayaran.
func (h *OrdersHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("orderCode")
	o, p, err := h.svc.GetStatus(r.Context(), code)
	if err != nil {
		switch {
		case errors.Is(err, orders.ErrNotFound):
			writeError(w, http.StatusNotFound, "ORDER_NOT_FOUND", "order tidak ditemukan")
		default:
			writeError(w, http.StatusInternalServerError, "INTERNAL", "gagal mengambil status order")
		}
		return
	}

	resp := orderStatusResponse{
		OrderCode:      o.OrderCode,
		PublicSlug:     o.PublicSlug,
		Status:         o.Status,
		Amount:         o.Amount,
		Currency:       o.Currency,
		PaymentStatus:  "pending",
		GiftUnlockPath: "/v1/gifts/" + o.PublicSlug,
	}
	if p != nil {
		resp.PaymentStatus = p.Status
		resp.PaymentType = p.PaymentType
		if p.PaidAt != nil {
			resp.PaidAt = p.PaidAt.Format(time.RFC3339)
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// PaymentsHandler menangani webhook Midtrans.
type PaymentsHandler struct {
	svc *payments.Service
}

func NewPaymentsHandler(svc *payments.Service) *PaymentsHandler { return &PaymentsHandler{svc: svc} }

// Notification: POST /v1/payments/notifications
// Midtrans menganggap pengiriman berhasil hanya untuk response 2xx.
func (h *PaymentsHandler) Notification(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "gagal membaca body")
		return
	}

	var notif midtrans.Notification
	if err := json.Unmarshal(raw, &notif); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "payload notifikasi bukan JSON valid")
		return
	}

	result, err := h.svc.HandleNotification(r.Context(), notif, raw)
	if err != nil {
		switch {
		case errors.Is(err, payments.ErrInvalidSignature):
			// Signature salah tidak boleh mengubah status apa pun.
			writeError(w, http.StatusForbidden, "INVALID_SIGNATURE", "signature tidak valid")
		case errors.Is(err, payments.ErrInvalidNotification):
			writeError(w, http.StatusBadRequest, "INVALID_NOTIFICATION", "field notifikasi tidak lengkap")
		case errors.Is(err, payments.ErrOrderNotFound):
			// 200 supaya Midtrans berhenti retry notifikasi untuk order tak dikenal.
			writeJSON(w, http.StatusOK, map[string]string{"status": "ignored", "reason": "order_not_found"})
		case errors.Is(err, payments.ErrAmountMismatch):
			slog.Error("webhook amount mismatch", "order_id", notif.OrderID, "err", err)
			writeError(w, http.StatusUnprocessableEntity, "AMOUNT_MISMATCH", "amount tidak cocok dengan order")
		default:
			slog.Error("webhook gagal diproses", "order_id", notif.OrderID, "err", err)
			writeError(w, http.StatusInternalServerError, "INTERNAL", "gagal memproses notifikasi")
		}
		return
	}

	// Notifikasi duplikat tetap 200 agar Midtrans berhenti kirim ulang.
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "processed",
		"duplicate": result.Duplicate,
	})
}

// GiftsHandler menangani endpoint publik gift box.
type GiftsHandler struct {
	svc *orders.Service
}

func NewGiftsHandler(svc *orders.Service) *GiftsHandler { return &GiftsHandler{svc: svc} }

// Get: GET /v1/gifts/{publicSlug} — isi box hanya untuk order paid.
func (h *GiftsHandler) Get(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("publicSlug")
	o, err := h.svc.GetPaidBySlug(r.Context(), slug)
	if err != nil {
		switch {
		case errors.Is(err, orders.ErrNotFound):
			writeError(w, http.StatusNotFound, "GIFT_NOT_FOUND", "gift tidak ditemukan")
		case errors.Is(err, orders.ErrNotPaid):
			writeError(w, http.StatusForbidden, "GIFT_LOCKED",
				"gift ini belum bisa dibuka karena pembayaran belum selesai")
		default:
			writeError(w, http.StatusInternalServerError, "INTERNAL", "gagal mengambil gift")
		}
		return
	}

	// box_payload sudah persis builder state untuk halaman penerima.
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(o.PayloadJSON)
}
