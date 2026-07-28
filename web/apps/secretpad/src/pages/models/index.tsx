import React, { useEffect, useMemo, useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Card, Badge, Button, Modal, ConfirmDialog, toast } from '@secretpad/design-system';
import type { ModelPackVO, GraphNodeDetail, ModelPackDetailVO, ServingDetailVO } from '@secretpad/api-client';
import { apiClient } from '@secretpad/api-client';
import { useTranslation } from '../../shared/lib/i18n';
import { AccessGuard } from '../../features/auth/ui/access-guard';
import { Platform } from '../../shared/lib/platform';

const PACKING_STATS = ['PACKING', 'PUBLISHING', 'EXPORTING', 'INITIATED'];

/**
 * 模型管理页面。
 *
 * 设计要点：
 * 1. 按项目维度展示模型产物，支持打包(Pack)、部署(Deploy)、废弃(Discard)与删除。
 * 2. 打包流程需要调用 `/api/v1alpha1/model/modelPartyPath` 获取每个参与方的默认数据源，
 *    再构造 `modelPartyConfig` 与 `modelComponent` 提交给 `/api/v1alpha1/model/pack`。
 *    仅传空数组会导致后端 NPE/越界错误，因此必须根据训练节点图信息填充。
 * 3. 打包是异步作业，提交成功后使用 `getModelStatus` 轮询，直到 SUCCEED 或 FAILED，
 *    成功后刷新模型列表。
 * 4. 部署服务基于已打包完成的模型，调用 `model/serving/create`。
 */
