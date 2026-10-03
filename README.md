# SIAKAD Mini REST API

Backend REST API sederhana untuk mengelola pengguna, data mahasiswa, mata kuliah,
dan Kartu Rencana Studi (KRS). Aplikasi menggunakan Go, Fiber v2, dan PostgreSQL.

## Persyaratan

- Go 1.23 atau lebih baru
- PostgreSQL 14 atau lebih baru

## Menyiapkan database

Buat database PostgreSQL bernama `siakad_mini`, lalu atur variabel lingkungan.
Contoh PowerShell:

```powershell
$env:DATABASE_URL = "postgres://postgres:password@localhost:5432/siakad_mini?sslmode=disable"
$env:JWT_SECRET = "ganti-dengan-random-secret-minimal-32-karakter"
$env:ADMIN_EMAIL = "admin@siakad.local"
$env:ADMIN_PASSWORD = "Admin123!"
```

Migration awal dijalankan otomatis saat server mulai. Isi data awal (idempotent)
dengan perintah berikut:

```powershell
go run ./cmd/seed
go run ./cmd/api
```

Seeder membuat satu admin, 20 mahasiswa, serta 10 mata kuliah. Password awal
admin mengikuti `ADMIN_PASSWORD` (default lokal `Admin123!`); password mahasiswa
seed adalah NIM masing-masing. Password disimpan menggunakan bcrypt. Ganti
kredensial contoh sebelum aplikasi digunakan di lingkungan selain lokal.

## Endpoint

Semua endpoint kecuali login memerlukan header `Authorization: Bearer <token>`.
Response sukses dan error menggunakan JSON dengan `success` serta `message`;
validasi menyediakan objek `errors`.

| Method | Endpoint | Akses |
|---|---|---|
| POST | `/api/v1/auth/login` | Publik |
| GET | `/api/v1/auth/me` | Semua role |
| GET | `/api/v1/students` | Admin |
| POST | `/api/v1/students` | Admin |
| GET | `/api/v1/students/{id}` | Admin atau mahasiswa pemilik |
| PUT | `/api/v1/students/{id}` | Admin |
| DELETE | `/api/v1/students/{id}` | Admin |
| GET | `/api/v1/courses` | Semua role |
| POST | `/api/v1/enrollments` | Mahasiswa |
| DELETE | `/api/v1/enrollments/{id}` | Mahasiswa pemilik |

Daftar mahasiswa menerima `page`, `per_page` (maksimum 50), `prodi`, `angkatan`,
`search`, dan `sort` (`nama` atau `-ipk_terakhir`). Daftar mata kuliah menerima
`semester`, `search`, dan `available=true`.

KRS divalidasi dalam transaksi: duplikasi dicegah oleh constraint unik, baris mata
kuliah dikunci saat pemeriksaan kuota, dan baris mahasiswa dikunci agar permintaan
bersamaan tidak melampaui batas SKS. Batas SKS mengikuti IPK: minimal 3,00 = 24,
2,50–2,99 = 21, dan di bawah 2,50 = 18 SKS per tahun akademik.

## Pemeriksaan

```powershell
go test ./...
go vet ./...
```

Uji API langsung memerlukan database yang sudah dimigrasikan dan di-seed.
