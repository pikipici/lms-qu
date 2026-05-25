# Platform Corrections Backlog

> Catatan koreksi platform dari user. Jangan implementasi dulu sebelum user kasih lampu hijau.

## Context
- Active focus: Fase 8 — Polish + E2E / production-readiness.
- Tujuan file ini: nyimpen koreksi UI/behavior yang akan dikerjakan belakangan secara terstruktur.

## Corrections

### 1. Login page: hapus copy pendaftaran mandiri
- Status: Pending
- Area: `/login`
- Request: Hapus teks `Akun dibuat oleh admin sekolah. Tidak ada pendaftaran mandiri.`
- Expected behavior: Halaman login tidak menampilkan kalimat tersebut lagi.
- Acceptance criteria:
  - Teks tersebut tidak muncul di UI `/login`.
  - Tidak ada perubahan behavior login/auth.
  - Layout login tetap rapi setelah teks dihapus.
- Catatan implementasi nanti:
  - Cari komponen/page login di frontend.
  - Hapus copy saja, jangan ubah auth flow.

### 2. Guru kelas page: card kelas tampilkan jumlah murid
- Status: Pending
- Area: `/guru/kelas`
- Request: Pada UI card kelas, hapus keterangan `Bobot soal` dan `Bobot tugas`; ganti menjadi keterangan jumlah murid saja.
- Expected behavior: Card kelas di `/guru/kelas` fokus menampilkan informasi jumlah murid, bukan bobot penilaian.
- Acceptance criteria:
  - Teks/metadata `Bobot soal` tidak muncul di card kelas `/guru/kelas`.
  - Teks/metadata `Bobot tugas` tidak muncul di card kelas `/guru/kelas`.
  - Card kelas menampilkan jumlah murid dengan label yang jelas.
  - Tidak ada perubahan behavior navigasi/detail kelas.
- Catatan implementasi nanti:
  - Cari komponen list/card kelas guru di frontend.
  - Pastikan data jumlah murid sudah tersedia dari API; kalau belum, catat kebutuhan penyesuaian API sebelum implementasi.

### 3. Profile modal: popup masih transparan
- Status: Pending
- Area: Modal/popup profile pengguna
- Request: Perbaiki bug modal popup profile yang masih transparan.
- Expected behavior: Popup profile punya background solid/opaque yang jelas dan tidak tembus ke konten di belakangnya.
- Acceptance criteria:
  - Modal profile tidak transparan saat dibuka.
  - Konten modal tetap terbaca jelas di desktop dan mobile.
  - Overlay/backdrop tetap berfungsi sesuai design system.
  - Tidak ada regresi pada trigger buka/tutup modal profile.
- Catatan implementasi nanti:
  - Cari komponen profile modal/dropdown/dialog yang dipakai layout admin/guru/siswa.
  - Cek class background dialog content dan z-index/overlay.
  - Pastikan fix konsisten dengan shadcn/ui theme yang sudah dipakai.

### 4. Detail kelas pengaturan: evaluasi lokasi bobot soal/tugas
- Status: Pending / needs decision
- Area: Detail kelas, bagian pengaturan
- Request: Pertanyakan keberadaan setting `Bobot soal` dan `Bobot tugas` di pengaturan detail kelas.
- Expected behavior: Bobot penilaian kemungkinan tidak semestinya berada di level pengaturan kelas; lebih masuk akal berada di setup tugas atau setup ulangan sesuai konteks penilaiannya.
- Acceptance criteria:
  - Ada keputusan produk yang jelas: bobot tetap di kelas, dipindah ke setup tugas/ulangan, atau dihapus dari UI kelas.
  - Jika dipindah, UI detail kelas tidak lagi menampilkan setting `Bobot soal` dan `Bobot tugas`.
  - Jika bobot tetap dibutuhkan, lokasi dan labelnya harus menjelaskan scope penilaian dengan jelas.
  - Tidak ada perubahan formula nilai tanpa keputusan eksplisit.
- Catatan implementasi nanti:
  - Audit dulu pemakaian `Bobot soal` dan `Bobot tugas` di BE/FE sebelum menghapus atau memindahkan.
  - Cek apakah bobot ini dipakai untuk formula NilaiBab atau hanya metadata kelas.
  - Bahas keputusan scope: bobot per kelas, per bab, per tugas, per ulangan, atau fixed formula.

