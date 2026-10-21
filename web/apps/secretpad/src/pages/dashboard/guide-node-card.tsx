/**
 * Dashboard guide-node card (port of legacy `modules/guide-node`): shows the
 * built-in nodes with their authorized-node count and datatable count, plus CTA
 * buttons to create a project (wizard with the nodes preselected) or manage data.
 */
import React, { useState } from 'react';
import { useNavigate } from '@tanstack/react-router';
import { useQuery } from '@tanstack/react-query';
import { Button, Card } from '@secretpad/design-system';
import { listNodesJava } from '@secretpad/api-client';
import { useTranslation } from '../../shared/lib/i18n';
import { usePlatform } from '../../shared/lib/platform';
import { CreateProjectWizard } from '../../features/create-project';
import { projectPermissions } from '../projects/project-list.logic';
import { authorizedNodeIds, guideNodes } from './guide-node.logic';

export const GuideNodeCard: React.FC<{ tableCounts?: Record<string, number> }> = ({ tableCounts = {} }) => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const platform = usePlatform();
  const { canCreate } = projectPermissions(platform);
  const [wizardOpen, setWizardOpen] = useState(false);

  const nodesQuery = useQuery({ queryKey: ['nodes-java'], queryFn: listNodesJava });
  const nodes = guideNodes(nodesQuery.data ?? []);

  return (
    <Card title={t('guideNode.title')} data-tour="guide-node">
      <div className="space-y-3">
        {nodesQuery.isLoading && <div className="text-xs text-gray-400">{t('common.loading')}</div>}
        {!nodesQuery.isLoading && nodes.length === 0 && (
          <div className="text-xs text-gray-400 text-center py-2">{t('nodes.noNodes')}</div>
        )}
        {nodes.map((node) => {
          const authorized = authorizedNodeIds(node.nodeRoutes, node.nodeId);
          const tables = node.datatables && node.datatables.length > 0 ? node.datatables.length : tableCounts[node.nodeId] ?? 0;
          return (
            <div key={node.nodeId} className="p-3 rounded-lg border border-gray-100 dark:border-gray-800 text-xs">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2 min-w-0">
                  <span className="w-2 h-2 rounded-full bg-emerald-500 flex-shrink-0" />
                  <span className="font-semibold text-gray-800 dark:text-gray-200 truncate">{node.nodeName || node.nodeId}</span>
                </div>
                <span
                  className="px-2 py-0.5 rounded bg-blue-50 dark:bg-blue-950/50 text-blue-600 dark:text-blue-300"
                  title={authorized.join(', ')}
                >
                  {t('guideNode.authorized', { count: authorized.length })}
                </span>
              </div>
              <div className="mt-1 text-[11px] text-gray-400 font-mono truncate">ID: {node.nodeId}</div>
              <div className="mt-1.5 text-gray-500">
                {t('guideNode.tables', { count: tables })}
                {node.datatables?.[0]?.datatableName && <span className="ml-1 text-gray-400">({node.datatables[0].datatableName}…)</span>}
              </div>
            </div>
          );
        })}
        <div className="grid grid-cols-2 gap-2 pt-1">
          {canCreate && (
            <Button variant="primary" size="sm" onClick={() => setWizardOpen(true)}>
              👉 {t('guideNode.createProject')}
            </Button>
          )}
          <Button variant="outline" size="sm" onClick={() => navigate({ to: '/data-tables' })}>
            🗄️ {t('guideNode.manageData')}
          </Button>
        </div>
      </div>
      <CreateProjectWizard
        isOpen={wizardOpen}
        onClose={() => setWizardOpen(false)}
        preset={{ nodeIds: nodes.slice(0, 2).map((n) => n.nodeId) }}
      />
    </Card>
  );
};
