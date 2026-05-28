# Rekomendasi Admin Kelas + Self-Registration

## Keputusan Utama

Admin harus jadi pemilik master data sekolah dan rombel/kelas resmi. Guru tetap boleh membuat kelas pembelajaran sendiri dan punya akses penuh atas kelas yang dia buat.

Definisi yang disepakati:

- `Rombel / Kelas resmi`: struktur sekolah dari admin, contoh `VII-A`, `VII-B`, `VIII-A`. Dipakai untuk pendaftaran mandiri siswa.
- `Kelas pembelajaran / Course`: kelas operasional guru, contoh `Matematika VII-A Semester 1`. Dipakai untuk materi, tugas, soal, ujian, dan rekap.

Alur yang disarankan:

1. Admin membuat `Sekolah`.
2. Admin membuat `Rombel / Kelas resmi` untuk struktur sekolah.
3. Guru tetap bisa membuat `Kelas pembelajaran` sendiri dan mengelola penuh kelas tersebut.
4. Admin mengaktifkan pendaftaran mandiri siswa per sekolah jika sudah siap.
5. Siswa daftar dari `/register` dan memilih sekolah + rombel/kelas resmi dari data admin.
6. Jika mode `approval_required`, admin/guru approve request siswa.
7. Jika mode `auto_approve`, siswa masuk ke rombel/kelas resmi sesuai aturan onboarding.

## Rekomendasi Lanjutan

### 1. Admin Class Detail

Tambahkan halaman detail kelas khusus admin, misalnya `/admin/kelas/detail?id=<kelas_id>`.

Isi minimal:

- Nama kelas, sekolah, guru pengampu, kode invite, status archive.
- Daftar siswa aktif.
- Request siswa pending untuk kelas itu.
- Aksi edit kelas dan arsipkan.
- Ringkasan jumlah siswa dan aktivitas terakhir.

Tujuan: admin bisa audit kondisi kelas tanpa masuk panel guru.

### 2. Transfer Guru dan Sekolah

Edit kelas admin sebaiknya bisa mengubah:

- `guru_id` untuk pindah guru pengampu.
- `sekolah_id` untuk pindah kelas ke sekolah lain.

Catatan keamanan:

- Validasi `guru_id` harus role `guru`.
- Jika `sekolah_id` berubah, pastikan request siswa pending lama tetap masuk akal atau diberi aturan migrasi.
- Audit log wajib mencatat perubahan guru/sekolah.

### 3. Pending Request Badge

Tambahkan badge angka pending di menu `Request Siswa` untuk admin dan guru.

Tujuan:

- Admin/guru cepat tahu ada siswa menunggu approval.
- Mengurangi risiko request siswa kelamaan tidak diproses.

### 4. Empty State Self-Registration

Jika `/register` tidak punya sekolah yang membuka pendaftaran, tampilkan empty state eksplisit:

> Pendaftaran mandiri belum dibuka. Hubungi admin sekolah.

Jangan hanya dropdown kosong.

### 5. Audit Log Assignment

Tambahkan audit event untuk perubahan kelas:

- `kelas_assigned_guru`
- `kelas_changed_sekolah`
- `kelas_registration_context_changed`

Tujuan: admin punya jejak perubahan jika siswa salah masuk kelas/sekolah.

### 6. Default Aman

Default tetap:

- `siswa_registration_enabled = false`
- `siswa_registration_mode = approval_required`

Auto-approve hanya dipakai jika sekolah sudah yakin data kelas/guru sudah rapi.

## Prioritas Implementasi

1. Empty state `/register` saat belum ada sekolah aktif.
2. Badge pending request di admin/guru nav.
3. Admin class detail.
4. Transfer guru/sekolah di edit kelas.
5. Audit log assignment changes.

## Prinsip Produk

- Admin mengatur struktur organisasi.
- Guru mengatur pembelajaran di kelas yang dipegang.
- Siswa hanya memilih opsi yang sudah disiapkan admin.
- Self-registration boleh ada, tapi akses kelas tetap terkontrol.
