# Platform Activity Test Plan

> Tujuan: checklist testing end-to-end untuk semua aktivitas utama LMS sebelum/selama sesi QA manual dan automation.
>
> Scope: admin, guru, siswa, sistem nilai, file/storage, keamanan, dan operasional.
>
> Status: draft eksekusi. Jalankan bertahap per modul, catat hasil di bagian Log Eksekusi.

---

## 1. Prinsip Testing

- Pakai data dummy khusus QA, jangan data produksi asli.
- Setiap test harus mencatat: akun, role, kelas, sekolah, bab, tugas/ujian yang dipakai.
- Untuk test destructive seperti archive/hapus/reset attempt, pakai objek dummy.
- Untuk bug, catat: langkah reproduksi, expected, actual, screenshot/log, request_id kalau ada.
- Jalankan validasi backend/frontend setelah batch fix:
  - `cd backend && go test ./...`
  - `cd frontend && npm run typecheck`
  - `cd frontend && npm run build`
- Setelah deploy, minimal cek:
  - `/healthz`
  - `/readyz`
  - login admin/guru/siswa
  - satu flow smoke guru + siswa

---

## 2. Data Setup QA

### 2.1 Akun

- Admin QA:
  - email: `admin@sekolah.id` atau akun admin khusus test
- Guru QA:
  - nama: `Guru QA`
  - email: `guru.qa@example.test`
- Siswa QA 1:
  - nama: `Siswa QA Satu`
  - email: `siswa.qa1@example.test`
- Siswa QA 2:
  - nama: `Siswa QA Dua`
  - email: `siswa.qa2@example.test`

### 2.2 Master Data

- Sekolah A: `Sekolah QA A`
- Sekolah B: `Sekolah QA B`
- Kelas A: `Kelas QA Aktif`
- Kelas B: `Kelas QA Arsip Test`
- Bab 1: `Bab QA Materi dan Latihan`
- Bab 2: `Bab QA Nilai`

### 2.3 Konten Minimal

- Materi PDF kecil valid.
- Materi YouTube URL valid.
- Materi markdown pendek.
- 5 soal latihan.
- 5 soal ulangan bab.
- 2 tugas dengan bobot berbeda.
- 1 ujian dengan bobot custom.
- 1 pengumuman kelas.
- 1 pengumuman bab.

---

## 3. Test Admin

### 3.1 Login dan Session Admin

| ID | Skenario | Langkah | Expected |
|---|---|---|---|
| ADM-AUTH-01 | Login admin valid | Login pakai admin | Masuk dashboard admin |
| ADM-AUTH-02 | Logout admin | Klik logout | Token hilang, redirect login |
| ADM-AUTH-03 | Session list | Buka perangkat aktif | Session aktif tampil |
| ADM-AUTH-04 | Logout semua perangkat | Trigger logout all | Session lain revoked |

### 3.2 Kelola Pengguna

| ID | Skenario | Langkah | Expected |
|---|---|---|---|
| ADM-USER-01 | Tambah guru manual password | Buat guru dengan password manual | User guru aktif, must_change_password sesuai aturan |
| ADM-USER-02 | Tambah siswa generate password | Buat siswa generate | Password tampil sekali |
| ADM-USER-03 | Duplicate email | Buat user email sama | Error ramah `email_already_exists` |
| ADM-USER-04 | Reset password user | Reset password guru/siswa | Password baru bisa login, token lama revoked |
| ADM-USER-05 | Suspend user | Suspend siswa | Login ditolak / session invalid sesuai aturan |
| ADM-USER-06 | Unsuspend user | Unsuspend siswa | Bisa login lagi |
| ADM-USER-07 | Lock/unlock user | Simulasikan locked lalu unlock | Status balik active, failed count reset |
| ADM-USER-08 | Ubah role dengan re-auth | Change role user | Butuh password admin valid |
| ADM-USER-09 | Revoke session user | Revoke sessions user | User harus login ulang |

### 3.3 Import CSV Siswa

| ID | Skenario | Langkah | Expected |
|---|---|---|---|
| ADM-CSV-01 | Upload CSV valid | Upload CSV siswa valid | Preview valid |
| ADM-CSV-02 | CSV invalid row | Upload email invalid/kolom kurang | Row invalid tampil dengan alasan |
| ADM-CSV-03 | Confirm import | Confirm preview valid | User dibuat, credentials CSV tersedia |
| ADM-CSV-04 | Download credentials | Klik download | File CSV terunduh |
| ADM-CSV-05 | Duplicate email import | Import email existing | Error/row invalid jelas |

