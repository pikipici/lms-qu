# LMS Chat Implementation Plan

> Rencana implementasi fitur chat LMS berdasarkan pola fitur chat `premium-hub`, tapi disesuaikan untuk LMS: siswa berinteraksi dengan guru kelas, admin sebagai monitor.

## Tujuan

Membuat fitur chat text-only agar siswa bisa bertanya kepada guru pengampu dalam konteks kelas. Admin dapat memonitor semua percakapan untuk kebutuhan support dan pengawasan.

## Rekomendasi Scope v1

- Chat berbasis kelas: satu conversation untuk kombinasi `kelas_id + siswa_id`.
- Penerima utama: guru pengampu kelas.
- Admin: monitor semua conversation dan boleh membalas sebagai admin.
- Siswa: hanya bisa mengakses chat miliknya dalam kelas yang aktif/enrolled.
- Guru: hanya bisa mengakses chat siswa di kelas yang dia ampu.
- Text-only dulu, tanpa attachment/gambar.
- REST + polling dulu, WebSocket ditunda ke v2.
- Conversation status: `open`, `closed`.
- Jika siswa mengirim pesan ke conversation `closed`, conversation otomatis `open` lagi.

## Non-Goals v1

- Tidak ada attachment/file upload.
- Tidak ada WebSocket/realtime push.
- Tidak ada DM siswa ke siswa.
- Tidak ada multi-guru routing kompleks.
- Tidak ada typing indicator/read receipt per-message detail.
- Tidak ada moderation otomatis.

## UX Flow

### Siswa

Route rekomendasi:

```text
/siswa/kelas/detail/chat?id=<kelas_id>
```

Flow:

1. Siswa membuka detail kelas.
2. Ada menu/tab `Chat Guru`.
3. Sistem memuat atau membuat conversation untuk `kelas_id + siswa_id`.
4. Siswa melihat riwayat pesan.
5. Siswa mengirim pesan text.
6. Jika conversation sebelumnya `closed`, status otomatis menjadi `open`.
7. Halaman polling messages tiap 10 detik saat terbuka.

UI copy:

- Empty state: `Tanyakan materi, tugas, ulangan, atau nilai ke guru kelas ini.`
- Closed state: `Percakapan ini sudah ditutup. Kirim pesan baru untuk membuka kembali.`

### Guru

Route rekomendasi:

```text
/guru/kelas/detail/chat?id=<kelas_id>
```

Flow:

1. Guru membuka detail kelas.
2. Ada menu/tab `Chat Siswa`.
3. Guru melihat inbox conversation untuk kelas tersebut.
4. Guru memilih conversation siswa.
5. Guru membaca dan membalas pesan.
6. Guru bisa tandai dibaca.
7. Guru bisa menutup conversation.
8. Inbox polling tiap 15-30 detik.

Filter v1:

- Semua
- Belum dibaca
- Terbuka
- Ditutup

### Admin

Route rekomendasi:

```text
/admin/chat
```

Flow:

1. Admin melihat semua conversation.
2. Filter berdasarkan sekolah/kelas/guru/status/search siswa.
3. Admin bisa membuka detail conversation.
4. Admin boleh membalas sebagai `admin`.
5. Admin boleh menutup conversation.

Admin bisa ditunda setelah siswa+guru stabil, tapi schema/API sebaiknya sudah mendukung admin dari awal.

## Data Model

### Migration: `chat_conversations`

Kolom rekomendasi:

```sql
CREATE TABLE chat_conversations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kelas_id UUID NOT NULL REFERENCES kelas(id),
    siswa_id UUID NOT NULL REFERENCES users(id),
    guru_id UUID NOT NULL REFERENCES users(id),
    status TEXT NOT NULL DEFAULT 'open',
    last_message_at TIMESTAMPTZ,
    last_message_preview TEXT NOT NULL DEFAULT '',
    siswa_unread_count INTEGER NOT NULL DEFAULT 0,
    guru_unread_count INTEGER NOT NULL DEFAULT 0,
    admin_unread_count INTEGER NOT NULL DEFAULT 0,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT chat_conversations_status_check CHECK (status IN ('open', 'closed')),
    CONSTRAINT chat_conversations_unique_active UNIQUE (kelas_id, siswa_id)
);
```

Catatan:

- `guru_id` disimpan sebagai snapshot guru pengampu saat conversation dibuat.
- Jika kelas berpindah guru, service bisa update `guru_id` saat conversation dimuat, atau tetap pakai snapshot. Rekomendasi v1: update saat akses jika guru kelas berubah.
- Jika project menggunakan soft-delete partial unique index, gunakan unique index `WHERE deleted_at IS NULL` daripada constraint biasa.

