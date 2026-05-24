'use client';

/**
 * Section identity helpers — single source of truth for the per-section
 * accent palette. Use these to keep wayfinding consistent across pages.
 */

import { BookOpen, ClipboardList, GraduationCap, Megaphone, Target, Trophy } from 'lucide-react';
import type { LucideIcon } from 'lucide-react';
import type { SiswaCardTone } from './siswa-card';

export type SiswaSectionKind =
  | 'materi'
  | 'latihan'
  | 'ulangan'
  | 'tugas'
  | 'nilai'
  | 'umum';

interface SectionMeta {
  label: string;
  Icon: LucideIcon;
  /** Tailwind solid background utility — for chips, accents. */
  solid: string;
  /** Tailwind background tinted utility — for card surfaces. */
  tint: string;
  /** Maps to SiswaCard tone variant. */
  tone: SiswaCardTone;
}

export const SECTION_META: Record<SiswaSectionKind, SectionMeta> = {
  materi: {
    label: 'Materi',
    Icon: BookOpen,
    solid: 'bg-siswa-blue',
    tint: 'bg-siswa-blue/30',
    tone: 'materi',
  },
  latihan: {
    label: 'Latihan',
    Icon: Target,
    solid: 'bg-siswa-green',
    tint: 'bg-siswa-green/30',
    tone: 'latihan',
  },
  ulangan: {
    label: 'Ulangan',
    Icon: GraduationCap,
    solid: 'bg-siswa-yellow',
    tint: 'bg-siswa-yellow/40',
    tone: 'ulangan',
  },
  tugas: {
    label: 'Tugas',
    Icon: ClipboardList,
    solid: 'bg-siswa-pink',
    tint: 'bg-siswa-pink/30',
    tone: 'tugas',
  },
  nilai: {
    label: 'Nilai',
    Icon: Trophy,
    solid: 'bg-siswa-lavender',
    tint: 'bg-siswa-lavender/30',
    tone: 'nilai',
  },
  umum: {
    label: 'Umum',
    Icon: Megaphone,
    solid: 'bg-siswa-cream',
    tint: 'bg-siswa-cream/60',
    tone: 'umum',
  },
};

/**
 * Deterministic hash → tone for "kelas card" coloring so each kelas always
 * gets the same accent across reloads (memorable wayfinding).
 */
const KELAS_TONE_ROTATION: SiswaSectionKind[] = [
  'materi',
  'tugas',
  'latihan',
  'nilai',
  'ulangan',
  'umum',
];

export function kelasToneFromId(id: string): SiswaSectionKind {
  if (!id) return 'umum';
  let h = 0;
  for (let i = 0; i < id.length; i += 1) {
    h = (h * 31 + id.charCodeAt(i)) | 0;
  }
  const idx = Math.abs(h) % KELAS_TONE_ROTATION.length;
  return KELAS_TONE_ROTATION[idx]!;
}