### 3.4 Master Sekolah

| ID | Skenario | Langkah | Expected |
|---|---|---|---|
| ADM-SCH-01 | Tambah sekolah | Isi nama/NPSN/alamat | Sekolah tampil di list |
| ADM-SCH-02 | Edit sekolah | Ubah nama/alamat | Data terupdate |
| ADM-SCH-03 | Hapus sekolah kosong | Hapus sekolah tanpa kelas | Berhasil atau error sesuai constraint |
| ADM-SCH-04 | Cari sekolah | Search nama/NPSN | Hasil terfilter |
| ADM-SCH-05 | Jumlah kelas aktif | Buat kelas di sekolah | Row sekolah tampil `X kelas aktif` |
| ADM-SCH-06 | Kelas archived tidak dihitung | Archive kelas terkait | Count aktif berkurang |

### 3.5 Audit dan Login Attempts

| ID | Skenario | Langkah | Expected |
|---|---|---|---|
| ADM-AUD-01 | Audit user action | Reset password/suspend | Audit log masuk |
| ADM-AUD-02 | Filter audit | Filter actor/target/action | Data sesuai filter |
| ADM-LOG-01 | Login gagal | Salah password beberapa kali | Login attempt tercatat |
| ADM-LOG-02 | Login sukses | Login valid | Success attempt tercatat |

---

## 4. Test Guru

### 4.1 Login Guru dan Shell

| ID | Skenario | Langkah | Expected |
|---|---|---|---|
| G-AUTH-01 | Login guru | Login guru valid | Masuk dashboard guru |
| G-AUTH-02 | Role guard | Guru akses `/admin` | Ditolak/redirect |
| G-AUTH-03 | Force change password | Login user must_change_password | Dipaksa ke security page |

### 4.2 Kelas

| ID | Skenario | Langkah | Expected |
|---|---|---|---|
| G-KLS-01 | Buat kelas dengan sekolah | Pilih sekolah, isi nama | Kelas dibuat, sekolah tampil di card |
| G-KLS-02 | Buat kelas tanpa sekolah | Kosongkan sekolah | Kelas dibuat sebagai `Tanpa sekolah` |
| G-KLS-03 | Filter kelas by sekolah | Pilih filter sekolah | Hanya kelas sekolah tersebut tampil |
| G-KLS-04 | Copy kode invite | Klik copy kode | Clipboard berisi kode |
| G-KLS-05 | Edit kelas | Ubah nama/deskripsi | Data berubah, version bump |
| G-KLS-06 | Bobot kelas tidak muncul | Buka create/edit/detail kelas | Tidak ada input/keterangan bobot kelas |
| G-KLS-07 | Duplicate kelas | Duplicate kelas | Kelas baru dibuat, kode invite baru |
| G-KLS-08 | Archive kelas | Archive kelas dummy | Hilang dari list aktif, muncul jika include archived |
| G-KLS-09 | Kelas archived di siswa | Siswa cek dashboard | Kelas archived tidak muncul |

### 4.3 Enrollment Siswa

| ID | Skenario | Langkah | Expected |
|---|---|---|---|
| G-ENR-01 | Lihat siswa kelas | Siswa join, guru buka tab siswa | Siswa tampil |
| G-ENR-02 | Cross-guru guard | Guru lain akses kelas | Forbidden/not found |

### 4.4 Bab

| ID | Skenario | Langkah | Expected |
|---|---|---|---|
| G-BAB-01 | Buat bab draft | Isi judul/deskripsi | Bab draft tampil guru |
| G-BAB-02 | Publish bab | Ubah status published | Siswa bisa lihat bab |
| G-BAB-03 | Archive bab | Archive bab | Siswa tidak melihat bab |
| G-BAB-04 | Reorder bab | Drag/drop urutan | Urutan tersimpan |
| G-BAB-05 | Duplicate bab | Duplicate bab | Bab baru tercopy sesuai scope |
| G-BAB-06 | Version conflict | Simulasikan stale version | Error 409 ramah |

### 4.5 Materi

