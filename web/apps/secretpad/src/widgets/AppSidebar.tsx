import React from 'react';
import { Link, useRouterState } from '@tanstack/react-router';
import { useTranslation } from '../shared/lib/i18n';
import { usePlatform } from '../shared/lib/platform';

export const AppSidebar: React.FC = () => {
  const { t } = useTranslation();
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  const { isP2p } = usePlatform();

  const menuItems = isP2p
    ? [
        { section: t('sidebar.overview') },
        { path: '/dashboard', label: t('sidebar.dashboard'), icon: '📊' },
        { path: '/workbench', label: t('sidebar.workbench'), icon: '🧭' },
        { path: '/guide', label: t('sidebar.guide'), icon: '🚀' },
        { section: t('sidebar.p2p') },
        { path: '/p2p/projects', label: t('sidebar.p2pProjects'), icon: '📁' },
        { path: '/p2p/my-node', label: t('sidebar.p2pMyNode'), icon: '🖥️' },
        { section: t('sidebar.resources') },
        { path: '/nodes', label: t('sidebar.nodes'), icon: '🖥️' },
        { path: '/data-tables', label: t('sidebar.dataTables'), icon: '🗄️' },
        { path: '/data-sources', label: t('sidebar.dataSources'), icon: '🔌' },
        { path: '/feature-datasource', label: t('sidebar.featureDatasource'), icon: '🧬' },
        { section: t('sidebar.governance') },
        { path: '/models', label: t('sidebar.models'), icon: '🤖' },
        { path: '/messages', label: t('sidebar.messages'), icon: '🔔' },
        { path: '/cloud-logs', label: t('sidebar.cloudLogs'), icon: '☁️' },
        { path: '/privacy-scenes', label: t('sidebar.privacyScenes'), icon: '🛡️' },
        { path: '/account', label: t('sidebar.account'), icon: '👤' },
      ]
    : [
        { section: t('sidebar.overview') },
        { path: '/dashboard', label: t('sidebar.dashboard'), icon: '📊' },
        { path: '/workbench', label: t('sidebar.workbench'), icon: '🧭' },
        { path: '/guide', label: t('sidebar.guide'), icon: '🚀' },
        { section: t('sidebar.collaboration') },
        { path: '/projects', label: t('sidebar.projects'), icon: '📁' },
        { path: '/dag', label: t('sidebar.dag'), icon: '⚡' },
        { section: t('sidebar.resources') },
        { path: '/nodes', label: t('sidebar.nodes'), icon: '🖥️' },
        { path: '/data-tables', label: t('sidebar.dataTables'), icon: '🗄️' },
        { path: '/data-sources', label: t('sidebar.dataSources'), icon: '🔌' },
        { path: '/feature-datasource', label: t('sidebar.featureDatasource'), icon: '🧬' },
        { path: '/node-routes', label: t('sidebar.nodeRoutes'), icon: '🔗' },
        { path: '/institutions', label: t('sidebar.institutions'), icon: '🏢' },
        { section: t('sidebar.governance') },
        { path: '/models', label: t('sidebar.models'), icon: '🤖' },
        { path: '/results', label: t('sidebar.results'), icon: '📦' },
        { path: '/periodic-tasks', label: t('sidebar.periodicTasks'), icon: '⏰' },
        { path: '/messages', label: t('sidebar.messages'), icon: '🔔' },
        { path: '/cloud-logs', label: t('sidebar.cloudLogs'), icon: '☁️' },
        { path: '/component-versions', label: t('sidebar.componentVersions'), icon: '🏷️' },
        { path: '/privacy-scenes', label: t('sidebar.privacyScenes'), icon: '🛡️' },
        { path: '/account', label: t('sidebar.account'), icon: '👤' },
      ];

  return (
    <aside className="w-56 bg-gray-900 text-gray-300 flex flex-col flex-shrink-0 border-r border-gray-800 select-none">
      {/* Brand Logo */}
      <div className="h-14 px-4 flex items-center gap-3 border-b border-gray-800">
        <div className="w-8 h-8 rounded-lg bg-gradient-to-br from-blue-500 to-purple-600 flex items-center justify-center font-bold text-white text-sm shadow-md">
          SP
        </div>
        <div className="flex flex-col">
          <span className="font-bold text-gray-100 text-sm tracking-wide">SecretPad</span>
          <span className="text-[10px] text-blue-400 font-mono">v3.0.0</span>
        </div>
      </div>

      {/* Nav List */}
      <nav className="flex-1 px-3 py-4 space-y-1 overflow-y-auto">
        {menuItems.map((item, idx) => {
          if (item.section) {
            return (
              <div key={idx} className="pt-3 pb-1 px-2 text-[10px] font-bold text-gray-500 uppercase tracking-wider">
                {item.section}
              </div>
            );
          }

          const isActive = pathname === item.path || (item.path !== '/dashboard' && pathname.startsWith(item.path!));
          return (
            <Link
              key={item.path}
              to={item.path}
              className={`w-full flex items-center justify-between px-3 py-2 rounded-lg text-xs font-medium transition-all ${
                isActive
                  ? 'bg-blue-600 text-white shadow-sm font-semibold'
                  : 'text-gray-400 hover:bg-gray-800/80 hover:text-gray-200'
              }`}
            >
              <div className="flex items-center gap-2.5">
                <span className="text-sm">{item.icon}</span>
                <span>{item.label}</span>
              </div>
            </Link>
          );
        })}
      </nav>

      {/* Footer Info */}
      <div className="p-3 border-t border-gray-800 bg-gray-950/50 flex items-center justify-between text-[11px] text-gray-500">
        <span className="flex items-center gap-1.5">
          <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse" />
          Kuscia Master
        </span>
        <span className="font-mono text-gray-600">v0.5.0</span>
      </div>
    </aside>
  );
};
