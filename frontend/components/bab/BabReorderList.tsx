'use client';

/**
 * Daftar bab yang bisa diurut ulang via DnD (@dnd-kit/sortable).
 *
 * Optimistic reorder pattern:
 *  1. onMutate    → cancel queries + snapshot prev state + apply optimistic
 *                   list ke cache.
 *  2. onError     → restore snapshot + invalidate (kalau 409, server jadi
 *                   source of truth).
 *  3. onSettled   → invalidate untuk samain ke server response (urutan/version
 *                   field di-bump).
 *
 * Backend kontrak: POST /kelas/:id/bab/reorder body
 *   { order: [babId, ...], versions: { [babId]: version } }
 * mengembalikan list bab terbaru { items, total }. 409 version_conflict body
 * include `conflicts: [{ bab_id, current_version }]`.
 */

import * as React from 'react';
import {
  DndContext,
  KeyboardSensor,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
  type DragEndEvent,
} from '@dnd-kit/core';
import {
  SortableContext,
  arrayMove,
  sortableKeyboardCoordinates,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable';
import { useMutation, useQueryClient } from '@tanstack/react-query';

import {
  type Bab,
  type BabListResponse,
  type ReorderInput,
  friendlyBabError,
  reorderBab,
} from '@/lib/bab-api';
import { ApiError } from '@/lib/api';
import { useToast } from '@/hooks/use-toast';

import { BabCardReadOnly, BabSortableCard } from './BabSortableCard';

interface BabReorderListProps {
  items: Bab[];
  kelasID: string;
  queryKey: readonly unknown[];
  disabled: boolean;
  onEdit: (bab: Bab) => void;
  onDuplicate: (bab: Bab) => void;
  onArchive: (bab: Bab) => void;
}

interface ReorderContext {
  previous: BabListResponse | undefined;
}

export function BabReorderList({
  items,
  kelasID,
  queryKey,
  disabled,
  onEdit,
  onDuplicate,
  onArchive,
}: BabReorderListProps) {
  const queryClient = useQueryClient();
  const { toast } = useToast();

  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: { distance: 4 },
    }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    }),
  );

  const mutation = useMutation<
    BabListResponse,
    Error,
    { input: ReorderInput; nextItems: Bab[] },
    ReorderContext
  >({
    mutationFn: ({ input }) => reorderBab(kelasID, input),
    onMutate: async ({ nextItems }) => {
      await queryClient.cancelQueries({ queryKey });
      const previous = queryClient.getQueryData<BabListResponse>(queryKey);
      queryClient.setQueryData<BabListResponse>(queryKey, {
        items: nextItems,
        total: nextItems.length,
      });
      return { previous };
    },
    onError: (err, _vars, ctx) => {
      if (ctx?.previous) {
        queryClient.setQueryData(queryKey, ctx.previous);
      }
      const apiErr = err instanceof ApiError ? err : null;
      const message = apiErr
        ? friendlyBabError(apiErr, 'reorder')
        : 'Gagal menyimpan urutan bab.';
      const requestId = apiErr?.requestId;
      toast({
        title:
          apiErr?.code === 'version_conflict'
            ? 'Urutan bab sudah berubah'
            : 'Gagal menyimpan urutan',
        description: requestId ? `${message} (req: ${requestId})` : message,
        variant: 'destructive',
      });
      if (apiErr?.code === 'version_conflict') {
        queryClient.invalidateQueries({ queryKey });
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey });
    },
  });

  const handleDragEnd = React.useCallback(
    (event: DragEndEvent) => {
      const { active, over } = event;
      if (!over || active.id === over.id) return;

      const oldIndex = items.findIndex((b) => b.id === String(active.id));
      const newIndex = items.findIndex((b) => b.id === String(over.id));
      if (oldIndex < 0 || newIndex < 0) return;

      const nextItems = arrayMove(items, oldIndex, newIndex);
      const input: ReorderInput = {
        order: nextItems.map((b) => b.id),
        versions: Object.fromEntries(items.map((b) => [b.id, b.version])),
      };
      mutation.mutate({ input, nextItems });
    },
    [items, mutation],
  );

  if (disabled) {
    return (
      <div className="space-y-2">
        {items.map((b) => (
          <BabCardReadOnly key={b.id} bab={b} />
        ))}
      </div>
    );
  }

  return (
    <DndContext
      sensors={sensors}
      collisionDetection={closestCenter}
      onDragEnd={handleDragEnd}
    >
      <SortableContext
        items={items.map((b) => b.id)}
        strategy={verticalListSortingStrategy}
      >
        <div className="space-y-2">
          {items.map((b) => (
            <BabSortableCard
              key={b.id}
              bab={b}
              onEdit={onEdit}
              onDuplicate={onDuplicate}
              onArchive={onArchive}
            />
          ))}
        </div>
      </SortableContext>
      {mutation.isPending && (
        <p className="mt-2 text-xs text-muted-foreground">
          Menyimpan urutan baru…
        </p>
      )}
    </DndContext>
  );
}
