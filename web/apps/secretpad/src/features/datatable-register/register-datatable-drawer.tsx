/**
 * 基于已注册数据源添加数据表（旧前端 `DataAddDrawer` 非 LOCAL 分支的迁移版）。
 *
 * - 选择数据源（仅列出至少一个节点可用的 OSS / HTTP / ODPS / MYSQL 数据源）；
 * - 按类型填写：OSS 文件相对路径、MYSQL 原始表名、ODPS 表名（前缀为 ODPS project）+ 分区、HTTP 地址（附请求示例）；
 * - 多节点模式（AUTONOMY）可选择最多 5 个所属节点，部分节点失败按节点提示；
 * - 数据表结构使用共享的 `SchemaEditor`，提交 `datatable/create`。
 */
import React, { useEffect, useMemo, useState } from 'react';
import { useMutation, useQuery } from '@tanstack/react-query';
import { Button, CheckboxGroup, Drawer, FormField, Input, Select, Textarea, toast } from '@secretpad/design-system';
import {
  REMOTE_DATASOURCE_TYPES,
  createDatatableJava,
  getDatasourceDetailJava,
  getDatasourceNodesJava,
  listDatasourcesJava,
} from '@secretpad/api-client';
import { useTranslation } from '@/shared/lib/i18n';
import { NullStrsField, SchemaEditor } from '../datatable-schema';
import { notifyFailedNodes } from '../datasource-form/notify-failed-nodes';
import {
  MAX_REGISTER_NODES,
  buildCreateDatatableRequest,
  emptyRegisterForm,
  supportsNullStrs,
  tableKeywordWarnings,
  validateRegisterForm,
} from './model';
import type { RegisterFormValues } from './model';

export interface RegisterDatatableDrawerProps {
  isOpen: boolean;
  onClose: () => void;
  onCreated?: () => void;
  /** Owner (node id in CENTER/EDGE; institution-side node in AUTONOMY). */
  ownerId: string;
  /** AUTONOMY: pick up to 5 nodes among the datasource's available nodes. */
  multiNode?: boolean;
  /** Preselect a datasource. */
  defaultDatasourceId?: string;
}

