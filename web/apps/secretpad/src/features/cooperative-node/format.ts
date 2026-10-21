/** Pure display helpers shared by the node / route / model / result pages. */

/** Kuscia node / route status → badge semantics (legacy NodeStateText / RouteStateText). */
export function nodeStatusMeta(status?: string): { badge: 'success' | 'error' | 'default'; key: string } {
  switch (status) {
    case 'Ready':
    case 'Succeeded':
      return { badge: 'success', key: 'coop.status.available' };
    case 'Pending':
      return { badge: 'default', key: 'coop.status.pending' };
    default:
      return { badge: 'error', key: 'coop.status.unavailable' };
  }
}

/** Legacy `formatTimestamp`: accepts ISO strings or epoch millis. */
export function formatTime(value?: string | number): string {
  if (value === undefined || value === null || value === '') return '-';
  const n = typeof value === 'number' ? value : /^\d+$/.test(value) ? Number(value) : NaN;
  const d = Number.isNaN(n) ? new Date(value) : new Date(n);
  return Number.isNaN(d.getTime()) ? String(value) : d.toLocaleString();
}

/** Error message of a thrown value (Error or anything else). */
export function errorText(e: unknown): string {
  return e instanceof Error ? e.message : String(e);
}
