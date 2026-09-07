package orders

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/andreanpradana/goodiebox-server/internal/midtrans"
	"github.com/andreanpradana/goodiebox-server/internal/payments"
)

// MidtransClient adalah kebutuhan orders terhadap PSP.
type MidtransClient interface {
	CreateSnapTransaction(ctx context.Context, orderCode string, amountIDR int64, cust midtrans.Customer) (token, redirectURL string, err error)
}

// ErrMidtransUnavailable ketika Snap token gagal dibuat.
var ErrMidtransUnavailable = errors.New("gagal membuat transaksi Midtrans")

// Service logika bisnis order gift box.
type Service struct {
	repo     *Repo
	payments *payments.Repo
	mt       MidtransClient
	priceIDR int64
}

func NewService(repo *Repo, payRepo *payments.Repo, mt MidtransClient, priceIDR int64) *Service {
	return &Service{repo: repo, payments: payRepo, mt: mt, priceIDR: priceIDR}
}

// CreateResult adalah response POST /v1/orders.
type CreateResult struct {
	Order           *Order `json:"order"`
	SnapToken       string `json:"snap_token"`
	SnapRedirectURL string `json:"snap_redirect_url"`
}

// Create membuat order baru (atau mengembalikan order yang sudah ada jika
// idempotency-key sama), lalu memastikan Snap token tersedia.
// Amount selalu dihitung server dari harga konfigurasi — amount dari
// client tidak pernah dipercaya.
func (s *Service) Create(ctx context.Context, req CreateRequest, idempotencyKey string) (*CreateResult, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// Idempotency: header idempotency-key yang sama mengembalikan order
	// yang sama (unique constraint + lookup, sesuai materi payment design).
	if idempotencyKey != "" {
		existing, err := s.repo.GetByIDempotencyKey(ctx, idempotencyKey)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return nil, err
		}
		if existing != nil {
			return s.ensureSnapToken(ctx, existing, req)
		}
	}

	payload, err := req.BoxPayloadJSON()
	if err != nil {
		return nil, fmt.Errorf("payload box tidak valid: %w", err)
	}
	cust := req.CustomerForPayment()

	order := &Order{
		Status:      "pending",
		Amount:      s.priceIDR,
		Currency:    "IDR",
		PayloadJSON: payload,
		SenderName:  cust.Name,
		SenderEmail: cust.Email,
		SenderPhone: cust.Phone,
	}

	// order_code dipakai Midtrans sebagai order_id (idempotency di sisi PSP),
	// public_slug untuk link kejutan. Collision jarang tapi ditangani retry.
	for attempt := 0; attempt < 3; attempt++ {
		order.OrderCode = NewOrderCode(time.Now().UTC())
		order.PublicSlug = NewPublicSlug()
		err = s.repo.Insert(ctx, order, idempotencyKey)
		if err == nil {
			break
		}
		if !isUniqueViolation(err) {
			return nil, err
		}
		// Idempotency key bentrok: order-nya sudah dibuat request lain.
		if idempotencyKey != "" {
			if existing, getErr := s.repo.GetByIDempotencyKey(ctx, idempotencyKey); getErr == nil {
				return s.ensureSnapToken(ctx, existing, req)
			}
		}
	}
	if err != nil {
		return nil, fmt.Errorf("gagal membuat order unik setelah 3 percobaan: %w", err)
	}

	return s.ensureSnapToken(ctx, order, req)
}

// ensureSnapToken mengembalikan token Snap untuk order, memintanya ke
// Midtrans hanya jika belum ada. Dipanggil ulang pada request idempotent.
func (s *Service) ensureSnapToken(ctx context.Context, o *Order, req CreateRequest) (*CreateResult, error) {
	token, redirectURL := "", ""
	p, err := s.payments.GetByOrderID(ctx, o.ID)
	switch {
	case err == nil:
		token, redirectURL = p.SnapToken, p.SnapRedirectURL
	case errors.Is(err, payments.ErrNotFound):
		// belum ada, buat di bawah
	default:
		return nil, err
	}

	if token == "" {
		cust := midtrans.Customer{
			Name:  req.CustomerForPayment().Name,
			Email: req.CustomerForPayment().Email,
			Phone: req.CustomerForPayment().Phone,
		}
		token, redirectURL, err = s.mt.CreateSnapTransaction(ctx, o.OrderCode, o.Amount, cust)
		if err != nil {
			slog.Error("gagal membuat snap token", "order_code", o.OrderCode, "err", err)
			return nil, ErrMidtransUnavailable
		}
		if err := s.payments.UpsertSnap(ctx, o.ID, token, redirectURL); err != nil {
			return nil, err
		}
	}

	return &CreateResult{Order: o, SnapToken: token, SnapRedirectURL: redirectURL}, nil
}

// GetStatus dipakai frontend untuk polling status pembayaran.
func (s *Service) GetStatus(ctx context.Context, orderCode string) (*Order, *payments.Payment, error) {
	o, err := s.repo.GetByOrderCode(ctx, orderCode)
	if err != nil {
		return nil, nil, err
	}
	p, err := s.payments.GetByOrderID(ctx, o.ID)
	if errors.Is(err, payments.ErrNotFound) {
		return o, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	return o, p, nil
}

// GetPaidBySlug dipakai endpoint publik gift: hanya order paid yang boleh
// menampilkan isi box.
func (s *Service) GetPaidBySlug(ctx context.Context, slug string) (*Order, error) {
	o, err := s.repo.GetByPublicSlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	if o.Status != "paid" {
		return nil, ErrNotPaid
	}
	return o, nil
}
