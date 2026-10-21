/** Components used by the route tree (kept out of router.tsx so it only exports routes). */
import React, { Suspense } from 'react';
import { Outlet, useNavigate } from '@tanstack/react-router';
import { ToastContainer, Button } from '@secretpad/design-system';
import { LoginPage } from '../pages/login';
import { landingAfterLogin } from './landing';
import { clearStoredSession } from '@secretpad/api-client';

export const PageFallback: React.FC = () => (
  <div className="min-h-[60vh] flex items-center justify-center">
    <span className="w-8 h-8 border-2 border-blue-500 border-t-transparent rounded-full animate-spin" aria-label="loading" />
  </div>
);

export const RootComponent: React.FC = () => (
  <>
    <ToastContainer />
    <Suspense fallback={<PageFallback />}>
      <Outlet />
    </Suspense>
  </>
);

/**
 * Route-level error fallback. TanStack Router renders this when a route's
 * loader or component throws, so a single bad page degrades locally instead
 * of blanking the whole app.
 *
 * Authentication errors are handled centrally: when the backend returns 401
 * ("login is required"), we clear the stale token and send the user back to
 * the login page. This avoids showing the raw error screen on the initial
 * visit when an expired token is still in localStorage.
 */
const isAuthError = (error: Error): boolean => {
  const message = error.message || '';
  return (
    message.includes('login is required') ||
    message.includes('用户认证失败') ||
    message.includes('Authentication failed') ||
    message.includes('Unauthorized')
  );
};

export const RouteErrorComponent: React.FC<{ error: Error; reset: () => void }> = ({ error, reset }) => {
  if (isAuthError(error)) {
    // Clear stale credentials and redirect to login. Use replace to avoid
    // leaving the broken route in the history stack.
    clearStoredSession();
    if (typeof window !== 'undefined') {
      window.location.replace('/login');
    }
    return (
      <div className="min-h-[60vh] flex items-center justify-center p-6">
        <div className="text-xs text-gray-400">{error.message}</div>
      </div>
    );
  }

  return (
    <div className="min-h-[60vh] flex items-center justify-center p-6">
      <div className="max-w-md w-full bg-white dark:bg-gray-900 border border-gray-200 dark:border-gray-800 rounded-2xl shadow-lg p-8 text-center">
        <div className="text-4xl mb-4" aria-hidden>
          ⚠️
        </div>
        <h1 className="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-2">Page failed to load</h1>
        <p className="text-xs text-gray-400 dark:text-gray-500 font-mono break-all mb-6">{error.message}</p>
        <div className="flex justify-center gap-3">
          <Button variant="outline" onClick={reset}>
            Try again
          </Button>
          <Button variant="primary" onClick={() => (window.location.href = '/dashboard')}>
            Back to dashboard
          </Button>
        </div>
      </div>
    </div>
  );
};

export const LoginRouteComponent: React.FC = () => {
  const navigate = useNavigate();
  return (
    <LoginPage
      onLoginSuccess={(user) => {
        if (user?.mustChangePassword) {
          navigate({ to: '/change-password' } as Parameters<typeof navigate>[0]);
          return;
        }
        const target = landingAfterLogin(user);
        navigate({ to: target.to, params: target.params, search: target.search } as Parameters<typeof navigate>[0]);
      }}
    />
  );
};