| ID | Skenario | Langkah | Expected |
|---|---|---|---|
| G-MAT-01 | Upload PDF valid | Upload PDF <20MB | Materi tersimpan, bisa dibuka siswa |
| G-MAT-02 | Upload PDF invalid | Upload non-PDF | Error unsupported mime |
| G-MAT-03 | Tambah YouTube | Input URL valid | Embed tampil siswa |
| G-MAT-04 | YouTube invalid | Input URL invalid | Validasi gagal |
| G-MAT-05 | Tambah markdown | Isi markdown | Render siswa benar |
| G-MAT-06 | Edit materi | Ubah judul/konten | Data berubah |
| G-MAT-07 | Hapus materi | Delete materi dummy | Hilang, R2 cleanup sesuai log |

### 4.6 Pengumuman

| ID | Skenario | Langkah | Expected |
|---|---|---|---|
| G-PNG-01 | Buat pengumuman kelas | Isi judul/body | Siswa kelas melihat |
| G-PNG-02 | Buat pengumuman bab | Attach ke bab | Tampil di konteks bab |
| G-PNG-03 | Edit pengumuman | Ubah body | Update tampil |
| G-PNG-04 | Archive/aktifkan | Archive lalu aktifkan | Visibility berubah |
| G-PNG-05 | Hapus pengumuman | Delete dummy | Hilang |

### 4.7 Soal Bab dan Latihan

| ID | Skenario | Langkah | Expected |
|---|---|---|---|
| G-SOAL-01 | Buat soal latihan | Isi opsi/jawaban/poin | Soal tersimpan |
| G-SOAL-02 | Bulk paste valid | Paste format pipe | Soal dibuat |
| G-SOAL-03 | Bulk paste invalid | Kolom kurang | Error per baris |
| G-SOAL-04 | Preview soal | Buka preview | Tampilan sesuai soal |
| G-SOAL-05 | Mode keduanya | Set mode keduanya | Muncul di latihan dan ulangan |

### 4.8 Ulangan Bab

| ID | Skenario | Langkah | Expected |
|---|---|---|---|
| G-ULG-01 | Atur ulangan bab | Publish setting ulangan | Siswa bisa mulai |
| G-ULG-02 | Review locked | IzinkanReview=false | Siswa tidak bisa lihat pembahasan |
| G-ULG-03 | Review open | IzinkanReview=true | Siswa bisa lihat pembahasan |
| G-ULG-04 | Reset attempt | Reset hasil siswa | Siswa bisa mulai ulang |
| G-ULG-05 | Hasil ulangan | Siswa submit | Guru melihat hasil |

### 4.9 Tugas

| ID | Skenario | Langkah | Expected |
|---|---|---|---|
| G-TGS-01 | Buat tugas | Isi judul/deadline | Tugas tampil siswa |
| G-TGS-02 | Set bobot tugas | Bobot 100/50/0 | Tersimpan dan tampil di UI nilai |
| G-TGS-03 | Edit tugas | Ubah bobot/deskripsi | Update tersimpan |
| G-TGS-04 | Archive tugas | Archive dummy | Tidak tampil aktif |
| G-TGS-05 | Nilai submission | Isi nilai/feedback | Siswa melihat nilai/feedback |
| G-TGS-06 | Late penalty | Set late + penalty | Nilai setelah penalty sesuai |
| G-TGS-07 | Bobot 0 exclude | Tugas bobot 0 dinilai | Tidak mempengaruhi rata-rata tugas |

### 4.10 Bank Soal dan Ujian

| ID | Skenario | Langkah | Expected |
|---|---|---|---|
| G-BANK-01 | Buat soal bank | Isi soal MCQ | Tersimpan di bank guru |
| G-BANK-02 | Filter bank soal | Filter mapel/topik | Data sesuai |
| G-UJIAN-01 | Buat ujian manual | Pilih soal manual | Ujian dibuat |
| G-UJIAN-02 | Buat ujian random | Source random dari bank | Pool soal terbentuk saat siswa mulai |
| G-UJIAN-03 | Set bobot ujian | Isi bobot | Bobot tampil di list/rekap |
| G-UJIAN-04 | Timer ujian | Set durasi pendek | Auto submit/grade saat expired |
| G-UJIAN-05 | Reset attempt ujian | Reset siswa | Siswa bisa mengulang |

### 4.11 Rekap Nilai Guru