### 5. Form tambah pengumuman: field isi tidak user-friendly
- Status: Pending
- Area: Form tambah/edit pengumuman guru
- Request: Bagian `Isi` pada form tambah pengumuman tidak user-friendly untuk guru yang tidak mengerti format Markdown.
- Expected behavior: Guru bisa menulis isi pengumuman tanpa perlu memahami syntax Markdown.
- Acceptance criteria:
  - Field `Isi` tidak mengandalkan pengetahuan Markdown sebagai satu-satunya cara formatting.
  - Ada UX yang lebih mudah, misalnya rich text editor sederhana, toolbar formatting, preview yang jelas, atau helper yang sangat mudah dipahami.
  - Pengalaman menulis tetap nyaman di desktop dan mobile.
  - Output pengumuman tetap aman dari XSS/sanitization issue.
  - Tidak merusak rendering pengumuman lama yang sudah tersimpan.
- Catatan implementasi nanti:
  - Audit cara pengumuman saat ini disimpan dan dirender: plain text, Markdown, atau HTML sanitized.
  - Pilih pendekatan paling aman: WYSIWYG ringan, Markdown dengan toolbar/preview, atau textarea plain text dengan auto-line-break.
  - Pastikan kompatibel dengan static export Next.js dan tidak menambah dependency berat tanpa alasan kuat.

### 6. Form soal: tambahkan opsi input gambar
- Status: Pending
- Area: Form tambah/edit soal guru
- Request: Form soal perlu ditambahkan opsi input gambar karena saat ini terlihat belum support guru upload/input gambar untuk soal.
- Expected behavior: Guru bisa menambahkan gambar pendukung pada soal dari UI form soal.
- Acceptance criteria:
  - Form tambah/edit soal menyediakan input gambar yang jelas untuk guru.
  - Guru bisa upload atau mengganti gambar soal sesuai batasan file yang berlaku.
  - Preview gambar muncul sebelum/atau setelah disimpan agar guru yakin gambar terpasang.
  - Siswa dapat melihat gambar soal saat mengerjakan latihan/ulangan/ujian sesuai konteks soal.
  - Validasi MIME, ukuran file, dan error upload tampil ramah di UI.
  - Tidak merusak soal lama yang tidak punya gambar.
- Catatan implementasi nanti:
  - Audit dukungan BE yang sudah ada untuk gambar soal/bank soal dan apakah FE belum mengekspos inputnya.
  - Cek flow R2 presigned upload/object key untuk kategori `soal`.
  - Pastikan rendering gambar tersedia di semua player/review yang memakai soal: soal bab, ulangan bab, dan ujian cross-bab.

### 7. Dashboard guru: kartu ringkasan tidak punya tujuan navigasi jelas
- Status: Pending
- Area: Dashboard guru
- Request: Kartu/kolom `Total Kelas Aktif`, `Tugas perlu dinilai`, `Review Ulangan Bab`, dan `Review Ujian` saat dibuka hanya melempar ke `/guru/kelas` tanpa tujuan yang jelas.
- Expected behavior: Setiap kartu ringkasan mengarah ke halaman/daftar yang relevan dengan konteks metriknya, bukan semua diarahkan generik ke `/guru/kelas`.
- Acceptance criteria:
  - `Total Kelas Aktif` mengarah ke daftar kelas aktif atau section yang memang menjelaskan kelas aktif.
  - `Tugas perlu dinilai` mengarah ke daftar tugas/submission yang perlu dinilai, idealnya terfilter pending review.
  - `Review Ulangan Bab` mengarah ke daftar hasil ulangan bab yang perlu review.
  - `Review Ujian` mengarah ke daftar hasil ujian yang perlu review.
  - Jika halaman tujuan belum ada, kartu tidak boleh terasa misleading; tampilkan CTA/empty state yang jelas atau bangun route tujuan yang sesuai.
  - Tidak ada navigasi click-through yang membingungkan guru.
- Catatan implementasi nanti:
  - Audit route yang sudah tersedia untuk review tugas, ulangan bab, dan ujian.
  - Tentukan apakah perlu query params/filter khusus dari dashboard ke halaman tujuan.
  - Jika backend belum menyediakan daftar pending consolidated, catat kebutuhan endpoint sebelum implementasi UI final.

### 8. Master sekolah: asal sekolah dikelola admin dan dipilih guru
- Status: Pending / needs product and data model decision
- Area: Admin sekolah, guru buat kelas, data kelas, filter/laporan terkait
- Request: Tambahkan keterangan `asal sekolah` atau master `sekolah`. Data sekolah hanya bisa diinput/dikelola admin, lalu guru memilih sekolah saat membuat kelas dan fitur terkait lainnya.
- Expected behavior: Sekolah menjadi data master terkontrol admin, bukan input bebas oleh guru.
- Acceptance criteria:
  - Admin dapat membuat, melihat, mengubah, dan menonaktifkan/menghapus data sekolah sesuai aturan yang diputuskan.
  - Guru memilih sekolah dari daftar yang sudah tersedia saat membuat atau mengatur kelas.
  - Kelas menyimpan relasi ke sekolah yang dipilih.
  - UI kelas menampilkan asal sekolah secara jelas jika relevan.
  - Validasi mencegah guru memakai sekolah yang tidak tersedia/tidak aktif.
  - Migrasi data lama punya strategi aman untuk kelas existing yang belum punya sekolah.
