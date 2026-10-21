/**
 * 注册数据源抽屉（旧前端 `CreateDataSourceModal` 的迁移版）。
 *
 * - 按数据源类型渲染结构化表单（OSS / ODPS / MYSQL / HTTP / LOCAL），不再使用 JSON；
 * - 节点连接配置：多节点模式（AUTONOMY）可添加最多 5 个节点，其余模式固定为当前节点；
 * - 提交调用 `datasource/create`，部分节点失败时按节点展示 `failedCreatedNodes`。
 */
import React, { useEffect, useMemo, useState } from 'react';
import { useMutation } from '@tanstack/react-query';
import { Button, Drawer, FormField, Input, RadioGroup, Select, toast } from '@secretpad/design-system';
import type { SelectOption } from '@secretpad/design-system';
import { createDatasourceJava } from '@secretpad/api-client';
import { useTranslation } from '@/shared/lib/i18n';
import {
  DS_CREATABLE_TYPES,
  MAX_DATASOURCE_NODES,
  ODPS_NAME_PREFIX,
  buildCreateDatasourceRequest,
  datasourceSubmitName,
  emptyDatasourceForm,
  validateDatasourceForm,
} from './model';
import type { DatasourceFormValues, DsFormErrors, DsFormType } from './model';
import { notifyFailedNodes } from './notify-failed-nodes';

export interface DatasourceFormDrawerProps {
  isOpen: boolean;
  onClose: () => void;
  onCreated?: () => void;
  ownerId: string;
  /** Candidate nodes (label/value). */
  nodeOptions: SelectOption[];
  /** Initial node (single-node mode uses it as the fixed node). */
  defaultNodeId?: string;
  /** Allow adding up to 5 node configurations (AUTONOMY). */
  multiNode?: boolean;
}