| ID | Skenario | Langkah | Expected |
|---|---|---|---|
| G-NILAI-01 | Matrix rekap | Buka rekap kelas | Matrix siswa x bab/ujian tampil |
| G-NILAI-02 | Export CSV | Klik export | CSV terunduh dan isi sesuai |
| G-NILAI-03 | Bobot item tugas | Dua tugas bobot beda | Nilai tugas bab weighted benar |
| G-NILAI-04 | Bobot kelas deprecated | Ubah legacy bobot via API jika mungkin | Tidak mempengaruhi NilaiBab |
| G-NILAI-05 | NULL skip | Belum ada tugas/ulangan | Komponen kosong di-skip, total sesuai |

---

## 5. Test Siswa

### 5.1 Login dan Join Kelas

| ID | Skenario | Langkah | Expected |
|---|---|---|---|
| S-AUTH-01 | Login siswa | Login valid | Masuk dashboard siswa |
| S-AUTH-02 | Role guard | Siswa akses `/guru`/`/admin` | Ditolak/redirect |
| S-JOIN-01 | Join kode valid | Input kode invite | Kelas tampil dashboard |
| S-JOIN-02 | Join kode lowercase | Input lowercase | Auto uppercase/berhasil |
| S-JOIN-03 | Join kode invalid | Input salah | Error ramah |
| S-JOIN-04 | Join kelas archived | Input kode kelas archived | Ditolak |
| S-JOIN-05 | Join ulang | Input kode yang sama | Idempotent/tidak duplikat |

### 5.2 Kelas, Bab, Materi, Progress

| ID | Skenario | Langkah | Expected |
|---|---|---|---|
| S-KLS-01 | Lihat kelas aktif | Dashboard siswa | Kelas aktif tampil |
| S-KLS-02 | Kelas archived hilang | Guru archive kelas | Kelas tidak tampil |
| S-BAB-01 | Lihat bab published | Buka detail kelas | Hanya bab published tampil |
| S-MAT-01 | Buka PDF | Klik materi PDF | PDF tampil, mark read setelah delay |
| S-MAT-02 | Buka YouTube | Klik materi YouTube | Embed tampil, mark read |
| S-MAT-03 | Baca markdown | Klik materi markdown | Render benar, mark read |
| S-PROG-01 | Progress materi | Baca sebagian materi | Progress naik sesuai jumlah read |
| S-PROG-02 | Bab kosong | Bab tanpa materi | Progress 0/kosong jelas |

### 5.3 Pengumuman

| ID | Skenario | Langkah | Expected |
|---|---|---|---|
| S-PNG-01 | Lihat pengumuman kelas | Guru publish pengumuman | Tampil di siswa |
| S-PNG-02 | Lihat pengumuman bab | Buka bab | Pengumuman bab tampil |
| S-PNG-03 | Pengumuman archived | Guru archive | Tidak tampil siswa |

### 5.4 Latihan

| ID | Skenario | Langkah | Expected |
|---|---|---|---|
| S-LAT-01 | Mulai latihan | Klik latihan | Soal tampil |
| S-LAT-02 | Submit latihan | Jawab semua | Skor/review tampil |
| S-LAT-03 | Retry latihan | Ulangi latihan | Bisa retry unlimited |
| S-LAT-04 | Latihan tidak masuk nilai | Cek nilai | Nilai bab tidak berubah karena latihan |

### 5.5 Ulangan Bab

| ID | Skenario | Langkah | Expected |
|---|---|---|---|
| S-ULG-01 | Mulai ulangan | Klik mulai | Attempt dibuat, timer jalan |
| S-ULG-02 | Save jawaban | Pilih jawaban | Jawaban tersimpan |
| S-ULG-03 | Resume | Reload/login ulang | Bisa lanjut attempt |
| S-ULG-04 | Submit | Submit ulangan | Status selesai/submitted, skor tampil |
| S-ULG-05 | Auto expire | Biarkan timer habis | Auto-grade/expired |
| S-ULG-06 | Review locked | Cek pembahasan locked | Tidak bisa akses |
| S-ULG-07 | Review open | Guru buka review | Pembahasan tampil |

### 5.6 Tugas

