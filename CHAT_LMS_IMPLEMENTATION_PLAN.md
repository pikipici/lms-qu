# LMS Chat Implementation Plan

> Plan aktif untuk menghidupkan fitur chat LMS. Keputusan terbaru: chat di tab detail kelas adalah `Chat Guru` berbasis kelas; `Bantuan Admin` akan dibuat terpisah di sidebar siswa nanti.

## Status Saat Ini

Status repo per 2026-05-29 setelah inspeksi ulang:

- Backend chat sudah tersedia, bukan kosong:
  - Package `backend/internal/chat/` berisi `model.go`, `repo.go`, `service.go`, `handler.go`, dan `service_test.go`.
  - Migration sudah ada: `backend/migrations/000020_chat.up.sql` dan `backend/migrations/000020_chat.down.sql`.
  - Routes chat sudah terdaftar di `backend/cmd/server/main.go` untuk siswa, guru, admin, dan unread summary.
- Frontend siswa sudah punya tab chat inline di `frontend/app/(authed)/siswa/kelas/detail/page.tsx` lewat komponen `SiswaChatBox`.
- Frontend API client sudah ada:
  - `frontend/lib/siswa-chat-api.ts`
  - `frontend/lib/guru-chat-api.ts`
  - `frontend/lib/admin-chat-api.ts`
- Frontend admin route `/admin/chat` sudah ada.
- Frontend guru detail kelas sudah mengimpor API chat dan ikon chat; perlu audit lanjut apakah inbox/thread sudah render penuh dan flow reply/close sudah solid.
- Plan produk terbaru: tab detail kelas harus menjadi `Chat Guru`/`Tanya Guru`; chat admin dipisah nanti sebagai menu sidebar siswa `Bantuan Admin`.
- Update implementasi 2026-05-29:
  - Migration tambahan `000021_chat_scope_sekolah` sudah disiapkan untuk menambah `scope` + `sekolah_id`, scope check, dan index scope/sekolah tanpa mengubah migration `000020_chat` yang sudah applied di server.
  - Model `Conversation` sudah punya `Scope` dan `SekolahID`.
  - Repo/service chat sudah membatasi flow v1 ke `scope='kelas'`, mengisi snapshot `sekolah_id`, dan update snapshot guru/sekolah saat kelas berubah.
  - UI siswa detail kelas sudah dipoles dari `Chat` ke `Chat Guru`/`Tanya guru`, copy diarahkan ke guru pengampu, dan closed-state memberi tahu bahwa pesan baru membuka chat lagi.
  - UI guru detail kelas sudah dipoles dari `Chat` ke `Chat Siswa`.
  - Validasi lokal PASS: `go test ./internal/chat`, `go test ./...`, dan frontend `npm run typecheck`.
- Belum dilakukan di sesi ini: frontend production build terbaru, remote deploy, dan smoke chat live.

Progress kasar:

- Phase 1 Backend Foundation: sekitar 95%; migration/model/service/routes/tests ada dan test lokal PASS, tinggal build/deploy/smoke.
- Phase 2 Siswa Chat Guru: sekitar 85%; UI/API ada dan copy sudah dipoles, tinggal live smoke.
- Phase 3 Guru Chat Siswa: sekitar 75%; inbox/thread/reply/close UI ada dan copy sudah dipoles, tinggal live smoke.
- Phase 4 Admin Monitor: sekitar 70%; `/admin/chat` ada, masih perlu live smoke endpoint/filter.
- Phase 5 Bantuan Admin v2: 0%; sengaja ditunda setelah v1 stabil.

## Keputusan Produk

### v1: Chat Guru Di Tab Kelas

Chat pada detail kelas dipakai untuk percakapan siswa dengan guru pengampu kelas tersebut.

- Scope conversation: `kelas_id + siswa_id`.
- Penerima utama: guru pengampu kelas.
- Konteks: materi, tugas, ulangan, nilai, atau pertanyaan terkait kelas itu.
- Admin boleh monitor dari panel admin, tapi bukan penerima utama.
- Label UI rekomendasi: `Chat Guru` atau `Tanya Guru`, bukan hanya `Chat`.

### v2: Bantuan Admin Di Sidebar Siswa

Chat admin dibuat sebagai menu global siswa, bukan tab kelas.

- Scope conversation: `sekolah_id + siswa_id` atau `siswa_id` global.
- Penerima utama: admin sekolah.
- Konteks: akun, login, salah kelas, request pindah kelas, kendala teknis, bantuan administratif.
- Label UI rekomendasi: `Bantuan Admin`.

## Non-Goals v1

