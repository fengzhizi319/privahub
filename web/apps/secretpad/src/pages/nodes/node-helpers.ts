import type { NodeDetailJava, NodeResourceJava } from '@secretpad/api-client';

/** Why a node cannot be deleted (null → deletable). */
export function nodeDeleteBlockReason(node: NodeDetailJava | undefined, t: (k: string) => string): string | null {
  if (!node) return null;
  if (node.type === 'embedded') return t('nodes.deleteEmbedded');
  if (node.allowDeletion === false) return node.isMainNode ? t('nodes.deleteMainNode') : t('nodes.deleteRunningJobs');
  return null;
}

const UNIT: Record<string, number> = { Bi: 1 / 1024, Ki: 1, Mi: 1024, Gi: 1024 * 1024, Ti: 1024 * 1024 * 1024 };

/** Parse kuscia quantities ('8', '500m', '16Gi', '1024Ki') into a comparable number. */
export function parseQuantity(q?: string): number {
  if (!q) return 0;
  const m = q.trim().match(/^([\d.]+)\s*([A-Za-z]*)$/);
  if (!m) return 0;
  const n = Number(m[1]);
  const unit = m[2];
  if (unit === 'm') return n / 1000;
  return n * (UNIT[unit] ?? 1);
}

/** Used percentage (capacity - allocatable) / capacity, clamped to [0,100]. */
export function resourceUsage(r: NodeResourceJava): number {
  const cap = parseQuantity(r.capacity);
  const alloc = parseQuantity(r.allocatable);
  if (!cap) return 0;
  return Math.max(0, Math.min(100, ((cap - alloc) / cap) * 100));
}
