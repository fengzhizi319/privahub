/**
 * DAG 内模型提交（旧版 dag-model-submission）。
 *
 * 1. 以选中的训练 / 预测组件为入口，自动选出提交链路：前处理 + 训练（或预测）+ 后处理
 *    （见 pages/dag/adapters.ts#computeModelChain，对应旧版 dag-submit/util.ts）；
 * 2. DAG 预览中高亮链路节点；
 * 3. model/modelPartyPath 取各参与方数据源，model/pack 提交；
 * 4. 轮询 model/status 直到 SUCCEED / FAILED。
 */
import React, { useEffect, useMemo, useState } from 'react';
import { useMutation, useQuery } from '@tanstack/react-query';
import { Button, Modal, toast } from '@secretpad/design-system';
import type { DAGEdge, DAGNode, DAGNodeStatus } from '@secretpad/dag-next';
import { parseAnchor } from '@secretpad/dag-next';
import { apiClient } from '@secretpad/api-client';
import { useTranslation } from '../../shared/lib/i18n';
import { computeModelChain, submitNodesOf, splitCodeName } from '../../pages/dag/adapters';
import { DagPreview } from '../lineage/dag-preview';
import { NODE_STATUS, normalizeStatus } from '@secretpad/dag-next';

interface ModelPackModalProps {
  isOpen: boolean;
  onClose: () => void;
  projectId: string;
  graphId: string;
  /** 入口节点（训练或预测组件）。 */
  trainNode: DAGNode;
  /** 当前画布节点 / 边（用于自动选择链路与预览）。 */
  nodes?: DAGNode[];
  edges?: DAGEdge[];
  statusOf?: (n: DAGNode) => DAGNodeStatus;
  onPacked?: () => void;
}

/** 选模型输出锚点：优先 type=model 的结果输出。 */
function modelOutputOf(node: DAGNode | undefined): string | undefined {
  if (!node) return undefined;
  const model = node.resultOutputs?.find((o) => o.type === 'model');
  return model?.id ?? node.outputs?.find((o) => /-output-\d+$/.test(o)) ?? node.outputs?.[0];
}