Index:

```sql
CREATE INDEX idx_chat_conversations_guru_status_last ON chat_conversations (guru_id, status, last_message_at DESC);
CREATE INDEX idx_chat_conversations_kelas_last ON chat_conversations (kelas_id, last_message_at DESC);
CREATE INDEX idx_chat_conversations_siswa_last ON chat_conversations (siswa_id, last_message_at DESC);
CREATE INDEX idx_chat_conversations_deleted_at ON chat_conversations (deleted_at);
```

### Migration: `chat_messages`

Kolom rekomendasi:

```sql
CREATE TABLE chat_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES chat_conversations(id),
    sender_id UUID NOT NULL REFERENCES users(id),
    sender_role TEXT NOT NULL,
    body TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT chat_messages_sender_role_check CHECK (sender_role IN ('siswa', 'guru', 'admin')),
    CONSTRAINT chat_messages_body_len_check CHECK (char_length(body) BETWEEN 1 AND 4000)
);
```

Index:

```sql
CREATE INDEX idx_chat_messages_conversation_created ON chat_messages (conversation_id, created_at ASC);
CREATE INDEX idx_chat_messages_deleted_at ON chat_messages (deleted_at);
```

## Backend Package

Rekomendasi package:

```text
backend/internal/chat/
  model.go
  repo.go
  service.go
  handler.go
  dto.go
  service_test.go
  handler_test.go
```

## Backend API

### Siswa API

```text
GET /api/v1/siswa/kelas/:kelas_id/chat?limit=80
POST /api/v1/siswa/kelas/:kelas_id/chat/messages
POST /api/v1/siswa/kelas/:kelas_id/chat/read
```

`GET` response:

```json
{
  "data": {
    "conversation": {
      "id": "uuid",
      "kelas_id": "uuid",
      "siswa_id": "uuid",
      "guru_id": "uuid",
      "status": "open",
      "last_message_at": "...",
      "siswa_unread_count": 0,
      "guru_unread_count": 2
    },
    "messages": [
      {
        "id": "uuid",
        "conversation_id": "uuid",
        "sender_id": "uuid",
        "sender_role": "siswa",
        "sender_name": "Nama Siswa",
        "body": "Pak, tugasnya dikumpulkan kapan?",
        "created_at": "..."
      }
    ]
  }
}
```

`POST messages` body:

```json
{
  "body": "Pak, tugasnya dikumpulkan kapan?"
}
```

### Guru API

```text
GET /api/v1/guru/kelas/:kelas_id/chat/conversations?status=&unread=&search=&limit=20&offset=0
GET /api/v1/guru/kelas/:kelas_id/chat/conversations/:conversation_id/messages?limit=80
POST /api/v1/guru/kelas/:kelas_id/chat/conversations/:conversation_id/messages
POST /api/v1/guru/kelas/:kelas_id/chat/conversations/:conversation_id/read
PATCH /api/v1/guru/kelas/:kelas_id/chat/conversations/:conversation_id/status
```

Status body:

```json
{
  "status": "closed",
  "version": 3
}
```

### Admin API

```text
GET /api/v1/admin/chat/conversations?sekolah_id=&kelas_id=&guru_id=&status=&unread=&search=&limit=20&offset=0
GET /api/v1/admin/chat/conversations/:conversation_id/messages?limit=80
POST /api/v1/admin/chat/conversations/:conversation_id/messages
POST /api/v1/admin/chat/conversations/:conversation_id/read
PATCH /api/v1/admin/chat/conversations/:conversation_id/status
```

Admin route bisa diimplement setelah siswa+guru, tapi service/repo sebaiknya sudah siap.

## Authorization Rules

### Siswa

- `kelas_id` harus valid.
- User role harus `siswa`.
- Siswa harus enrolled aktif di kelas tersebut.
- Siswa hanya boleh membaca/mengirim ke conversation dengan `siswa_id = current_user_id`.

### Guru

- User role harus `guru`.
- Guru harus pengampu/owner kelas tersebut.
- Guru hanya boleh melihat conversation dengan `kelas_id` miliknya.

### Admin

- User role harus `admin`.
- Admin boleh melihat semua conversation.
- Admin balasan disimpan sebagai `sender_role='admin'`.

## Service Logic

### GetOrCreateConversationForSiswa

1. Validasi siswa enrolled aktif di kelas.
2. Cari conversation aktif berdasarkan `kelas_id + siswa_id`.
3. Jika belum ada, buat conversation dengan `guru_id` dari kelas.
4. Jika ada tapi `guru_id` tidak sama dengan guru kelas saat ini, update `guru_id`.
5. Return conversation + messages terbaru.

