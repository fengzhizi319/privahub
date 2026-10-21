import React from 'react';
import { createRootRoute, createRoute, createRouter, redirect } from '@tanstack/react-router';

import { AppLayout } from './app/AppLayout';
import { LoginRouteComponent, RootComponent, RouteErrorComponent } from './app/route-components';
import { hasStoredSession } from '@secretpad/api-client';
import { getStoredUser } from './features/auth/model/auth-store';
import { resolveMyNodeId, toPlatformContext } from './shared/lib/platform';
import { canAccessPath, resolveHomePath, type HomeTarget } from './shared/lib/access';

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
const PredefinedCanvasesPage = lazyPage(() => import('./pages/dag-templates'), 'PredefinedCanvasesPage');
const DAGPage = lazyPage(() => import('./pages/dag'), 'DAGPage');
const GraphsPage = lazyPage(() => import('./pages/graphs'), 'GraphsPage');
const ModelsPage = lazyPage(() => import('./pages/models'), 'ModelsPage');
const ResultsPage = lazyPage(() => import('./pages/results'), 'ResultsPage');
const JobRecordsPage = lazyPage(() => import('./pages/job-records'), 'JobRecordsPage');
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
const NodeLayoutPage = lazyPage(() => import('./pages/node'), 'NodeLayoutPage');
const AllDataSourcesPage = lazyPage(() => import('./pages/all-data'), 'AllDataSourcesPage');
const AllDataTablesPage = lazyPage(() => import('./pages/all-data'), 'AllDataTablesPage');
const InstRegisterPage = lazyPage(() => import('./pages/institutions/register'), 'InstRegisterPage');
const PeriodicTaskDetailPage = lazyPage(() => import('./pages/periodic-tasks/detail'), 'PeriodicTaskDetailPage');
const AuditLogPage = lazyPage(() => import('./pages/audit'), 'AuditLogPage');

const ChangePasswordPage = lazyPage(() => import('./pages/change-password'), 'ChangePasswordPage');

/**
 * Whether the browser holds a session. The session credential itself is an
 * HttpOnly cookie (unreadable by JS); the persisted non-secret user context
 * is the login marker. Read at navigation time (not the Zustand snapshot) so
 * it is always current; an expired cookie session is caught by the 401
 * handling of the API client.
 */
const getAuthToken = () => hasStoredSession();

/** The session is restricted to the password change (initial / reset password). */
const mustChangePassword = () => getStoredUser()?.mustChangePassword === true;

/** Platform context of the persisted user (sync; refreshed by AppLayout via user/get). */
const storedContext = () => toPlatformContext(getStoredUser());

/** Build a TanStack `redirect` to a HomeTarget. */
const redirectTo = (target: HomeTarget) =>
  redirect({ to: target.to, params: target.params, search: target.search } as Parameters<typeof redirect>[0]);

export const rootRoute = createRootRoute({
  component: RootComponent,
  errorComponent: RouteErrorComponent,
});

const optStr = (v: unknown): string | undefined =>
  v === undefined || v === null || v === '' ? undefined : String(v);

/** /dag search: project/graph to open; P2P entry also passes compute mode/type. */
export interface DagSearch {
  projectId?: string;
  graphId?: string;
  dagId?: string;
  mode?: string;
  type?: string;
}

/** /results search: legacy deep link `?ownerId=&resultName=`. */
export interface ResultsSearch {
  ownerId?: string;
  resultName?: string;
}

export const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/login',
  beforeLoad: () => {
    if (getAuthToken()) {
      if (mustChangePassword()) throw redirect({ to: '/change-password' });
      throw redirectTo(resolveHomePath(storedContext()));
    }
  },
  component: LoginRouteComponent,
});

/** Forced password change, outside the app layout (sandboxed session). */
export const changePasswordRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/change-password',
  beforeLoad: () => {
    if (!getAuthToken()) throw redirect({ to: '/login' });
  },
  component: ChangePasswordPage,
});

export const appRoute = createRoute({
  getParentRoute: () => rootRoute,
  id: 'app',
  beforeLoad: ({ location }) => {
    if (!getAuthToken()) {
      throw redirect({ to: '/login' });
    }
    if (mustChangePassword()) {
      throw redirect({ to: '/change-password' });
    }
    // Route-level guard per platformType / ownerType (legacy *-auth wrappers).
    const ctx = storedContext();
    if (location.pathname !== '/' && !canAccessPath(location.pathname, ctx)) {
      throw redirectTo(resolveHomePath(ctx));
    }
  },
  component: AppLayout,
});

