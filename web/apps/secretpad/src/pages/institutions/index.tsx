import React, { useState } from 'react';
import { Link } from '@tanstack/react-router';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Button, Card, Drawer } from '@secretpad/design-system';
import { deleteInstNodeJava, getInstJava, listMyInstNodesJava, type NodeDetailJava } from '@secretpad/api-client';
import { useTranslation } from '@/shared/lib/i18n';
import { usePlatform } from '@/shared/lib/platform';
import { errorText, formatTime } from '@/features/cooperative-node/format';
import { InfoRow, NodeStatusBadge, SecretText } from '@/features/cooperative-node/ui-common';
import { TokenPanel } from '../nodes/token-panel';
import { DeleteNodeDialog } from '../nodes/node-detail-drawer';

/**
 * 机构管理：机构信息（inst/get）+ 机构下节点（inst/node/list），
 * 节点 token（inst/node/token，30 分钟有效，可 inst/node/newToken 刷新）、删除节点。
 * 新增计算节点 / 切换节点在「我的机构」页完成（与旧版 my-node 一致）。
 */
export const InstitutionsPage: React.FC = () => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const platform = usePlatform();
  const [tokenNode, setTokenNode] = useState<NodeDetailJava | null>(null);
  const [deleting, setDeleting] = useState<NodeDetailJava | null>(null);

  // CENTER has no institution (owner = kuscia-system → INST_NOT_EXISTS): never call inst/* there.
  const hasInst = !platform.isCenter;
  const instQuery = useQuery({ queryKey: ['inst-info'], queryFn: getInstJava, enabled: hasInst });
  const nodesQuery = useQuery({ queryKey: ['my-inst-nodes'], queryFn: listMyInstNodesJava, enabled: hasInst });
  const nodes = nodesQuery.data ?? [];
  const inst = instQuery.data;
  const invalidate = () => void queryClient.invalidateQueries({ queryKey: ['my-inst-nodes'] });

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between bg-white dark:bg-gray-900 p-4 rounded-xl border border-gray-200 dark:border-gray-800">
        <div>
          <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">{t('institutions.title')}</h2>
          <p className="text-xs text-gray-500">{t('institutions.subtitle')}</p>
        </div>
        <div className="flex gap-2">
          <Link to="/inst-register">
            <Button variant="outline">{t('institutions.registerNode')}</Button>
          </Link>
          <Link to="/p2p/my-node">
            <Button variant="primary">{t('institutions.manageNodes')}</Button>
          </Link>
        </div>
      </div>

      {!hasInst && <div className="text-xs text-gray-500">{t('institutions.centerNoInst')}</div>}
      {(instQuery.error || nodesQuery.error) && (
        <div className="text-xs text-rose-500">{errorText(instQuery.error || nodesQuery.error)}</div>
      )}

      <Card>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4 text-xs">
          <div>
            <div className="text-gray-400 mb-1">{t('institutions.instName')}</div>
            <div className="font-semibold text-gray-800 dark:text-gray-200">{inst?.instName || '-'}</div>
          </div>
          <div>
            <div className="text-gray-400 mb-1">{t('institutions.instId')}</div>
            <div className="font-mono text-gray-600 dark:text-gray-300">{inst?.instId || '-'}</div>
          </div>
          <div>
            <div className="text-gray-400 mb-1">{t('institutions.localNode')}</div>
            <div className="font-mono text-gray-600 dark:text-gray-300">{inst?.localNodeId || '-'}</div>
          </div>
        </div>
      </Card>

      <Card bodyClassName="p-0">
        <div className="px-4 py-3 border-b border-gray-100 dark:border-gray-800 text-sm font-semibold text-gray-700 dark:text-gray-200">
          {t('institutions.nodeList')}
        </div>
        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead className="bg-gray-50 dark:bg-gray-850 text-gray-500 font-semibold border-b border-gray-200 dark:border-gray-800">
              <tr>
                <th className="p-4">{t('institutions.nodeName')}</th>
                <th className="p-4">{t('institutions.status')}</th>
                <th className="p-4">{t('nodes.netAddress')}</th>
                <th className="p-4">{t('nodes.authCode')}</th>
                <th className="p-4">{t('institutions.createTime')}</th>
                <th className="p-4">{t('common.action')}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-gray-800 text-gray-800 dark:text-gray-200">
              {nodes.length === 0 && (
                <tr>
                  <td colSpan={6} className="p-4 text-center text-gray-400">
                    {nodesQuery.isLoading ? t('common.loading') : t('institutions.noNodes')}
                  </td>
                </tr>
              )}
              {nodes.map((node) => {
                const deleteReason = node.allowDeletion === false ? (node.isMainNode ? t('p2p.mainNodeNoDelete') : t('p2p.authorizedNoDelete')) : null;
                return (
                  <tr key={node.nodeId} className="hover:bg-gray-50/50 dark:hover:bg-gray-850/50">
                    <td className="p-4">
                      <div className="font-semibold">
                        {node.isMainNode && (
                          <span className="mr-1 px-1.5 py-0.5 rounded text-[10px] bg-blue-100 dark:bg-blue-950 text-blue-600">
                            {t('p2p.mainNode')}
                          </span>
                        )}
                        {node.nodeName}
                      </div>
                      <div className="font-mono text-[10px] text-gray-400">{node.nodeId}</div>
                    </td>
                    <td className="p-4">
                      <NodeStatusBadge status={node.nodeStatus} />
                    </td>
                    <td className="p-4 font-mono text-gray-500">{node.netAddress || '-'}</td>
                    <td className="p-4">
                      <SecretText text={node.nodeAuthenticationCode} />
                    </td>
                    <td className="p-4 text-gray-400">{formatTime(node.gmtCreate)}</td>
                    <td className="p-4">
                      <div className="flex items-center gap-2">
                        <Button size="sm" variant="ghost" onClick={() => setTokenNode(node)}>
                          {t('institutions.token')}
                        </Button>
                        {platform.canWriteNode() && (
                          <span title={deleteReason || undefined}>
                            <Button size="sm" variant="ghost" disabled={!!deleteReason} onClick={() => setDeleting(node)}>
                              {t('common.delete')}
                            </Button>
                          </span>
                        )}
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </Card>

      <Drawer
        isOpen={!!tokenNode}
        onClose={() => setTokenNode(null)}
        title={`${t('institutions.tokenTitle')} - ${tokenNode?.nodeName || ''}`}
      >
        {tokenNode?.nodeId && (
          <div className="space-y-3">
            <InfoRow label={t('nodes.id')}>{tokenNode.nodeId}</InfoRow>
            <TokenPanel kind="inst" nodeId={tokenNode.nodeId} canRefresh={platform.canWriteNode()} />
          </div>
        )}
      </Drawer>
      <DeleteNodeDialog node={deleting} onClose={() => setDeleting(null)} deleteFn={deleteInstNodeJava} onDeleted={invalidate} />
    </div>
  );
};
