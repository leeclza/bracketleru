# Bracket Leru

Web untuk bikin dan mengelola bracket turnamen: masukkan nama peserta, pilih format, dan bracket langsung jadi. Skor diisi langsung di kartu pertandingan, dan pemenang otomatis maju ke ronde berikutnya.

Cocok untuk turnamen game (Tekken, eFootball, Mobile Legends, dan lain-lain), olahraga, atau lomba antar teman.

## Fitur

- **5 format turnamen**
  - Single Elimination (bisa pakai perebutan juara 3)
  - Double Elimination (dengan grand final reset)
  - Round Robin (1 atau 2 kali ketemu)
  - Swiss (dengan Buchholz)
  - Group Stage, lalu playoff gugur
- **Preview langsung** saat mengisi peserta, sebelum turnamen dibuat
- **Klasemen otomatis** untuk round robin, swiss, dan grup
- **Login Google khusus email `@student.itera.ac.id`**, lalu wajib isi username. Hanya pembuat turnamen yang bisa mengisi skor dan mengedit
- **Link tonton publik**: penonton bisa buka `/t/:id` tanpa login, dan halamannya refresh otomatis
- **Export PDF**: bracket digambar sebagai bagan bergaris (halaman landscape), klasemen sebagai tabel
- **Hapus otomatis** setelah 7 hari tanpa perubahan, dengan countdown di profil
- Tampilan hitam putih yang nyaman di desktop dan HP

## Tech stack

| Bagian | Pakai |
|---|---|
| Bahasa | Go |
| Web framework | [Gin](https://github.com/gin-gonic/gin) |
| Template | `html/template` (server-side render), vanilla JS untuk preview dan garis bracket |
| Database | PostgreSQL di [Neon](https://neon.tech) (driver [pgx](https://github.com/jackc/pgx)). Saat development pakai file JSON lokal |
| Auth | Google OAuth 2.0, sesi di cookie bertanda tangan HMAC |
| PDF | [go-pdf/fpdf](https://github.com/go-pdf/fpdf) |
| Hosting | [Vercel](https://vercel.com) (Go serverless function) |

Template dan file statis di-embed ke binary dengan `go:embed`, jadi deploy cukup satu binary.

## Struktur folder

```
api/index.go          entry point untuk Vercel
main.go               entry point untuk lokal (baca .env otomatis)
internal/app          pilih penyimpanan (Postgres / file) dan bikin router
internal/bracket      logika turnamen: build bracket, skor, klasemen
internal/store        penyimpanan data (FileStore & SQLStore Postgres)
internal/web          route, handler, auth, PDF, template, CSS/JS
```

## Menjalankan di lokal

Butuh Go 1.26 atau lebih baru.

1. Buat OAuth client di [Google Cloud Console](https://console.cloud.google.com/auth/clients) dengan tipe **Web application** dan redirect URI `http://localhost:8080/auth/callback`.
2. Buat file `.env` di root project:
   ```
   GOOGLE_CLIENT_ID=xxx.apps.googleusercontent.com
   GOOGLE_CLIENT_SECRET=xxx
   SESSION_SECRET=string-acak-minimal-32-karakter
   ```
3. Jalankan:
   ```
   go build -o bracketleru.exe .
   ./bracketleru.exe
   ```
4. Buka http://localhost:8080

Tanpa `DATABASE_URL`, data disimpan sebagai file JSON di folder `data/`. Kalau mau pakai Postgres juga di lokal, isi `DATABASE_URL` di `.env`.

## Environment variables

| Nama | Wajib | Keterangan |
|---|---|---|
| `GOOGLE_CLIENT_ID` | ya | Client ID dari Google |
| `GOOGLE_CLIENT_SECRET` | ya | Client secret dari Google |
| `SESSION_SECRET` | ya | Kunci tanda tangan cookie, minimal 32 karakter |
| `BASE_URL` | di production | Contoh `https://bracketleru.vercel.app`, dipakai untuk redirect OAuth |
| `DATABASE_URL` | di production | Connection string Postgres, contoh `postgresql://user:pass@ep-xxx.neon.tech/neondb?sslmode=require` |
| `PORT` | tidak | Port lokal, default `8080` |
| `DATA_DIR` | tidak | Folder data lokal, default `data` |
| `ALLOWED_EMAIL_DOMAINS` | tidak | Domain email yang boleh login, default `student.itera.ac.id` (harus persis sama) |

## Deploy ke Vercel

1. Import repo ini di Vercel.
2. Isi semua environment variable di atas di **Settings → Environment Variables**.
3. Tambahkan `https://<domain>/auth/callback` ke redirect URI di Google Cloud.
4. Deploy.

Filesystem Vercel bersifat sementara, jadi production **wajib** pakai database. Kalau tidak, data akan hilang.

### Database (Neon)

1. Buat project di [neon.tech](https://neon.tech) (free tier cukup), atau lewat Vercel → **Storage → Neon** supaya `DATABASE_URL` terisi otomatis.
2. Salin connection string. Pakai yang *pooled* (ada `-pooler` di host-nya), lalu isi ke `DATABASE_URL`.
3. Tabel `tournaments` dibuat otomatis waktu app pertama kali jalan, jadi tidak perlu migrasi manual.

Di free tier Neon, compute akan tidur kalau tidak dipakai beberapa menit dan nyala lagi otomatis saat ada request. Datanya tetap tersimpan.

## Siapa yang bisa login

Hanya akun Google dengan email **`@student.itera.ac.id`** yang bisa login. Email lain, termasuk `@itera.ac.id` (staf) dan Gmail, ditolak di halaman login. Daftar domain bisa diubah lewat env `ALLOWED_EMAIL_DOMAINS` (dipisah koma, harus persis sama).

Setelah login pertama, user **wajib mengisi username** (3–20 karakter: huruf kecil, angka, titik, underscore; unik). Username disimpan di tabel `users` dan tampil di pojok kanan atas.

Penonton tetap bisa membuka link tonton `/t/:id` tanpa login. Halaman penonton tidak refresh otomatis, jadi ada tombol **Muat skor terbaru** supaya hemat request.



## Test

```
go test ./...
```
