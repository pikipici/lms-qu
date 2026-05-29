# Plan Implementasi Ujian Siswa di Tab Kelas

## Kesimpulan Cek Backend

Backend sudah mendukung kebutuhan utama untuk mindahin pengalaman `Ujian` siswa ke detail kelas.

Endpoint yang sudah ada:

- `GET /api/v1/siswa/kelas/:id/ujian`
  - List ujian per kelas dari sisi siswa.
  - Service backend otomatis filter role siswa: hanya `published` dan validasi siswa enrolled di kelas.
  - Sudah dibungkus client frontend `listSiswaUjianByKelas(kelasID)` di `frontend/lib/siswa-ujian-api.ts`.
- `GET /api/v1/siswa/kelas/:id/ujian/hasil`
  - List hasil/attempt siswa untuk semua ujian di kelas tersebut.
  - Sudah dibungkus client frontend `listSiswaUjianHasil(kelasID)`.
- `POST /api/v1/siswa/ujian/:id/start`
  - Start/resume attempt ujian.
  - Sudah dibungkus `startSiswaUjian(ujianID)`.
- `GET /api/v1/siswa/hasil-ujian/:id/items`
  - Ambil soal live attempt, tanpa kunci jawaban.
- `POST /api/v1/siswa/hasil-ujian/:id/answer`
  - Autosave jawaban.
- `POST /api/v1/siswa/hasil-ujian/:id/submit`
  - Submit + grading.
- `GET /api/v1/siswa/hasil-ujian/:id/review`
  - Review setelah submit, mengikuti gate review backend.

File backend yang mengonfirmasi dukungan:

- `backend/cmd/server/main.go`
  - Route `siswaGroup.Get("/kelas/:id/ujian", ujianHandler.ListByKelas)` sudah ada.
  - Route `siswaGroup.Get("/kelas/:id/ujian/hasil", ujianHasilHandler.ListSiswa)` sudah ada.
  - Flow start/items/answer/submit/review siswa juga sudah ada.
- `backend/internal/ujian/handler.go`
  - `ListByKelas` menerima role siswa dan service melakukan branch role.

Jadi untuk perubahan ini mayoritas kerja ada di frontend. Backend tidak perlu route baru untuk versi awal.

## Tujuan UX

Ubah pola akses ujian siswa dari menu global `Ujian saya` menjadi konteks per kelas:

- Siswa buka kelas.
- Di detail kelas, tab berisi `Materi`, `Tugas`, `Pengumuman`, `Chat`, `Ujian`, `Nilai`.
- Ujian yang tampil hanya ujian milik kelas itu.
- Sidebar siswa jadi lebih ringkas dan tidak membuat fitur kelas terasa kepisah-pisah.

## File yang Akan Diubah

- `frontend/app/(authed)/siswa/kelas/detail/page.tsx`
  - Tambah tab `ujian`.
  - Fetch list ujian dan hasil per kelas.
  - Render daftar ujian dalam tab kelas.
- `frontend/app/(authed)/siswa/layout.tsx`
  - Hapus atau turunkan prioritas menu `Ujian saya` dari sidebar.
  - Rekomendasi: hapus dari NAV utama setelah tab ujian kelas siap.
- `frontend/app/(authed)/siswa/ujian/page.tsx`
  - Jangan langsung hapus route.
  - Opsi aman: ubah jadi halaman redirect/landing ringan yang mengarahkan siswa pilih kelas dulu, atau tetap sebagai fallback sementara.
- `frontend/lib/siswa-ujian-api.ts`
  - Kemungkinan tidak perlu perubahan besar karena API client sudah lengkap.
  - Bisa tambah helper kecil kalau dibutuhkan untuk mapping hasil by `ujian_id`.

## Rencana Implementasi

### 1. Discovery Detail Frontend

- Cek struktur `frontend/app/(authed)/siswa/ujian/page.tsx`.
- Ambil logic penting:
  - status ujian: belum mulai, berlangsung, selesai, expired, review tersedia.
  - tombol start/resume/review.
  - empty state dan error state.
- Pastikan link player/review existing tetap dipakai, bukan bikin flow baru.

### 2. Tambah Tab Ujian di Detail Kelas

Di `frontend/app/(authed)/siswa/kelas/detail/page.tsx`:

- Update type tab menjadi mencakup `ujian`.
- Tambah item `Ujian` di `DETAIL_TABS`, icon bisa pakai `GraduationCap` atau `FileQuestion`.
- Urutan rekomendasi:
  - `Materi`
  - `Tugas`
  - `Ujian`
  - `Pengumuman`
  - `Chat`
  - `Nilai`
- Karena jadi 6 tab, mobile bisa pakai grid 2 kolom x 3 baris.
- Hapus special-case `Nilai` full-width kalau sebelumnya ada, supaya layout rata dan tidak berat sebelah.

### 3. Fetch Data Ujian Per Kelas

Di detail kelas:

