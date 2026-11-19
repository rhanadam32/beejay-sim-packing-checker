# Packing Checker

Aplikasi Go untuk membandingkan data packing dari tiga sumber:

1. **Master** — `database/data master.csv` (mapping produk chat ↔ produk apps)
2. **Chat** — `packing from chat/chat.txt` (nilai "mc")
3. **Apps** — file PDF tally dari folder `packing from apps/` (nilai "qty")

UI menggunakan Tailwind CSS (gaya shadcn) + Vue 3.

## Cara Menjalankan

### Prasyarat

- Go 1.21+ (modul sudah menyertakan dependency `github.com/ledongthuc/pdf`)
- Koneksi internet untuk CDN Tailwind/Vue (opsional bisa diganti lokal)

### Langkah

1. Masuk ke folder proyek:
   ```powershell
   cd E:\golang\packing-checker
   ```
2. Download dependency (sekali saja):
   ```powershell
   go mod tidy
   ```
3. Jalankan aplikasi:
   ```powershell
   go run .
   ```
   atau build dulu:
   ```powershell
   go build -o packing-checker.exe .
   .\packing-checker.exe
   ```
4. Buka browser: `http://localhost:8080`

## Cara Kerja Program

### 1. Load Master
Saat startup, aplikasi membaca `database/data master.csv` (pemisah `;`) dan memuat daftar master ke memori. Setiap baris berisi:
- `ide` — ID
- `master_product_chat` — nama produk di chat
- `master_product_apps` — nama produk di apps (PDF)

### 2. Load Chat
Jika file `packing from chat/chat.txt` ada, aplikasi mem-parse otomatis. Format yang dikenali:
```
Nama produk = 64 mc
```
Bagian `Luke` di chat akan otomatis diberi prefix `Luke ` pada nama produk agar cocok dengan master.

### 3. Upload PDF (Apps)
Di tab **Upload**, pilih file PDF tally. Aplikasi:
- Menyimpan file sementara
- Mengekstrak teks PDF dengan `ledongthuc/pdf`
- Mencocokkan setiap `master_product_apps` ke teks PDF (case-insensitive, normalisasi spasi/ligatur)
- Mengambil angka qty setelah nama produk
- Menjumlahkan qty jika beberapa PDF di-upload (merge)

### 4. Perbandingan
Tab **Perbandingan** menampilkan tabel:
- Produk chat (master) & qty chat
- Produk apps (master) & qty apps
- Status:
  - `OK` — qty sama
  - `MISMATCH` — qty berbeda
  - `MISSING` — salah satu sumber tidak ditemukan

Pencocokan nama menggunakan normalisasi (lowercase, collapse spasi) + fuzzy (Levenshtein ≤ 3) untuk toleransi typo.

### 5. Reset
Tombol **Reset Chat** / **Reset PDF** mengosongkan data yang sudah di-upload tanpa menghapus file sumber.

### 6. CRUD Master
Tab **Master CRUD** memungkinkan:
- Tambah master baru (chat name + apps name)
- Edit inline
- Hapus
Perubahan disimpan kembali ke `database/data master.csv`.

## Struktur File

```
packing-checker/
├── main.go
├── go.mod
├── database/
│   └── data master.csv
├── packing from chat/
│   └── chat.txt
├── packing from apps/
│   └── *.pdf
├── static/
│   ├── index.html
│   └── app.js
└── README.md
```

## Endpoint API

| Method | Path | Keterangan |
|--------|------|------------|
| GET | `/api/compare` | Data perbandingan |
| POST | `/api/upload-chat` | Upload chat txt |
| POST | `/api/upload-pdf` | Upload PDF tally |
| POST | `/api/reset-chat` | Reset data chat |
| POST | `/api/reset-pdf` | Reset data PDF |
| GET | `/api/masters` | List master |
| POST | `/api/masters` | Tambah master |
| PUT | `/api/masters/{id}` | Update master |
| DELETE | `/api/masters/{id}` | Hapus master |
