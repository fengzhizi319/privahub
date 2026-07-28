import React, { Suspense } from 'react';
import {
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  redirect,
  useNavigate,
} from '@tanstack/react-router';
import { ToastContainer, Button } from '@secretpad/design-system';

import { AppLayout } from './app/AppLayout';
import { LoginPage } from './pages/login';

// Route-level code splitting: each authenticated page is loaded on demand.
// React.lazy requires a default export, so named page components are adapted.
const lazyPage = <T extends Record<string, React.ComponentType>>(
  factory: () => Promise<T>,
  name: keyof T
) => React.lazy(() => factory().then((m) => ({ default: m[name] })));

const DashboardPage = lazyPage(() => import('./pages/dashboard'), 'DashboardPage');
const ProjectsPage = lazyPage(() => import('./pages/projects'), 'ProjectsPage');
const NodesPage = lazyPage(() => import('./pages/nodes'), 'NodesPage');
const DataTablesPage = lazyPage(() => import('./pages/data-tables'), 'DataTablesPage');
const DataSourcesPage = lazyPage(() => import('./pages/data-sources'), 'DataSourcesPage');
const DataSourceDetailPage = lazyPage(() => import('./pages/data-sources/detail'), 'DataSourceDetailPage');
const DAGPage = lazyPage(() => import('./pages/dag'), 'DAGPage');
const ModelsPage = lazyPage(() => import('./pages/models'), 'ModelsPage');
const ResultsPage = lazyPage(() => import('./pages/results'), 'ResultsPage');
const PeriodicTasksPage = lazyPage(() => import('./pages/periodic-tasks'), 'PeriodicTasksPage');
const MessagesPage = lazyPage(() => import('./pages/messages'), 'MessagesPage');
const NodeRoutesPage = lazyPage(() => import('./pages/node-routes'), 'NodeRoutesPage');
const InstitutionsPage = lazyPage(() => import('./pages/institutions'), 'InstitutionsPage');
const P2pProjectsPage = lazyPage(() => import('./pages/p2p/projects'), 'P2pProjectsPage');
const P2pMyNodePage = lazyPage(() => import('./pages/p2p/my-node'), 'P2pMyNodePage');
const AccountPage = lazyPage(() => import('./pages/account'), 'AccountPage');
const PrivacyScenesPage = lazyPage(() => import('./pages/privacy-scenes'), 'PrivacyScenesPage');
const GuidePage = lazyPage(() => import('./pages/guide'), 'GuidePage');
const WorkbenchPage = lazyPage(() => import('./pages/workbench'), 'WorkbenchPage');
const CloudLogsPage = lazyPage(() => import('./pages/cloud-logs'), 'CloudLogsPage');
const FeatureDatasourcePage = lazyPage(() => import('./pages/feature-datasource'), 'FeatureDatasourcePage');
const ComponentVersionsPage = lazyPage(() => import('./pages/component-versions'), 'ComponentVersionsPage');

/**
 * Read the auth token directly from localStorage (not the Zustand store).
 * The store is a module-level singleton whose `isAuthenticated` snapshot is taken
 * at import time; in tests the token is written to localStorage afterwards, so
 * the store value would be stale. Reading localStorage at navigation time is
 * always current.
 */
const getAuthToken = () => localStorage.getItem('secretpad-token');

const PageFallback: React.FC = () => (
  <div className="min-h-[60vh] flex items-center justify-center">
    <span className="w-8 h-8 border-2 border-blue-500 border-t-transparent rounded-full animate-spin" aria-label="loading" />
  </div>
);

const RootComponent: React.FC = () => (
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

const RouteErrorComponent: React.FC<{ error: Error; reset: () => void }> = ({ error, reset }) => {
  if (isAuthError(error)) {
    // Clear stale credentials and redirect to login. Use replace to avoid
    // leaving the broken route in the history stack.
    if (typeof localStorage !== 'undefined') {
      localStorage.removeItem('secretpad-token');
      localStorage.removeItem('secretpad-user');
    }
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

export const rootRoute = createRootRoute({
  component: RootComponent,
  errorComponent: RouteErrorComponent,
});

const LoginRouteComponent: React.FC = () => {
  const navigate = useNavigate();
  return <LoginPage onLoginSuccess={() => navigate({ to: '/dashboard' })} />;
};

export const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/login',
  beforeLoad: () => {
    if (getAuthToken()) {
      throw redirect({ to: '/dashboard' });
    }
  },
  component: LoginRouteComponent,
});

export const appRoute = createRoute({
  getParentRoute: () => rootRoute,
  id: 'app',
  beforeLoad: () => {
    if (!getAuthToken()) {
      throw redirect({ to: '/login' });
    }
  },
  component: AppLayout,
});

export const indexRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/',
  beforeLoad: () => {
    throw redirect({ to: '/dashboard' });
  },
});

export const dashboardRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/dashboard',
  component: DashboardPage,
});

export const projectsRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/projects',
  component: ProjectsPage,
});

export const nodesRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/nodes',
  component: NodesPage,
});

export const dataTablesRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/data-tables',
  component: DataTablesPage,
});

export const dataSourcesRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/data-sources',
  component: DataSourcesPage,
});

export const dataSourceDetailRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/data-sources/detail',
  validateSearch: (search: Record<string, unknown>) => ({
    ownerId: String(search.ownerId ?? ''),
    datasourceId: String(search.datasourceId ?? ''),
    type: String(search.type ?? ''),
  }),
  component: DataSourceDetailPage,
});

export const dagRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/dag',
  component: DAGPage,
});

export const modelsRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/models',
  component: ModelsPage,
});

export const resultsRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/results',
  component: ResultsPage,
});

export const periodicTasksRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/periodic-tasks',
  component: PeriodicTasksPage,
});

export const messagesRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/messages',
  component: MessagesPage,
});

export const privacyScenesRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/privacy-scenes',
  component: PrivacyScenesPage,
});

export const nodeRoutesRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/node-routes',
  component: NodeRoutesPage,
});

export const institutionsRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/institutions',
  component: InstitutionsPage,
});

export const p2pProjectsRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/p2p/projects',
  component: P2pProjectsPage,
});

export const p2pMyNodeRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/p2p/my-node',
  component: P2pMyNodePage,
});

export const accountRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/account',
  component: AccountPage,
});

export const guideRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/guide',
  component: GuidePage,
});

export const workbenchRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/workbench',
  component: WorkbenchPage,
});

export const cloudLogsRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/cloud-logs',
  component: CloudLogsPage,
});

export const featureDatasourceRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/feature-datasource',
  component: FeatureDatasourcePage,
});

export const componentVersionsRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/component-versions',
  component: ComponentVersionsPage,
});

const routeTree = rootRoute.addChildren([
  loginRoute,
  appRoute.addChildren([
    indexRoute,
    dashboardRoute,
    projectsRoute,
    nodesRoute,
    dataTablesRoute,
    dataSourcesRoute,
    dataSourceDetailRoute,
    dagRoute,
    modelsRoute,
    resultsRoute,
    periodicTasksRoute,
    messagesRoute,
    privacyScenesRoute,
    nodeRoutesRoute,
    institutionsRoute,
    p2pProjectsRoute,
    p2pMyNodeRoute,
    accountRoute,
    guideRoute,
    workbenchRoute,
    cloudLogsRoute,
    featureDatasourceRoute,
    componentVersionsRoute,
  ]),
]);

export const router = createRouter({ routeTree });

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router;
  }
}