- Panggil `listSiswaUjianByKelas(kelasID, { limit: 100 })`.
- Panggil `listSiswaUjianHasil(kelasID)`.
- Jalankan fetch saat kelas detail sudah punya `kelasID`.
- Buat map hasil:
  - key: `ujian_id`
  - value: attempt terbaru / berlangsung / selesai terbaik sesuai kebutuhan UI.

Rekomendasi data derivation:

- Jika ada hasil status `berlangsung`, tampilkan CTA `Lanjutkan`.
- Jika tidak ada berlangsung dan attempt masih boleh, tampilkan CTA `Mulai`.
- Jika sudah selesai, tampilkan nilai terakhir/terbaik dan CTA `Review` jika backend mengizinkan.
- Jika ujian belum dibuka atau sudah lewat, tampilkan badge status saja.

### 4. Render UI Tab Ujian

Komponen kecil yang bisa dibuat di file detail kelas dulu:

- `SiswaUjianTab`
  - Props: `kelasID`.
  - Fetch ujian + hasil.
  - Render loading, error, empty, list.
- `SiswaUjianCard`
  - Props: `ujian`, `hasilSummary`.
  - Tampilkan judul, deskripsi, durasi, waktu buka/tutup, status, attempt.
  - CTA ke route existing player/review.

Prinsip UI:

- Ikuti style siswa neo-brutal yang sudah ada.
- Pakai `SiswaCard`, `SiswaBadge`, border tebal, active yellow.
- Mobile first: card stack 1 kolom, CTA full width di mobile.

### 5. Sidebar Siswa

Di `frontend/app/(authed)/siswa/layout.tsx`:

- Hapus item `{ href: '/siswa/ujian', label: 'Ujian saya', ... }` dari `NAV` setelah tab ujian berfungsi.
- Pertahankan `Tugas saya` dan `Nilai saya` dulu kalau masih berfungsi sebagai agregat global.
- Kalau mau super konsisten nanti, `Tugas saya` juga bisa dipindah/deprioritize, tapi jangan dicampur di implementasi ini.

### 6. Fallback Route `/siswa/ujian`

Jangan langsung delete halaman agar link lama/bookmark tidak mati.

Opsi yang direkomendasikan:

- Ubah halaman `/siswa/ujian` jadi landing singkat:
  - Judul: `Ujian sekarang ada di masing-masing kelas`.
  - Tampilkan tombol ke `/siswa`.
  - Kalau memungkinkan, tampilkan daftar kelas dengan shortcut ke `/siswa/kelas/detail?id=...&tab=ujian`.

Opsi minimal:

- Biarkan route lama tetap ada untuk sementara, tapi hilangkan dari sidebar.

Rekomendasi untuk rilis pertama: opsi minimal dulu, biar risiko kecil.

### 7. Validasi

Lokal:

- `cd frontend && npm run typecheck`
- `cd frontend && npm run build`

Remote setelah push/deploy:

- Login siswa di mobile viewport 390px.
- Buka dashboard.
- Buka salah satu kelas.
- Klik tab `Ujian`.
- Pastikan:
  - tab tidak overflow.
  - list ujian tampil sesuai kelas.
  - CTA start/resume/review menuju route existing.
  - sidebar tidak lagi menampilkan `Ujian saya` kalau sudah diputuskan dihapus.

### 8. Commit, Push, Deploy

Setelah validasi lokal:

- Commit contoh: `Move siswa ujian into kelas detail tab`.
- Push ke GitHub `origin main`.
- Push ke remote server repo `server main`.
- Deploy:
  - `ssh -tt rdpkhorur 'cd /home/ubuntu/lms && git pull --ff-only origin main && bash deploy/deploy.sh --remote'`
- Verifikasi output:
  - `readyz OK`
  - `DEPLOY SUCCESS`

## Risiko dan Mitigasi

- Risiko: halaman ujian lama punya logic kompleks yang kalau dicopy penuh bikin file detail kelas terlalu besar.
  - Mitigasi: ekstrak komponen reusable kalau ukurannya mulai membesar.
- Risiko: attempt/review CTA butuh route param berbeda dari yang diasumsikan.
  - Mitigasi: reuse persis link/action dari halaman `/siswa/ujian` existing.
- Risiko: 6 tab di mobile jadi terlalu padat.
  - Mitigasi: pakai grid 2 kolom, text kecil mobile, semua border visible, tanpa horizontal scroll.
- Risiko: sidebar `Ujian saya` dihapus terlalu cepat padahal user masih butuh agregat semua ujian.
  - Mitigasi: route lama tetap ada sebagai fallback; yang dihapus hanya nav utama.

## Rekomendasi Eksekusi

Implementasi frontend bisa lanjut tanpa perubahan backend. Backend sudah siap untuk model per kelas. Jalur paling aman:

1. Tambah tab `Ujian` di detail kelas.
2. Reuse API dan logic dari `/siswa/ujian`.
3. Hilangkan `Ujian saya` dari sidebar utama.
4. Biarkan `/siswa/ujian` tetap hidup sebagai fallback sementara.