export const RegisterDatatableDrawer: React.FC<RegisterDatatableDrawerProps> = ({
  isOpen,
  onClose,
  onCreated,
  ownerId,
  multiNode = false,
  defaultDatasourceId,
}) => {
  const { t } = useTranslation();
  const [datasourceId, setDatasourceId] = useState(defaultDatasourceId || '');
  const [values, setValues] = useState<RegisterFormValues>(emptyRegisterForm);
  const [nodeIds, setNodeIds] = useState<string[]>([]);
  const [touched, setTouched] = useState(false);

  useEffect(() => {
    if (isOpen) {
      setDatasourceId(defaultDatasourceId || '');
      setValues(emptyRegisterForm());
      setNodeIds([]);
      setTouched(false);
    }
  }, [isOpen, defaultDatasourceId]);

  const sourcesQuery = useQuery({
    queryKey: ['register-datasources', ownerId],
    queryFn: () => listDatasourcesJava({ ownerId, types: REMOTE_DATASOURCE_TYPES, page: 1, size: 1000, status: '', name: '' }),
    enabled: isOpen && !!ownerId,
  });
  const sources = useMemo(
    () => (sourcesQuery.data?.infos || []).filter((s) => (s.nodes || []).some((n) => n.status === 'Available')),
    [sourcesQuery.data],
  );
  const current = sources.find((s) => s.datasourceId === datasourceId);
  const type = current?.type || '';

  const nodesQuery = useQuery({
    queryKey: ['register-datasource-nodes', ownerId, datasourceId],
    queryFn: () => getDatasourceNodesJava({ ownerId, datasourceId }),
    enabled: isOpen && multiNode && !!datasourceId,
  });
  const nodeOptions = (nodesQuery.data || [])
    .filter((n) => n.status === 'Available')
    .map((n) => ({ value: n.nodeId || '', label: n.nodeName || n.nodeId || '' }));

  const odpsQuery = useQuery({
    queryKey: ['register-odps-project', ownerId, datasourceId],
    queryFn: () => getDatasourceDetailJava({ ownerId, datasourceId, type: 'ODPS' }),
    enabled: isOpen && type === 'ODPS',
  });
  const odpsProject = String(odpsQuery.data?.info?.project || '');

  const effectiveNodeIds = multiNode ? nodeIds : [ownerId];
  const issues = validateRegisterForm(type, values, effectiveNodeIds, { multiNode });
  const warnings = tableKeywordWarnings(type, values);
  const canSubmit = !!current && Object.keys(issues).length === 0;
  const showErr = (k: keyof typeof issues) => touched && !!issues[k];

  const set = <K extends keyof RegisterFormValues>(k: K, v: RegisterFormValues[K]) => setValues((p) => ({ ...p, [k]: v }));

  const createMutation = useMutation({
    mutationFn: () =>
      createDatatableJava(
        buildCreateDatatableRequest({
          ownerId,
          nodeIds: effectiveNodeIds,
          datasource: { datasourceId, name: current?.name || '', type },
          values,
        }),
      ),
    onSuccess: (vo) => {
      toast.success(t('dataTables.createSuccess'));
      notifyFailedNodes(vo.failedCreatedNodes, t('dataCommon.partialFailed'));
      onCreated?.();
      onClose();
    },
    onError: (e) => toast.error(e instanceof Error ? e.message : String(e)),
  });

  const submit = () => {
    setTouched(true);
    if (!canSubmit) return;
    createMutation.mutate();
  };

  const addressLabel: Record<string, string> = {
    OSS: t('dtRegister.ossAddress'),
    HTTP: t('dtRegister.httpAddress'),
    ODPS: t('dtRegister.odpsTable'),
    MYSQL: t('dtRegister.mysqlTable'),
  };
  const addressPlaceholder: Record<string, string> = {
    OSS: t('dtRegister.ossAddressPlaceholder'),
    HTTP: t('dsForm.placeholder'),
    ODPS: t('dtRegister.odpsTablePlaceholder'),
    MYSQL: t('dtRegister.mysqlTablePlaceholder'),
  };

  return (
    <Drawer
      isOpen={isOpen}
      onClose={onClose}
      title={t('dtRegister.title')}
      width="max-w-3xl"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button variant="primary" onClick={submit} loading={createMutation.isPending} disabled={!current || (touched && !canSubmit)}>
            {t('common.confirm')}
          </Button>
        </>
      }
    >
      <div className="space-y-4 text-xs">
        <FormField label={t('dtRegister.datasource')} required help={sources.length === 0 && !sourcesQuery.isLoading ? t('dtRegister.noDatasource') : undefined}>
          <Select
            aria-label={t('dtRegister.datasource')}
            value={datasourceId}
            placeholder={t('dsForm.selectPlaceholder')}
            options={sources.map((s) => ({ value: s.datasourceId || '', label: `${s.name} (${s.type})` }))}
            onChange={(v) => {
              setDatasourceId(v);
              setValues(emptyRegisterForm());
              setNodeIds([]);
              setTouched(false);
            }}
          />
        </FormField>

        {current && (
          <>
            <FormField
              label={addressLabel[type] || t('dtRegister.address')}
              required
              error={showErr('address') ? t('dsForm.err.required') : warnings.address ? t('dtRegister.keywordWarn') : undefined}
            >
              <div className="flex items-stretch">
                {type === 'ODPS' && odpsProject && (
                  <span className="inline-flex items-center px-2 rounded-l-lg border border-r-0 border-gray-200 dark:border-gray-700 bg-gray-100 dark:bg-gray-800 text-gray-500 font-mono">
                    {odpsProject}.
                  </span>
                )}
                <Input
                  aria-label={addressLabel[type] || t('dtRegister.address')}
                  value={values.address}
                  invalid={showErr('address')}
                  placeholder={addressPlaceholder[type]}
                  onChange={(e) => set('address', e.target.value)}
                />
              </div>
            </FormField>

            <FormField
              label={t('dataTables.nameLabel')}
              required
              error={
                showErr('tableName')
                  ? t(`dataCommon.nameIssue.${issues.tableName}`)
                  : warnings.tableName
                    ? t('dtRegister.keywordWarn')
                    : undefined
              }
            >
              <Input
                aria-label={t('dataTables.nameLabel')}
                value={values.tableName}
                invalid={showErr('tableName')}
                placeholder={t('dsForm.namePlaceholder')}
                onChange={(e) => set('tableName', e.target.value)}
              />
            </FormField>

            {multiNode && (
              <FormField
                label={t('dtRegister.nodes')}
                required
                help={t('dsForm.maxNodes', { n: MAX_REGISTER_NODES })}
                error={showErr('nodeIds') ? t(`dtRegister.nodesIssue.${issues.nodeIds}`) : undefined}
              >
                {nodeOptions.length === 0 ? (
                  <div className="text-gray-400">{t('dtRegister.noAvailableNodes')}</div>
                ) : (
                  <CheckboxGroup options={nodeOptions} value={nodeIds} onChange={setNodeIds} max={MAX_REGISTER_NODES} />
                )}
              </FormField>
            )}

            {type === 'HTTP' && (
              <details className="p-3 rounded-lg bg-gray-50 dark:bg-gray-800">
                <summary className="cursor-pointer font-semibold">{t('dtRegister.httpExample')}</summary>
                <pre className="mt-2 font-mono text-[11px] whitespace-pre-wrap">{t('dtRegister.httpExampleBody')}</pre>
                <a className="text-blue-600 hover:underline" href="https://www.c-life.com/docs" target="_blank" rel="noreferrer">
                  {t('dtRegister.httpDocs')}
                </a>
              </details>
            )}

            <FormField label={t('dataCommon.description')} error={showErr('description') ? t('dtRegister.descTooLong') : undefined}>
              <Textarea
                aria-label={t('dataCommon.description')}
                value={values.description}
                maxLength={100}
                rows={2}
                placeholder={t('dtRegister.descPlaceholder')}
                onChange={(e) => set('description', e.target.value)}
              />
            </FormField>

            {supportsNullStrs(type) && (
              <NullStrsField value={values.nullStrsText} onChange={(v) => set('nullStrsText', v)} error={!!issues.nullStrs} />
            )}

            <SchemaEditor value={values.fields} onChange={(f) => set('fields', f)} showErrors={touched} />

            {type === 'ODPS' && (
              <FormField label={t('dtRegister.partition')} error={showErr('partitions') ? t('dtRegister.partitionUnknown') : undefined}>
                <div className="space-y-2">
                  {values.partitions.map((p, i) => (
                    <div key={i} className="flex items-center gap-2">
                      <span className="w-16 shrink-0 text-gray-500">{t(i === 0 ? 'dtRegister.partition1' : 'dtRegister.partition2')}</span>
                      <Input
                        aria-label={t(i === 0 ? 'dtRegister.partition1' : 'dtRegister.partition2')}
                        value={p}
                        placeholder={t('dtRegister.partitionPlaceholder')}
                        onChange={(e) => set('partitions', values.partitions.map((x, j) => (j === i ? e.target.value : x)))}
                      />
                    </div>
                  ))}
                </div>
              </FormField>
            )}
          </>
        )}
      </div>
    </Drawer>
  );
};