export const ModelPackModal: React.FC<ModelPackModalProps> = ({ isOpen, onClose, projectId, graphId, trainNode, nodes, edges, statusOf, onPacked }) => {
  const { t } = useTranslation();
  const [modelName, setModelName] = useState('');
  const [modelDesc, setModelDesc] = useState('');
  const [partySources, setPartySources] = useState<Record<string, string>>({});
  const [packJobId, setPackJobId] = useState<string | null>(null);

  const allNodes = useMemo(() => (nodes && nodes.length > 0 ? nodes : [trainNode]), [nodes, trainNode]);
  const allEdges = useMemo(() => edges ?? [], [edges]);
  const chain = useMemo(() => computeModelChain(trainNode.id, allNodes, allEdges, statusOf), [trainNode.id, allNodes, allEdges, statusOf]);
  const submitNodes = submitNodesOf(chain);
  const modelNode = chain.modelNode[0] ?? trainNode;
  const outputId = modelOutputOf(modelNode);
  const highlight = useMemo(() => new Set([...submitNodes, ...chain.modelNode].map((n) => n.id)), [submitNodes, chain.modelNode]);

  useEffect(() => {
    if (isOpen) {
      setModelName(`${trainNode.name || trainNode.codeName || 'model'}_${Date.now()}`);
      setModelDesc('');
      setPartySources({});
      setPackJobId(null);
    }
  }, [isOpen, trainNode]);

  const partyPathQuery = useQuery({
    queryKey: ['model-party-path', projectId, graphId, modelNode.id, outputId],
    queryFn: async () => {
      if (!outputId) return [];
      return apiClient.getModelPartyPath({ projectId, graphNodeId: parseAnchor(outputId)?.nodeId ?? modelNode.id, graphNodeOutPutId: outputId });
    },
    enabled: isOpen && !!outputId,
  });
  const partyPaths = useMemo(() => partyPathQuery.data ?? [], [partyPathQuery.data]);

  useEffect(() => {
    const defaults: Record<string, string> = {};
    partyPaths.forEach((p) => {
      const firstSource = (p.dataSources?.[0] as { dataSourceId?: string } | undefined)?.dataSourceId;
      if (firstSource && p.nodeId) defaults[p.nodeId] = firstSource;
    });
    setPartySources((prev) => ({ ...defaults, ...prev }));
  }, [partyPaths]);

  const packStatusQuery = useQuery({
    queryKey: ['model-pack-status', packJobId, projectId],
    queryFn: async () => (packJobId ? apiClient.getModelStatus(packJobId, projectId) : null),
    enabled: !!packJobId && !!projectId,
    refetchInterval: (query) => {
      const status = normalizeStatus(String(query.state.data || ''));
      return status === NODE_STATUS.SUCCEED || status === NODE_STATUS.FAILED ? false : 3000;
    },
  });

  useEffect(() => {
    const status = normalizeStatus(String(packStatusQuery.data || ''));
    if (!packJobId || !status) return;
    if (status === NODE_STATUS.SUCCEED) {
      toast.success(t('models.packSucceeded'));
      setPackJobId(null);
      onClose();
      onPacked?.();
    } else if (status === NODE_STATUS.FAILED) {
      toast.error(t('models.packFailed'));
      setPackJobId(null);
    }
  }, [packStatusQuery.data, packJobId, onClose, onPacked, t]);

  const packMutation = useMutation({
    mutationFn: async () => {
      if (!outputId) throw new Error(t('models.noTrainNodeOutput'));
      if (!partyPaths.length) throw new Error(t('models.noPartyPath'));
      const modelPartyConfig = partyPaths.map((p) => {
        const partyId = p.nodeId || '';
        const sources = (p.dataSources || []) as Array<Record<string, string>>;
        const selected = sources.find((s) => s.dataSourceId === partySources[partyId]) || sources[0];
        return {
          modelParty: partyId,
          modelDataSource: selected?.dataSourceId || 'default-data-source',
          modelDataPath: selected?.dataSourcePath ?? selected?.path,
          modelDataName: p.nodeName || partyId,
        };
      });
      // 有预测组件时 modelComponent 只提交预测链路，trainId 为训练组件（旧版 submission-drawer）。
      const modelComponent = (submitNodes.length > 0 ? submitNodes : [modelNode]).map((n) => {
        const { domain, name } = splitCodeName(n.codeName);
        return {
          graphNodeId: n.id,
          domain: (n.nodeDef?.domain as string) || domain,
          name: (n.nodeDef?.name as string) || name,
          version: (n.nodeDef?.version as string) || '1.0.0',
        };
      });
      return apiClient.packModel({
        projectId,
        graphId,
        trainId: modelNode.id,
        modelName,
        modelDesc,
        graphNodeOutPutId: outputId,
        modelPartyConfig,
        modelComponent,
      });
    },
    onSuccess: (res) => {
      if (res?.jobId) {
        setPackJobId(res.jobId);
        toast.success(t('models.packPolling'));
      } else {
        onClose();
        onPacked?.();
        toast.success(t('models.packSuccess'));
      }
    },
    onError: (e) => toast.error(e instanceof Error ? e.message : String(e)),
  });

  const isValid = modelName.trim() && partyPaths.length > 0 && chain.modelNode.length > 0;
  const isLoading = partyPathQuery.isLoading || packMutation.isPending || !!packJobId;
  const chainRow = (label: string, list: DAGNode[]) =>
    list.length > 0 && (
      <div className="flex gap-2">
        <span className="w-20 text-gray-500 shrink-0">{label}</span>
        <span className="flex flex-wrap gap-1">
          {list.map((n) => (
            <span key={n.id} className="px-1.5 py-0.5 rounded bg-blue-50 dark:bg-blue-900/30 text-blue-700 dark:text-blue-300">
              {n.name}
            </span>
          ))}
        </span>
      </div>
    );

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={t('models.packFromDagTitle')}
      width="max-w-2xl"
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={!!packJobId}>
            {t('common.cancel')}
          </Button>
          <Button variant="primary" onClick={() => packMutation.mutate()} loading={isLoading} disabled={!isValid}>
            {t('models.pack')}
          </Button>
        </>
      }
    >
      <div className="space-y-4 text-xs max-h-[70vh] overflow-y-auto">
        <div>
          <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('models.name')}</label>
          <input
            type="text"
            value={modelName}
            onChange={(e) => setModelName(e.target.value)}
            className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100"
          />
        </div>
        <div>
          <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('dagx.modelDesc')}</label>
          <textarea
            value={modelDesc}
            onChange={(e) => setModelDesc(e.target.value)}
            rows={2}
            className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100"
          />
        </div>
        <div className="space-y-1.5">
          <div className="font-semibold text-gray-700 dark:text-gray-300">{t('dagx.modelChain')}</div>
          {chain.modelNode.length === 0 ? (
            <div className="text-amber-600">{t('dagx.modelChainEmpty')}</div>
          ) : (
            <>
              {chainRow(t('dagx.modelChainPre'), chain.preNodes)}
              {chainRow(t('dagx.modelChainModel'), chain.modelNode)}
              {chainRow(t('dagx.modelChainPredict'), chain.predictNode)}
              {chainRow(t('dagx.modelChainPost'), chain.postNodes)}
            </>
          )}
          {nodes && nodes.length > 0 && <DagPreview nodes={allNodes} edges={allEdges} highlight={highlight} focusId={trainNode.id} />}
        </div>

        {partyPathQuery.isLoading && <div className="text-gray-500">{t('models.loadingPartyPath')}</div>}
        {partyPaths.length > 0 && (
          <div className="space-y-2">
            <label className="block font-semibold text-gray-700 dark:text-gray-300">{t('models.partyDataSources')}</label>
            {partyPaths.map((p) => {
              const partyId = p.nodeId || '';
              const sources = (p.dataSources || []) as Array<Record<string, string>>;
              return (
                <div key={partyId} className="p-2 rounded-lg border border-gray-200 dark:border-gray-700">
                  <div className="font-medium text-gray-900 dark:text-gray-100 mb-1">{p.nodeName || partyId}</div>
                  <select
                    value={partySources[partyId] || ''}
                    onChange={(e) => setPartySources((prev) => ({ ...prev, [partyId]: e.target.value }))}
                    className="w-full p-2 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100"
                  >
                    {sources.map((s) => (
                      <option key={s.dataSourceId} value={s.dataSourceId}>
                        {s.dataSourceName || s.dataSourceId}
                      </option>
                    ))}
                  </select>
                </div>
              );
            })}
          </div>
        )}
        {packJobId && <div className="text-blue-600 dark:text-blue-400">{t('models.packPolling')}</div>}
      </div>
    </Modal>
  );
};

ModelPackModal.displayName = 'ModelPackModal';
