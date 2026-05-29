'use client';

/**
 * /admin - dashboard shortcuts.
 *
 * The shell + role guard live in the parent (authed)/admin/layout.tsx.
 */

import * as React from 'react';
import Link from 'next/link';
import { Users, ScrollText, ShieldAlert, ArrowRight, MessageCircle } from 'lucide-react';

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { Button } from '@/components/ui/button';

interface StatCard {
  label: string;
  description: string;
  href: string;
  Icon: React.ComponentType<{ className?: string }>;
}

const CARDS: StatCard[] = [
  {
    label: 'Pengguna',
    description: 'Kelola admin, guru, dan siswa.',
    href: '/admin/pengguna',
    Icon: Users,
  },
  {
    label: 'Monitor Chat',
    description: 'Pantau chat siswa dan guru lintas kelas.',
    href: '/admin/chat',
    Icon: MessageCircle,
  },
  {
    label: 'Audit Log',
    description: 'Riwayat aksi admin & guru.',
    href: '/admin/audit-log',
    Icon: ScrollText,
  },
  {
    label: 'Login Attempts',
    description: 'Pantau percobaan login (sukses & gagal).',
    href: '/admin/login-attempts',
    Icon: ShieldAlert,
  },
];

export default function AdminDashboardPage() {
  return (
    <div className="space-y-6">
      <header className="space-y-1">
        <h1 className="text-2xl font-semibold tracking-tight">Dashboard</h1>
        <p className="text-sm text-muted-foreground">
          Jalan pintas pengelolaan sistem untuk admin.
        </p>
      </header>

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {CARDS.map(({ label, description, href, Icon }) => (
          <Card key={href} className="flex flex-col">
            <CardHeader className="flex flex-row items-start justify-between space-y-0 pb-2">
              <div className="space-y-1">
                <CardTitle className="text-base">{label}</CardTitle>
                <CardDescription>{description}</CardDescription>
              </div>
              <Icon className="size-5 text-muted-foreground" />
            </CardHeader>
            <CardContent className="mt-auto flex items-end justify-between gap-2">
              <span className="text-sm text-muted-foreground">Siap dibuka</span>
              <Button asChild variant="ghost" size="sm">
                <Link href={href}>
                  Buka
                  <ArrowRight className="size-4" />
                </Link>
              </Button>
            </CardContent>
          </Card>
        ))}
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Panel admin</CardTitle>
          <CardDescription>
            Gunakan kartu di atas untuk membuka manajemen pengguna, monitor chat,
            audit log, dan riwayat login.
          </CardDescription>
        </CardHeader>
      </Card>
    </div>
  );
}