- Catatan implementasi nanti:
  - Perlu desain schema, misalnya tabel `sekolah` dan kolom `sekolah_id` di `kelas`.
  - Tentukan apakah sekolah wajib atau opsional untuk kelas existing dan kelas baru.
  - Tentukan dampak ke siswa, enrollment, rekap nilai, CSV, audit log, dan filter admin/guru.
  - Tentukan permission: admin full CRUD sekolah, guru read/select only.

### 9. Semua form Markdown: perbaiki UX input konten
- Status: Pending
- Area: Semua form yang meminta input Markdown
- Request: Form yang memerlukan input Markdown perlu diperbaiki karena tidak user-friendly untuk pengguna non-teknis.
- Expected behavior: Pengguna, terutama guru, bisa mengisi konten berformat tanpa harus memahami syntax Markdown.
- Acceptance criteria:
  - Audit semua field yang saat ini meminta atau menyarankan Markdown.
  - Field Markdown tidak dibiarkan sebagai textarea mentah tanpa bantuan UX.
  - Ada pola input yang konsisten: rich text editor ringan, toolbar Markdown, preview langsung, atau mode plain text dengan auto-format yang jelas.
  - Helper text memakai bahasa sederhana, bukan instruksi teknis yang membebani guru.
  - Rendering konten lama tetap kompatibel dan aman.
  - Sanitization/XSS tetap dijaga jika output berubah ke HTML atau rich text.
- Catatan implementasi nanti:
  - Item ini menggeneralisasi masalah pengumuman di item 5 ke seluruh platform.
  - Cari field seperti `deskripsi`, `isi`, `instruksi`, `materi`, `tugas`, `pengumuman`, dan konten lain yang memakai Markdown.
  - Putuskan satu komponen editor reusable agar UX konsisten dan maintenance ringan.

### 10. Dashboard siswa: tambahkan section Kelas Saya
- Status: Pending
- Area: Dashboard siswa/user murid
- Request: Tambahkan `Kelas Saya` di dashboard user murid.
- Expected behavior: Siswa bisa langsung melihat kelas yang diikuti dari dashboard tanpa harus mencari lewat navigasi lain.
- Acceptance criteria:
  - Dashboard siswa menampilkan section/card/list `Kelas Saya`.
  - Setiap kelas minimal menampilkan nama kelas dan informasi ringkas yang relevan.
  - Setiap item kelas punya navigasi jelas ke detail kelas siswa.
  - Jika siswa belum tergabung kelas apa pun, tampilkan empty state yang jelas dan ramah.
  - Tampilan tetap nyaman di desktop dan mobile.
- Catatan implementasi nanti:
  - Audit endpoint/list kelas siswa yang sudah ada.
  - Reuse card kelas siswa jika sudah tersedia agar visual konsisten.
  - Pertimbangkan menampilkan progress/ringkasan tugas atau aktivitas terbaru kalau datanya sudah tersedia.

### 11. Sidebar responsive: adjustable desktop dan hamburger mobile
- Status: Pending
- Area: Layout sidebar admin/guru/siswa
- Request: Perbaiki sidebar agar bisa adjustable dan gunakan hamburger/sidebar drawer untuk tampilan mobile.
- Expected behavior: Sidebar nyaman dipakai di desktop dan mobile, tidak memakan ruang berlebihan, dan navigasi tetap mudah diakses di layar kecil.
- Acceptance criteria:
  - Di desktop, sidebar bisa di-adjust/collapse sesuai pola yang diputuskan.
  - Di mobile, sidebar tidak tampil permanen memenuhi layar; gunakan tombol hamburger untuk membuka navigation drawer.
  - State buka/tutup jelas dan tidak mengganggu konten utama.
  - Navigation drawer bisa ditutup lewat tombol close, klik backdrop, atau setelah memilih menu.
  - Layout tidak overflow horizontal di mobile.
  - Berlaku konsisten untuk role admin, guru, dan siswa jika mereka memakai shell layout yang sama/serupa.
- Catatan implementasi nanti:
  - Audit komponen layout dan sidebar per role.
  - Tentukan apakah adjustable berarti collapse icon-only, resizable width, atau hide/show toggle.
  - Pastikan aksesibilitas tombol hamburger: label, focus state, keyboard escape, dan aria state.
  - Test minimal di viewport mobile, tablet, dan desktop.
