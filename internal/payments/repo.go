package payments

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound ketika payment tidak ditemukan.
var ErrNotFound = errors.New("payment tidak ditemukan")

// Payment adalah satu entri pembayaran per order (satu order = satu attempt
// pada versi minimal ini; percobaan ulang setelah expiry membuat order baru).
type Payment struct {
	ID                string     `json:"id"`
	OrderID           string     `json:"order_id"`
	SnapToken         string     `json:"-"`
	SnapRedirectURL   string     `json:"-"`
	TransactionID     string     `json:"transaction_id"`
	TransactionStatus string     `json:"transaction_status"`
	FraudStatus       string     `json:"fraud_status"`
	PaymentType       string     `json:"payment_type"`
	GrossAmount       string     `json:"gross_amount"`
	Status            string     `json:"status"`
	PaidAt            *time.Time `json:"paid_at"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

const selectPaymentCols = `id, order_id, snap_token, snap_redirect_url, transaction_id,
	transaction_status, fraud_status, payment_type, gross_amount, status, paid_at,
	created_at, updated_at`

// Repo akses tabel payments.
type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

func (r *Repo) GetByOrderID(ctx context.Context, orderID string) (*Payment, error) {
	var p Payment
	err := r.pool.QueryRow(ctx, `
		SELECT `+selectPaymentCols+` FROM payments WHERE order_id = $1`, orderID).
		Scan(&p.ID, &p.OrderID, &p.SnapToken, &p.SnapRedirectURL, &p.TransactionID,
			&p.TransactionStatus, &p.FraudStatus, &p.PaymentType, &p.GrossAmount, &p.Status,
			&p.PaidAt, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// UpsertSnap menyimpan token Snap untuk order (dipanggil saat pembuatan order).
// Baris dibuat jika belum ada agar webhook tidak pernah menemukan payment hilang.
func (r *Repo) UpsertSnap(ctx context.Context, orderID, token, redirectURL string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO payments (order_id, snap_token, snap_redirect_url)
		VALUES ($1, $2, $3)
		ON CONFLICT (order_id) DO UPDATE
		SET snap_token = EXCLUDED.snap_token,
		    snap_redirect_url = EXCLUDED.snap_redirect_url,
		    updated_at = now()`,
		orderID, token, redirectURL)
	return err
}

// GetOrderForUpdate mengunci baris order di dalam transaksi notifikasi
// agar update status tidak saling menimpa dengan request webhook lain.
type OrderRow struct {
	ID     string
	Status string
	Amount int64
}

func (r *Repo) GetOrderForUpdate(ctx context.Context, tx pgx.Tx, orderCode string) (*OrderRow, error) {
	var o OrderRow
	err := tx.QueryRow(ctx, `
		SELECT id, status, amount FROM orders WHERE order_code = $1 FOR UPDATE`, orderCode).
		Scan(&o.ID, &o.Status, &o.Amount)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}
