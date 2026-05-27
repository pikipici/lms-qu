'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { approveJoinRequest, listJoinRequests, rejectJoinRequest, type JoinRequest } from '@/lib/registration-api';
import { useToast } from '@/hooks/use-toast';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';

interface Props {
  scope: 'admin' | 'guru';
}

export function JoinRequestsPanel({ scope }: Props) {
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const queryKey = [scope, 'siswa-join-requests'];
  const requests = useQuery({ queryKey, queryFn: () => listJoinRequests(scope, 'pending') });

  const approve = useMutation({
    mutationFn: (id: string) => approveJoinRequest(scope, id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey });
      toast({ title: 'Permintaan disetujui' });
    },
    onError: () => toast({ title: 'Gagal menyetujui permintaan', variant: 'destructive' }),
  });

  const reject = useMutation({
    mutationFn: (id: string) => rejectJoinRequest(scope, id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey });
      toast({ title: 'Permintaan ditolak' });
    },
    onError: () => toast({ title: 'Gagal menolak permintaan', variant: 'destructive' }),
  });

  const rows = requests.data?.items ?? [];

  return (
    <Card>
      <CardHeader>
        <CardTitle>Permintaan Gabung Siswa</CardTitle>
        <CardDescription>Siswa yang daftar mandiri dan menunggu persetujuan.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {requests.isLoading ? <p className="text-sm text-muted-foreground">Memuat permintaan...</p> : null}
        {rows.map((row: JoinRequest) => (
          <div key={row.id} className="flex flex-col gap-3 rounded-lg border p-3 md:flex-row md:items-center md:justify-between">
            <div>
              <div className="font-medium">{row.siswa_name || row.username || 'Siswa'}</div>
              <div className="text-sm text-muted-foreground">
                {row.username} · {row.sekolah_nama || 'Sekolah'} · {row.kelas_nama || 'Kelas'}
              </div>
              <div className="mt-1 text-xs text-muted-foreground">Diajukan {new Date(row.requested_at).toLocaleString('id-ID')}</div>
            </div>
            <div className="flex gap-2">
              <Button size="sm" onClick={() => approve.mutate(row.id)} disabled={approve.isPending || reject.isPending}>Setujui</Button>
              <Button size="sm" variant="outline" onClick={() => reject.mutate(row.id)} disabled={approve.isPending || reject.isPending}>Tolak</Button>
            </div>
          </div>
        ))}
        {!requests.isLoading && rows.length === 0 ? (
          <p className="py-8 text-center text-sm text-muted-foreground">Tidak ada permintaan pending.</p>
        ) : null}
      </CardContent>
    </Card>
  );
}
