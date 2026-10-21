import { NavigationIcon } from './navigation-icon';
import React, { useEffect, useRef, useState } from 'react';
import { useNavigate } from '@tanstack/react-router';
import { useQuery } from '@tanstack/react-query';
import { apiClient } from '@secretpad/api-client';
import { Button } from '@secretpad/design-system';
import { useAuthStore } from '../features/auth/model/auth-store';
import { useGuideTourStore } from '../features/guide-tour';
import { useThemeStore } from '../shared/lib/theme';
import type { ThemeMode } from '../shared/lib/theme';
import type { Locale } from '../shared/lib/i18n';
import { useTranslation } from '../shared/lib/i18n';
import { usePlatform } from '../shared/lib/platform';
import { HELP_LINKS, formatBadgeCount } from './header-utils';

export interface AppHeaderProps {
  title?: string;
  onOpenNavigation?: () => void;
}

/** Pending message count polling interval (ms). */
const PENDING_POLL_MS = 30_000;

export const AppHeader: React.FC<AppHeaderProps> = ({ title = 'Console Overview', onOpenNavigation }) => {
  const { user, logout } = useAuthStore();
  const { mode, setMode } = useThemeStore();
  const { t, locale, setLocale } = useTranslation();
  const navigate = useNavigate();
  const { platformType, ownerId, isAutonomy, isEdge, isCenterAdmin, deployMode } = usePlatform();
  const startTour = useGuideTourStore((s) => s.start);
  const [menuOpen, setMenuOpen] = useState(false);
  const [versionsOpen, setVersionsOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);

  // Real pending message count (message/pending), polled.
  const pendingQuery = useQuery({
    queryKey: ['message-pending', ownerId],
    queryFn: () => apiClient.getPendingMessageCount(ownerId),
    enabled: !!ownerId,
    refetchInterval: PENDING_POLL_MS,
    retry: false,
  });
  const pending = pendingQuery.data ?? 0;

  const versionsQuery = useQuery({
    queryKey: ['component-versions-header'],
    queryFn: () => apiClient.listComponentVersions(),
    enabled: versionsOpen,
    retry: false,
  });

  useEffect(() => {
    if (!menuOpen) return;
    const onDoc = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setMenuOpen(false);
        setVersionsOpen(false);
      }
    };
    document.addEventListener('mousedown', onDoc);
    return () => document.removeEventListener('mousedown', onDoc);
  }, [menuOpen]);

  const handleLogout = async () => {
    setMenuOpen(false);
    try {
      await logout();
    } finally {
      navigate({ to: '/login' });
    }
  };

  const reopenGuide = () => {
    try {
      localStorage.removeItem('secretpad-guide-tour-done');
    } catch {
      /* ignore */
    }
    startTour(0);
    navigate({ to: '/guide' });
  };

  const openExternal = (url: string) => window.open(url, '_blank', 'noopener,noreferrer');

  const subtitle = t(`header.platform.${platformType}`);
  // "My institution" (AUTONOMY) / "My node" (EDGE) shortcut.
  const showMyInst = isAutonomy || isEdge;

  const versionEntries = Object.entries((versionsQuery.data || {}) as Record<string, unknown>).filter(
    ([, v]) => typeof v === 'string' && v,
  );

  return (
    <header className="h-14 sm:h-[76px] bg-white/90 dark:bg-gray-900/90 border-b border-gray-200/70 dark:border-gray-800 px-3 sm:px-6 lg:px-8 gap-2 sm:gap-3 flex items-center justify-between flex-shrink-0">
      <div className="flex flex-1 items-center gap-2 sm:gap-3 min-w-0">
        <button type="button" aria-label={t('header.openNavigation')} className="lg:hidden shrink-0 rounded-lg p-2 text-slate-500 hover:bg-slate-100 dark:hover:bg-slate-800" onClick={onOpenNavigation}><NavigationIcon name="menu" /></button>
        <h1 className="min-w-0 text-sm sm:text-base font-semibold text-gray-900 dark:text-gray-100 truncate">{title}</h1>
        <span className="hidden 2xl:inline-flex px-2 py-0.5 rounded-md text-[11px] font-mono bg-blue-50 dark:bg-blue-950 text-blue-600 dark:text-blue-400 border border-blue-200 dark:border-blue-800 whitespace-nowrap">
          {subtitle} · {ownerId || '-'}
          {deployMode ? ` · ${deployMode}` : ''}
        </span>
        {showMyInst && (
          <button
            type="button"
            onClick={() => navigate({ to: '/p2p/my-node' })}
            className="hidden sm:inline-flex text-xs text-gray-500 hover:text-blue-600 whitespace-nowrap"
          >
            🏢 {isAutonomy ? t('header.myInstitution') : t('header.myNode')}
          </button>
        )}
      </div>

      <div className="flex shrink-0 items-center gap-1.5 sm:gap-3">
        {isCenterAdmin && (
          <Button className="hidden 2xl:inline-flex whitespace-nowrap" size="sm" variant="outline" onClick={reopenGuide} title={t('header.reopenGuide')}>
            {t('header.reopenGuide')}
          </Button>
        )}
        <button
          type="button"
          onClick={() => openExternal(HELP_LINKS.community)}
          className="hidden 2xl:inline text-xs whitespace-nowrap text-gray-500 hover:text-blue-600"
        >
          {t('header.community')}
        </button>
        <button
          type="button"
          onClick={() => openExternal(HELP_LINKS.help)}
          className="hidden 2xl:inline text-xs whitespace-nowrap text-gray-500 hover:text-blue-600"
        >
          {t('header.helpCenter')}
        </button>
        {import.meta.env.DEV && (
          // 开发期：后端 OpenAPI 文档（GET /api/v1alpha1/docs）。
          <a href="/api/v1alpha1/docs" target="_blank" rel="noreferrer" className="hidden 2xl:inline text-xs whitespace-nowrap text-gray-500 hover:text-blue-600">
            API
          </a>
        )}

        <select
          value={locale}
          onChange={(e) => setLocale(e.target.value as Locale)}
          aria-label={t('header.language')}
          className="hidden sm:block text-xs bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg px-2 py-1 text-gray-700 dark:text-gray-200 focus:outline-none focus:border-blue-500"
        >
          <option value="zh-CN">{t('header.zh')}</option>
          <option value="en-US">{t('header.en')}</option>
        </select>

        <select
          value={mode}
          onChange={(e) => setMode(e.target.value as ThemeMode)}
          aria-label={t('header.theme')}
          title={t('header.theme')}
          className="hidden sm:block text-xs bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg px-2 py-1 text-gray-700 dark:text-gray-200 focus:outline-none focus:border-blue-500"
        >
          <option value="light">{t('header.light')}</option>
          <option value="dark">{t('header.dark')}</option>
          <option value="system">{t('header.system')}</option>
        </select>

        <button
          type="button"
          onClick={() => navigate({ to: '/messages' })}
          aria-label={t('header.messages')}
          title={t('header.messages')}
          className="relative p-1.5 text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-800 transition-colors"
        >
          <NavigationIcon name="bell" />
          {pending > 0 && (
            <span
              data-testid="pending-badge"
              className="absolute -top-0.5 -right-1 min-w-[16px] h-4 px-1 rounded-full bg-red-500 text-white text-[10px] leading-4 text-center"
            >
              {formatBadgeCount(pending)}
            </span>
          )}
        </button>

        <div className="h-4 w-px bg-gray-200 dark:bg-gray-800" />

        <div className="relative" ref={menuRef}>
          <button
            type="button"
            className="flex items-center gap-2.5 pl-1"
            onClick={() => setMenuOpen((v) => !v)}
            aria-haspopup="menu"
            aria-expanded={menuOpen}
          >
            <div className="w-8 h-8 rounded-full bg-gradient-to-br from-blue-500 to-indigo-600 flex items-center justify-center font-bold text-white text-xs">
              {user?.name?.[0]?.toUpperCase() || '?'}
            </div>
            <div className="hidden sm:flex flex-col text-xs text-left">
              <span className="font-semibold text-gray-800 dark:text-gray-200">{user?.name || '-'}</span>
              <span className="text-[10px] text-gray-400">{user?.ownerType || platformType}</span>
            </div>
            <span className="text-gray-400 text-xs">▾</span>
          </button>
          {menuOpen && (
            <div
              role="menu"
              className="absolute right-0 mt-2 w-56 z-40 bg-white dark:bg-gray-900 border border-gray-200 dark:border-gray-800 rounded-xl shadow-lg py-1 text-sm"
            >
              <button
                role="menuitem"
                type="button"
                className="w-full text-left px-4 py-2 hover:bg-gray-50 dark:hover:bg-gray-800"
                onClick={() => setVersionsOpen((v) => !v)}
              >
                🏷️ {t('header.componentVersions')}
              </button>
              {versionsOpen && (
                <div className="px-4 pb-2 text-[11px] text-gray-500 space-y-0.5 max-h-40 overflow-y-auto">
                  {versionsQuery.isLoading && <div>{t('app.loading')}</div>}
                  {!versionsQuery.isLoading && versionEntries.length === 0 && <div>—</div>}
                  {versionEntries.map(([k, v]) => (
                    <div key={k} className="font-mono break-all">
                      {k.replace(/Image$/, '')}: {String(v)}
                    </div>
                  ))}
                </div>
              )}
              {showMyInst && (
                <button
                  role="menuitem"
                  type="button"
                  className="w-full text-left px-4 py-2 hover:bg-gray-50 dark:hover:bg-gray-800"
                  onClick={() => {
                    setMenuOpen(false);
                    navigate({ to: '/p2p/my-node' });
                  }}
                >
                  🏢 {isAutonomy ? t('header.myInstitution') : t('header.myNode')}
                </button>
              )}
              <button
                role="menuitem"
                type="button"
                className="w-full text-left px-4 py-2 hover:bg-gray-50 dark:hover:bg-gray-800"
                onClick={() => {
                  setMenuOpen(false);
                  navigate({ to: '/account' });
                }}
              >
                🔑 {t('header.changePassword')}
              </button>
              <button
                role="menuitem"
                type="button"
                className="w-full text-left px-4 py-2 hover:bg-gray-50 dark:hover:bg-gray-800 sm:hidden"
                onClick={() => {
                  setLocale(locale === 'zh-CN' ? 'en-US' : 'zh-CN');
                  setMenuOpen(false);
                }}
              >
                🌐 {t('header.language')}: {locale === 'zh-CN' ? t('header.en') : t('header.zh')}
              </button>
              <div className="px-4 py-2 sm:hidden">
                <div className="text-[11px] text-gray-500 mb-1">◐ {t('header.theme')}</div>
                <div className="flex gap-1">
                  {(['light', 'dark', 'system'] as ThemeMode[]).map((m) => (
                    <button
                      key={m}
                      type="button"
                      onClick={() => {
                        setMode(m);
                        setMenuOpen(false);
                      }}
                      className={`flex-1 px-2 py-1 rounded-lg border text-[11px] ${
                        mode === m
                          ? 'border-blue-500 text-blue-600 dark:text-blue-400 bg-blue-50 dark:bg-blue-950/40'
                          : 'border-gray-200 dark:border-gray-700 text-gray-600 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-800'
                      }`}
                    >
                      {t(`header.${m}`)}
                    </button>
                  ))}
                </div>
              </div>
              <button
                role="menuitem"
                type="button"
                className="w-full text-left px-4 py-2 text-red-600 hover:bg-red-50 dark:hover:bg-red-950/30"
                onClick={handleLogout}
              >
                ➔ {t('header.logout')}
              </button>
            </div>
          )}
        </div>
      </div>
    </header>
  );
};