### SendMessage

1. Trim body.
2. Validasi panjang 1-4000 karakter.
3. Validasi authorization sesuai role.
4. Transaction:
   - Insert `chat_messages`.
   - Update `chat_conversations`:
     - `status='open'` jika sender siswa.
     - `last_message_at=now()`.
     - `last_message_preview` = potongan body, misalnya 160 char.
     - increment unread counter sisi penerima:
       - sender siswa: `guru_unread_count += 1`, `admin_unread_count += 1` optional.
       - sender guru/admin: `siswa_unread_count += 1`.
     - `version += 1`.
5. Return message.

### MarkRead

- Siswa membuka chat: set `siswa_unread_count=0`.
- Guru membuka chat: set `guru_unread_count=0`.
- Admin membuka chat: set `admin_unread_count=0`.

### SetStatus

- Hanya guru/admin.
- Status valid: `open`, `closed`.
- Gunakan optimistic concurrency `version`.
- Return updated conversation.

## Frontend Implementation

### Shared API Client

File rekomendasi:

```text
frontend/lib/chat-api.ts
```

Fungsi:

- `getSiswaKelasChat(kelasId)`
- `sendSiswaKelasChatMessage(kelasId, body)`
- `markSiswaKelasChatRead(kelasId)`
- `listGuruKelasChatConversations(kelasId, filters)`
- `getGuruKelasChatMessages(kelasId, conversationId)`
- `sendGuruKelasChatMessage(kelasId, conversationId, body)`
- `markGuruKelasChatRead(kelasId, conversationId)`
- `setGuruKelasChatStatus(kelasId, conversationId, status, version)`
- admin equivalents jika dikerjakan.

### Siswa UI

File rekomendasi:

```text
frontend/app/(authed)/siswa/kelas/detail/chat/page.tsx
frontend/components/siswa/SiswaKelasChat.tsx
```

UI:

- Pakai `.siswa-theme` dan primitives `@/components/siswa-ui`.
- Message bubble:
  - pesan siswa align kanan.
  - pesan guru/admin align kiri.
  - label sender role jelas.
- Polling messages tiap 10 detik saat tab aktif.
- Disable send saat body kosong atau conversation loading.
- Jika status closed, tampilkan notice tapi tetap izinkan siswa kirim untuk reopen.

### Guru UI

File rekomendasi:

```text
frontend/app/(authed)/guru/kelas/detail/chat/page.tsx
frontend/components/guru/GuruKelasChatInbox.tsx
```

UI:

- Neutral shadcn, jangan pakai siswa theme.
- Split layout desktop: inbox kiri, conversation kanan.
- Mobile: inbox dan detail bisa satu kolom.
- Polling inbox tiap 15-30 detik.
- Badge unread di conversation row.
- Tombol `Tutup percakapan` / `Buka kembali`.

### Admin UI

File rekomendasi:

```text
frontend/app/(authed)/admin/chat/page.tsx
frontend/components/admin/AdminChatInbox.tsx
```

UI:

- Filter sekolah/kelas/guru/status/search.
- Detail conversation + reply sebagai admin.
- Bisa dikerjakan setelah siswa+guru.

## Navigation

Siswa:

- Tambah link `Chat Guru` pada detail kelas atau action card.
- Optional tambah badge unread di dashboard kelas.

Guru:

- Tambah tab/action `Chat siswa` di detail kelas.
- Optional tambah dashboard card `Pesan belum dibaca`.

Admin:

- Tambah sidebar menu `Chat` saat admin UI siap.

## Testing Plan

### Backend Unit/Handler Tests

Minimal test:

- Siswa enrolled bisa get/create conversation.
- Siswa non-enrolled ditolak 403/404.
- Siswa tidak bisa akses conversation siswa lain.
- Guru owner kelas bisa list conversation.
- Guru non-owner ditolak.
- Admin bisa list all.
- Send message trim body.
- Empty body ditolak 400.
- Body >4000 ditolak 400.
- Siswa send ke closed conversation auto-open.
- Unread counters naik sesuai sender.
- Mark read reset counter sesuai role.
- Set status pakai version dan conflict ditangani.

### Frontend Checks

- `npm run typecheck`.
- Siswa page render empty state dan send message.
- Guru inbox render list dan reply.
- Admin page jika dikerjakan.

### Remote Validation

Jalankan di `rdpkhorur`:

