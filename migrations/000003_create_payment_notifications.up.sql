CREATE TABLE IF NOT EXISTS payment_notifications (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_code         TEXT NOT NULL,
    transaction_id     TEXT NOT NULL,
    status_code        TEXT NOT NULL,
    gross_amount       TEXT NOT NULL,
    transaction_status TEXT NOT NULL,
    payload            JSONB NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Dedupe idempotency: notifikasi ulang dari Midtrans punya kombinasi identik
    UNIQUE (order_code, transaction_id, status_code, gross_amount)
);

CREATE INDEX IF NOT EXISTS idx_payment_notifications_order ON payment_notifications (order_code);
