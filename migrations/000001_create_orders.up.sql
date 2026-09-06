CREATE TABLE IF NOT EXISTS orders (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_code      TEXT NOT NULL UNIQUE,
    idempotency_key TEXT UNIQUE,
    public_slug     TEXT NOT NULL UNIQUE,
    status          TEXT NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'paid', 'expired', 'cancelled')),
    amount          BIGINT NOT NULL CHECK (amount > 0),
    currency        TEXT NOT NULL DEFAULT 'IDR',
    box_payload     JSONB NOT NULL,
    sender_name     TEXT NOT NULL,
    sender_email    TEXT NOT NULL,
    sender_phone    TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_orders_status ON orders (status);
