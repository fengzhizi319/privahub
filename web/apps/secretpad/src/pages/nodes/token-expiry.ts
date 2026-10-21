/**
 * Deploy / institution tokens are valid for 30 minutes (legacy hint:
 * "有效期30分钟，如超过30分钟请刷新使用最新token"). The expiry is derived from
 * the token's issue time (`NodeTokenVO.lastTransitionTime` or
 * `InstTokenVO.createTime`).
 */
export const TOKEN_TTL_MS = 30 * 60 * 1000;

/** Parse ISO / 'YYYY-MM-DD HH:mm:ss' / epoch-millis strings. */
export function parseTime(value?: string | number | null): number | null {
  if (value === undefined || value === null || value === '') return null;
  if (typeof value === 'number') return Number.isFinite(value) ? value : null;
  if (/^\d+$/.test(value)) return Number(value);
  const t = Date.parse(value.includes('T') || !value.includes(' ') ? value : value.replace(' ', 'T'));
  return Number.isNaN(t) ? null : t;
}

export interface TokenExpiry {
  /** Epoch ms when the token expires, or null when the issue time is unknown. */
  expiresAt: number | null;
  expired: boolean;
  /** Whole minutes left (0 when expired, null when unknown). */
  minutesLeft: number | null;
}

export function getTokenExpiry(issuedAt?: string | number | null, now: number = Date.now(), ttl = TOKEN_TTL_MS): TokenExpiry {
  const issued = parseTime(issuedAt);
  if (issued === null) return { expiresAt: null, expired: false, minutesLeft: null };
  const expiresAt = issued + ttl;
  const left = expiresAt - now;
  return { expiresAt, expired: left <= 0, minutesLeft: left <= 0 ? 0 : Math.ceil(left / 60000) };
}

/**
 * Refetch interval for token queries: refetch when the current token expires
 * (bounded to [10s, 30min]); unknown issue time → every 30 minutes (legacy).
 */
export function tokenRefetchInterval(issuedAt?: string | number | null, now: number = Date.now()): number {
  const { expiresAt } = getTokenExpiry(issuedAt, now);
  if (expiresAt === null) return TOKEN_TTL_MS;
  return Math.min(TOKEN_TTL_MS, Math.max(10_000, expiresAt - now));
}