export const indexRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/',
  beforeLoad: () => {
    throw redirectTo(resolveHomePath(storedContext()));
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
  validateSearch: (search: Record<string, unknown>): DagSearch => ({
    projectId: optStr(search.projectId),
    graphId: optStr(search.graphId),
    dagId: optStr(search.dagId),
    mode: optStr(search.mode),
    type: optStr(search.type),
  }),
  component: DAGPage,
});

export const predefinedCanvasesRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/dag-templates',
  component: PredefinedCanvasesPage,
});

export const graphsRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/graphs',
  component: GraphsPage,
});

export const modelsRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/models',
  component: ModelsPage,
});

export const resultsRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/results',
  validateSearch: (search: Record<string, unknown>): ResultsSearch => ({
    ownerId: optStr(search.ownerId),
    resultName: optStr(search.resultName),
  }),
  component: ResultsPage,
});

export const jobRecordsRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/job-records',
  component: JobRecordsPage,
});

export const periodicTasksRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/periodic-tasks',
  validateSearch: (search: Record<string, unknown>): { projectId?: string } => ({
    projectId: optStr(search.projectId),
  }),
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
  validateSearch: (search: Record<string, unknown>): { ownerId?: string } => ({ ownerId: optStr(search.ownerId) }),
  beforeLoad: ({ search }) => {
    // CENTER admins own no node: only built-in nodes via ?ownerId= (legacy edge-auth).
    if (resolveMyNodeId(storedContext(), search.ownerId) === null) throw redirect({ to: '/nodes' });
  },
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

export const periodicTaskDetailRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/periodic-tasks/detail',
  validateSearch: (
    search: Record<string, unknown>,
  ): { scheduleId: string; projectId: string; graphId?: string; scheduleTaskId?: string } => ({
    scheduleId: String(search.scheduleId ?? ''),
    projectId: String(search.projectId ?? ''),
    graphId: optStr(search.graphId),
    // Present → a single run (scheduled/task/info + its jobs); absent → whole schedule.
    scheduleTaskId: optStr(search.scheduleTaskId),
  }),
  component: PeriodicTaskDetailPage,
});

export const allDataSourcesRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/all-data-sources',
  component: AllDataSourcesPage,
});

export const allDataTablesRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/all-data-tables',
  component: AllDataTablesPage,
});

export const instRegisterRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/inst-register',
  component: InstRegisterPage,
});

/** Security audit trail (ADMIN / AUDITOR; enforced by the backend). */
export const auditRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/audit',
  component: AuditLogPage,
});

/** Node-context view (legacy `/node?ownerId=`): tabs scoped to one node. */
export const nodeLayoutRoute = createRoute({
  getParentRoute: () => appRoute,
  path: '/node/$nodeId',
  component: NodeLayoutPage,
});

export const nodeIndexRoute = createRoute({
  getParentRoute: () => nodeLayoutRoute,
  path: '/',
  beforeLoad: ({ params }) => {
    // TEE nodes have no datasource tab.
    throw redirect({
      to: params.nodeId === 'tee' ? '/node/$nodeId/data-tables' : '/node/$nodeId/data-sources',
      params: { nodeId: params.nodeId },
    });
  },
});

export const nodeDataSourcesRoute = createRoute({
  getParentRoute: () => nodeLayoutRoute,
  path: '/data-sources',
  component: DataSourcesPage,
});

export const nodeDataTablesRoute = createRoute({
  getParentRoute: () => nodeLayoutRoute,
  path: '/data-tables',
  component: DataTablesPage,
});

export const nodeCooperativeNodesRoute = createRoute({
  getParentRoute: () => nodeLayoutRoute,
  path: '/cooperative-nodes',
  component: NodeRoutesPage,
});

export const nodeResultsRoute = createRoute({
  getParentRoute: () => nodeLayoutRoute,
  path: '/results',
  component: ResultsPage,
});

const routeTree = rootRoute.addChildren([
  loginRoute,
  changePasswordRoute,
  appRoute.addChildren([
    indexRoute,
    dashboardRoute,
    projectsRoute,
    nodesRoute,
    dataTablesRoute,
    dataSourcesRoute,
    dataSourceDetailRoute,
    dagRoute,
    predefinedCanvasesRoute,
    graphsRoute,
    modelsRoute,
    resultsRoute,
    jobRecordsRoute,
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
    periodicTaskDetailRoute,
    allDataSourcesRoute,
    allDataTablesRoute,
    instRegisterRoute,
    auditRoute,
    nodeLayoutRoute.addChildren([
      nodeIndexRoute,
      nodeDataSourcesRoute,
      nodeDataTablesRoute,
      nodeCooperativeNodesRoute,
      nodeResultsRoute,
    ]),
  ]),
]);

export const router = createRouter({ routeTree });

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router;
  }
}
