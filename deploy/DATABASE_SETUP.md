# Database Setup Guide — PostgreSQL di VPS

Setup sekali, sebelum deploy app pertama kali.

## 1. Install PostgreSQL 16

SSH ke VPS, lalu:

```bash
sudo apt update
sudo apt install -y postgresql postgresql-contrib postgresql-16
```

Start service:

```bash
sudo systemctl start postgresql
sudo systemctl enable postgresql
```

Verify running:

```bash
sudo systemctl status postgresql
```

## 2. Create Database & User

Login sebagai `postgres`:

```bash
sudo -u postgres psql
```

Dalam prompt `postgres=#`, jalankan (ganti `STRONG_PASSWORD` dengan password aman):

```sql
CREATE USER goodiebox WITH PASSWORD 'STRONG_PASSWORD';
CREATE DATABASE goodiebox OWNER goodiebox;
GRANT CONNECT ON DATABASE goodiebox TO goodiebox;
GRANT USAGE ON SCHEMA public TO goodiebox;
GRANT CREATE ON SCHEMA public TO goodiebox;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO goodiebox;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON SEQUENCES TO goodiebox;
\q
```

## 3. Update `.env` di VPS

Copy ke VPS atau edit langsung:

```bash
sudo nano /opt/goodiebox/.env
```

Isi:

```
PORT=8080
ALLOWED_ORIGINS=https://yourdomain.com

DATABASE_URL=postgres://goodiebox:STRONG_PASSWORD@localhost:5432/goodiebox?sslmode=disable

MIDTRANS_SERVER_KEY=SB-Mid-server-xxxxx
MIDTRANS_CLIENT_KEY=SB-Mid-client-xxxxx
MIDTRANS_IS_PRODUCTION=false

BOX_PRICE_IDR=49000
```

**Penting:**
- Ganti `STRONG_PASSWORD` dengan password yang sama seperti di step 2
- Gunakan port **5432** (default PostgreSQL), bukan 5433
- `sslmode=disable` untuk internal VPS (aman karena localhost)

Save: `Ctrl+X` → `Y` → `Enter`

Konfigurasi permissions:

```bash
sudo chown goodiebox:goodiebox /opt/goodiebox/.env
sudo chmod 600 /opt/goodiebox/.env
```

## 4. Test Koneksi (Opsional)

Sebelum deploy, test koneksi:

```bash
sudo -u goodiebox psql "postgres://goodiebox:STRONG_PASSWORD@localhost:5432/goodiebox?sslmode=disable" -c "\dt"
```

Harusnya output kosong (belum ada tabel) atau error jika password salah.

## 5. Deploy App & Auto-Migrate

Setelah binary di `/opt/goodiebox/server`, start service:

```bash
sudo systemctl start goodiebox-server
sudo systemctl status goodiebox-server
```

Check logs untuk verify migrations jalan:

```bash
sudo journalctl -u goodiebox-server -f
```

Harusnya terlihat:
```
migrasi database selesai
server berjalan port=8080
```

## 6. Verify Tabel Terbuat

Setelah app start, tabel otomatis terbuat via migrations:

```bash
sudo -u goodiebox psql "postgres://goodiebox:STRONG_PASSWORD@localhost:5432/goodiebox" -c "\dt"
```

Harusnya muncul:
```
              List of relations
 Schema |          Name           | Type  | Owner
--------+-------------------------+-------+----------
 public | orders                  | table | goodiebox
 public | payment_notifications   | table | goodiebox
 public | payments                | table | goodiebox
 public | schema_migrations       | table | goodiebox
```

Lihat migration history:

```bash
sudo -u goodiebox psql "postgres://goodiebox:STRONG_PASSWORD@localhost:5432/goodiebox" -c "SELECT * FROM schema_migrations ORDER BY installed_on;"
```

## 7. Backup Strategy (Recommended)

Tambah cron backup harian:

```bash
sudo nano /etc/cron.daily/goodiebox-backup
```

Isi:

```bash
#!/bin/bash
BACKUP_DIR="/var/backups/goodiebox"
mkdir -p "$BACKUP_DIR"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

sudo -u postgres pg_dump goodiebox | gzip > "$BACKUP_DIR/goodiebox_$TIMESTAMP.sql.gz"

# Keep hanya 30 hari terakhir
find "$BACKUP_DIR" -name "goodiebox_*.sql.gz" -mtime +30 -delete
```

Chmod:

```bash
sudo chmod +x /etc/cron.daily/goodiebox-backup
```

Manual backup kapan saja:

```bash
sudo -u postgres pg_dump goodiebox | gzip > ~/goodiebox_backup_$(date +%Y%m%d).sql.gz
```

Restore:

```bash
gunzip < goodiebox_backup_20240915.sql.gz | sudo -u postgres psql goodiebox
```

## Troubleshooting

### Error: "password authentication failed"
- Verify `STRONG_PASSWORD` di `.env` sama dengan saat create user
- Test: `sudo -u postgres psql` langsung work? Kalau yes, user ada tapi password salah

### Error: "database \"goodiebox\" does not exist"
- Verify database sudah create: `sudo -u postgres psql -l | grep goodiebox`
- Jika tidak ada, buat ulang via step 2

### Error: "permission denied for schema public"
- Verify GRANT statements di step 2 dijalankan untuk user `goodiebox`

### App stuck di "migrasi gagal"
- Check logs: `sudo journalctl -u goodiebox-server | tail -50`
- Verify database URL di `.env`
- Verify PostgreSQL running: `sudo systemctl status postgresql`

---

**Setup selesai** — setiap deploy binary baru, migrations auto-check & run yang belum ada.
