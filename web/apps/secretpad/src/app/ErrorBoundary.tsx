import React from 'react';
import { Button } from '@secretpad/design-system';
import { captureException } from '../shared/lib/observability';

interface ErrorBoundaryProps {
  children: React.ReactNode;
  /** Optional fallback renderer; defaults to a full-screen error page. */
  fallback?: (error: Error, reset: () => void) => React.ReactNode;
}

interface ErrorBoundaryState {
  error: Error | null;
}

/**
 * Global error boundary that prevents a blank screen when a render-phase
 * error occurs anywhere below it. Shows a friendly recovery page and logs
 * the error so it can be shipped to an observability backend (e.g. Sentry).
 */
export class ErrorBoundary extends React.Component<ErrorBoundaryProps, ErrorBoundaryState> {
  state: ErrorBoundaryState = { error: null };

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { error };
  }

  componentDidCatch(error: Error, info: React.ErrorInfo): void {
    // 结构化记录错误；同时转发给可观测性模块（Sentry）。
    // captureException 在未配置 DSN 时为安全的空操作，不会报错。
    console.error('[ErrorBoundary] Uncaught render error:', error, info.componentStack);
    captureException(error, { componentStack: info.componentStack });
  }

  private reset = (): void => {
    this.setState({ error: null });
  };

  render(): React.ReactNode {
    const { error } = this.state;
    if (!error) return this.props.children;

    if (this.props.fallback) return this.props.fallback(error, this.reset);

    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-50 dark:bg-gray-950 p-6">
        <div className="max-w-md w-full bg-white dark:bg-gray-900 border border-gray-200 dark:border-gray-800 rounded-2xl shadow-lg p-8 text-center">
          <div className="text-4xl mb-4" aria-hidden>
            ⚠️
          </div>
          <h1 className="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-2">
            Something went wrong
          </h1>
          <p className="text-sm text-gray-500 dark:text-gray-400 mb-1">
            The page encountered an unexpected error.
          </p>
          <p className="text-xs text-gray-400 dark:text-gray-500 font-mono break-all mb-6">
            {error.message}
          </p>
          <div className="flex justify-center gap-3">
            <Button variant="outline" onClick={this.reset}>
              Try again
            </Button>
            <Button variant="primary" onClick={() => window.location.reload()}>
              Reload page
            </Button>
          </div>
        </div>
      </div>
    );
  }
}