- Tidak ada attachment/file upload.
- Tidak ada WebSocket; pakai REST + polling dulu.
- Tidak ada chat siswa ke siswa.
- Tidak ada grup chat kelas.
- Tidak ada multi-guru routing kompleks.
- Tidak ada typing indicator.
- Tidak ada read receipt per pesan.
- Tidak ada moderation otomatis.

## Data Model v1

Migration rekomendasi: `000019_chat_class_conversations` atau nomor berikutnya sesuai migration terakhir.

### `chat_conversations`

```sql
CREATE TABLE chat_conversations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    scope TEXT NOT NULL DEFAULT 'kelas',
    kelas_id UUID NOT NULL REFERENCES kelas(id),
    sekolah_id UUID,
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
    CONSTRAINT chat_conversations_scope_check CHECK (scope IN ('kelas', 'admin')),
    CONSTRAINT chat_conversations_status_check CHECK (status IN ('open', 'closed'))
);
```

Indexes:

```sql
CREATE UNIQUE INDEX idx_chat_conversations_kelas_unique_active
    ON chat_conversations (kelas_id, siswa_id)
    WHERE deleted_at IS NULL AND scope = 'kelas';

CREATE INDEX idx_chat_conversations_guru_status_last
    ON chat_conversations (guru_id, status, last_message_at DESC)
    WHERE deleted_at IS NULL AND scope = 'kelas';

CREATE INDEX idx_chat_conversations_siswa_last
    ON chat_conversations (siswa_id, last_message_at DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_chat_conversations_kelas_last
    ON chat_conversations (kelas_id, last_message_at DESC)
    WHERE deleted_at IS NULL;
```

Catatan:

- `scope` disiapkan dari awal agar v2 `Bantuan Admin` bisa memakai tabel yang sama.
- Untuk v1, hanya `scope='kelas'` yang diaktifkan.
- `sekolah_id` bisa diisi dari kelas/sekolah untuk filter admin dan persiapan `scope='admin'`.
- `guru_id` mengikuti guru pengampu kelas. Jika kelas berpindah guru, service boleh update `guru_id` saat conversation dibuka/list.

### `chat_messages`

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

Indexes:

```sql
CREATE INDEX idx_chat_messages_conversation_created
    ON chat_messages (conversation_id, created_at ASC)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_chat_messages_deleted_at
    ON chat_messages (deleted_at);
```

## Backend Package

Buat package baru:

```text
backend/internal/chat/
  model.go
  dto.go
  repo.go
  service.go
  handler.go
  service_test.go
  handler_test.go
```

### Service Utama

- `GetOrCreateSiswaKelasConversation(ctx, siswaID, kelasID, limit)`
- `SendSiswaKelasMessage(ctx, siswaID, kelasID, body)`
- `ListGuruKelasConversations(ctx, guruID, kelasID, filter)`
- `GetGuruKelasConversationMessages(ctx, guruID, kelasID, conversationID, limit)`
- `SendGuruKelasMessage(ctx, guruID, kelasID, conversationID, body)`
- `MarkRead(ctx, actorRole, actorID, conversationID)`
- `SetStatus(ctx, actorRole, actorID, conversationID, status, version)`
- `ListAdminConversations(ctx, filter)`
- `SendAdminMessage(ctx, adminID, conversationID, body)`

### Authorization Rules

Siswa:

- Role harus `siswa`.
- Siswa harus enrolled aktif di `kelas_id`.
- Siswa hanya boleh akses conversation miliknya (`siswa_id=current_user_id`).

Guru:

- Role harus `guru`.
- Guru harus pengampu/owner kelas.
- Guru hanya boleh list/read/reply conversation di kelas miliknya.

Admin:

- Role harus `admin`.
- Admin boleh list/read semua `scope='kelas'` conversation.
- Admin boleh reply sebagai `sender_role='admin'` jika decision final mengizinkan.

## Backend API v1

### Siswa

Endpoint ini sudah dipakai frontend sekarang, jadi sebaiknya dipertahankan:

```text
GET  /api/v1/siswa/kelas/:kelas_id/chat?limit=80
POST /api/v1/siswa/kelas/:kelas_id/chat/messages
POST /api/v1/siswa/kelas/:kelas_id/chat/read
GET  /api/v1/siswa/chat/unread
```

`POST messages` body:

```json
{
  "body": "Pak, tugasnya dikumpulkan kapan?"
}
```

Response `GET chat`:

```json
{
  "data": {
    "conversation": {
      "id": "uuid",
      "scope": "kelas",
      "kelas_id": "uuid",
      "siswa_id": "uuid",
      "guru_id": "uuid",
      "status": "open",
      "last_message_at": "2026-05-29T10:00:00Z",
      "last_message_preview": "Pak, tugasnya...",
      "siswa_unread_count": 0,
      "guru_unread_count": 1,
      "admin_unread_count": 1,
      "version": 1
    },
    "messages": []
  }
}
```

