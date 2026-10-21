import React, { useId, useState } from 'react';
import type { Project } from '@secretpad/api-client';
import { Button } from '@secretpad/design-system';
import { useTranslation } from '../../shared/lib/i18n';

/** Bound the visible card count so large project collections remain navigable.
 * Paging and folding are local display state and never mutate project data.
 */
export type ProjectView = 'list' | 'cards' | 'all';

export function ProjectGroup({ title, projects, children, view = 'cards' }: {
  title: string; projects: Project[]; view?: ProjectView; children: (project: Project) => React.ReactNode;
}) {
  const { t } = useTranslation();
  const id = useId();
  const [expanded, setExpanded] = useState(true);
  const [page, setPage] = useState(0);
  const pages = Math.ceil(projects.length / 6);
  // Deletion/refetch may remove the last page; keep the remaining cards visible.
  const current = Math.min(page, Math.max(0, pages - 1));
  const visible = view === 'cards' ? projects.slice(current * 6, (current + 1) * 6) : projects;
  return <section className="rounded-2xl border border-gray-200 bg-gray-50/50 dark:border-gray-800 dark:bg-gray-900/30">
    <button type="button" aria-expanded={expanded} aria-controls={id} onClick={() => setExpanded(value => !value)} className="flex w-full items-center gap-3 p-4 text-left font-semibold text-gray-900 dark:text-gray-100">
      <span aria-hidden="true" className={`text-blue-500 transition-transform ${expanded ? 'rotate-90' : ''}`}>▸</span>
      <span>{title}</span><span className="rounded-full bg-blue-50 px-2 py-0.5 text-xs text-blue-600 dark:bg-blue-950 dark:text-blue-300">{projects.length}</span>
    </button>
    <div id={id} hidden={!expanded} className="px-4 pb-4">
      <div className={view === 'list' ? 'overflow-x-auto rounded-xl border border-gray-200 bg-white dark:border-gray-800 dark:bg-gray-900' : 'grid grid-cols-1 gap-4 xl:grid-cols-2 2xl:grid-cols-3'}>
        {view === 'list' && <div className="grid min-w-[980px] grid-cols-[minmax(160px,2fr)_80px_90px_70px_70px_minmax(100px,1fr)_200px] items-center gap-3 border-b border-gray-200 bg-gray-50 px-4 py-2 text-xs font-semibold text-gray-500 dark:border-gray-700 dark:bg-gray-800" aria-hidden="true">
          <span>{t('projects.nameLabel')}</span><span>{t('projects.modeLabel')}</span><span>{t('projectBrowser.status')}</span><span>{t('projects.graphCount')}</span><span>{t('projects.jobCount')}</span><span>{t('projects.joinedNodes')}</span><span>{t('projectBrowser.actions')}</span>
        </div>}
        {visible.map(children)}
      </div>
      {view === 'cards' && pages > 1 && <div className="mt-4 flex flex-wrap items-center justify-end gap-3 text-xs text-gray-500">
        <span>{t('projectBrowser.page', {page: current + 1, pages})}</span>
        <Button size="sm" variant="outline" disabled={current === 0} onClick={() => setPage(current - 1)}>{t('projectBrowser.previous')}</Button>
        <Button size="sm" variant="outline" disabled={current + 1 >= pages} onClick={() => setPage(current + 1)}>{t('projectBrowser.next')}</Button>
      </div>}
    </div>
  </section>;
}
