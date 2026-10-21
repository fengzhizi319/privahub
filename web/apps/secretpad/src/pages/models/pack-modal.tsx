import React, { useEffect, useMemo, useState } from 'react';
import { useMutation, useQuery } from '@tanstack/react-query';
import { Button, Modal, toast } from '@secretpad/design-system';
import type { GraphNodeDetail } from '@secretpad/api-client';
import { apiClient } from '@secretpad/api-client';
import { useTranslation } from '@/shared/lib/i18n';
import { errorText } from '@/features/cooperative-node/format';
import { NODE_STATUS, isTerminalStatus, normalizeStatus } from '@secretpad/dag-next';

const selectCls =
  'w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500';

/**
 * 打包模型（model/pack）：选择画布与训练节点，按 model/modelPartyPath 为每个参与方
 * 选择数据源，提交后轮询 model/status 直到 SUCCEED / FAILED。
 * （从原 models 页面抽出，逻辑保持不变。）
 */
export const PackModelModal: React.FC<{ open: boolean; onClose: () => void; projectId: string; onPacked: () => void }> = ({
  open,
  onClose,
  projectId,
  onPacked,
}) => {
  const { t } = useTranslation();
  const [graphId, setGraphId] = useState('');
  const [trainNodeId, setTrainNodeId] = useState('');
  const [modelName, setModelName] = useState('');
  const [partySources, setPartySources] = useState<Record<string, string>>({});
  const [jobId, setJobId] = useState<string | null>(null);

  const reset = () => {
    setGraphId('');
    setTrainNodeId('');
    setModelName('');
    setPartySources({});
    setJobId(null);
  };
  useEffect(() => {
    if (open) reset();
  }, [open]);

  const graphsQuery = useQuery({
    queryKey: ['graphs', projectId],
    queryFn: () => apiClient.getGraphs(projectId),
    enabled: open && !!projectId,
  });
  const graphDetailQuery = useQuery({
    queryKey: ['graph-detail', projectId, graphId],
    queryFn: () => apiClient.getGraphDetail(projectId, graphId),
    enabled: open && !!graphId,
  });
  const trainNodes: GraphNodeDetail[] = useMemo(
    () => (graphDetailQuery.data?.nodes || []).filter((n) => (n.codeName || '').includes('train')),
    [graphDetailQuery.data],
  );
  const trainNode = useMemo(() => trainNodes.find((n) => n.graphNodeId === trainNodeId), [trainNodes, trainNodeId]);

  const partyPathQuery = useQuery({
    queryKey: ['model-party-path', projectId, graphId, trainNodeId],
    queryFn: () =>
      apiClient.getModelPartyPath({
        projectId,
        graphNodeId: trainNode!.graphNodeId!,
        graphNodeOutPutId: trainNode!.outputs![0],
      }),
    enabled: open && !!trainNode?.outputs?.[0],
  });
  const partyPaths = useMemo(() => partyPathQuery.data ?? [], [partyPathQuery.data]);

  useEffect(() => {
    const defaults: Record<string, string> = {};
    partyPaths.forEach((p) => {
      const first = (p.dataSources?.[0] as { dataSourceId?: string } | undefined)?.dataSourceId;
      if (first && p.nodeId) defaults[p.nodeId] = first;
    });
    setPartySources((prev) => ({ ...defaults, ...prev }));
  }, [partyPaths]);

  const statusQuery = useQuery({
    queryKey: ['model-pack-status', jobId, projectId],
    queryFn: () => apiClient.getModelStatus(jobId!, projectId),
    enabled: !!jobId,
    refetchInterval: (q) => (isTerminalStatus(q.state.data || '') ? false : 3000),
  });
  useEffect(() => {
    const status = normalizeStatus(statusQuery.data || '');
    if (!jobId || !status) return;
    if (status === NODE_STATUS.SUCCEED) {
      toast.success(t('models.packSucceeded'));
      setJobId(null);
      onPacked();
      onClose();
    } else if (status === NODE_STATUS.FAILED || status === NODE_STATUS.STOPPED) {
      toast.error(t('models.packFailed'));
      setJobId(null);
    }
  }, [statusQuery.data, jobId, t, onPacked, onClose]);

  const pack = useMutation({
    mutationFn: async () => {
      if (!trainNode?.outputs?.[0]) throw new Error(t('models.noTrainNodeOutput'));
      if (!partyPaths.length) throw new Error(t('models.noPartyPath'));
      const modelPartyConfig = partyPaths.map((p) => {
        const partyId = p.nodeId || '';
        const sources = (p.dataSources || []) as Array<Record<string, string>>;
        const src = sources.find((s) => s.dataSourceId === partySources[partyId]) || sources[0];
        return { modelParty: partyId, modelDataSource: src?.dataSourceId || 'default-data-source', modelDataName: p.nodeName || partyId };
      });
      const modelComponent = [
        {
          graphNodeId: trainNode.graphNodeId,
          domain: trainNode.nodeDef?.domain || 'ml.train',
          name: trainNode.nodeDef?.name || trainNode.codeName?.split('/')[1] || 'ss_sgd_train',
          version: trainNode.nodeDef?.version || '1.0.0',
        },
      ];
      return apiClient.packModel({
        projectId,
        graphId,
        trainId: trainNodeId,
        modelName,
        graphNodeOutPutId: trainNode.outputs[0],
        modelPartyConfig,
        modelComponent,
      });
    },
    onSuccess: (res) => {
      if (res?.jobId) {
        setJobId(res.jobId);
        toast.success(t('models.packPolling'));
      } else {
        toast.success(t('models.packSuccess'));
        onPacked();
        onClose();
      }
    },
    onError: (e) => toast.error(errorText(e)),
  });

  const polling = !!jobId;
  return (
    <Modal
      isOpen={open}
      onClose={onClose}
      title={t('models.pack')}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button
            variant="primary"
            onClick={() => pack.mutate()}
            loading={pack.isPending || polling}
            disabled={!graphId || !trainNodeId || !modelName.trim() || !partyPaths.length || Object.keys(partySources).length < partyPaths.length}
          >
            {polling ? t('models.packing') : t('models.pack')}
          </Button>
        </>
      }
    >
      <div className="text-xs space-y-4">
        <div>
          <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('periodicTasks.selectGraph')}</label>
          <select
            value={graphId}
            onChange={(e) => {
              setGraphId(e.target.value);
              setTrainNodeId('');
            }}
            className={selectCls}
          >
            <option value="">-</option>
            {(graphsQuery.data ?? []).map((g) => (
              <option key={g.graphId} value={g.graphId || ''}>
                {g.name}
              </option>
            ))}
          </select>
        </div>
        <div>
          <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('models.trainNode')}</label>
          <select value={trainNodeId} onChange={(e) => setTrainNodeId(e.target.value)} className={selectCls}>
            <option value="">-</option>
            {trainNodes.map((n) => (
              <option key={n.graphNodeId} value={n.graphNodeId || ''}>
                {n.label || n.codeName}
              </option>
            ))}
          </select>
        </div>
        <div>
          <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('models.name')}</label>
          <input type="text" value={modelName} onChange={(e) => setModelName(e.target.value)} className={selectCls} />
        </div>
        {partyPaths.length > 0 && (
          <div>
            <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('models.partyDataSources')}</label>
            <div className="space-y-2">
              {partyPaths.map((p, idx) => {
                const partyId = p.nodeId || `party-${idx}`;
                const sources = (p.dataSources || []) as Array<{ dataSourceId?: string; dataSourceName?: string; type?: string }>;
                return (
                  <div key={partyId} className="flex items-center gap-2">
                    <span className="w-20 font-medium text-gray-600 dark:text-gray-400">{p.nodeName || partyId}</span>
                    <select
                      value={partySources[partyId] || sources[0]?.dataSourceId || ''}
                      onChange={(e) => setPartySources((prev) => ({ ...prev, [partyId]: e.target.value }))}
                      className={selectCls}
                    >
                      {sources.map((s) => (
                        <option key={s.dataSourceId} value={s.dataSourceId}>
                          {s.dataSourceName || s.dataSourceId} ({s.type})
                        </option>
                      ))}
                    </select>
                  </div>
                );
              })}
            </div>
          </div>
        )}
        {partyPathQuery.isLoading && <div className="text-gray-400">{t('models.loadingPartyPath')}</div>}
        {partyPathQuery.error && <div className="text-red-500">{t('models.partyPathError', { message: partyPathQuery.error.message })}</div>}
      </div>
    </Modal>
  );
};