### Guru

```text
GET   /api/v1/guru/kelas/:kelas_id/chat/conversations?status=&unread=&search=&limit=20&offset=0
GET   /api/v1/guru/kelas/:kelas_id/chat/conversations/:conversation_id/messages?limit=80
POST  /api/v1/guru/kelas/:kelas_id/chat/conversations/:conversation_id/messages
POST  /api/v1/guru/kelas/:kelas_id/chat/conversations/:conversation_id/read
PATCH /api/v1/guru/kelas/:kelas_id/chat/conversations/:conversation_id/status
GET   /api/v1/guru/chat/unread
```

Status body:

```json
{
  "status": "closed",
  "version": 3
}
```

### Admin Monitor v1

Admin monitor boleh dibuat setelah siswa+guru, tapi backend schema/service sebaiknya siap.

```text
GET   /api/v1/admin/chat/conversations?sekolah_id=&kelas_id=&guru_id=&status=&unread=&search=&limit=20&offset=0
GET   /api/v1/admin/chat/conversations/:conversation_id/messages?limit=80
POST  /api/v1/admin/chat/conversations/:conversation_id/messages
POST  /api/v1/admin/chat/conversations/:conversation_id/read
PATCH /api/v1/admin/chat/conversations/:conversation_id/status
```

## Service Logic Detail

### Get Or Create Siswa Chat

1. Parse dan validasi `kelas_id` UUID.
2. Pastikan user role siswa dan enrolled aktif di kelas.
3. Ambil kelas + guru pengampu + sekolah.
4. Cari active conversation `scope='kelas' AND kelas_id=? AND siswa_id=?`.
5. Jika belum ada, buat conversation baru.
6. Jika ada tapi `guru_id` berbeda dari guru pengampu saat ini, update `guru_id`.
7. Load messages terbaru limit 80.
8. Return conversation + messages.

### Send Message

1. Trim body.
2. Reject empty atau `>4000` karakter dengan `400 invalid_body`.
3. Validate authorization sesuai role.
4. Transaction:
   - Lock conversation row `FOR UPDATE`.
   - Insert `chat_messages`.
   - Update `last_message_at`, `last_message_preview`, `updated_at`, `version+1`.
   - Jika sender siswa: set `status='open'`, increment `guru_unread_count`, optional increment `admin_unread_count`.
   - Jika sender guru/admin: increment `siswa_unread_count`.
5. Return inserted message dengan `sender_name`.

### Mark Read

- Siswa: set `siswa_unread_count=0`.
- Guru: set `guru_unread_count=0`.
- Admin: set `admin_unread_count=0`.

### Close/Reopen

- Guru/admin boleh set `open`/`closed` pakai optimistic `version`.
- Siswa tidak punya tombol close.
- Jika siswa kirim pesan ke conversation `closed`, status auto balik `open`.

## Frontend Work

### Siswa UI

File existing:

```text
frontend/app/(authed)/siswa/kelas/detail/page.tsx
frontend/lib/siswa-chat-api.ts
```

Perubahan:

- Ganti label tab `Chat` menjadi `Chat Guru` atau `Tanya Guru`.
- Copy header: `Tanya guru kelas ini`.
- Empty state: `Tanyakan materi, tugas, ulangan, atau nilai ke guru kelas ini.`
- Pastikan error state menjelaskan jika chat belum tersedia/connection gagal.
- Polling tetap 12 detik atau ubah ke 10 detik.
- Bubble siswa align kanan, guru/admin align kiri.
- Jika `status='closed'`, tampilkan notice bahwa kirim pesan baru akan membuka kembali percakapan.

### Guru UI

Jika frontend guru belum ada/masih partial, buat:

```text
frontend/components/guru/GuruKelasChatInbox.tsx
```

Integrasi ke detail kelas guru:

- Tambah tab/action `Chat Siswa` di `/guru/kelas/detail?id=<kelas_id>`.
- Layout desktop split inbox + thread.
- Layout mobile single column.
- Filter: `Semua`, `Belum dibaca`, `Terbuka`, `Ditutup`.
- Reply form + close/open conversation.
- Badge unread di row conversation.

### Admin UI

Existing route:

```text
frontend/app/(authed)/admin/chat/page.tsx
frontend/lib/admin-chat-api.ts
```

Perubahan saat backend siap:

- Pastikan endpoint match backend baru.
- Filter minimal: status, unread.
- Filter lanjutan: sekolah, kelas, guru, search siswa.
- Admin reply diberi label jelas `Admin`.

### Siswa Sidebar v2

Nanti buat menu global:

```text
/siswa/bantuan-admin
```

Label sidebar: `Bantuan Admin`.

