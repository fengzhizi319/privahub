import React from 'react';
import { NavigationIcon } from './navigation-icon';
import { Link, useRouterState } from '@tanstack/react-router';
import { useTranslation } from '../shared/lib/i18n';
import { usePlatformContext } from '../shared/lib/platform';
import { getMenu, homeHref, isMenuActive } from '../shared/lib/access';

export const AppSidebar: React.FC<{ onNavigate?: () => void }> = ({ onNavigate }) => {
  const { t } = useTranslation();
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  const ctx = usePlatformContext();

  // Menus differ per platformType / ownerType (CENTER admin, EDGE account on
  // CENTER, EDGE node view, AUTONOMY/P2P workbench) — see shared/lib/access.ts.
  const menuItems = getMenu(ctx).map((item) => ({
    ...item,
    section: item.section ? t(item.section) : undefined,
    label: item.labelKey ? t(item.labelKey) : '',
    href: item.path ? homeHref({ to: item.path, params: item.params }) : '',
  }));

  return (
    <aside className="w-60 h-full bg-[#101b30] text-slate-300 flex flex-col flex-shrink-0 border-r border-white/5 select-none">
      {/* Brand Logo */}
      <div className="h-[76px] px-6 flex items-center gap-3 border-b border-white/5">
        <div className="w-9 h-9 rounded-xl bg-blue-500/15 border border-blue-400/30 flex items-center justify-center text-blue-300">
          <NavigationIcon name="shield" className="w-5 h-5" />
        </div>
        <div className="flex flex-col">
          <span className="font-bold text-gray-100 text-sm tracking-wide">Privahub</span>
          <span className="text-[10px] text-slate-400 tracking-[0.16em] uppercase">Privacy Computing</span>
        </div>
      </div>

      {/* Nav List */}
      <nav aria-label={t('header.navigation')} className="flex-1 px-3 py-4 space-y-1 overflow-y-auto">
        {menuItems.map((item) => {
          if (item.section) {
            return (
              <div key={item.section} className="pt-5 pb-2 px-3 text-[10px] font-semibold text-slate-500 uppercase tracking-[0.16em]">
                {item.section}
              </div>
            );
          }

          const isActive = isMenuActive(pathname, item.href);
          return (
            <Link
              key={item.href}
              {...({ to: item.path, params: item.params, search: item.search } as React.ComponentProps<typeof Link>)}
              onClick={onNavigate}
              aria-current={isActive ? 'page' : undefined}
              className={`w-full flex items-center justify-between px-3 py-2.5 rounded-xl text-[13px] font-medium transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-blue-300 ${
                isActive
                  ? 'bg-blue-500/15 text-blue-200 ring-1 ring-inset ring-blue-400/20 font-semibold'
                  : 'text-slate-400 hover:bg-white/5 hover:text-slate-100'
              }`}
            >
              <div className="flex items-center gap-2.5">
                <NavigationIcon name={item.href} />
                <span>{item.label}</span>
              </div>
            </Link>
          );
        })}
      </nav>

      {/* Show platform identity, never a hard-coded "online" health claim. */}
      <div className="m-3 p-3 rounded-xl border border-white/10 bg-white/[0.03] flex items-center gap-3">
        <span className="p-2 rounded-lg bg-white/5 text-slate-400"><NavigationIcon name="shield" /></span>
        <div className="min-w-0 text-xs">
          <div className="font-medium text-slate-200">{t(`header.platform.${ctx.platformType}`)}</div>
          <div className="mt-1 truncate text-[11px] text-slate-500">{ctx.ownerId || 'Privahub'}</div>
        </div>
      </div>
    </aside>
  );
};
