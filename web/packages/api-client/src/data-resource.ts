/**
 * Backend data-permission contract (Go `middleware/data_resource.go`, Java
 * `@DataResource`): for a non-CENTER-owner account every listed route reads its
 * resource id from the JSON body, and a request that omits it is rejected with
 * AUTH_FAILED (202011602). CENTER owners are never checked.
 *
 * Only the routes whose id means "the caller's own node / institution" are
 * listed here; for them the logged-in user's `ownerId` is the right default,
 * so {@link withDataResourceOwner} fills it in when a caller left it out.
 * Routes keyed by a *target* id (`projectId`, `nodeId` of node/get, …) are
 * always passed explicitly by the callers and are not guessed.
 */
export const OWNER_SCOPED_ROUTES: Record<string, string> = {
  'node/result/list': 'ownerId',
  'nodeRoute/page': 'ownerId',
  'datasource/create': 'ownerId',
  'datasource/delete': 'ownerId',
  'datasource/list': 'ownerId',
  'datasource/detail': 'ownerId',
  'datasource/nodes': 'ownerId',
  'datatable/create': 'ownerId',
  'datatable/list': 'ownerId',
  'feature_datasource/create': 'ownerId',
  'message/list': 'ownerId',
  'message/detail': 'ownerId',
  'message/pending': 'ownerId',
  'inst/get': 'instId',
  'approval/create': 'initiatorId',
  'message/reply': 'voteParticipantId',
};

export interface DataResourceUser {
  ownerId?: string;
  ownerType?: string;
}

/** The owner-scoped id field `path` needs for `user`, or null (CENTER owner / not listed). */
export function ownerFieldFor(path: string, user: DataResourceUser | null | undefined): string | null {
  const route = path.replace(/^.*\/api\/v1alpha1\//, '').replace(/^\//, '').split('?')[0];
  const field = OWNER_SCOPED_ROUTES[route];
  if (!field || !user?.ownerId || (user.ownerType || '').toUpperCase() === 'CENTER') return null;
  return field;
}

const snake = (s: string) => s.replace(/[A-Z]/g, (c) => `_${c.toLowerCase()}`);

/**
 * Return `body` with the owner-scoped id field filled from `user` when the
 * route needs it and the body lacks it (camelCase and snake_case both count).
 * Returns the same object when nothing changes.
 */
export function withDataResourceOwner(path: string, body: unknown, user: DataResourceUser | null | undefined): unknown {
  const field = ownerFieldFor(path, user);
  if (!field || !user?.ownerId) return body;
  if (body !== undefined && (body === null || typeof body !== 'object' || Array.isArray(body))) return body;
  const obj = (body || {}) as Record<string, unknown>;
  const has = (k: string) => typeof obj[k] === 'string' && (obj[k] as string).trim() !== '';
  if (has(field) || has(snake(field))) return body;
  return { ...obj, [field]: user.ownerId };
}

/** Persisted user (auth store writes it to localStorage on login / user/get). */
export function storedDataResourceUser(): DataResourceUser | null {
  if (typeof localStorage === 'undefined') return null;
  try {
    const raw = localStorage.getItem('secretpad-user');
    return raw ? (JSON.parse(raw) as DataResourceUser) : null;
  } catch {
    return null;
  }
}
