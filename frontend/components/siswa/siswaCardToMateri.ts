'use client';

/**
 * siswaCardToMateri — adapter SiswaMateriCard → Materi (FE-side).
 *
 * BE siswa endpoint (Task 3.E.1) returns minimal SiswaMateriCard yang
 * sengaja strip field guru-only (object_key, mime_type, size_bytes,
 * version, kelas_id, timestamps) — siswa download PDF lewat presigned URL
 * endpoint, jadi metadata file gak perlu di-leak.
 *
 * Tapi `<MateriViewer>` butuh shape `Materi` lengkap (dipakai juga di
 * guru flow). Daripada double-prop signature, kita adapt di FE: isi
 * field missing dengan safe defaults (null / 0 / '') yang gak
 * mengganggu render path siswa.
 *
 * Field yang benar-benar dipakai oleh viewer subcomponents:
 *   - PdfViewer:    id, original_filename (fallback 'materi.pdf')
 *   - YouTubeEmbed: id, konten (video_id)
 *   - MarkdownView: id, konten
 *   - MateriViewer header: judul, tipe, size_bytes (pdf only — kalau null
 *                          gak render). Kita pakai hideHeader untuk siswa
 *                          karena parent render sendiri.
 */

import type { Materi } from '@/lib/materi-api';
import type { SiswaMateriCard } from '@/lib/siswa-bab-api';

export function siswaCardToMateri(card: SiswaMateriCard): Materi {
  return {
    id: card.id,
    kelas_id: '',
    bab_id: card.bab_id,
    judul: card.judul,
    tipe: card.tipe,
    konten: card.konten,
    object_key: null,
    original_filename: null,
    mime_type: null,
    size_bytes: null,
    urutan: card.urutan,
    version: 0,
    created_at: '',
    updated_at: '',
  };
}
