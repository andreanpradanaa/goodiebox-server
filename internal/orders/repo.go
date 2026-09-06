package orders

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repo akses tabel orders.
type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

// uniqueViolation adalah kode error postgres untuk pelanggaran unique constraint.
const uniqueViolation = "23505"

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation
}

func (r *Repo) Insert(ctx context.Context, o *Order, idempotencyKey string) error {
	return r.pool.QueryRow(ctx, `
		INSERT INTO orders (order_code, idempotency_key, public_slug, amount, box_payload,
		                    sender_name, sender_email, sender_phone)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at, updated_at`,
		o.OrderCode, idempotencyKey, o.PublicSlug, o.Amount, o.PayloadJSON,
		o.SenderName, o.SenderEmail, o.SenderPhone,
	).Scan(&o.ID, &o.CreatedAt, &o.UpdatedAt)
}

func (r *Repo) GetByIDempotencyKey(ctx context.Context, key string) (*Order, error) {
	o, err := scanOrder(r.pool.QueryRow(ctx, `
		SELECT `+selectOrderCols+` FROM orders WHERE idempotency_key = $1`, key))
	return o, wrapScanErr(err)
}

func (r *Repo) GetByOrderCode(ctx context.Context, code string) (*Order, error) {
	o, err := scanOrder(r.pool.QueryRow(ctx, `
		SELECT `+selectOrderCols+` FROM orders WHERE order_code = $1`, code))
	return o, wrapScanErr(err)
}

func (r *Repo) GetByPublicSlug(ctx context.Context, slug string) (*Order, error) {
	o, err := scanOrder(r.pool.QueryRow(ctx, `
		SELECT `+selectOrderCols+` FROM orders WHERE public_slug = $1`, slug))
	return o, wrapScanErr(err)
}

// wrapScanErr menerjemahkan ErrNoRows ke ErrNotFound package ini.
func wrapScanErr(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
