import React from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { RouterProvider } from '@tanstack/react-router';
import { router } from '../router';
import { ErrorBoundary } from './ErrorBoundary';

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
});

/**
 * Application entry component.
 *
 * Providers live here (rather than in main.tsx) so that tests rendering
 * `<App />` directly still get the full router + query context.
 * The ErrorBoundary wraps the router so a render-phase error anywhere in the
 * tree degrades to a recovery page instead of a blank screen.
 */
export const App: React.FC = () => {
  return (
    <ErrorBoundary>
      <QueryClientProvider client={queryClient}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    </ErrorBoundary>
  );
};
