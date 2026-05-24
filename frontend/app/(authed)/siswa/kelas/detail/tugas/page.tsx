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

import { SubmissionPanel } from '@/components/submission/SubmissionPanel';
import { getTugas } from '@/lib/tugas-api';
import {
  SiswaButton,
  SiswaCard,
  SiswaCardBody,
  SiswaCardDescription,
  SiswaCardHeader,
  SiswaCardTitle,
} from '@/components/siswa-ui';

export default function SiswaTugasDetailPage() {
  const searchParams = useSearchParams();
  const kelasID = searchParams?.get('id') ?? '';
  const tugasID = searchParams?.get('tid') ?? '';

  if (!kelasID || !tugasID) {
    return (
      <SiswaCard tone="surface" shadow="md">
        <SiswaCardHeader>
          <SiswaCardTitle>Parameter tidak lengkap</SiswaCardTitle>
          <SiswaCardDescription>
            URL ini butuh <code>?id=:kelasID&tid=:tugasID</code>.
          </SiswaCardDescription>
        </SiswaCardHeader>
        <SiswaCardBody>
          <SiswaButton asChild tone="surface" size="sm">
            <Link href="/siswa">
              <ArrowLeft className="size-4" />
              Daftar kelas
            </Link>
          </SiswaButton>
        </SiswaCardBody>
      </SiswaCard>
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
  const tugasQuery = useQuery({
    queryKey: ['siswa', 'tugas', 'detail', tugasID],
    queryFn: () => getTugas(tugasID),
  });

  return (
    <div className="space-y-4">
      <SiswaButton asChild tone="ghost" size="sm" className="-ml-2">
        <Link href={`/siswa/kelas/detail?id=${kelasID}`}>
          <ArrowLeft className="size-4" />
          Kembali ke kelas
        </Link>
      </SiswaButton>

      <SubmissionPanel
        tugasID={tugasID}
        initialDeskripsi={tugasQuery.data?.tugas?.deskripsi}
      />
    </div>
  );
}
