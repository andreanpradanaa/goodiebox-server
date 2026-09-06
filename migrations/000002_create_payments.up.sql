CREATE TABLE IF NOT EXISTS payments (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id           UUID NOT NULL UNIQUE REFERENCES orders (id) ON DELETE CASCADE,
    snap_token         TEXT NOT NULL DEFAULT '',
    snap_redirect_url  TEXT NOT NULL DEFAULT '',
    transaction_id     TEXT NOT NULL DEFAULT '',
    transaction_status TEXT NOT NULL DEFAULT '',
    fraud_status       TEXT NOT NULL DEFAULT '',
    payment_type       TEXT NOT NULL DEFAULT '',
    gross_amount       TEXT NOT NULL DEFAULT '',
    status             TEXT NOT NULL DEFAULT 'pending'
                       CHECK (status IN ('pending', 'success', 'failed', 'expired', 'cancelled', 'refunded')),
    paid_at            TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
