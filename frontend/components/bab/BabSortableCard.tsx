'use client';

/**
 * Sortable card untuk satu bab di tab "Bab" guru. Dipakai oleh
 * `BabReorderList` dengan @dnd-kit/sortable.
 *
 * Drag handle (icon GripVertical) di kiri sebagai listener-bound area;
 * judul di body adalah Link ke `/guru/kelas/detail/bab?id=<kelas>&bid=<bab>`
 * (Task 3.B.2 detail page) supaya guru bisa langsung masuk ke bab.
 *
 * `BabCardReadOnly` adalah varian non-DnD — dipakai saat kelas archived
 * (manajemen bab dinonaktifkan) atau di MVP untuk siswa view (Fase 3.E).
 */

import * as React from 'react';
import Link from 'next/link';
import { useSortable } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import {
  Archive,
  Copy as CopyIcon,
  GripVertical,
  MoreVertical,
  Pencil,
} from 'lucide-react';

import type { Bab, BabStatus } from '@/lib/bab-api';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { cn } from '@/lib/utils';

interface BabActionsCallbacks {
  onEdit: (bab: Bab) => void;
  onDuplicate: (bab: Bab) => void;
  onArchive: (bab: Bab) => void;
}

export function StatusBadge({ status }: { status: BabStatus }) {
  const label =
    status === 'draft'
      ? 'Draft'
      : status === 'published'
        ? 'Published'
        : 'Diarsipkan';
  const className = cn(
    'inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium',
    status === 'draft' && 'bg-muted text-muted-foreground',
    status === 'published' &&
      'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400',
    status === 'archived' &&
      'bg-orange-500/15 text-orange-700 dark:text-orange-400',
  );
  return <span className={className}>{label}</span>;
}

interface CardBodyProps {
  bab: Bab;
  /**
   * Saat true, judul di-render sebagai `<Link>` ke detail bab.
   * Disable di varian readonly atau saat link tidak diinginkan.
   */
  linkToDetail?: boolean;
}

function CardBody({ bab, linkToDetail = true }: CardBodyProps) {
  const titleNode = linkToDetail ? (
    <Link
      href={`/guru/kelas/detail/bab?id=${bab.kelas_id}&bid=${bab.id}`}
      className="font-medium hover:underline"
    >
      {bab.judul}
    </Link>
  ) : (
    <span className="font-medium">{bab.judul}</span>
  );

  return (
    <div className="min-w-0 flex-1 space-y-1">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-xs font-medium text-muted-foreground">
          Bab {bab.nomor}
        </span>
        {titleNode}
        <StatusBadge status={bab.status} />
      </div>
      {bab.deskripsi && (
        <p className="line-clamp-1 text-xs text-muted-foreground">
          {bab.deskripsi}
        </p>
      )}
    </div>
  );
}

function ActionsMenu({
  bab,
  onEdit,
  onDuplicate,
  onArchive,
}: { bab: Bab } & BabActionsCallbacks) {
  const isArchived = bab.status === 'archived';
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          aria-label="Aksi bab"
          className="size-8 p-0"
        >
          <MoreVertical className="size-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-44">
        <DropdownMenuItem onSelect={() => onEdit(bab)} disabled={isArchived}>
          <Pencil className="size-4" />
          Edit
        </DropdownMenuItem>
        <DropdownMenuItem onSelect={() => onDuplicate(bab)}>
          <CopyIcon className="size-4" />
          Duplikat
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem
          onSelect={() => onArchive(bab)}
          disabled={isArchived}
          className="text-destructive focus:text-destructive"
        >
          <Archive className="size-4" />
          Arsipkan
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

interface BabSortableCardProps extends BabActionsCallbacks {
  bab: Bab;
}

export function BabSortableCard({
  bab,
  onEdit,
  onDuplicate,
  onArchive,
}: BabSortableCardProps) {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: bab.id });

  const style: React.CSSProperties = {
    transform: CSS.Transform.toString(transform),
    transition,
  };

  return (
    <div
      ref={setNodeRef}
      style={style}
      className={cn(
        'flex items-center gap-3 rounded-md border bg-card px-3 py-2 shadow-sm transition-shadow',
        isDragging && 'z-10 ring-2 ring-primary/40 shadow-md',
      )}
    >
      <button
        type="button"
        {...attributes}
        {...listeners}
        aria-label={`Geser Bab ${bab.nomor}`}
        className="cursor-grab touch-none rounded p-1 text-muted-foreground hover:bg-muted hover:text-foreground active:cursor-grabbing"
      >
        <GripVertical className="size-4" />
      </button>
      <CardBody bab={bab} />
      <ActionsMenu
        bab={bab}
        onEdit={onEdit}
        onDuplicate={onDuplicate}
        onArchive={onArchive}
      />
    </div>
  );
}

interface BabCardReadOnlyProps {
  bab: Bab;
}

/**
 * Read-only variant — no drag handle, no actions, judul tidak linkified.
 * Dipakai saat kelas archived (manajemen bab dimatikan) supaya guru tetap
 * bisa lihat daftar tapi gak bisa modifikasi atau navigate ke detail edit.
 */
export function BabCardReadOnly({ bab }: BabCardReadOnlyProps) {
  return (
    <div className="flex items-center gap-3 rounded-md border bg-card/60 px-3 py-2 shadow-sm">
      <div className="rounded p-1 text-muted-foreground/40">
        <GripVertical className="size-4" />
      </div>
      <CardBody bab={bab} linkToDetail={false} />
    </div>
  );
}