| ID | Skenario | Langkah | Expected |
|---|---|---|---|
| S-TGS-01 | Lihat tugas | Buka tugas kelas/bab | Tugas aktif tampil |
| S-TGS-02 | Submit teks | Isi catatan | Submission submitted |
| S-TGS-03 | Submit file | Upload attachment valid | File tersimpan |
| S-TGS-04 | Resubmit sebelum graded | Submit ulang | Konten/file terganti |
| S-TGS-05 | Resubmit setelah graded | Coba resubmit | Ditolak sesuai aturan |
| S-TGS-06 | Deadline late ditolak | Submit setelah deadline no late | Ditolak |
| S-TGS-07 | Deadline late allowed | Submit telat allowed | Flag late dan penalty berlaku |
| S-TGS-08 | Lihat feedback | Guru grade | Nilai/feedback tampil |

### 5.7 Ujian

| ID | Skenario | Langkah | Expected |
|---|---|---|---|
| S-UJIAN-01 | Lihat ujian aktif | Buka halaman ujian | Ujian tampil |
| S-UJIAN-02 | Mulai ujian manual | Klik mulai | Soal manual tampil |
| S-UJIAN-03 | Mulai ujian random | Klik mulai random | Pool deterministic per attempt |
| S-UJIAN-04 | Resume ujian | Reload/login ulang | Attempt lanjut |
| S-UJIAN-05 | Submit ujian | Jawab dan submit | Skor tampil |
| S-UJIAN-06 | Timer expire ujian | Biarkan habis | Auto-grade |
| S-UJIAN-07 | Review ujian | Sesuai setting guru | Locked/open sesuai setting |

### 5.8 Nilai Siswa

| ID | Skenario | Langkah | Expected |
|---|---|---|---|
| S-NILAI-01 | Lihat nilai kelas | Buka nilai kelas | Nilai bab/tugas/ulangan tampil |
| S-NILAI-02 | Lihat nilai lintas kelas | Buka `/siswa/nilai` | Ringkasan lintas kelas tampil |
| S-NILAI-03 | Bobot item tampil | Ada tugas/ujian bobot custom | Label bobot tampil |
| S-NILAI-04 | Bobot kelas tidak tampil | Buka nilai siswa | Tidak ada bobot kelas |
| S-NILAI-05 | NULL komponen | Belum ada nilai tugas | UI tampil `-`/skip jelas |

---

## 6. Test Formula Nilai

### 6.1 Tugas Weighted Average

| ID | Setup | Expected |
|---|---|---|
| VAL-01 | Tugas A nilai 80 bobot 100, Tugas B nilai 100 bobot 100 | NilaiTugasBab = 90 |
| VAL-02 | Tugas A nilai 80 bobot 100, Tugas B nilai 100 bobot 50 | NilaiTugasBab = 86.67 |
| VAL-03 | Tugas A nilai 80 bobot 100, Tugas B nilai 100 bobot 0 | NilaiTugasBab = 80 |
| VAL-04 | Semua tugas bobot 0 | NilaiTugasBab = NULL / tidak masuk total |

### 6.2 NilaiBab Equal Component

| ID | Setup | Expected |
|---|---|---|
| VAL-05 | NilaiUlanganBab 80, NilaiTugasBab 90 | NilaiBab = 85 |
| VAL-06 | NilaiUlanganBab 80, NilaiTugasBab NULL | NilaiBab = 80 |
| VAL-07 | NilaiUlanganBab NULL, NilaiTugasBab 90 | NilaiBab = 90 |
| VAL-08 | Dua komponen NULL | NilaiBab = NULL |
| VAL-09 | Legacy bobot kelas 90/10 | NilaiBab tetap equal, bukan 81 |

### 6.3 Ujian Bobot

| ID | Setup | Expected |
|---|---|---|
| VAL-10 | Ujian bobot 100 | Rekap menampilkan bobot 100 |
| VAL-11 | Ujian bobot 50 | Rekap/list menampilkan bobot 50 |
| VAL-12 | Ujian bobot 0 | Tidak mempengaruhi agregat yang weighted by ujian jika ada |

---

## 7. Test Keamanan dan Guard

| ID | Skenario | Langkah | Expected |
|---|---|---|---|
| SEC-01 | Tanpa token | Hit endpoint protected | 401 |
| SEC-02 | Role salah | Siswa hit endpoint guru | 403 |
| SEC-03 | Cross-guru kelas | Guru A akses kelas Guru B | 403/404 |
| SEC-04 | Enrollment guard | Siswa akses kelas belum join | Forbidden/not found |
| SEC-05 | Login rate limit | Gagal login berulang | 429/lock sesuai policy |
| SEC-06 | Request ID | Trigger error API | Response punya request_id |
| SEC-07 | Refresh rotation | Pakai refresh token lama | Ditolak/reuse handling |
| SEC-08 | Force password change | MustChangePassword true | Endpoint selain allowed ditolak |

