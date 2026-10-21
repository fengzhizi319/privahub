/**
 * Concept explanation section of the guide page (legacy `modules/guide` +
 * `guide-pipeline`): node → datasource → datatable → project → pipeline (DAG) →
 * job / results, with a simple HTML/CSS flow diagram.
 */
import React from 'react';
import { Card } from '@secretpad/design-system';
import { useTranslation } from '../../shared/lib/i18n';

const GUIDE_CONCEPTS = [
  { key: 'node', icon: '🖥️' },
  { key: 'datasource', icon: '🔌' },
  { key: 'datatable', icon: '🗄️' },
  { key: 'project', icon: '📁' },
  { key: 'pipeline', icon: '⚡' },
  { key: 'result', icon: '📦' },
] as const;

export const GuideConcepts: React.FC = () => {
  const { t } = useTranslation();
  return (
    <Card title={t('guideConcepts.title')} bodyClassName="p-5 space-y-5">
      {/* Flow diagram */}
      <div className="overflow-x-auto">
        <ol className="flex items-stretch gap-0 min-w-max" aria-label={t('guideConcepts.flowLabel')}>
          {GUIDE_CONCEPTS.map((c, idx) => (
            <li key={c.key} className="flex items-center">
              <div className="w-28 flex flex-col items-center text-center px-2 py-3 rounded-xl border border-blue-200 dark:border-blue-900 bg-blue-50/60 dark:bg-blue-950/30">
                <span className="text-xl">{c.icon}</span>
                <span className="mt-1 text-[11px] font-semibold text-gray-800 dark:text-gray-200">
                  {t(`guideConcepts.${c.key}.name`)}
                </span>
              </div>
              {idx < GUIDE_CONCEPTS.length - 1 && (
                <div className="flex items-center px-1" aria-hidden>
                  <div className="w-5 h-0.5 bg-blue-300 dark:bg-blue-800" />
                  <div className="w-0 h-0 border-y-4 border-y-transparent border-l-[6px] border-l-blue-300 dark:border-l-blue-800" />
                </div>
              )}
            </li>
          ))}
        </ol>
      </div>
      {/* Concept explanations */}
      <dl className="grid grid-cols-1 md:grid-cols-2 gap-3 text-xs">
        {GUIDE_CONCEPTS.map((c) => (
          <div key={c.key} className="p-3 rounded-lg border border-gray-100 dark:border-gray-800">
            <dt className="font-semibold text-gray-900 dark:text-gray-100">
              {c.icon} {t(`guideConcepts.${c.key}.name`)}
            </dt>
            <dd className="mt-1 text-gray-500 dark:text-gray-400 leading-relaxed">{t(`guideConcepts.${c.key}.desc`)}</dd>
          </div>
        ))}
      </dl>
    </Card>
  );
};