```text
cd /home/ubuntu/lms
set -a; . ./.env; set +a
bash deploy/deploy.sh --remote
curl -fsS http://127.0.0.1:8200/api/v1/readyz
```

Smoke manual/API:

1. Login siswa enrolled.
2. `GET /siswa/kelas/:id/chat` menghasilkan conversation.
3. Siswa kirim pesan.
4. Login guru kelas.
5. Guru list inbox melihat unread.
6. Guru buka messages dan reply.
7. Siswa polling melihat balasan.
8. Guru close conversation.
9. Siswa kirim pesan baru dan conversation reopen.
10. Admin list melihat conversation jika admin API/UI sudah ada.

## Progress Update

Status per 2026-05-29:

- [x] Phase 1 Backend Foundation: migration `000020_chat`, package `internal/chat`, routes siswa/guru/admin, service/repo/handler, dan tests backend tersedia.
- [x] Phase 2 Siswa Chat UI: chat inline sudah tersedia di `/siswa/kelas/detail?id=<kelas_id>` dengan polling, send message, dan mark-read.
- [x] Phase 3 Guru Inbox UI: tab `Chat` sudah tersedia di `/guru/kelas/detail?id=<kelas_id>` dengan inbox, thread, reply, mark-read, dan close/open.
- [x] Phase 4 Admin Monitor: `/admin/chat` sudah masuk build; admin API dapat list/read/reply conversation.
- [x] Phase 5 Polish awal: unread summary endpoint + badge dashboard/list kelas sudah deploy (`GET /api/v1/siswa/chat/unread`, `GET /api/v1/kelas/chat/unread`).
- [x] Remote smoke API v1: siswa create/send, guru unread/list/read/reply, siswa unread/read, guru close, siswa reopen, admin list/read/reply PASS di rdpkhorur.

Catatan lanjutan:

- UI smoke manual via browser masih direkomendasikan untuk cek layout/responsiveness dan copy.
- E2E Playwright chat flow belum dibuat; kandidat hardening Fase 8.
- Admin unread policy masih mengikuti implementasi saat ini; decision detail tetap perlu dikunci jika ingin behavior final.

## Implementation Phases

### Phase 1 - Backend Foundation

- Tambah migration chat tables.
- Tambah `internal/chat` model/repo/service/handler.
- Register routes siswa/guru/admin minimal.
- Tambah tests backend utama.

Acceptance:

- `go test ./internal/chat ./...` pass.
- API smoke siswa/guru pass.

### Phase 2 - Siswa Chat UI

- Tambah API client frontend.
- Tambah page siswa chat per kelas.
- Tambah navigasi dari detail kelas.
- Polling 10 detik.

Acceptance:

- Siswa bisa lihat, kirim, dan menerima balasan setelah refresh/polling.
- UI sesuai siswa theme.

### Phase 3 - Guru Inbox UI

- Tambah page guru chat per kelas.
- Inbox + detail + reply + mark read + close/open.
- Polling 15-30 detik.

Acceptance:

- Guru melihat chat siswa di kelasnya saja.
- Guru bisa membalas dan menutup conversation.

### Phase 4 - Admin Monitor

- Tambah admin API filter lengkap jika belum.
- Tambah admin UI `/admin/chat`.
- Tambah sidebar menu.

Acceptance:

- Admin bisa melihat semua chat, filter, reply, dan close.

### Phase 5 - Polish + Optional Realtime

- Unread badge dashboard/sidebar.
- Empty/loading/error polish.
- E2E smoke.
- Evaluasi WebSocket jika polling tidak cukup.

## Risiko dan Mitigasi

- Risiko scope bocor antar siswa/guru: wajib test authorization ketat.
- Risiko unread counter drift: semua send/read dilakukan transaction.
- Risiko UI terlalu berat: mulai text-only, polling sederhana.
- Risiko spam: tambahkan rate limit endpoint message jika middleware mendukung.
- Risiko WebSocket kompleks: tunda ke v2.

## Keputusan yang Perlu Dikunci

- [x] v1 berbasis kelas, bukan global chat.
- [x] Penerima utama guru kelas.
- [x] Admin monitor semua.
- [x] Text-only.
- [x] REST + polling dulu.
- [ ] Apakah admin boleh membalas? Rekomendasi: ya, sebagai admin.
- [ ] Apakah unread admin dihitung dari semua pesan siswa/guru atau hanya pesan siswa? Rekomendasi: pesan siswa saja.
- [ ] Apakah conversation dibuat otomatis saat siswa membuka halaman atau saat pesan pertama? Rekomendasi: saat membuka halaman agar UX sederhana.
