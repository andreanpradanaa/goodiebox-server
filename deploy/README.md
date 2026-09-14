# Deploy ke VPS (binary only, via GitHub Actions)

Setup sekali di VPS (Ubuntu 24.04 x86_64):

```bash
sudo useradd --system --home /opt/goodiebox --shell /usr/sbin/nologin goodiebox
sudo mkdir -p /opt/goodiebox
sudo chown goodiebox:goodiebox /opt/goodiebox
```

Copy `.env.example` jadi `/opt/goodiebox/.env` di VPS, isi dengan value produksi (DATABASE_URL, MIDTRANS_*, dll), lalu:

```bash
sudo chown goodiebox:goodiebox /opt/goodiebox/.env
sudo chmod 600 /opt/goodiebox/.env
```

Copy `goodiebox-server.service` ke `/etc/systemd/system/goodiebox-server.service`, lalu:

```bash
sudo systemctl daemon-reload
sudo systemctl enable goodiebox-server
```

Buat user deploy (bisa `goodiebox` sendiri atau user lain) yang punya sudo tanpa password khusus untuk dua command ini saja (biar GitHub Actions tidak butuh full sudo):

```bash
# /etc/sudoers.d/goodiebox-deploy
deployuser ALL=(root) NOPASSWD: /usr/bin/install -m 755 /tmp/goodiebox-deploy/server /opt/goodiebox/server, /usr/bin/systemctl restart goodiebox-server, /usr/bin/systemctl status goodiebox-server
```

## GitHub Secrets yang perlu diisi (Settings → Secrets and variables → Actions)

- `VPS_HOST` — IP/hostname VPS
- `VPS_USER` — user SSH (mis. `deployuser`)
- `VPS_SSH_KEY` — private key SSH (generate pasangan khusus deploy, taruh public key di `~/.ssh/authorized_keys` user itu di VPS)
- `VPS_PORT` — opsional, default 22

Setelah itu, setiap push ke `main` otomatis build binary Go (`linux/amd64`) dan deploy ke VPS via SSH — VPS tidak perlu compile apa pun, cuma jalankan binary jadi.
