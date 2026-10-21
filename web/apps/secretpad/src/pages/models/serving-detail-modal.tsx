import React, { useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Badge, Button, Empty, Modal, Drawer, Tabs } from '@secretpad/design-system';
import { apiClient, getModelPartiesJava, getModelServingJava, type ModelPackJava } from '@secretpad/api-client';
import { DAGNextWorkspace } from '@secretpad/dag-next';
import { DagPreview } from '@/features/lineage/dag-preview';
import { modelInfoPreview } from './model-info-preview';
import { useTranslation } from '@/shared/lib/i18n';
import { errorText } from '@/features/cooperative-node/format';
import { InfoRow } from '@/features/cooperative-node/ui-common';

/**
 * 模型详情：model/info（旧 model-detail：各节点模型路径 + 训练流链路预览/全屏）
 * + model/detail（参与方/列）+ model/serving/detail（旧 model-info 服务详情）。
 */
export const ServingDetailModal: React.FC<{ model: ModelPackJava | null; projectId: string; onClose: () => void }> = ({
  model,
  projectId,
  onClose,
}) => {
  const { t } = useTranslation();
  const servingQuery = useQuery({
    queryKey: ['model-serving', model?.servingId],
    queryFn: () => getModelServingJava(model!.servingId!),
    enabled: !!model?.servingId,
  });
  const partiesQuery = useQuery({
    queryKey: ['model-parties', projectId, model?.modelId],
    queryFn: () => getModelPartiesJava(model!.modelId!, projectId),
    enabled: !!model?.modelId && !!projectId,
  });
  const infoQuery = useQuery({
    queryKey: ['model-info', projectId, model?.modelId],
    queryFn: () => apiClient.getModelInfo(model!.modelId!, projectId),
    enabled: !!model?.modelId && !!projectId,
  });
  const preview = useMemo(() => modelInfoPreview(infoQuery.data), [infoQuery.data]);
  const [tab,setTab] = useState('basics');
  const [fullscreen, setFullscreen] = useState(false);
  React.useEffect(()=>{setTab('basics');setFullscreen(false);},[model?.modelId]);

  const details = servingQuery.data?.servingDetails || [];

  return (
    <Drawer
      isOpen={!!model}
      onClose={onClose}
      width="max-w-2xl"
      title={model?.modelName || t('models.detail')}
      footer={
        <Button variant="primary" onClick={onClose}>
          {t('common.close')}
        </Button>
      }
    >
      <div className="text-xs space-y-4">
        <Tabs activeKey={tab} onChange={setTab} items={[{key:'basics',label:t('display.basics')},{key:'data',label:t('display.data')},{key:'lineage',label:t('models.modelGraph')},{key:'serving',label:t('models.servingDetail')}]}/>
        <div hidden={tab !== 'basics'}>
          <InfoRow label={t('models.modelId')}>{model?.modelId}</InfoRow>
          <InfoRow label={t('models.stats')}>{model?.modelStats || '-'}</InfoRow>
          {model?.servingId && <InfoRow label={t('models.servingId')}>{model.servingId}</InfoRow>}
          {model?.modelDesc && <InfoRow label={t('models.desc')}>{model.modelDesc}</InfoRow>}
          {preview.modelPaths.map((p) => (
            <InfoRow key={p.nodeId} label={t('models.modelPath', { name: p.nodeName })}>
              {p.sourcePath || '-'}
            </InfoRow>
          ))}
        </div>

        <div hidden={tab !== 'lineage'}>
          <div className="flex items-center justify-between mb-1.5">
            <span className="text-gray-500 text-[10px] font-semibold uppercase tracking-wide">{t('models.modelGraph')}</span>
            {preview.nodes.length > 0 && (
              <Button size="sm" variant="ghost" onClick={() => setFullscreen(true)}>
                ⛶ {t('common.fullscreen')}
              </Button>
            )}
          </div>
          {infoQuery.isLoading && <div className="text-gray-400">{t('common.loading')}</div>}
          {infoQuery.error && <div className="text-rose-500">{errorText(infoQuery.error)}</div>}
          {!infoQuery.isLoading && !infoQuery.error && preview.nodes.length === 0 && <div className="text-gray-400">{t('models.modelGraphEmpty')}</div>}
          <DagPreview nodes={preview.nodes} edges={preview.edges} highlight={preview.highlight} />
        </div>

        <div hidden={tab !== 'data'}>
          <div className="text-gray-500 text-[10px] font-semibold uppercase tracking-wide mb-1.5">{t('models.exportDetail')}</div>
          {partiesQuery.isLoading && <div className="text-gray-400">{t('common.loading')}</div>}
          {!partiesQuery.isLoading && (partiesQuery.data || []).length === 0 && <div className="text-gray-400">{t('models.exportNoParties')}</div>}
          <div className="space-y-2">
            {(partiesQuery.data || []).map((p) => (
              <div key={p.nodeId} className="p-2.5 rounded bg-gray-50 dark:bg-gray-800">
                <div className="font-semibold mb-1">
                  {p.nodeName || p.nodeId} <span className="font-mono text-[10px] text-gray-400">{p.nodeId}</span>
                </div>
                <div className="flex flex-wrap gap-1">
                  {(p.columns || []).map((c) => (
                    <span key={c} className="px-1.5 py-0.5 rounded bg-blue-50 dark:bg-blue-950/50 text-blue-700 dark:text-blue-300 font-mono text-[10px]">
                      {c}
                    </span>
                  ))}
                </div>
              </div>
            ))}
          </div>
        </div>

        <div hidden={tab !== 'serving'}>
          <div className="text-gray-500 text-[10px] font-semibold uppercase tracking-wide mb-1.5">{t('models.servingDetail')}</div>
          {!model?.servingId && <Empty>{t('models.servingNoDetail')}</Empty>}
          {servingQuery.isLoading && <div className="text-gray-400">{t('common.loading')}</div>}
          {servingQuery.error && <div className="text-rose-500">{errorText(servingQuery.error)}</div>}
          <div className="space-y-2">
            {details.map((sd, idx) => {
              const res = sd.resources?.[0];
              return (
                <div key={sd.nodeId || idx} className="p-2.5 rounded bg-gray-50 dark:bg-gray-800 border border-gray-100 dark:border-gray-700">
                  <div className="font-semibold flex items-center gap-2 mb-1">
                    {sd.nodeName || sd.nodeId}
                    {sd.isMock && <Badge status="default">{t('models.servingMock')}</Badge>}
                  </div>
                  <InfoRow label={t('models.servingEndpoints')}>{sd.endpoints || '-'}</InfoRow>
                  <InfoRow label={t('models.servingFeatureHttp')}>{sd.featureHttp || '-'}</InfoRow>
                  <InfoRow label={t('models.servingSourcePath')}>{sd.sourcePath || '-'}</InfoRow>
                  {res && (
                    <InfoRow label={t('models.resourceConfig')}>
                      CPU {res.minCPU ?? '-'}~{res.maxCPU ?? '-'} · Memory {res.minMemory ?? '-'}~{res.maxMemory ?? '-'}
                    </InfoRow>
                  )}
                  {sd.featureMappings && Object.keys(sd.featureMappings).length > 0 && (
                    <InfoRow label={t('models.servingFeatureMappings')}>
                      <div className="flex flex-wrap gap-1">
                        {Object.entries(sd.featureMappings).map(([k, v]) => (
                          <span key={k} className="px-1.5 py-0.5 rounded bg-emerald-50 dark:bg-emerald-950/50 text-emerald-700 dark:text-emerald-300 font-mono text-[10px]">
                            {k} → {v}
                          </span>
                        ))}
                      </div>
                    </InfoRow>
                  )}
                </div>
              );
            })}
          </div>
        </div>
      </div>
      <Modal isOpen={fullscreen} onClose={() => setFullscreen(false)} width="max-w-6xl" title={preview.graphName || t('models.modelGraph')}>
        <div style={{ height: '65vh' }}>
          {fullscreen && <DAGNextWorkspace readOnly title={preview.graphName} initialNodes={preview.nodes} initialEdges={preview.edges} />}
        </div>
      </Modal>
    </Drawer>
  );
};
