import React from 'react';
import type { NodeInstanceJava, NodeResourceJava } from '@secretpad/api-client';
import { useTranslation } from '@/shared/lib/i18n';
import { formatTime } from '@/features/cooperative-node/format';
import { NodeStatusBadge } from '@/features/cooperative-node/ui-common';
import { resourceUsage } from './node-helpers';

/** Node instances (legacy my-node "节点实例") + optional resources. */
export const NodeInstancesSection: React.FC<{ instances?: NodeInstanceJava[]; resources?: NodeResourceJava[] }> = ({
  instances = [],
  resources = [],
}) => {
  const { t } = useTranslation();
  const ready = instances.filter((i) => i.status === 'Ready').length;
  return (
    <div className="space-y-3">
      <div>
        <div className="text-sm font-semibold text-gray-800 dark:text-gray-200">{t('nodes.instances')}</div>
        <p className="text-[11px] text-gray-400 mt-0.5">{t('nodes.instancesHint')}</p>
      </div>
      <div className="text-xs text-gray-600 dark:text-gray-300">
        {t('nodes.instancesReady')}: <span className="font-semibold">{ready}</span> / {instances.length}
        <span className="ml-2 inline-flex gap-0.5 align-middle">
          {instances.map((i, idx) => (
            <span key={idx} className={`w-2.5 h-2.5 rounded-sm ${i.status === 'Ready' ? 'bg-emerald-500' : 'bg-gray-300 dark:bg-gray-600'}`} />
          ))}
        </span>
      </div>
      <div className="overflow-x-auto rounded-lg border border-gray-100 dark:border-gray-800">
        <table className="w-full text-left text-xs">
          <thead className="bg-gray-50 dark:bg-gray-850 text-gray-500">
            <tr>
              <th className="p-2">HostName</th>
              <th className="p-2">{t('nodes.status')}</th>
              <th className="p-2">{t('nodes.instanceVersion')}</th>
              <th className="p-2">{t('nodes.instanceCreateTime')}</th>
              <th className="p-2">{t('nodes.instanceHeartbeat')}</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-100 dark:divide-gray-800">
            {instances.length === 0 && (
              <tr>
                <td colSpan={5} className="p-3 text-center text-gray-400">
                  {t('common.empty')}
                </td>
              </tr>
            )}
            {instances.map((i, idx) => (
              <tr key={i.name || idx}>
                <td className="p-2 font-mono">{i.name || '-'}</td>
                <td className="p-2">
                  <NodeStatusBadge status={i.status} />
                </td>
                <td className="p-2">{i.version || '-'}</td>
                <td className="p-2 text-gray-500">{formatTime(i.lastTransitionTime)}</td>
                <td className="p-2 text-gray-500">{formatTime(i.lastHeartbeatTime)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {resources.length > 0 && (
        <div className="space-y-2">
          <div className="text-sm font-semibold text-gray-800 dark:text-gray-200">{t('nodes.resources')}</div>
          {resources.map((r) => {
            const pct = resourceUsage(r);
            return (
              <div key={r.name} className="text-xs">
                <div className="flex justify-between text-gray-600 dark:text-gray-300">
                  <span className="uppercase">{r.name}</span>
                  <span className="font-mono text-gray-400">
                    {t('nodes.allocatable')} {r.allocatable || '-'} / {t('nodes.capacity')} {r.capacity || '-'}
                  </span>
                </div>
                <div className="h-1.5 mt-1 rounded bg-gray-100 dark:bg-gray-800 overflow-hidden">
                  <div className="h-full bg-blue-500" style={{ width: `${pct}%` }} />
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
};