export const DatasourceFormDrawer: React.FC<DatasourceFormDrawerProps> = ({
  isOpen,
  onClose,
  onCreated,
  ownerId,
  nodeOptions,
  defaultNodeId,
  multiNode = false,
}) => {
  const { t } = useTranslation();
  const [values, setValues] = useState<DatasourceFormValues>(() => emptyDatasourceForm());
  const [nodeIds, setNodeIds] = useState<string[]>([defaultNodeId || '']);
  const [touched, setTouched] = useState(false);

  useEffect(() => {
    if (isOpen) {
      setValues(emptyDatasourceForm());
      setNodeIds([multiNode ? '' : defaultNodeId || '']);
      setTouched(false);
    }
  }, [isOpen, defaultNodeId, multiNode]);

  const errors: DsFormErrors = useMemo(() => validateDatasourceForm(values, nodeIds), [values, nodeIds]);
  const hasErrors = Object.keys(errors).length > 0;
  const err = (k: keyof DsFormErrors) => (touched && errors[k] ? t(`dsForm.err.${errors[k]}`) : undefined);

  const set = <K extends keyof DatasourceFormValues>(k: K, v: DatasourceFormValues[K]) =>
    setValues((prev) => ({ ...prev, [k]: v }));

  const createMutation = useMutation({
    mutationFn: () => createDatasourceJava(buildCreateDatasourceRequest(values, ownerId, nodeIds)),
    onSuccess: (vo) => {
      toast.success(t('dsForm.createSuccess', { name: datasourceSubmitName(values) }));
      notifyFailedNodes(vo.failedCreatedNodes, t('dataCommon.partialFailed'));
      onCreated?.();
      onClose();
    },
    onError: (e) => toast.error(e instanceof Error ? e.message : String(e)),
  });

  const submit = () => {
    setTouched(true);
    if (hasErrors) return;
    createMutation.mutate();
  };

  const typeOptions = DS_CREATABLE_TYPES.map((ty) => ({ value: ty, label: ty }));

  const text = (k: keyof DatasourceFormValues, label: React.ReactNode, opts: { required?: boolean; secret?: boolean; placeholder?: string; prefix?: string; help?: React.ReactNode } = {}) => (
    <FormField label={label} required={opts.required !== false} error={err(k)} help={opts.help}>
      <div className="flex items-stretch">
        {opts.prefix && (
          <span className="inline-flex items-center px-2 rounded-l-lg border border-r-0 border-gray-200 dark:border-gray-700 bg-gray-100 dark:bg-gray-800 text-gray-500 text-xs font-mono">
            {opts.prefix}
          </span>
        )}
        <Input
          type={opts.secret ? 'password' : 'text'}
          autoComplete={opts.secret ? 'new-password' : 'off'}
          value={values[k] as string}
          invalid={!!err(k)}
          placeholder={opts.placeholder ?? t('dsForm.placeholder')}
          onChange={(e) => set(k, e.target.value as never)}
          className={opts.prefix ? 'rounded-l-none' : ''}
          aria-label={typeof label === 'string' ? label : String(k)}
        />
      </div>
    </FormField>
  );

  const nameField = text('name', t('dsForm.displayName'), {
    placeholder: t('dsForm.namePlaceholder'),
    prefix: values.type === 'ODPS' ? ODPS_NAME_PREFIX : undefined,
  });

  const usedNodeIds = new Set(nodeIds.filter(Boolean));

  return (
    <Drawer
      isOpen={isOpen}
      onClose={onClose}
      title={t('dsForm.title')}
      width="max-w-2xl"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button variant="primary" onClick={submit} loading={createMutation.isPending} disabled={touched && hasErrors}>
            {t('dsForm.submit')}
          </Button>
        </>
      }
    >
      <div className="space-y-4 text-xs">
        <FormField label={t('dsForm.type')} required>
          <RadioGroup
            name={t('dsForm.type')}
            options={typeOptions}
            value={values.type}
            onChange={(v) => setValues((prev) => ({ ...emptyDatasourceForm(v as DsFormType), name: prev.name }))}
          />
        </FormField>
        {values.type === 'OSS' && <p className="text-gray-500">{t('dsForm.ossTip')}</p>}
        {values.type === 'MYSQL' && (
          <div className="p-2 rounded-lg bg-blue-50 dark:bg-blue-950/30 text-blue-700 dark:text-blue-300">{t('dsForm.mysqlTip')}</div>
        )}
        {values.type === 'HTTP' && <p className="text-gray-500">{t('dsForm.httpTip')}</p>}

        {values.type === 'ODPS' && text('odpsProject', t('dsForm.odpsProject'), { placeholder: t('dsForm.namePlaceholder') })}
        {nameField}

        {values.type === 'OSS' && (
          <>
            {text('ossEndpoint', t('dsForm.endpoint'))}
            {text('ak', 'AccessKeyID')}
            {text('sk', 'AccessKeySecret', { secret: true })}
            <FormField label="virtualhost" required>
              <label className="inline-flex items-center gap-2 cursor-pointer">
                <input
                  type="checkbox"
                  role="switch"
                  aria-label="virtualhost"
                  checked={values.virtualhost}
                  onChange={(e) => set('virtualhost', e.target.checked)}
                  className="w-4 h-4 rounded"
                />
                <span className="text-gray-500">{values.virtualhost ? t('dsForm.on') : t('dsForm.off')}</span>
              </label>
            </FormField>
            {text('bucket', 'bucket', { help: t('dsForm.bucketHelp') })}
            {text('prefix', t('dsForm.prefix'), { required: false })}
          </>
        )}

        {values.type === 'MYSQL' && (
          <>
            <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
              <div className="sm:col-span-2">
                {text('mysqlHost', t('dsForm.host'), { prefix: 'jdbc:mysql://', placeholder: 'hostname | ip' })}
              </div>
              {text('mysqlPort', t('dsForm.port'), { placeholder: '3306' })}
            </div>
            {text('mysqlDatabase', 'database', { help: t('dsForm.databaseHelp') })}
            {text('mysqlUser', 'user')}
            {text('mysqlPassword', 'password', { secret: true })}
          </>
        )}

        {values.type === 'ODPS' && (
          <>
            {text('odpsEndpoint', t('dsForm.endpoint'))}
            {text('accessId', 'AccessKeyID')}
            {text('accessKey', 'AccessKeySecret', { secret: true })}
          </>
        )}

        {values.type === 'LOCAL' && text('localPath', t('dsForm.localPath'), { placeholder: '/home/kuscia/var/storage/data' })}

        <div className="pt-2 border-t border-gray-100 dark:border-gray-800">
          <div className="font-semibold text-gray-700 dark:text-gray-300 mb-2">{t('dsForm.nodeConfig')}</div>
          <div className="space-y-2">
            {nodeIds.map((id, idx) => (
              <div key={idx} className="flex items-center gap-2">
                <span className="w-16 shrink-0 text-gray-500">{t('dsForm.nodeN', { n: idx + 1 })}</span>
                <Select
                  aria-label={t('dsForm.nodeN', { n: idx + 1 })}
                  value={id}
                  placeholder={t('dsForm.selectNode')}
                  disabled={!multiNode}
                  options={nodeOptions.map((o) => ({ ...o, disabled: o.value !== id && usedNodeIds.has(o.value) }))}
                  onChange={(v) => setNodeIds((prev) => prev.map((p, i) => (i === idx ? v : p)))}
                  className="max-w-xs"
                />
                {multiNode && nodeIds.length > 1 && (
                  <Button
                    size="sm"
                    variant="ghost"
                    aria-label={t('common.delete')}
                    onClick={() => setNodeIds((prev) => prev.filter((_, i) => i !== idx))}
                  >
                    ✕
                  </Button>
                )}
              </div>
            ))}
          </div>
          {err('nodeIds') && <div className="text-rose-500 mt-1">{err('nodeIds')}</div>}
          {multiNode && (
            <Button
              size="sm"
              variant="link"
              className="mt-2"
              disabled={nodeIds.length >= MAX_DATASOURCE_NODES}
              title={nodeIds.length >= MAX_DATASOURCE_NODES ? t('dsForm.maxNodes', { n: MAX_DATASOURCE_NODES }) : ''}
              onClick={() => setNodeIds((prev) => [...prev, ''])}
            >
              ＋ {t('dsForm.addNode')}
            </Button>
          )}
        </div>
      </div>
    </Drawer>
  );
};
