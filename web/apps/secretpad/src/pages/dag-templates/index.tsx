import React, { useState } from 'react';
import { Link } from '@tanstack/react-router';
import { Button } from '@secretpad/design-system';
import { allTemplates, categoryNameKey } from '../../features/dag-templates/registry';
import { buildTemplateConfigs } from '../../features/create-project/templates';
import { CreateProjectWizard, type CreateProjectPreset } from '../../features/create-project';
import { useTranslation } from '../../shared/lib/i18n';
import { usePlatform } from '../../shared/lib/platform';
import { projectPermissions } from '../projects/project-list.logic';
import { scenePreset } from '../privacy-scenes/scenes';

/** A template is a read-only recipe. Using it creates a separate project/graph;
 * preview switches replace the complete SVG tree and never touch saved graphs.
 */
export const PredefinedCanvasesPage: React.FC = () => {
  const { t } = useTranslation();
  const platform = usePlatform();
  const { canCreate } = projectPermissions(platform);
  const templates = allTemplates.filter(template => template.metadata.key !== 'blank');
  const [key, setKey] = useState('logisticRegression');
  const [search, setSearch] = useState('');
  const categories = ['ml', 'basic', 'privacy', 'guide'] as const;
  const [collapsed, setCollapsed] = useState<string[]>(['basic', 'privacy', 'guide']);
  const [fitPreview, setFitPreview] = useState(true);
  const [preset, setPreset] = useState<CreateProjectPreset | null>(null);
  const template = templates.find(item => item.metadata.key === key)!;
  const title = t(`dag.templateName.${key}`);
  const graph = template.build({ graphId: `preview-${key}`, configs: buildTemplateConfigs(key, [
    {nodeId:'alice',datatableId:'alice-table'}, {nodeId:'bob',datatableId:'bob-table'},
  ]) });
  const positions = new Map(graph.nodes.map(node => [node.graphNodeId, node]));
  const width = Math.max(500, ...graph.nodes.map(node => (node.x ?? 0) + 260));
  const height = Math.max(300, ...graph.nodes.map(node => (node.y ?? 0) + 150));
  const selectedPreset = scenePreset({key,templateKey:key,tags:[]}, title, platform);
  const filtered = templates.filter(item => t(`dag.templateName.${item.metadata.key}`).toLowerCase().includes(search.trim().toLowerCase()));
  return (
    <div className="space-y-6">
      <header className="relative overflow-hidden rounded-2xl border border-blue-100 bg-gradient-to-br from-blue-50 via-white to-indigo-50 p-6 dark:border-slate-700 dark:from-slate-900 dark:via-slate-900 dark:to-blue-950">
        <span className="text-xs font-semibold tracking-widest text-blue-600 dark:text-blue-300">DAG · TEMPLATE LIBRARY</span>
        <h1 className="mt-2 text-2xl font-bold text-slate-900 dark:text-white">{t('canvasLibrary.title')}</h1>
        <p className="mt-2 max-w-3xl text-sm leading-6 text-slate-500 dark:text-slate-400">{t('canvasLibrary.subtitle')}</p>
        <Link to="/dag" className="mt-4 inline-flex text-sm font-semibold text-blue-600 dark:text-blue-300">{t('canvasLibrary.openCustom')} →</Link>
      </header>
      <div className="grid grid-cols-1 items-start gap-5 xl:grid-cols-[340px_minmax(0,1fr)]">
        <section className="rounded-2xl border border-slate-200 bg-white p-4 dark:border-slate-800 dark:bg-slate-900">
          <label className="sr-only" htmlFor="template-search">{t('canvasLibrary.search')}</label>
          <input id="template-search" value={search} onChange={event=>setSearch(event.target.value)} placeholder={t('canvasLibrary.search')} className="mb-4 w-full rounded-xl border border-slate-200 bg-slate-50 px-3 py-2.5 text-sm dark:border-slate-700 dark:bg-slate-800" />
          <div className="max-h-[690px] space-y-2 overflow-y-auto" aria-label={t('canvasLibrary.list')}>
            {categories.map(category => {
              const items = filtered.filter(item => (item.metadata.category || 'basic') === category);
              if (!items.length) return null;
              // Search reveals matches even inside a previously collapsed group;
              // clearing it restores the user's folding choices without changing the preview.
              const expanded = !!search.trim() || !collapsed.includes(category);
              return <section key={category} className="overflow-hidden rounded-xl border border-slate-100 dark:border-slate-800">
                <button type="button" aria-expanded={expanded} aria-controls={`template-group-${category}`}
                  onClick={() => setCollapsed(previous => previous.includes(category) ? previous.filter(value => value !== category) : [...previous, category])}
                  disabled={!!search.trim()}
                  className="flex w-full items-center gap-2 bg-slate-50 px-3 py-3 text-left text-sm font-semibold text-slate-700 dark:bg-slate-800/60 dark:text-slate-200">
                  <span aria-hidden="true" className={`transition-transform ${expanded ? 'rotate-90' : ''}`}>▸</span>
                  <span className="flex-1">{t(categoryNameKey[category])}</span>
                  <span className="rounded-full bg-white px-2 py-0.5 text-xs text-slate-500 dark:bg-slate-900">{items.length}</span>
                </button>
                <div id={`template-group-${category}`} hidden={!expanded} className="space-y-1 p-1">
            {items.map(item=>(
              <button key={item.metadata.key} type="button" aria-pressed={key===item.metadata.key} onClick={()=>{setKey(item.metadata.key);setFitPreview(true);}} className={`w-full rounded-xl border p-3 text-left transition ${key===item.metadata.key?'border-blue-400 bg-blue-50 shadow-sm dark:bg-blue-950/40':'border-transparent hover:border-slate-200 hover:bg-slate-50 dark:hover:bg-slate-800'}`}>
                <span className="block text-sm font-semibold text-slate-900 dark:text-slate-100">{t(`dag.templateName.${item.metadata.key}`)}</span>
                <span className="mt-1 block text-xs leading-5 text-slate-500 dark:text-slate-400">{t(`dag.templateDesc.${item.metadata.key}`)}</span>
              </button>
            ))}
                </div>
              </section>;
            })}
            {!filtered.length && <p className="p-6 text-center text-sm text-slate-500">{t('canvasLibrary.empty')}</p>}
          </div>
        </section>
        <section className="overflow-hidden rounded-2xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900">
          <div className="flex flex-wrap items-start justify-between gap-4 border-b border-slate-100 p-5 dark:border-slate-800">
            <div><h2 className="text-lg font-bold text-slate-900 dark:text-white">{title}</h2><p className="mt-1 text-xs text-slate-500">{graph.nodes.length} {t('canvasLibrary.nodes')} · {graph.edges.length} {t('canvasLibrary.edges')} · {t('canvasLibrary.readOnly')}</p></div>
            <Button disabled={!canCreate||!selectedPreset} onClick={()=>selectedPreset&&setPreset(selectedPreset)}>{t('canvasLibrary.use')}</Button>
          </div>
          <div className="flex justify-end gap-2 border-b border-slate-100 px-5 py-2 dark:border-slate-800">
            <Button size="sm" variant="ghost" onClick={()=>setFitPreview(true)}>{t('canvasLibrary.fit')}</Button>
            <Button size="sm" variant="ghost" onClick={()=>setFitPreview(false)}>{t('canvasLibrary.actualSize')}</Button>
          </div>
          <div className="max-h-[680px] overflow-auto bg-slate-50 p-6 dark:bg-[#101b30]">
            <svg key={key} role="img" aria-label={`${title} · ${t('canvasLibrary.preview')}`} viewBox={`0 0 ${width} ${height}`} className="mx-auto" style={fitPreview?{width:'100%',minWidth:Math.min(width,500),maxHeight:620}:{width,height,maxWidth:'none'}}>
              {graph.edges.map(edge=>{
                const a=positions.get(edge.source),b=positions.get(edge.target);if(!a||!b)return null;
                const x1=a.x!+90,y1=a.y!+70,x2=b.x!+90,y2=b.y!;
                return <path key={edge.edgeId} d={`M${x1} ${y1} C${x1} ${(y1+y2)/2} ${x2} ${(y1+y2)/2} ${x2} ${y2}`} fill="none" stroke="#93b4f5" strokeWidth="2" />;
              })}
              {graph.nodes.map(node=><g key={node.graphNodeId} data-template-node={node.graphNodeId} transform={`translate(${node.x},${node.y})`}><rect width="180" height="70" rx="12" fill="white" stroke="#93b4f5" /><rect width="4" height="34" x="0" y="18" rx="2" fill="#3b82f6" /><text x="90" y="29" textAnchor="middle" fill="#172554" fontSize="14" fontWeight="600">{node.codeName?.startsWith('ml.train/')?t('canvasLibrary.train'):node.codeName?.startsWith('ml.predict/')?t('canvasLibrary.predict'):node.codeName?.startsWith('ml.eval/')?t('canvasLibrary.evaluate'):node.codeName==='data_prep/psi'?t('canvasLibrary.align'):node.codeName==='data_prep/train_test_split'?t('canvasLibrary.split'):(node.label||node.codeName||'').slice(0,20)}</text><text x="90" y="49" textAnchor="middle" fill="#64748b" fontSize="10">{node.codeName?.split('/')[0]}</text></g>)}
            </svg>
          </div>
          <p className="border-t border-slate-100 px-5 py-4 text-xs leading-5 text-slate-500 dark:border-slate-800">{t('canvasLibrary.independent')}</p>
        </section>
      </div>
      <CreateProjectWizard isOpen={!!preset} onClose={()=>setPreset(null)} preset={preset??undefined} />
    </div>
  );
};
