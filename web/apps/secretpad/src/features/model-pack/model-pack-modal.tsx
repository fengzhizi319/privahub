/**
 * DAG 内模型提交（打包）组件。
 *
 * 旧前端对应 `dag-model-submission`：在 DAG 画布中选中训练节点并运行成功后，
 * 可直接将模型打包为产物。本组件复用 `models` 页面已有的打包逻辑，
 * 但参数已知（projectId / graphId / trainNode），因此表单更简洁。
 *
 * 打包流程：
 * 1. 调用 `model/modelPartyPath` 获取每个参与方可用的数据源。
 * 2. 用户确认/调整每个参与方的数据源，填写模型名称。
 * 3. 构造 `modelPartyConfig` 与 `modelComponent`，调用 `model/pack`。
 * 4. 提交成功后轮询 `model/status` 直到 SUCCEED / FAILED。
 */
import React, { useEffect, useState } from 'react';
import { useMutation, useQuery } from '@tanstack/react-query';
import { Button, Modal, toast } from '@secretpad/design-system';
import type { DAGNode } from '@secretpad/dag-next';
import { apiClient } from '@secretpad/api-client';
import { useTranslation } from '../../shared/lib/i18n';

interface ModelPackModalProps {
  isOpen: boolean;
  onClose: () => void;
  projectId: string;
  graphId: string;
  trainNode: DAGNode;
  onPacked?: () => void;
}


export const ModelPackModal: React.FC<ModelPackModalProps> = ({
  isOpen,
  onClose,
  projectId,
  graphId,
  trainNode,
  onPacked,
}) => {
  const { t } = useTranslation();
  const [modelName, setModelName] = useState('');
  const [partySources, setPartySources] = useState<Record<string, string>>({});
  const [packJobId, setPackJobId] = useState<string | null>(null);

  useEffect(() => {
    if (isOpen) {
      setModelName(`${trainNode.name || trainNode.codeName || 'model'}_${Date.now()}`);
      setPartySources({});
      setPackJobId(null);
    }
  }, [isOpen, trainNode]);

  const outputId = trainNode.outputs?.[0];

  const partyPathQuery = useQuery({
    queryKey: ['model-party-path', projectId, graphId, trainNode.id, outputId],
    queryFn: async () => {
      if (!outputId) return [];
      return apiClient.getModelPartyPath({
        projectId,
        graphNodeId: trainNode.id,
        graphNodeOutPutId: outputId,
      });
    },
    enabled: isOpen && !!outputId,
  });
  const partyPaths = partyPathQuery.data ?? [];

  useEffect(() => {
    const defaults: Record<string, string> = {};
    partyPaths.forEach((p) => {
      const firstSource = (p.dataSources?.[0] as any)?.dataSourceId as string | undefined;
      const partyId = p.nodeId || '';
      if (firstSource && partyId) {
        defaults[partyId] = firstSource;
      }
    });
    setPartySources((prev) => ({ ...defaults, ...prev }));
  }, [partyPaths]);

  const packStatusQuery = useQuery({
    queryKey: ['model-pack-status', packJobId, projectId],
    queryFn: async () => {
      if (!packJobId || !projectId) return null;
      return apiClient.getModelStatus(packJobId, projectId);
    },
    enabled: !!packJobId && !!projectId,
    refetchInterval: (query) => {
      const status = query.state.data?.toUpperCase?.() || '';
      const ongoing = !['SUCCEED', 'FAILED'].includes(status);
      return ongoing ? 3000 : false;
    },
  });

  useEffect(() => {
    const status = packStatusQuery.data?.toUpperCase?.() || '';
    if (!packJobId || !status) return;
    if (status === 'SUCCEED') {
      toast.success(t('models.packSucceeded'));
      setPackJobId(null);
      onClose();
      onPacked?.();
    } else if (status === 'FAILED') {
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
        const sources = (p.dataSources || []) as Array<Record<string, any>>;
        const selectedSource =
          sources.find((s) => s.dataSourceId === partySources[partyId]) || sources[0];
        return {
          modelParty: partyId,
          modelDataSource: selectedSource?.dataSourceId || 'default-data-source',
          modelDataName: p.nodeName || partyId,
        };
      });

      const modelComponent = [
        {
          graphNodeId: trainNode.id,
          domain: trainNode.codeName?.split('/')[0] || 'ml.train',
          name: trainNode.codeName?.split('/')[1] || 'ss_sgd_train',
          version: (trainNode as any).nodeDef?.version || '1.0.0',
        },
      ];

      return apiClient.packModel({
        projectId,
        graphId,
        trainId: trainNode.id,
        modelName,
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

  const isValid = modelName.trim() && partyPaths.length > 0 && Object.keys(partySources).length > 0;
  const isLoading = partyPathQuery.isLoading || packMutation.isPending || !!packJobId;

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={t('models.packFromDagTitle')}
      width="max-w-xl"
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
      <div className="space-y-4 text-xs">
        <div>
          <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">
            {t('models.name')}
          </label>
          <input
            type="text"
            value={modelName}
            onChange={(e) => setModelName(e.target.value)}
            className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
          />
        </div>

        <div>
          <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">
            {t('models.trainNode')}
          </label>
          <div className="p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100">
            {trainNode.name} ({trainNode.codeName})
          </div>
        </div>

        {partyPathQuery.isLoading && <div className="text-gray-500">{t('models.loadingPartyPath')}</div>}

        {partyPaths.length > 0 && (
          <div className="space-y-2">
            <label className="block font-semibold text-gray-700 dark:text-gray-300">
              {t('models.partyDataSources')}
            </label>
            {partyPaths.map((p) => {
              const partyId = p.nodeId || '';
              const sources = (p.dataSources || []) as Array<Record<string, any>>;
              return (
                <div key={partyId} className="p-2 rounded-lg border border-gray-200 dark:border-gray-700">
                  <div className="font-medium text-gray-900 dark:text-gray-100 mb-1">
                    {p.nodeName || partyId}
                  </div>
                  <select
                    value={partySources[partyId] || ''}
                    onChange={(e) => setPartySources((prev) => ({ ...prev, [partyId]: e.target.value }))}
                    className="w-full p-2 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
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

        {packJobId && (
          <div className="text-blue-600 dark:text-blue-400">{t('models.packPolling')}</div>
        )}
      </div>
    </Modal>
  );
};

ModelPackModal.displayName = 'ModelPackModal';
