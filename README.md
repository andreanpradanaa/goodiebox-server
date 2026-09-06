# goodiebox-server

Backend untuk [goodiebox-v2](../goodiebox-v2): order gift box digital + payment gateway **Midtrans Snap** + gift link publik. Ditulis di **Go**, database **PostgreSQL**.

Desain pembayaran mengadaptasi konsep kunci dari [Payment System Design Notes](https://github.com/liquidslr/system-design-notes/tree/main/26.%20Payment%20System) dalam bentuk satu service minimal (lihat [Roadmap](#roadmap-sesuai-materi-system-design)).

## Arsitektur

```
Frontend (goodiebox-v2)                goodiebox-server                 Midtrans
       │                                     │                            │
       │ POST /v1/orders                     │                            │
       ├────────────────────────────────────►│  POST /snap/v1/transactions│
       │◄── order_code + snap_token ─────────┤───────────────────────────►│
       │                                     │                            │
       │ window.snap.popup(token)  ───────────┼───────────────────────────►│  (hosted payment page)
       │                                     │                            │
       │                                     │◄── HTTP notification ──────┤  (webhook, async)
       │                                     │  verifikasi sha512         │
       │                                     │  dedupe + update status    │
       │ GET /v1/orders/{code} (polling)     │                            │
       │◄── status: paid ────────────────────┤                            │
       │                                     │                            │
Penerima│ GET /v1/gifts/{public_slug}         │                            │
       ├────────────────────────────────────►│  (hanya jika status paid)  │
       │◄── isi gift box ────────────────────┤                            │
```

Konsep dari materi yang dipakai pada versi minimal ini:

- **`order_code` server sebagai idempotency key ke PSP** — order_id yang sama tidak diproses dua kali Midtrans.
- **Amount selalu dihitung server** (`BOX_PRICE_IDR`), gross_amount pada notifikasi diverifikasi ulang terhadap order.
- **Webhook = sumber kebenaran final**; verifikasi `sha512(order_id + status_code + gross_amount + server_key)` sebelum menyentuh database.
- **Idempotent processing** — semua notifikasi diaudit di `payment_notifications` dengan unique key `(order_code, transaction_id, status_code, gross_amount)`; notifikasi ulang otomatis didedupe. Audit insert + update status dalam **satu transaksi** agar tidak ada notifikasi terlewat.
- **State machine** — payment: `pending → success | failed | expired | cancelled | refunded`, order: `pending → paid | expired | cancelled`.
- **Amount integer IDR**, tidak pernah float.

## Menjalankan

Prasyarat: Go 1.22+, Docker.

```bash
cp .env.example .env
# isi MIDTRANS_SERVER_KEY & MIDTRANS_CLIENT_KEY dari
# https://dashboard.sandbox.midtrans.com/settings/config_info

make compose-up   # postgres di localhost:5433
make run          # migrasi jalan otomatis saat startup, server di :8080
```

Cek `curl http://localhost:8080/healthz` → `{"status":"ok"}`.

### Environment

| Variable | Default | Keterangan |
|---|---|---|
| `PORT` | `8080` | Port HTTP server |
| `DATABASE_URL` | — (wajib) | URL Postgres |
| `MIDTRANS_SERVER_KEY` | — (wajib) | Server key Midtrans |
| `MIDTRANS_CLIENT_KEY` | — (wajib) | Client key, dikirim ke frontend untuk Snap popup |
| `MIDTRANS_IS_PRODUCTION` | `false` | `true` = production, `false` = sandbox |
| `BOX_PRICE_IDR` | `49000` | Harga satu box (IDR), ditentukan server |
| `ALLOWED_ORIGINS` | `http://localhost:5173` | Origin frontend untuk CORS (dipisah koma) |
| `MIDTRANS_SNAP_API_BASE` | (kosong = host resmi) | Override host Snap API, khusus dev dengan [mock Midtrans](#e2e-dev-tanpa-server-key-asli-mock-midtrans) |

## API

### `POST /v1/orders` — buat order + Snap token

Body = builder state frontend + kontak pembayar:

```bash
curl -X POST http://localhost:8080/v1/orders \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: 01890a5d-ac96-774b-bcce-b302099a8057' \
  -d '{
    "recipientName": "Adik",
    "senderName": "Kakak",
    "mood": "warm",
    "innerNote": "Selamat ulang tahun!",
    "boxColor": "#f59e0b",
    "boxTheme": "polkadot",
    "items": [
      {"id": "letter-1", "type": "letter", "title": "Surat", "message": "Hai!", "signature": "Kakak"}
    ],
    "customer": {"name": "Kakak", "email": "kakak@example.com", "phone": "081234567890"}
  }'
```

Response `201`:

```json
{
  "order_code": "GBX-260906-K3M8XA",
  "public_slug": "k7bdm2xq9r",
  "status": "pending",
  "amount": 49000,
  "currency": "IDR",
  "snap_token": "a1b2c3d4...",
  "snap_redirect_url": "https://app.sandbox.midtrans.com/snap/v2/vtweb/a1b2c3d4...",
  "client_key": "SB-Mid-client-...",
  "is_production": false,
  "created_at": "2026-09-06T10:00:00Z"
}
```

- Kirim header `Idempotency-Key` (UUID) agar retry tidak membuat order ganda — order yang sama dikembalikan.
- Frontend membuka Snap popup: muat `<script src="https://app.sandbox.midtrans.com/snap/snap.js" data-client-key="...">` lalu `window.snap.pay(snap_token, { onSuccess, onPending, onError, onClose })` — implementasi lengkapnya sudah ada di `goodiebox-v2` (`src/midtransSnap.ts` + alur checkout di `src/App.tsx`).

### `GET /v1/orders/{order_code}` — polling status

```json
{
  "order_code": "GBX-260906-K3M8XA",
  "public_slug": "k7bdm2xq9r",
  "status": "paid",
  "amount": 49000,
  "currency": "IDR",
  "payment_status": "success",
  "payment_type": "qris",
  "paid_at": "2026-09-06T10:05:00Z",
  "gift_unlock_path": "/v1/gifts/k7bdm2xq9r"
}
```

### `POST /v1/payments/notifications` — webhook Midtrans

Diisi otomatis oleh Midtrans (Dashboard → Settings → Configuration → Payment Notification URL). Response `2xx` = sukses; `403` jika signature tidak valid. Tidak dipanggil manual.

### `GET /v1/gifts/{public_slug}` — link kejutan penerima

Isi box (builder state lengkap) hanya jika order sudah `paid`; selain itu `403 GIFT_LOCKED`.

## E2E dev tanpa server key asli (mock Midtrans)

Alur pembayaran penuh bisa diuji lokal tanpa akun Midtrans memakai mock di `dev/mockmidtrans`:

```bash
# Terminal 1: mock Snap API + halaman simulasi bayar di :4010
MOCK_BACKEND_URL=http://localhost:8080 go run ./dev/mockmidtrans

# Terminal 2: server mengarahkan Snap API ke mock
MIDTRANS_SNAP_API_BASE=http://localhost:4010 make run

# Terminal 3: frontend goodiebox-v2 memuat snap.js dari mock
cd ../goodiebox-v2 && VITE_MIDTRANS_SNAP_JS_URL=http://localhost:4010/snap.js npm run dev
```

Flow yang dihasilkan identik dengan sandbox asli: klik **Buat tautan kejutan** di builder → popup simulasi bayar (menampilkan order code + nominal) → tombol **Bayar sekarang (settlement)** → mock memPOST webhook bergambar signature sha512 valid ke server → order `paid` → frontend menampilkan tautan kejutan → halaman `/gift/{slug}` terbuka. Tombol **Simulasikan pending** menguji jalur status pending.

Server key mock diambil dari Basic auth request Snap, sehingga signature selalu cocok dengan `MIDTRANS_SERVER_KEY` yang dipakai server.

## Menguji pembayaran di sandbox asli

1. `make run` dengan server key sandbox.
2. Buat order, buka `snap_redirect_url` di browser.
3. Di halaman Snap sandbox, pilih metode (mis. QRIS / GoPay / kartu kredit sandbox).
4. Setelah bayar, Midtrans mengirim webhook ke server (butuh URL publik — untuk lokal gunakan [ngrok](https://ngrok.com): `ngrok http 8080`, lalu set Payment Notification URL ke `https://<subdomain>.ngrok-free.dev/v1/payments/notifications`).
5. Alternatif tanpa pembayaran nyata: simulasi webhook manual (signature dihitung dengan server key sandbox-mu):

```bash
SERVER_KEY="SB-Mid-server-xxxx"
ORDER_CODE="GBX-..."      # dari POST /v1/orders
AMOUNT="49000.00"
SIG=$(printf '%s' "${ORDER_CODE}200${AMOUNT}${SERVER_KEY}" | openssl dgst -sha512 | awk '{print $NF}')

curl -X POST http://localhost:8080/v1/payments/notifications -H 'Content-Type: application/json' -d "{
  \"order_id\": \"${ORDER_CODE}\", \"status_code\": \"200\", \"gross_amount\": \"${AMOUNT}\",
  \"transaction_status\": \"settlement\", \"fraud_status\": \"accept\",
  \"transaction_id\": \"txn-sim-001\", \"payment_type\": \"qris\", \"signature_key\": \"${SIG}\"
}"
```

## Struktur

```
cmd/server/            entrypoint + graceful shutdown
internal/config/       env config, gagal cepat
internal/db/           pgx pool + migrasi (golang-migrate, jalan otomatis saat startup)
internal/midtrans/     Snap client, verifikasi signature sha512, mapping status, parsing amount
internal/orders/       domain + service + repo order (builder state, order code, public slug)
internal/payments/     service webhook (dedupe, verifikasi amount, update status transaksional) + repo
internal/api/          handler chi + CORS + error envelope
migrations/            SQL migrasi (orders, payments, payment_notifications)
```

## Roadmap (sesuai materi system design)

Versi minimal ini sengaja satu service. Urutan pengembangan lanjutan:

1. **Payment executor terpisah** — pemisahan koordinasi vs eksekusi per payment order.
2. **Ledger double-entry** — setiap pergerakan uang tercatat dua sisi (debit/kredit) agar total selalu nol.
3. **Retry queue + dead-letter queue** — notifikasi yang gagal diproses di-push ke retry dengan exponential backoff.
4. **Reconcilisation** — job malam membandingkan settlement report Midtrans vs status internal, dengan tier penanganan mismatch.
5. **File upload** — foto/audio/video dari builder masih blob client-side; butuh endpoint upload + object storage.
6. Monitoring/alerting, rate limiting, dan API key untuk endpoint order creation.
