'use client';

/**
 * /guru/bank-soal — Bank Soal pribadi guru (Task 6.F.1).
 *
 * Top-level page (bukan nested di kelas) — locked #84: BankSoal scope
 * per-guru pribadi cross-kelas. Soal di sini bisa dipakai untuk Ulangan
 * Harian dengan sumber manual atau random filter.
 *
 * Komponen utama:
 *   - BankSoalList: tabel soal + filter mapel/tingkat/topik + pagination,
 *     wired ke BankSoalEditDialog + BankSoalBulkPasteDialog.
 *
 * Mirror pattern dari /guru/kelas/detail/bab tab "Soal" (Task 5.F.1)
 * tapi tanpa dependency ke Bab — Bank Soal independent.
 */

import * as React from 'react';
import { Library } from 'lucide-react';

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { BankSoalList } from '@/components/banksoal/BankSoalList';

export default function GuruBankSoalPage() {
  return (
    <div className="space-y-6">
      <header className="space-y-1">
        <div className="flex items-center gap-2">
          <Library className="size-5 text-muted-foreground" />
          <h1 className="text-2xl font-semibold tracking-tight">Bank Soal</h1>
        </div>
        <p className="text-sm text-muted-foreground">
          Bank soal pribadi lu — sumber soal untuk Ulangan Harian. Tag soal
          dengan mapel/tingkat/topik supaya gampang difilter atau dipilih
          random saat menyusun ulangan.
        </p>
      </header>

      <BankSoalList />

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Tips</CardTitle>
          <CardDescription>
            Cara cepat mengelola Bank Soal.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-2 text-sm text-muted-foreground">
          <p>
            <span className="font-medium text-foreground">Bulk paste</span>{' '}
            cepat — tempel sampai 200 soal sekaligus dalam format
            pipe-delimited. Tag mapel/tingkat/topik default akan otomatis
            di-apply ke semua soal hasil parse.
          </p>
          <p>
            <span className="font-medium text-foreground">
              Filter sebelum bulk
            </span>{' '}
            — set chip Mapel/Tingkat aktif lebih dulu, lalu klik{' '}
            <span className="font-mono text-xs">Tambah soal</span> atau{' '}
            <span className="font-mono text-xs">Bulk paste</span>; tag aktif
            otomatis terisi sebagai default.
          </p>
          <p>
            <span className="font-medium text-foreground">Soft delete</span>{' '}
            — soal yang sudah pernah dipakai untuk Ulangan boleh dihapus;
            hasil siswa lama tetap valid (snapshot tersimpan di Hasil
            Ulangan).
          </p>
        </CardContent>
      </Card>
    </div>
  );
}
