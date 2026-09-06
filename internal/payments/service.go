package payments

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/andreanpradana/goodiebox-server/internal/midtrans"
)

var (
	// ErrInvalidSignature: notifikasi ditolak karena signature tidak cocok.
	ErrInvalidSignature = errors.New("signature notifikasi tidak valid")
	// ErrInvalidNotification: payload notifikasi tidak lengkap.
	ErrInvalidNotification = errors.New("notifikasi tidak valid")
	// ErrOrderNotFound: notifikasi untuk order yang tidak dikenal.
	ErrOrderNotFound = errors.New("order untuk notifikasi tidak ditemukan")
	// ErrAmountMismatch: gross_amount tidak cocok dengan amount order.
	ErrAmountMismatch = errors.New("gross_amount tidak cocok dengan amount order")
)

// ApplyResult adalah hasil pemrosesan satu notifikasi.
type ApplyResult struct {
	// Duplicate true jika notifikasi identik sudah pernah diproses (dedupe).
	Duplicate    bool
	PaymentState midtrans.PaymentState
	OrderState   midtrans.OrderState
}

// Service memproses HTTP notification dari Midtrans.
type Service struct {
	repo *Repo
	mt   *midtrans.Client
}

func NewService(repo *Repo, mt *midtrans.Client) *Service {
	return &Service{repo: repo, mt: mt}
}

// HandleNotification memverifikasi signature, mendedupe notifikasi identik,
// lalu memperbarui status payment dan order dalam SATU transaksi database.
// Audit insert + status update di-transaksikan agar tidak ada notifikasi
// yang terlewat: jika update gagal, dedupe row ikut ter-rollback sehingga
// retry Midtrans diproses ulang.
func (s *Service) HandleNotification(ctx context.Context, n midtrans.Notification, rawPayload []byte) (*ApplyResult, error) {
	if !s.mt.VerifySignature(n) {
		return nil, ErrInvalidSignature
	}
	if n.OrderID == "" || n.TransactionID == "" || n.StatusCode == "" || n.GrossAmount == "" {
		return nil, ErrInvalidNotification
	}

	tx, err := s.repo.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// 1) Dedupe idempotent: kombinasi (order_id, transaction_id, status_code,
	// gross_amount) identik berarti notifikasi ulang dari Midtrans.
	var notifID string
	err = tx.QueryRow(ctx, `
		INSERT INTO payment_notifications (order_code, transaction_id, status_code, gross_amount, transaction_status, payload)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT DO NOTHING
		RETURNING id`,
		n.OrderID, n.TransactionID, n.StatusCode, n.GrossAmount, n.TransactionStatus, rawPayload,
	).Scan(&notifID)
	if errors.Is(err, pgx.ErrNoRows) {
		return &ApplyResult{Duplicate: true}, nil
	}
	if err != nil {
		return nil, err
	}

	// 2) Kunci baris order dan validasi amount dari server, bukan dari notifikasi.
	order, err := s.repo.GetOrderForUpdate(ctx, tx, n.OrderID)
	if errors.Is(err, ErrNotFound) {
		return nil, ErrOrderNotFound
	}
	if err != nil {
		return nil, err
	}
	amountIDR, err := midtrans.ParseGrossAmountIDR(n.GrossAmount)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAmountMismatch, err)
	}
	if amountIDR != order.Amount {
		return nil, fmt.Errorf("%w: order %s amount %d vs notifikasi %d",
			ErrAmountMismatch, order.ID, order.Amount, amountIDR)
	}

	// 3) Petakan status Midtrans ke status internal.
	payState, orderState := midtrans.MapStatus(n.TransactionStatus, n.FraudStatus)
	var paidAt *time.Time
	if payState == midtrans.PaymentSuccess {
		t := time.Now()
		paidAt = &t
	}

	// 4) Perbarui payment (upsert) dan order.
	_, err = tx.Exec(ctx, `
		INSERT INTO payments (order_id, transaction_id, transaction_status, fraud_status,
		                      payment_type, gross_amount, status, paid_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (order_id) DO UPDATE
		SET transaction_id     = EXCLUDED.transaction_id,
		    transaction_status = EXCLUDED.transaction_status,
		    fraud_status       = EXCLUDED.fraud_status,
		    payment_type       = EXCLUDED.payment_type,
		    gross_amount       = EXCLUDED.gross_amount,
		    status             = EXCLUDED.status,
		    paid_at            = COALESCE(payments.paid_at, EXCLUDED.paid_at),
		    updated_at         = now()`,
		order.ID, n.TransactionID, n.TransactionStatus, n.FraudStatus,
		n.PaymentType, n.GrossAmount, string(payState), paidAt)
	if err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE orders SET status = $2, updated_at = now() WHERE id = $1`,
		order.ID, string(orderState)); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	slog.Info("notifikasi midtrans diproses",
		"order_code", n.OrderID, "transaction_status", n.TransactionStatus,
		"fraud_status", n.FraudStatus, "payment", string(payState), "order", string(orderState))

	return &ApplyResult{PaymentState: payState, OrderState: orderState}, nil
}