Jangan gabungkan dengan `Chat Guru` di tab kelas karena konteks dan permission berbeda.

## Testing Plan

### Backend Tests

Minimal wajib:

- Siswa enrolled bisa get/create conversation.
- Siswa non-enrolled ditolak.
- Siswa tidak bisa akses chat siswa lain.
- Guru owner kelas bisa list/read/reply.
- Guru non-owner ditolak.
- Admin bisa list/read semua conversation.
- Empty body ditolak.
- Body lebih dari 4000 ditolak.
- Siswa send ke closed conversation auto-open.
- Unread counter naik sesuai sender.
- Mark read reset counter sesuai actor.
- Set status dengan version benar sukses.
- Set status dengan version stale return conflict.

### Frontend Checks

- `npm run typecheck`.
- `npm run build`.
- Siswa detail kelas tab `Chat Guru` render empty state.
- Siswa bisa kirim pesan.
- Guru inbox melihat pesan dan unread.
- Guru bisa reply.
- Siswa melihat reply setelah polling/refetch.
- Guru close, siswa kirim lagi, conversation reopen.

### Remote Smoke

Di server:

```text
cd /home/ubuntu/lms
set -a; . ./.env; set +a
bash deploy/deploy.sh --remote
curl -fsS http://127.0.0.1:8200/api/v1/readyz
```

Manual/API smoke:

1. Login siswa enrolled.
2. Buka detail kelas > `Chat Guru`.
3. Kirim pesan.
4. Login guru pengampu.
5. Buka detail kelas > `Chat Siswa`.
6. Pastikan pesan masuk dan unread tampil.
7. Guru reply.
8. Siswa refresh/polling melihat balasan.
9. Guru close conversation.
10. Siswa kirim pesan lagi dan status reopen.
11. Admin `/admin/chat` bisa monitor conversation jika admin API/UI sudah aktif.

## Implementation Phases

### Phase 1 - Backend Foundation

- Tambah migration chat tables.
- Tambah `internal/chat` model/repo/service/handler.
- Register route siswa + guru minimal.
- Tambah unit/handler tests.

Acceptance:

- `go test ./internal/chat ./...` pass.
- API siswa create/send pass.
- API guru list/read/reply pass.

### Phase 2 - Siswa Chat Guru

- Sambungkan UI existing ke backend.
- Rename tab/copy jadi `Chat Guru`/`Tanya Guru`.
- Polish empty/loading/error/closed state.

Acceptance:

- Siswa bisa chat guru dari detail kelas.
- Tidak ada 404 endpoint chat.

### Phase 3 - Guru Chat Siswa

- Buat/integrasikan inbox guru per kelas.
- Reply, mark-read, close/open.
- Badge unread.

Acceptance:

- Guru hanya melihat chat kelas miliknya.
- Guru bisa reply dan close/open conversation.

### Phase 4 - Admin Monitor

- Aktifkan admin list/read/reply/close.
- Cocokkan `/admin/chat` dengan backend.
- Tambah filter dasar.

Acceptance:

- Admin bisa monitor semua chat kelas.
- Admin reply muncul sebagai `Admin`.

### Phase 5 - Bantuan Admin v2

- Tambah scope `admin` conversation.
- Tambah `/siswa/bantuan-admin` + sidebar link.
- Tambah admin inbox filter/scope admin support.

Acceptance:

- Siswa bisa chat admin tanpa masuk detail kelas.
- Chat admin tidak bercampur dengan Chat Guru.

## Risks

- Scope leak antar siswa/guru: mitigasi dengan authorization tests wajib.
- Unread counter drift: semua send/read dalam transaction.
- Polling terlalu berat: batasi interval 10-30 detik dan limit messages.
- Admin role ambiguity: label sender jelas, simpan `sender_role`.
- Kelas pindah guru: update `guru_id` saat load/list conversation.

## Decisions Locked

- [x] Chat tab kelas = siswa ke guru pengampu.
- [x] Admin chat/support = menu global siswa terpisah nanti.
- [x] v1 text-only.
- [x] v1 REST + polling.
- [x] Conversation kelas unik per `kelas_id + siswa_id`.
- [x] Siswa dapat reopen closed conversation dengan mengirim pesan baru.
- [x] Label produk: tab kelas pakai `Chat Guru`/`Tanya Guru`, bukan chat admin.
- [x] Label produk v2: sidebar siswa pakai `Bantuan Admin` untuk support/admin.
- [x] Migration tambahan `000021_chat_scope_sekolah` disiapkan untuk `scope` + `sekolah_id` karena `000020_chat` sudah applied di server.
- [x] Admin boleh reply di chat kelas sebagai `Admin`.
- [x] Admin unread dihitung dari pesan siswa saja, bukan semua non-admin.
