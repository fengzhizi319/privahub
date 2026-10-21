import React from 'react';
import { Navigate, useRouterState } from '@tanstack/react-router';
import type { AccessType } from '@/shared/lib/platform';
import { useHasAccess, usePlatformContext } from '@/shared/lib/platform';
import { canAccessPath, resolveHomePath } from '@/shared/lib/access';

export const AccessGuard: React.FC<{ access: AccessType; children: React.ReactNode; fallback?: React.ReactNode }> = ({
  access,
  children,
  fallback,
}) => {
  const allowed = useHasAccess(access);
  if (allowed) return <>{children}</>;
  if (fallback) return <>{fallback}</>;
  return null;
};

/**
 * Component-level route guard. The router `beforeLoad` already checks the
 * persisted user; this re-checks against the live auth store (after user/get
 * refreshed platformType/ownerType) and redirects to the account's landing page.
 */
export const RouteGuard: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const ctx = usePlatformContext();
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  if (canAccessPath(pathname, ctx)) return <>{children}</>;
  const home = resolveHomePath(ctx);
  if (home.to === pathname) return <>{children}</>;
  return <Navigate {...({ to: home.to, params: home.params, search: home.search, replace: true } as React.ComponentProps<typeof Navigate>)} />;
};
