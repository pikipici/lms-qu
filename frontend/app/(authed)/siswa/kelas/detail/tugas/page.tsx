'use client';

/**
 * Siswa tugas detail page — `?id=:kelasID&tid=:tugasID`.
 *
 * Wraps SubmissionPanel + breadcrumbs back to kelas/bab. Auth guard +
 * role guard inherited dari layout (authed)/siswa.
 */

import * as React from 'react';
import Link from 'next/link';
import { useSearchParams } from 'next/navigation';
import { useQuery } from '@tanstack/react-query';
import { ArrowLeft } from 'lucide-react';

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { Button } from '@/components/ui/button';

import { SubmissionPanel } from '@/components/submission/SubmissionPanel';
import { getTugas } from '@/lib/tugas-api';

export default function SiswaTugasDetailPage() {
  const searchParams = useSearchParams();
  const kelasID = searchParams?.get('id') ?? '';
  const tugasID = searchParams?.get('tid') ?? '';

  if (!kelasID || !tugasID) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Parameter tidak lengkap</CardTitle>
          <CardDescription>
            URL ini butuh <code>?id=:kelasID&tid=:tugasID</code>.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Button asChild variant="outline" size="sm">
            <Link href="/siswa">
              <ArrowLeft className="size-4" />
              Daftar kelas
            </Link>
          </Button>
        </CardContent>
      </Card>
    );
  }

  return <SiswaTugasDetailContent kelasID={kelasID} tugasID={tugasID} />;
}

function SiswaTugasDetailContent({
  kelasID,
  tugasID,
}: {
  kelasID: string;
  tugasID: string;
}) {
  // Hydrate deskripsi via getTugas (BE branches by role: siswa hanya
  // dapet kalau status=published + enrollment).
  const tugasQuery = useQuery({
    queryKey: ['siswa', 'tugas', 'detail', tugasID],
    queryFn: () => getTugas(tugasID),
  });

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <Button asChild variant="ghost" size="sm">
          <Link href={`/siswa/kelas/detail?id=${kelasID}`}>
            <ArrowLeft className="size-4" />
            Kembali ke kelas
          </Link>
        </Button>
      </div>

      <SubmissionPanel
        tugasID={tugasID}
        initialDeskripsi={tugasQuery.data?.tugas?.deskripsi}
      />
    </div>
  );
}