export const ModelsPage: React.FC = () => {
  const { t } = useTranslation();
  const queryClient = useQueryClient();

  const [selectedProjectId, setSelectedProjectId] = useState<string>('');
  const [error, setError] = useState<string | null>(null);
  const [detailModel, setDetailModel] = useState<ModelPackVO | null>(null);
  const [detailInfo, setDetailInfo] = useState<any>(null);
  // 模型导出详情（各参与方的训练列，来自 model/detail）。
  const [exportDetail, setExportDetail] = useState<ModelPackDetailVO | null>(null);
  // 发布详情（服务端点/特征映射等，来自 model/serving/detail）。
  const [servingDetail, setServingDetail] = useState<ServingDetailVO | null>(null);

  // Deploy modal
  const [deployOpen, setDeployOpen] = useState(false);
  const [deployModelId, setDeployModelId] = useState('');
  const [deployProjectId, setDeployProjectId] = useState('');

  // Pack modal
  const [packOpen, setPackOpen] = useState(false);
  const [packGraphId, setPackGraphId] = useState('');
  const [packTrainNodeId, setPackTrainNodeId] = useState('');
  const [packModelName, setPackModelName] = useState('');
  // 每个参与方的默认数据源选择：key 为 nodeId，value 为 dataSourceId
  const [packPartySources, setPackPartySources] = useState<Record<string, string>>({});
  // 打包任务 ID，用于轮询
  const [packJobId, setPackJobId] = useState<string | null>(null);

  // Discard confirm
  const [discardTarget, setDiscardTarget] = useState<ModelPackVO | null>(null);

  const projectsQuery = useQuery({
    queryKey: ['projects'],
    queryFn: () => apiClient.getProjects(),
  });
  const projects = useMemo(() => projectsQuery.data ?? [], [projectsQuery.data]);

  // Default the selected project to the first one once projects load.
  useEffect(() => {
    if (!selectedProjectId && projects.length > 0) {
      setSelectedProjectId(projects[0].projectId);
    }
  }, [projects, selectedProjectId]);

  const modelsQuery = useQuery({
    queryKey: ['models', selectedProjectId],
    queryFn: () => apiClient.getModels(selectedProjectId),
    enabled: !!selectedProjectId,
    // Poll while any model is still being packed/published.
    refetchInterval: (query) => {
      const hasPacking = query.state.data?.some((m) => PACKING_STATS.includes((m.modelStats || '').toUpperCase()));
      return hasPacking ? 4000 : false;
    },
  });
  const models = modelsQuery.data ?? [];

  const invalidateModels = () =>
    queryClient.invalidateQueries({ queryKey: ['models', selectedProjectId] });

  // Graphs for the pack form
  const graphsQuery = useQuery({
    queryKey: ['graphs', selectedProjectId],
    queryFn: () => apiClient.getGraphs(selectedProjectId),
    enabled: packOpen && !!selectedProjectId,
  });
  const graphs = graphsQuery.data ?? [];

  // Graph detail (train nodes) for the pack form
  const graphDetailQuery = useQuery({
    queryKey: ['graph-detail', selectedProjectId, packGraphId],
    queryFn: () => apiClient.getGraphDetail(selectedProjectId, packGraphId),
    enabled: packOpen && !!packGraphId,
  });
  const trainNodes: GraphNodeDetail[] = (graphDetailQuery.data?.nodes || []).filter((n) =>
    (n.codeName || '').includes('train')
  );

  // 当选择训练节点后，查询模型参与方路径，用于构造 modelPartyConfig。
  const selectedTrainNode = useMemo(
    () => trainNodes.find((n) => n.graphNodeId === packTrainNodeId),
    [trainNodes, packTrainNodeId]
  );
  const modelPartyPathQuery = useQuery({
    queryKey: ['model-party-path', selectedProjectId, packGraphId, packTrainNodeId],
    queryFn: async () => {
      if (!selectedTrainNode || !selectedTrainNode.outputs?.[0]) return [];
      return apiClient.getModelPartyPath({
        projectId: selectedProjectId,
        graphNodeId: selectedTrainNode.graphNodeId!,
        graphNodeOutPutId: selectedTrainNode.outputs[0],
      });
    },
    enabled: packOpen && !!selectedTrainNode && !!selectedTrainNode.outputs?.[0],
  });
  const partyPaths = modelPartyPathQuery.data ?? [];

  // 模型参与方路径加载后，将每个参与方的数据源默认设置为第一个可用数据源。
  useEffect(() => {
    const defaults: Record<string, string> = {};
    partyPaths.forEach((p) => {
      const firstSource = (p.dataSources?.[0] as any)?.dataSourceId as string | undefined;
      const partyId = p.nodeId || '';
      if (firstSource && partyId) {
        defaults[partyId] = firstSource;
      }
    });
    setPackPartySources((prev) => ({ ...defaults, ...prev }));
  }, [partyPaths]);

  // 打包状态轮询：提交成功后每 3 秒查询一次，直到成功或失败。
  const packStatusQuery = useQuery({
    queryKey: ['model-pack-status', packJobId, selectedProjectId],
    queryFn: async () => {
      if (!packJobId || !selectedProjectId) return null;
      return apiClient.getModelStatus(packJobId, selectedProjectId);
    },
    enabled: !!packJobId && !!selectedProjectId,
    refetchInterval: (query) => {
      const status = query.state.data?.toUpperCase?.() || '';
      const ongoing = !['SUCCEED', 'FAILED'].includes(status);
      return ongoing ? 3000 : false;
    },
  });

  const resetPackForm = () => {
    setPackGraphId('');
    setPackTrainNodeId('');
    setPackModelName('');
    setPackPartySources({});
    setPackJobId(null);
  };

  // 打包完成后自动刷新模型列表并给出提示。
  useEffect(() => {
    const status = packStatusQuery.data?.toUpperCase?.() || '';
    if (!packJobId || !status) return;
    if (status === 'SUCCEED') {
      toast.success(t('models.packSucceeded'));
      setPackJobId(null);
      setPackOpen(false);
      resetPackForm();
      invalidateModels();
    } else if (status === 'FAILED') {
      toast.error(t('models.packFailed'));
      setPackJobId(null);
    }
  }, [packStatusQuery.data, packJobId, t]);

  const deployMutation = useMutation({
    mutationFn: () => apiClient.createModelServing({ modelId: deployModelId, projectId: deployProjectId }),
    onSuccess: () => {
      setDeployOpen(false);
      setDeployModelId('');
      invalidateModels();
      toast.success(t('models.deploySuccess'));
    },
    onError: (e) => setError(e instanceof Error ? e.message : String(e)),
  });

  const packMutation = useMutation({
    mutationFn: async () => {
      const trainNode = trainNodes.find((n) => n.graphNodeId === packTrainNodeId);
      if (!trainNode || !trainNode.outputs?.[0]) {
        throw new Error(t('models.noTrainNodeOutput'));
      }
      if (!partyPaths.length) {
        throw new Error(t('models.noPartyPath'));
      }
      const outputId = trainNode.outputs[0];

      // 构造 modelPartyConfig：每个参与方必须指定 modelParty、modelDataSource、modelDataName。
      const modelPartyConfig = partyPaths.map((p) => {
        const partyId = p.nodeId || '';
        const sources = (p.dataSources || []) as Array<Record<string, any>>;
        const selectedSource =
          sources.find((s) => s.dataSourceId === packPartySources[partyId]) || sources[0];
        return {
          modelParty: partyId,
          modelDataSource: selectedSource?.dataSourceId || 'default-data-source',
          modelDataName: p.nodeName || partyId,
        };
      });

      // 构造 modelComponent：当前仅包含训练节点本身（无预处理/预测节点时）。
      const modelComponent = [
        {
          graphNodeId: trainNode.graphNodeId,
          domain: trainNode.nodeDef?.domain || 'ml.train',
          name: trainNode.nodeDef?.name || trainNode.codeName?.split('/')[1] || 'ss_sgd_train',
          version: trainNode.nodeDef?.version || '1.0.0',
        },
      ];

      return apiClient.packModel({
        projectId: selectedProjectId,
        graphId: packGraphId,
        trainId: packTrainNodeId,
        modelName: packModelName,
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
        setPackOpen(false);
        resetPackForm();
        invalidateModels();
        toast.success(t('models.packSuccess'));
      }
    },
    onError: (e) => setError(e instanceof Error ? e.message : String(e)),
  });

  const discardMutation = useMutation({
    mutationFn: (modelId: string) => apiClient.discardModel(modelId),
    onSuccess: () => {
      setDiscardTarget(null);
      invalidateModels();
      toast.success(t('models.discardSuccess'));
    },
    onError: (e) => {
      setDiscardTarget(null);
      setError(e instanceof Error ? e.message : String(e));
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (model: ModelPackVO) => apiClient.deleteModel(model.modelId!, model.ownerId!),
    onSuccess: invalidateModels,
    onError: (e) => setError(e instanceof Error ? e.message : String(e)),
  });

  /**
   * 查看模型详情：同时拉取三类信息并聚合展示。
   *
   * 1. `model/info`：模型状态与概要（原有逻辑）；
   * 2. `model/detail`：模型导出详情——各参与方及其训练列（parties/columns）；
   * 3. `model/serving/detail`：发布详情——服务端点、特征映射、源路径（仅在已发布时）。
   *
   * 三个请求互相独立，任一失败不影响其他，失败时对应区块展示空态。
   */
  const handleViewDetail = async (model: ModelPackVO) => {
    setDetailModel(model);
    setDetailInfo(null);
    setExportDetail(null);
    setServingDetail(null);
    if (!model.modelId || !selectedProjectId) return;

    // 概要信息（原有）。
    try {
      const info = await apiClient.getModelInfo(model.modelId, selectedProjectId);
      setDetailInfo(info);
    } catch (e) {
      setDetailInfo({ error: e instanceof Error ? e.message : String(e) });
    }

    // 模型导出详情（参与方与训练列）。
    try {
      const detail = await apiClient.getModelDetail(model.modelId, selectedProjectId);
      setExportDetail(detail);
    } catch {
      setExportDetail(null);
    }

    // 发布详情（仅在模型已发布 servingId 时拉取）。
    if (model.servingId) {
      try {
        const serving = await apiClient.getModelServingDetail(model.servingId);
        setServingDetail(serving);
      } catch {
        setServingDetail(null);
      }
    }
  };

  const openDeploy = () => {
    setDeployModelId(models[0]?.modelId || '');
    setDeployProjectId(selectedProjectId);
    setDeployOpen(true);
  };

  const openPack = () => {
    resetPackForm();
    setPackOpen(true);
  };

  const loading = modelsQuery.isLoading;
  const queryError = modelsQuery.error?.message || projectsQuery.error?.message || null;

  return (
    <div className="space-y-6">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 bg-white dark:bg-gray-900 p-4 rounded-xl border border-gray-200 dark:border-gray-800">
        <div>
          <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">{t('models.title')}</h2>
          <p className="text-xs text-gray-500">{t('models.subtitle')}</p>
        </div>
        <div className="flex items-center gap-3">
          <select
            value={selectedProjectId}
            onChange={(e) => setSelectedProjectId(e.target.value)}
            className="px-3 py-1.5 rounded-lg text-xs bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
          >
            {projects.map((p) => (
              <option key={p.projectId} value={p.projectId}>{p.projectName}</option>
            ))}
          </select>
          <AccessGuard access={{ types: [Platform.CENTER] }}>
            <Button variant="outline" onClick={openPack}>{t('models.pack')}</Button>
            <Button variant="primary" onClick={openDeploy}>{t('models.deploy')}</Button>
          </AccessGuard>
        </div>
      </div>

      {(error || queryError) && (
        <div className="text-xs text-red-500 bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-900 rounded-lg px-4 py-2">
          {t('common.error', { message: error || queryError || '' })}
        </div>
      )}

      {loading && <div className="text-xs text-gray-400">{t('common.loading')}</div>}

      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        {models.map((model) => (
          <Card key={model.modelId}>
            <div className="flex items-center justify-between mb-2">
              <h3 className="font-bold text-sm text-gray-900 dark:text-gray-100">{model.modelName}</h3>
              <div className="flex items-center gap-2">
                <Badge status={model.servingId ? 'success' : PACKING_STATS.includes((model.modelStats || '').toUpperCase()) ? 'processing' : 'default'}>
                  {model.servingId ? t('models.statusServing') : model.modelStats || t('models.statusPack')}
                </Badge>
                <Button size="sm" variant="ghost" onClick={() => handleViewDetail(model)}>{t('models.detail')}</Button>
                <AccessGuard access={{ types: [Platform.CENTER] }}>
                  <Button size="sm" variant="ghost" onClick={() => setDiscardTarget(model)}>{t('models.discard')}</Button>
                  <Button size="sm" variant="danger" onClick={() => model.modelId && model.ownerId && deleteMutation.mutate(model)}>{t('common.delete')}</Button>
                </AccessGuard>
              </div>
            </div>
            <div className="text-xs text-gray-500 space-y-1 font-mono">
              <div>ID: {model.modelId}</div>
              <div>{t('models.stats')}: {model.modelStats || '-'}</div>
              <div>{t('models.owner')}: {model.ownerId}</div>
              <div>{t('models.createTime')}: {model.gmtCreate}</div>
            </div>
          </Card>
        ))}
      </div>

      {models.length === 0 && !loading && !queryError && (
        <div className="text-center text-xs text-gray-400 py-10">{t('models.noData')}</div>
      )}

      {/* Deploy Serving Modal */}
      <Modal
        isOpen={deployOpen}
        onClose={() => setDeployOpen(false)}
        title={t('models.deployTitle')}
        footer={
          <>
            <Button variant="ghost" onClick={() => setDeployOpen(false)}>{t('common.cancel')}</Button>
            <Button variant="primary" onClick={() => deployMutation.mutate()} loading={deployMutation.isPending} disabled={!deployModelId || !deployProjectId}>{t('models.deploy')}</Button>
          </>
        }
      >
        <div className="text-xs space-y-4">
          <div>
            <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('models.name')}</label>
            <select
              value={deployModelId}
              onChange={(e) => setDeployModelId(e.target.value)}
              className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
            >
              {models.map((m) => (
                <option key={m.modelId} value={m.modelId || ''}>{m.modelName} ({m.modelId})</option>
              ))}
            </select>
          </div>
          <div>
            <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('models.selectProject')}</label>
            <select
              value={deployProjectId}
              onChange={(e) => setDeployProjectId(e.target.value)}
              className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
            >
              {projects.map((p) => (
                <option key={p.projectId} value={p.projectId}>{p.projectName}</option>
              ))}
            </select>
          </div>
        </div>
      </Modal>

      {/* Pack Model Modal */}
      <Modal
        isOpen={packOpen}
        onClose={() => { setPackOpen(false); resetPackForm(); }}
        title={t('models.pack')}
        footer={
          <>
            <Button variant="ghost" onClick={() => { setPackOpen(false); resetPackForm(); }}>{t('common.cancel')}</Button>
            <Button
              variant="primary"
              onClick={() => packMutation.mutate()}
              loading={packMutation.isPending || (!!packJobId && packStatusQuery.data?.toUpperCase?.() !== 'SUCCEED' && packStatusQuery.data?.toUpperCase?.() !== 'FAILED')}
              disabled={!packGraphId || !packTrainNodeId || !packModelName.trim() || !partyPaths.length || Object.keys(packPartySources).length < partyPaths.length}
            >
              {packJobId ? t('models.packing') : t('models.pack')}
            </Button>
          </>
        }
      >
        <div className="text-xs space-y-4">
          <div>
            <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('periodicTasks.selectGraph')}</label>
            <select
              value={packGraphId}
              onChange={(e) => { setPackGraphId(e.target.value); setPackTrainNodeId(''); }}
              className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
            >
              <option value="">-</option>
              {graphs.map((g) => (
                <option key={g.graphId} value={g.graphId || ''}>{g.name}</option>
              ))}
            </select>
          </div>
          <div>
            <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('models.sampleTables')}</label>
            <select
              value={packTrainNodeId}
              onChange={(e) => setPackTrainNodeId(e.target.value)}
              className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
            >
              <option value="">-</option>
              {trainNodes.map((n) => (
                <option key={n.graphNodeId} value={n.graphNodeId || ''}>{n.label || n.codeName}</option>
              ))}
            </select>
          </div>
          <div>
            <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('models.name')}</label>
            <input
              type="text"
              value={packModelName}
              onChange={(e) => setPackModelName(e.target.value)}
              className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
              required
            />
          </div>

          {partyPaths.length > 0 && (
            <div>
              <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">{t('models.partyDataSources')}</label>
              <div className="space-y-2">
                {partyPaths.map((p, idx) => {
                  const partyId = p.nodeId || `party-${idx}`;
                  const sources = (p.dataSources || []) as Array<{
                    dataSourceId?: string;
                    dataSourceName?: string;
                    type?: string;
                  }>;
                  return (
                    <div key={partyId} className="flex items-center gap-2">
                      <span className="w-20 font-medium text-gray-600 dark:text-gray-400">{p.nodeName || partyId}</span>
                      <select
                        value={packPartySources[partyId] || sources[0]?.dataSourceId || ''}
                        onChange={(e) => setPackPartySources((prev) => ({ ...prev, [partyId]: e.target.value }))}
                        className="flex-1 p-2 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
                      >
                        {sources.map((s) => (
                          <option key={s.dataSourceId} value={s.dataSourceId}>{s.dataSourceName || s.dataSourceId} ({s.type})</option>
                        ))}
                      </select>
                    </div>
                  );
                })}
              </div>
            </div>
          )}

          {modelPartyPathQuery.isLoading && <div className="text-gray-400">{t('models.loadingPartyPath')}</div>}
          {modelPartyPathQuery.error && (
            <div className="text-red-500">{t('models.partyPathError', { message: modelPartyPathQuery.error.message })}</div>
          )}
        </div>
      </Modal>

      {/* Model Detail Modal：聚合概要 + 导出详情 + 发布详情。 */}
      <Modal
        isOpen={!!detailModel}
        onClose={() => { setDetailModel(null); setDetailInfo(null); setExportDetail(null); setServingDetail(null); }}
        title={detailModel?.modelName || t('models.detail')}
        footer={
          <Button variant="primary" onClick={() => { setDetailModel(null); setDetailInfo(null); setExportDetail(null); setServingDetail(null); }}>{t('common.close')}</Button>
        }
      >
        <div className="text-xs space-y-4 max-h-[65vh] overflow-y-auto pr-1">
          {/* 概要信息 */}
          <div className="space-y-2">
            {detailInfo?.error ? (
              <div className="text-red-500">{detailInfo.error}</div>
            ) : !detailInfo ? (
              <div className="text-gray-400">{t('common.loading')}</div>
            ) : (
              <>
                <div><span className="font-semibold">{t('models.stats')}:</span> {detailInfo?.modelStats || '-'}</div>
                <div><span className="font-semibold">{t('models.servingCount')}:</span> {detailInfo?.servingDetails?.length || 0}</div>
              </>
            )}
          </div>

          {/* 模型导出详情：各参与方及其训练列。 */}
          <div>
            <div className="text-gray-500 text-[10px] font-semibold uppercase tracking-wide mb-1.5">
              {t('models.exportDetail')}
            </div>
            {exportDetail && (exportDetail.parties?.length ?? 0) > 0 ? (
              <div className="space-y-2">
                {(exportDetail.parties ?? []).map((party, idx) => (
                  <div key={idx} className="p-2.5 rounded bg-gray-50 dark:bg-gray-800 border border-gray-100 dark:border-gray-700">
                    <div className="font-semibold text-gray-800 dark:text-gray-200 mb-1">
                      {party.nodeName || party.nodeId || '-'}
                      <span className="ml-2 font-mono text-[10px] text-gray-400">{party.nodeId}</span>
                    </div>
                    <div className="text-[10px] text-gray-500 mb-1">{t('models.exportColumns')}:</div>
                    <div className="flex flex-wrap gap-1">
                      {(party.columns ?? []).length === 0 && <span className="text-gray-400">-</span>}
                      {(party.columns ?? []).map((col) => (
                        <span key={col} className="px-1.5 py-0.5 rounded bg-blue-50 dark:bg-blue-950/50 text-blue-700 dark:text-blue-300 font-mono text-[10px]">
                          {col}
                        </span>
                      ))}
                    </div>
                  </div>
                ))}
              </div>
            ) : (
              <div className="text-gray-400">{t('models.exportNoParties')}</div>
            )}
          </div>

          {/* 发布详情：服务端点 / 特征映射 / 源路径。 */}
          <div>
            <div className="text-gray-500 text-[10px] font-semibold uppercase tracking-wide mb-1.5">
              {t('models.servingDetail')}
            </div>
            {servingDetail && (servingDetail.servingDetails?.length ?? 0) > 0 ? (
              <div className="space-y-2">
                {servingDetail.servingId && (
                  <div className="font-mono text-[10px] text-gray-500">
                    {t('models.servingId')}: {servingDetail.servingId}
                  </div>
                )}
                {(servingDetail.servingDetails ?? []).map((sd, idx) => (
                  <div key={idx} className="p-2.5 rounded bg-gray-50 dark:bg-gray-800 border border-gray-100 dark:border-gray-700 space-y-1">
                    <div className="font-semibold text-gray-800 dark:text-gray-200 flex items-center gap-2">
                      {sd.nodeName || sd.nodeId || '-'}
                      {sd.isMock && <Badge status="default"><span className="text-[9px]">{t('models.servingMock')}</span></Badge>}
                    </div>
                    {sd.endpoints && (
                      <div className="font-mono text-[10px] text-gray-600 dark:text-gray-400">
                        <span className="text-gray-400">{t('models.servingEndpoints')}: </span>{sd.endpoints}
                      </div>
                    )}
                    {sd.featureHttp && (
                      <div className="font-mono text-[10px] text-gray-600 dark:text-gray-400">
                        <span className="text-gray-400">{t('models.servingFeatureHttp')}: </span>{sd.featureHttp}
                      </div>
                    )}
                    {sd.sourcePath && (
                      <div className="font-mono text-[10px] text-gray-600 dark:text-gray-400 break-all">
                        <span className="text-gray-400">{t('models.servingSourcePath')}: </span>{sd.sourcePath}
                      </div>
                    )}
                    {sd.featureMappings && Object.keys(sd.featureMappings).length > 0 && (
                      <div>
                        <div className="text-[10px] text-gray-400 mb-0.5">{t('models.servingFeatureMappings')}:</div>
                        <div className="flex flex-wrap gap-1">
                          {Object.entries(sd.featureMappings).map(([k, v]) => (
                            <span key={k} className="px-1.5 py-0.5 rounded bg-emerald-50 dark:bg-emerald-950/50 text-emerald-700 dark:text-emerald-300 font-mono text-[10px]">
                              {k} → {v}
                            </span>
                          ))}
                        </div>
                      </div>
                    )}
                  </div>
                ))}
              </div>
            ) : (
              <div className="text-gray-400">{t('models.servingNoDetail')}</div>
            )}
          </div>
        </div>
      </Modal>

      {/* Discard Confirm Dialog */}
      <ConfirmDialog
        isOpen={!!discardTarget}
        title={t('models.discard')}
        message={t('models.discardConfirm')}
        danger
        loading={discardMutation.isPending}
        confirmText={t('models.discard')}
        cancelText={t('common.cancel')}
        onConfirm={() => discardTarget?.modelId && discardMutation.mutate(discardTarget.modelId)}
        onCancel={() => setDiscardTarget(null)}
      />
    </div>
  );
};
