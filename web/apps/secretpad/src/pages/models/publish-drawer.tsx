import React, { useEffect, useMemo, useState } from 'react';
import { useMutation, useQueries, useQuery } from '@tanstack/react-query';
import { Badge, Button, Drawer, Empty, FormField, Input, Select, toast } from '@secretpad/design-system';
import {
  createModelServingJava,
  getModelPartiesJava,
  listProjectFeatureTablesJava,
  type FeatureTableJava,
  type ModelPackJava,
} from '@secretpad/api-client';
import { useTranslation } from '@/shared/lib/i18n';
import { errorText } from '@/features/cooperative-node/format';
import {
  DEFAULT_RESOURCE,
  MOCK_FEATURE_SERVICE,
  autoMatchFeatures,
  availableOnlineFeatures,
  buildPartyConfigs,
  canPublish,
  matchStatus,
  mockMatchFeatures,
  setFeatureMapping,
  unmatchedCount,
  validateResource,
  type PublishRow,
  type ResourceConfig,
} from './feature-mapping';

const STATUS_BADGE = { default: 'default', success: 'success', error: 'error' } as const;

export const PublishModelDrawer: React.FC<{
  open: boolean;
  onClose: () => void;
  onOk: () => void;
  projectId: string;
  /** Preselected model (row action). */
  modelId?: string;
  /** Models that may be published (INIT / OFFLINE / PUBLISH_FAIL). */
  models: ModelPackJava[];
}> = ({ open, onClose, onOk, projectId, modelId, models }) => {
  const { t } = useTranslation();
  const [selected, setSelected] = useState('');
  const [rows, setRows] = useState<PublishRow[]>([]);
  const [expanded, setExpanded] = useState<Record<number, boolean>>({});
  const [onlyErrors, setOnlyErrors] = useState<Record<number, boolean>>({});
  const [resources, setResources] = useState<Record<string, ResourceConfig>>({});

  useEffect(() => {
    if (open) {
      setSelected(modelId || '');
      setRows([]);
      setExpanded({});
      setOnlyErrors({});
      setResources({});
    }
  }, [open, modelId]);

  const partiesQuery = useQuery({
    queryKey: ['model-parties', projectId, selected],
    queryFn: () => getModelPartiesJava(selected, projectId),
    enabled: open && !!selected && !!projectId,
  });
  const parties = useMemo(() => partiesQuery.data || [], [partiesQuery.data]);
  const intoFeatures = useMemo(() => Object.fromEntries(parties.map((p) => [p.nodeId || '', p.columns || []])), [parties]);

  // One row per prediction party (legacy initialised one row per party).
  useEffect(() => {
    setRows(parties.map((p) => ({ nodeId: p.nodeId, featureTableId: undefined, items: (p.columns || []).map((into) => ({ into })) })));
    setResources(Object.fromEntries(parties.map((p) => [p.nodeId || '', { ...DEFAULT_RESOURCE }])));
  }, [parties]);

  const tableQueries = useQueries({
    queries: rows.map((r) => ({
      queryKey: ['feature-tables', projectId, r.nodeId],
      queryFn: () => listProjectFeatureTablesJava(projectId, r.nodeId!),
      enabled: open && !!r.nodeId,
    })),
  });
  const tablesOf = (idx: number): FeatureTableJava[] => tableQueries[idx]?.data || [];
  const onlineOf = (idx: number): string[] => {
    const row = rows[idx];
    if (!row?.featureTableId || row.featureTableId === MOCK_FEATURE_SERVICE) return [];
    const table = tablesOf(idx).find((x) => x.featureTableId === row.featureTableId);
    return (table?.columns || []).map((c) => c.colName || '').filter(Boolean);
  };

  const updateRow = (idx: number, patch: Partial<PublishRow>) => setRows((rs) => rs.map((r, i) => (i === idx ? { ...r, ...patch } : r)));

  const changeNode = (idx: number, nodeId: string) => {
    updateRow(idx, { nodeId: nodeId || undefined, featureTableId: undefined, items: (intoFeatures[nodeId] || []).map((into) => ({ into })) });
    setExpanded((e) => ({ ...e, [idx]: false }));
  };

  const match = (idx: number, featureTableId: string) => {
    const offline = intoFeatures[rows[idx]?.nodeId || ''] || [];
    if (featureTableId === MOCK_FEATURE_SERVICE) {
      updateRow(idx, { featureTableId, items: mockMatchFeatures(offline) });
      setExpanded((e) => ({ ...e, [idx]: false }));
      return;
    }
    const table = tablesOf(idx).find((x) => x.featureTableId === featureTableId);
    const online = (table?.columns || []).map((c) => c.colName || '').filter(Boolean);
    const items = autoMatchFeatures(offline, online);
    updateRow(idx, { featureTableId: featureTableId || undefined, items });
    // Auto-expand when something is left unmatched (legacy behaviour).
    setExpanded((e) => ({ ...e, [idx]: unmatchedCount(items) > 0 }));
  };

  const resourceErrors = Object.entries(resources)
    .filter(([nodeId]) => rows.some((r) => r.nodeId === nodeId))
    .map(([nodeId, r]) => [nodeId, validateResource(r)] as const)
    .filter(([, e]) => e);
  const publishable = canPublish(rows) && resourceErrors.length === 0;

  const mutation = useMutation({
    mutationFn: () => createModelServingJava({ modelId: selected, projectId, partyConfigs: buildPartyConfigs(rows, resources) }),
    onSuccess: () => {
      toast.success(t('models.publishSuccess'));
      onOk();
      onClose();
    },
    onError: (e) => toast.error(errorText(e)),
  });

  const usedNodes = rows.map((r) => r.nodeId).filter(Boolean);
  const nodeOptions = (idx: number) =>
    parties.map((p) => ({
      value: p.nodeId || '',
      label: p.nodeName || p.nodeId || '',
      disabled: p.nodeId !== rows[idx]?.nodeId && usedNodes.includes(p.nodeId),
    }));

  return (
    <Drawer
      isOpen={open}
      onClose={onClose}
      width="max-w-3xl"
      title={t('models.publishTitle')}
      footer={
        <>
          <Button variant="outline" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button variant="primary" disabled={!publishable} loading={mutation.isPending} onClick={() => mutation.mutate()}>
            {t('models.publish')}
          </Button>
        </>
      }
    >
      <div className="space-y-5">
        <FormField label={t('models.selectModel')} required>
          <Select
            value={selected}
            disabled={!!modelId}
            placeholder={t('models.selectModel')}
            onChange={setSelected}
            options={models.map((m) => ({ value: m.modelId || '', label: m.modelName || m.modelId || '' }))}
          />
        </FormField>

        <div>
          <div className="text-sm font-semibold mb-2">{t('models.featureMatch')}</div>
          {!selected && <Empty>{t('models.selectModelFirst')}</Empty>}
          {selected && partiesQuery.isLoading && <div className="text-xs text-gray-400">{t('common.loading')}</div>}
          {selected && partiesQuery.error && <div className="text-xs text-rose-500">{errorText(partiesQuery.error)}</div>}
          {selected && !partiesQuery.isLoading && rows.length === 0 && <Empty>{t('models.noParties')}</Empty>}
          <div className="space-y-3">
            {rows.map((row, idx) => {
              const status = matchStatus(row.nodeId, row.featureTableId, row.items);
              const online = onlineOf(idx);
              const errors = unmatchedCount(row.items);
              const shown = onlyErrors[idx] ? row.items.filter((i) => !i.online) : row.items;
              const isMock = row.featureTableId === MOCK_FEATURE_SERVICE;
              return (
                <div key={idx} className="rounded-lg border border-gray-200 dark:border-gray-800 p-3 space-y-2">
                  <div className="grid grid-cols-1 sm:grid-cols-[auto_1fr_1fr_auto_auto] gap-2 sm:items-center text-xs">
                    <button
                      type="button"
                      className="w-5 text-gray-500 disabled:opacity-40"
                      disabled={!row.featureTableId}
                      onClick={() => setExpanded((e) => ({ ...e, [idx]: !e[idx] }))}
                      aria-label="toggle"
                    >
                      {expanded[idx] ? '−' : '+'}
                    </button>
                    <Select value={row.nodeId || ''} placeholder={t('models.predictNode')} options={nodeOptions(idx)} onChange={(v) => changeNode(idx, v)} />
                    <Select
                      value={row.featureTableId || ''}
                      placeholder={t('models.featureService')}
                      disabled={!row.nodeId}
                      onChange={(v) => match(idx, v)}
                      options={[
                        { value: MOCK_FEATURE_SERVICE, label: t('models.mockService') },
                        ...tablesOf(idx).map((ft) => ({ value: ft.featureTableId || '', label: ft.featureTableName || ft.featureTableId || '' })),
                      ]}
                    />
                    <Badge status={STATUS_BADGE[status]}>{t(`models.match.${status}`)}</Badge>
                    <Button
                      size="sm"
                      variant="link"
                      disabled={!row.featureTableId || isMock}
                      onClick={() => row.featureTableId && match(idx, row.featureTableId)}
                    >
                      {t('models.sameNameMatch')}
                    </Button>
                  </div>
                  {expanded[idx] && (
                    <div className="rounded border border-gray-100 dark:border-gray-800">
                      <div className="grid grid-cols-2 gap-2 px-3 py-1.5 bg-gray-50 dark:bg-gray-800 text-[11px] text-gray-500">
                        <span>{t('models.offlineFeature')}</span>
                        <span className="flex items-center justify-between">
                          {t('models.onlineFeature')}
                          {errors > 0 && (
                            <label className="inline-flex items-center gap-1">
                              <input
                                type="checkbox"
                                checked={!!onlyErrors[idx]}
                                onChange={(e) => setOnlyErrors((o) => ({ ...o, [idx]: e.target.checked }))}
                              />
                              {t('models.onlyErrors', { count: errors })}
                            </label>
                          )}
                        </span>
                      </div>
                      <div className="max-h-60 overflow-y-auto divide-y divide-gray-50 dark:divide-gray-800">
                        {shown.map((item) => (
                          <div key={item.into} className="grid grid-cols-2 gap-2 px-3 py-1 text-xs items-center">
                            <span className="font-mono">{item.into}</span>
                            <Select
                              value={item.online || ''}
                              invalid={!item.online}
                              disabled={isMock}
                              title={isMock ? t('models.mockNoEdit') : undefined}
                              placeholder={t('models.selectManually')}
                              options={availableOnlineFeatures(online, row.items, item.online).map((o) => ({ value: o, label: o }))}
                              onChange={(v) => updateRow(idx, { items: setFeatureMapping(row.items, item.into, v) })}
                            />
                          </div>
                        ))}
                      </div>
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        </div>

        {rows.some((r) => r.nodeId) && (
          <div>
            <div className="text-sm font-semibold mb-2">{t('models.resourceConfig')}</div>
            <div className="space-y-3">
              {rows
                .filter((r) => r.nodeId)
                .map((r) => {
                  const nodeId = r.nodeId!;
                  const res = resources[nodeId] || DEFAULT_RESOURCE;
                  const err = validateResource(res);
                  const setRes = (k: keyof ResourceConfig, v: string) =>
                    setResources((all) => ({ ...all, [nodeId]: { ...res, [k]: Number(v) } }));
                  const name = parties.find((p) => p.nodeId === nodeId)?.nodeName || nodeId;
                  return (
                    <div key={nodeId} className="rounded-lg bg-gray-50 dark:bg-gray-800 p-3">
                      <div className="text-xs font-semibold mb-2">
                        {name} {t('models.predictNode')}
                      </div>
                      <div className="grid grid-cols-2 md:grid-cols-4 gap-2">
                        {(
                          [
                            ['minCpu', t('models.minCpu'), 1000],
                            ['maxCpu', t('models.maxCpu'), 1000],
                            ['minMemory', t('models.minMemory'), 10000],
                            ['maxMemory', t('models.maxMemory'), 10000],
                          ] as const
                        ).map(([k, label, max]) => (
                          <FormField key={k} label={label}>
                            <Input type="number" min={0} max={max} value={res[k]} onChange={(e) => setRes(k, e.target.value)} />
                          </FormField>
                        ))}
                      </div>
                      {err && <div className="text-[11px] text-rose-500 mt-1">{t(`models.resourceError.${err}`)}</div>}
                    </div>
                  );
                })}
            </div>
          </div>
        )}
      </div>
    </Drawer>
  );
};
