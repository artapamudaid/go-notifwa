# Go-Notifwa

WhatsApp Gateway written in Go using [Fiber](https://gofiber.io/) and [Whatsmeow](https://pkg.go.dev/go.mau.fi/whatsmeow). This service is designed to be a high-performance backend, connecting with the Laravel backend.

## Requirements

- Go 1.20+
- MySQL / MariaDB Server (Laravel database)
- SQLite (for WhatsApp session persistence, included via CGO)

---

## Konfigurasi Database

Aplikasi membaca konfigurasi MySQL dari environment variable (prioritas utama) atau file `.env`.

| Variable      | Default       | Keterangan                  |
|---------------|---------------|------------------------------|
| `DB_HOST`     | `127.0.0.1`   | Host MySQL                  |
| `DB_PORT`     | `3306`        | Port MySQL                  |
| `DB_DATABASE` | `notifwa`     | Nama database               |
| `DB_USERNAME` | `root`        | Username MySQL              |
| `DB_PASSWORD` | _(kosong)_    | Password MySQL              |

**Penting:** Jangan gunakan tanda kutip pada nilai di `.env`.
```
# BENAR
DB_PASSWORD=mypass123

# SALAH
DB_PASSWORD='mypass123'
```

---

## Local / Native Setup

```bash
# 1. Clone repository
git clone <repo-url> && cd go-notifwa

# 2. Copy dan sesuaikan konfigurasi
cp .env.example .env
nano .env

# 3. Install dependencies
go mod tidy

# 4. Jalankan
go run main.go
```

Server berjalan di `http://localhost:8088`.

---

## Docker Deployment

### Build Image

```bash
docker build -t go-notifwa .
```

### Skenario 1: MySQL di Host yang Sama (Bind 127.0.0.1)

Jika MySQL hanya listen di `127.0.0.1`, gunakan **host networking**:

```bash
# Siapkan file session
touch examplestore.db

# Buat .env dengan DB_HOST=127.0.0.1
cp .env.example .env

# Jalankan dengan --network host
docker run -d \
  --name go-notifwa \
  --network host \
  -v $(pwd)/.env:/app/.env \
  -v $(pwd)/examplestore.db:/app/examplestore.db \
  --restart unless-stopped \
  go-notifwa
```

> **Catatan:** `--network host` tidak memerlukan `-p` port mapping. Container langsung menggunakan network host.

### Skenario 2: MySQL di Host yang Sama (Bind 0.0.0.0)

Jika MySQL listen di semua interface (`bind-address = 0.0.0.0`), gunakan bridge networking:

```bash
# .env: DB_HOST=host.docker.internal
cp .env.example .env
sed -i 's/DB_HOST=.*/DB_HOST=host.docker.internal/' .env

touch examplestore.db

docker run -d \
  --name go-notifwa \
  -p 8088:8088 \
  --add-host host.docker.internal:host-gateway \
  -v $(pwd)/.env:/app/.env \
  -v $(pwd)/examplestore.db:/app/examplestore.db \
  --restart unless-stopped \
  go-notifwa
```

### Skenario 3: MySQL di Server Remote

Jika MySQL berada di server terpisah (misal `192.168.1.100`):

```bash
# .env: DB_HOST=192.168.1.100
cp .env.example .env
sed -i 's/DB_HOST=.*/DB_HOST=192.168.1.100/' .env
sed -i 's/DB_USERNAME=.*/DB_USERNAME=myuser/' .env
sed -i 's/DB_PASSWORD=.*/DB_PASSWORD=mypassword/' .env

touch examplestore.db

docker run -d \
  --name go-notifwa \
  -p 8088:8088 \
  -v $(pwd)/.env:/app/.env \
  -v $(pwd)/examplestore.db:/app/examplestore.db \
  --restart unless-stopped \
  go-notifwa
```

### Skenario 4: Tanpa File .env (Environment Variables)

Anda bisa melewatkan `.env` dan langsung inject environment variable:

```bash
touch examplestore.db

docker run -d \
  --name go-notifwa \
  --network host \
  -e DB_HOST=127.0.0.1 \
  -e DB_PORT=3306 \
  -e DB_DATABASE=notifwa \
  -e DB_USERNAME=root \
  -e DB_PASSWORD=mypassword \
  -v $(pwd)/examplestore.db:/app/examplestore.db \
  --restart unless-stopped \
  go-notifwa
```

> **Memory GC Optimization**: Dockerfile mengatur `GOMEMLIMIT=250MiB` agar Go Garbage Collector optimal di container, menjaga RAM di bawah 250MB.

### Hard Reset (Memaksa Semua User Scan Ulang)

Jika Anda perlu membersihkan semua *session* WhatsApp secara permanen dan memaksa semua user untuk melakukan *scan* ulang (misal untuk memperbaiki masalah integrasi atau token), Anda cukup menghapus file `examplestore.db`:

1. Hentikan aplikasi Go atau container Docker Anda.
2. Hapus file database SQLite:
   ```bash
   rm examplestore.db
   ```
3. Jalankan kembali aplikasi/container. Sistem akan otomatis membuat database kosong yang baru dan status semua perangkat akan ter-reset. Semua user akan diminta untuk *scan* ulang QR code.

---

## High-Performance Worker Queue

Gateway ini menggunakan **Multi-Worker Pool Queue**. Saat menerima request `POST` untuk mengirim pesan, request langsung dibalas HTTP success tanpa menunggu WhatsApp selesai mengirim. Job masuk ke background channel dan diproses worker.

Cocok untuk **blast ribuan nomor** secara concurrent tanpa HTTP timeout dari Laravel backend. Worker menerapkan delay acak 1-5 detik per pesan untuk mencegah ban.

### Arsitektur Queue

```
HTTP Request (Fiber)
      │
      ▼
  Controller
  - Parse request
  - Lookup client via GetClient() [thread-safe]
  - Push SendJob ke channel
      │
      ▼
  JobQueue (chan, buffer 10000)
      │
  ┌───┴────────────────────┐
  ▼           ▼            ▼
Worker 1   Worker 2  ... Worker N
  │           │
  ▼           ▼
SendMessage() SendMessage()
(whatsmeow — thread-safe)
```

Setiap `SendJob` membawa nilai **lengkap** (Client, TargetJID, Message) — bukan pointer ke variabel yang bisa berubah — sehingga tidak ada risiko salah kirim antar request.

---

## Keamanan Konkurensi (Thread Safety)

> **Penting untuk deployment multi-tenant / blast banyak nomor sekaligus.**

### Masalah Umum: Salah Kirim ke Nomor Lain

Saat mengirim ke banyak nomor secara bersamaan (concurrent requests), masalah **salah kirim** bisa terjadi jika:

1. **Map `Clients` diakses tanpa mutex** — Go map tidak thread-safe. Jika 2 goroutine membaca/menulis map bersamaan, hasilnya tidak terdefinisi (undefined behavior), bisa crash atau membaca client yang salah.
2. **Variabel loop di-capture oleh closure** — Goroutine yang menangkap referensi variabel loop akan membaca nilai terakhir dari variabel tersebut, bukan nilai saat goroutine dibuat.

### Solusi yang Diterapkan

Kode ini menggunakan `sync.RWMutex` untuk melindungi semua akses ke map `clients` dan `statusCallbacks`:

```go
// LAMA — BERBAHAYA ❌ (data race)
var Clients = make(map[string]*whatsmeow.Client)
client := Clients[token]  // tidak aman dari goroutine lain

// BARU — AMAN ✅ (protected by RWMutex)
client, exists := whatsapp.GetClient(token)  // thread-safe
```

| Fungsi | Tipe Lock | Keterangan |
|---|---|---|
| `GetClient()` | `RLock` (read lock) | Banyak reader bisa berjalan bersamaan |
| `setClient()` | `Lock` (write lock) | Exclusive, blokir semua reader/writer |
| `deleteClient()` | `Lock` (write lock) | Exclusive |
| `getAndDeleteCallback()` | `Lock` (write lock) | Atomic get+delete, cegah double-fire |

### Mendeteksi Race Condition

Gunakan Go race detector saat development:

```bash
go run -race main.go
```

Jika ada data race, output akan muncul seperti:
```
WARNING: DATA RACE
Write at 0x... by goroutine N:
  go-notifwa/whatsapp.setClient(...)
Read at 0x... by goroutine M:
  go-notifwa/controllers.SendText(...)
```

---

## API Documentation

### 1. WebSocket — QR Scan & Status
**Endpoint:** `WS /ws/connect/:device`

Frontend connect untuk scan QR code. `:device` adalah session token.

### 2. Send Text Message
**Endpoint:** `POST /backend-send-text`

| Parameter | Type   | Description |
|-----------|--------|-------------|
| `token`   | string | Session / Device token |
| `number`  | string | Nomor tujuan (e.g. `0812xxx`, `62812xxx`) atau Group JID |
| `text`    | string | Isi pesan |

**Response:**
```json
{ "status": true, "message": "Message queued successfully" }
```

### 3. Send Media Message
**Endpoint:** `POST /backend-send-media`

| Parameter  | Type   | Description |
|------------|--------|-------------|
| `token`    | string | Session / Device token |
| `number`   | string | Nomor tujuan atau Group JID |
| `url`      | string | URL file media |
| `type`     | string | `image`, `video`, `audio`, `document` |
| `caption`  | string | (Optional) Caption |
| `filename` | string | (Optional) Nama file untuk document |

### 4. Send Poll Message
**Endpoint:** `POST /backend-send-poll`

| Parameter   | Type    | Description |
|-------------|---------|-------------|
| `token`     | string  | Session / Device token |
| `number`    | string  | Nomor tujuan atau Group JID |
| `name`      | string  | Judul/pertanyaan poll |
| `options`   | string  | JSON array pilihan (e.g. `["Yes","No"]`) |
| `countable` | boolean | `1` / `true` jika multi-select |

### 5. Get Groups
**Endpoint:** `POST /backend-getgroups`

| Parameter | Type   | Description |
|-----------|--------|-------------|
| `token`   | string | Session / Device token |

**Response:**
```json
{
  "status": true,
  "data": [
    {
      "id": "12345678@g.us",
      "name": "My Group",
      "subject": "My Group",
      "participants": [{ "id": "628xxx@s.whatsapp.net" }]
    }
  ]
}
```

### 6. Logout Device
**Endpoint:** `POST /backend-logout`

| Parameter | Type   | Description |
|-----------|--------|-------------|
| `token`   | string | Session / Device token |

---

## Troubleshooting

| Error | Solusi |
|-------|--------|
| `connection refused` | Pastikan MySQL berjalan dan `DB_HOST` benar. Cek bind-address MySQL (`SHOW VARIABLES LIKE 'bind_address';`). |
| `Access denied for user` | Cek username/password. Jangan pakai tanda kutip di `.env`. |
| `Peringatan: Gagal load file .env` | Normal jika pakai `-e` flags tanpa `.env` mount. Gunakan environment variable langsung. |
| Pesan terkirim ke nomor yang salah | Pastikan versi terbaru digunakan. Versi lama memiliki bug race condition pada map `Clients` yang tidak dilindungi mutex. Jalankan `go run -race main.go` untuk verifikasi. |
| `WARNING: DATA RACE` saat `-race` flag | Update ke versi terbaru. Race condition sudah diperbaiki dengan `sync.RWMutex` di `whatsapp/whatsapp.go`. |
| `fatal error: concurrent map read and map write` | Sama seperti di atas — upgrade ke versi terbaru. |