---

## 8. Test File Storage / R2

| ID | Skenario | Langkah | Expected |
|---|---|---|---|
| R2-01 | Upload PDF materi | Upload file valid | Object tersimpan, URL bisa dibuka |
| R2-02 | Presigned URL expired | Tunggu TTL/ubah URL | Tidak bisa akses setelah expired |
| R2-03 | Delete materi | Delete materi PDF | Object cleanup best-effort |
| R2-04 | Submit attachment | Upload tugas file | Object tersimpan |
| R2-05 | Resubmit attachment | Resubmit file baru | File lama cleanup sesuai flow |
| R2-06 | Import CSV credentials cleanup | Setelah TTL cleanup | Credentials object hilang sesuai rule |

---

## 9. Test Operasional

| ID | Skenario | Langkah | Expected |
|---|---|---|---|
| OPS-01 | healthz | GET `/healthz` | 200 OK |
| OPS-02 | readyz | GET `/readyz` | 200 OK dan dependency siap |
| OPS-03 | Deploy no migration | Deploy tanpa migration baru | `no change`, service restart sukses |
| OPS-04 | Deploy dengan migration | Tambah migration di test env | Migration jalan sekali |
| OPS-05 | Backup runbook | Jalankan backup manual | File backup valid |
| OPS-06 | Restore drill | Restore ke disposable DB | Restore sukses |
| OPS-07 | Cleanup dry-run | Jalankan cleanup dry-run | Hanya count, tidak delete |
| OPS-08 | E2E smoke | Run Playwright smoke | Login smoke PASS |

---

## 10. Urutan Eksekusi Rekomendasi

1. Smoke auth admin/guru/siswa.
2. Admin Master Sekolah + create guru/siswa.
3. Guru create kelas + pilih sekolah + siswa join.
4. Guru create bab + publish + siswa lihat.
5. Materi/progress.
6. Pengumuman.
7. Soal latihan + siswa latihan.
8. Ulangan bab + review/reset.
9. Tugas + submission + grading + bobot tugas.
10. Bank soal + ujian + bobot ujian.
11. Rekap nilai guru + export CSV.
12. Nilai siswa + formula validation.
13. Archive/hapus kelas dan cek sisi siswa.
14. Security/guard/rate limit.
15. R2 cleanup/manual checks.
16. Deploy/operational checks.

---

## 11. Log Eksekusi

| Tanggal | Tester | Area | Hasil | Catatan/Bug |
|---|---|---|---|---|
| 2026-05-25 | Apis | Batch 1 smoke/setup QA | PASS | Admin login OK; Sekolah QA A ready; Guru/Siswa QA reset/ready; Kelas QA Aktif ready; siswa join idempotent; siswa class visible; guru school filter returns sekolah_nama. Found and fixed backend `sekolah_nama` mapping bug in commit `4ea7a40`. |
| 2026-05-25 | Apis | Batch 2 konten dasar | PASS | Bab QA Materi dan Latihan created/published; markdown + YouTube materi created; pengumuman kelas + bab created; siswa sees published bab; materi count=2; mark-read idempotent flow works; progress went 0 -> 100; siswa pengumuman endpoint `/siswa/kelas/:id/pengumuman` shows kelas/bab announcements. Note: `/kelas/:id/pengumuman` correctly 403 for siswa; use siswa alias. |
| 2026-05-25 | Apis | Batch 3 soal latihan | PASS | QA passwords normalized to `QaPass123!`; guru/siswa login OK; existing `Kelas QA Aktif` + `Bab QA Materi dan Latihan` reused; 5 soal mode `keduanya` ready; invalid bulk paste returns 400 `rows_required`; siswa starts latihan, answers 5 soal, immediate feedback includes `is_benar` + `jawaban_benar`; finish returns summary benar=5/total=5; retry after finish creates new attempt. |

---

## 12. Bug Template

```md
### BUG-ID

- Area:
- Role:
- Akun:
- Data test:
- Langkah reproduksi:
  1.
  2.
  3.
- Expected:
- Actual:
- Request ID:
- Screenshot/log:
- Severity: blocker/high/medium/low
```
