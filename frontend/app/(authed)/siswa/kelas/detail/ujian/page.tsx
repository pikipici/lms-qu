'use client';

/**
 * /siswa/kelas/detail/ujian?id=K&uid=U — siswa ujian player+review (Task 6.G.2).
 *
 * Static export tidak izinkan dynamic route tanpa generateStaticParams,
 * jadi pakai query-string `id=kelasID&uid=ujianID` (mirror /siswa/kelas/
 * detail/bab pattern).
 *
 * Page render UjianSection orchestrator — semua state machine
 * (lobby/playing/result/review) handled di dalamnya. Page wrapper hanya
 * validate query params + tampil error card kalau missing/invalid.
 */

import * as React from 'react';
import Link from 'next/link';
import { useSearchParams } from 'next/navigation';
import { ArrowLeft } from 'lucide-react';

import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { UjianSection } from '@/components/siswa-ujian/UjianSection';

export default function SiswaUjianPlayerPage() {
  const searchParams = useSearchParams();
  const kelasID = searchParams?.get('id') ?? '';
  const ujianID = searchParams?.get('uid') ?? '';

  if (!kelasID || !ujianID) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Parameter ujian tidak ada</CardTitle>
          <CardDescription>
            URL ini butuh parameter <code>?id=:kelasID&uid=:ujianID</code>. Buka
            ujian dari daftar.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Button asChild variant="outline" size="sm">
            <Link href="/siswa/ujian">
              <ArrowLeft className="size-4" />
              Daftar ujian
            </Link>
          </Button>
        </CardContent>
      </Card>
    );
  }

  return <UjianSection ujianID={ujianID} kelasID={kelasID} />;
}
