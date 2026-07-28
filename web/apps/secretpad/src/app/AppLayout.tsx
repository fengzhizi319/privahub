import React, { useEffect } from 'react';
import { Outlet, useRouterState } from '@tanstack/react-router';
import { AppSidebar } from '../widgets/AppSidebar';
import { AppHeader } from '../widgets/AppHeader';
import { useTranslation } from '../shared/lib/i18n';
import { RouteGuard } from '../features/auth/ui/access-guard';
import { useAuthStore } from '../features/auth/model/auth-store';

/**
 * Authenticated application shell: sidebar + header + routed content.
 * Rendered by the `app` layout route; child routes mount into <Outlet />.
 */
export const AppLayout: React.FC = () => {
  const { t } = useTranslation();
  const { rehydrate } = useAuthStore();
  const pathname = useRouterState({ select: (s) => s.location.pathname });

  // Sync the auth store (user/platform) from localStorage on shell mount.
  useEffect(() => {
    rehydrate();
  }, [rehydrate]);

  const getTitle = () => {
    switch (pathname) {
      case '/dashboard':
        return t('dashboard.title');
      case '/projects':
        return t('projects.title');
      case '/nodes':
        return t('nodes.title');
      case '/data-tables':
        return t('dataTables.title');
      case '/data-sources':
        return t('dataSources.title');
      case '/data-sources/detail':
        return t('dataSources.detailTitle');
      case '/dag':
        return t('sidebar.dag');
      case '/models':
        return t('sidebar.models');
      case '/results':
        return t('sidebar.results');
      case '/periodic-tasks':
        return t('sidebar.periodicTasks');
      case '/messages':
        return t('sidebar.messages');
      case '/node-routes':
        return t('nodeRoutes.title');
      case '/institutions':
        return t('institutions.title');
      case '/p2p/projects':
        return t('p2p.projectsTitle');
      case '/p2p/my-node':
        return t('p2p.myNodeTitle');
      case '/account':
        return t('account.title');
      default:
        return t('app.title');
    }
  };

  return (
    <div className="flex h-screen w-screen overflow-hidden bg-slate-50 dark:bg-gray-950 text-gray-900 dark:text-gray-100 font-sans">
      <AppSidebar />
      <div className="flex-1 flex flex-col min-w-0 overflow-hidden">
        <AppHeader title={getTitle()} />
        <main className="flex-1 overflow-y-auto p-6">
          <RouteGuard>
            <Outlet />
          </RouteGuard>
        </main>
      </div>
    </div>
  );
};
