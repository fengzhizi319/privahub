/**
 * DAG 模板向导弹窗。
 *
 * 设计说明：
 * - 左侧/顶部分类展示模板卡片（basic / privacy / ml），用户选择模板后右侧展示参数表单。
 * - 表单根据模板类型动态渲染：
 *   - 两表模板（PSI / Risk / TEE）：接收方/发送方节点、数据表、关联键，以及特征列/标签列（Risk/TEE）。
 *   - 单表模板（数据分类分级、脱敏、K-匿名、L-多样性、本地差分隐私、差分隐私）：节点、数据表，以及相关列选择。
 *   - 无参数模板（空白、查询混淆）：仅确认图名称。
 * - 参数填写后点击创建，调用 `createMutation.mutate()`。
 */
import React from 'react';
import { Button, Modal } from '@secretpad/design-system';
import type { DataTable, Project } from '@secretpad/api-client';
import { useTranslation } from '../../shared/lib/i18n';
import {
  isSingleTableTemplate,
  isTwoTableTemplate,
  needsFeatureColumns,
  needsLabelColumn,
  needsNoInputs,
  needsPredictionName,
} from './registry';
import type { UseTemplateWizardReturn } from './use-template-wizard';

interface TemplateWizardProps extends UseTemplateWizardReturn {
  project?: Project;
}

function TableSelect({
  label,
  nodeId,
  nodes,
  tableId,
  tables,
  onNodeChange,
  onTableChange,
  disabled,
}: {
  label: string;
  nodeId: string;
  nodes: { nodeId?: string; nodeName?: string }[];
  tableId: string;
  tables: DataTable[];
  onNodeChange: (id: string) => void;
  onTableChange: (id: string) => void;
  disabled?: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2">
      <label className="block font-semibold text-gray-700 dark:text-gray-300">{label}</label>
      <select
        value={nodeId}
        onChange={(e) => onNodeChange(e.target.value)}
        disabled={disabled}
        className="w-full p-2 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
      >
        <option value="">{t('dag.selectNode')}</option>
        {nodes.map((n) => (
          <option key={n.nodeId} value={n.nodeId}>
            {n.nodeName || n.nodeId}
          </option>
        ))}
      </select>
      <select
        value={tableId}
        onChange={(e) => onTableChange(e.target.value)}
        disabled={disabled || tables.length === 0}
        className="w-full p-2 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
      >
        <option value="">{t('dag.selectTable')}</option>
        {tables.map((table) => (
          <option key={table.tableId} value={table.tableId}>
            {table.tableName || table.tableId}
          </option>
        ))}
      </select>
    </div>
  );
}

function ColumnSelect({
  label,
  value,
  columns,
  onChange,
  multiple,
  disabled,
}: {
  label: string;
  value: string | string[];
  columns: { name: string; type?: string }[];
  onChange: (val: string | string[]) => void;
  multiple?: boolean;
  disabled?: boolean;
}) {
  return (
    <div className="space-y-2">
      <label className="block font-semibold text-gray-700 dark:text-gray-300">{label}</label>
      <select
        multiple={multiple}
        value={value}
        onChange={(e) => {
          if (multiple) {
            const options = Array.from(e.target.selectedOptions).map((o) => o.value);
            onChange(options);
          } else {
            onChange(e.target.value);
          }
        }}
        disabled={disabled}
        className="w-full p-2 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
        size={multiple && columns.length > 0 ? Math.min(5, columns.length) : 1}
      >
        {!multiple && <option value="">请选择列</option>}
        {columns.map((c) => (
          <option key={c.name} value={c.name}>
            {c.name} {c.type ? `(${c.type})` : ''}
          </option>
        ))}
      </select>
      {multiple && Array.isArray(value) && value.length > 0 && (
        <div className="text-xs text-gray-500">已选择 {value.length} 列</div>
      )}
    </div>
  );
}

export const TemplateWizard: React.FC<TemplateWizardProps> = ({
  isOpen,
  close,
  form,
  setField,
  selectTemplate,
  selectedTemplate,
  categories,
  projectNodes,
  receiverTables,
  senderTables,
  singleTables,
  allColumns,
  createMutation,
  isValid,
  project,
}) => {
  const { t } = useTranslation();
  if (!isOpen || !project) return null;

  const twoTable = isTwoTableTemplate(selectedTemplate);
  const singleTable = isSingleTableTemplate(selectedTemplate);
  const noInputs = needsNoInputs(selectedTemplate);
  const needsFeatures = needsFeatureColumns(selectedTemplate);
  const needsLabel = needsLabelColumn(selectedTemplate);
  const needsPred = needsPredictionName(selectedTemplate);

  return (
    <Modal
      isOpen={isOpen}
      onClose={close}
      title={t('dag.templateWizardTitle')}
      width="max-w-2xl"
      footer={
        <>
          <Button variant="ghost" onClick={close}>
            {t('common.cancel')}
          </Button>
          <Button
            variant="primary"
            onClick={() => createMutation.mutate()}
            loading={createMutation.isPending}
            disabled={!isValid}
          >
            {t('dag.createFromTemplate')}
          </Button>
        </>
      }
    >
      <div className="space-y-4 text-xs">
        {/* 图名称 */}
        <div>
          <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">
            {t('dag.nameLabel')}
          </label>
          <input
            type="text"
            value={form.graphName}
            onChange={(e) => setField('graphName', e.target.value)}
            className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
            required
          />
        </div>

        {/* 模板选择 */}
        <div className="space-y-2">
          <label className="block font-semibold text-gray-700 dark:text-gray-300">{t('dag.selectTemplate')}</label>
          <div className="space-y-3">
            {Object.entries(categories).map(([category, templates]) => (
              <div key={category}>
                <div className="text-xs font-medium text-gray-500 mb-1">
                  {t(`dag.templateCategory.${category}` as const)}
                </div>
                <div className="grid grid-cols-2 md:grid-cols-3 gap-2">
                  {templates.map((template) => {
                    const active = template.metadata.key === selectedTemplate.metadata.key;
                    return (
                      <button
                        key={template.metadata.key}
                        type="button"
                        onClick={() => selectTemplate(template.metadata.key)}
                        className={`
                          text-left p-2 rounded-lg border transition-colors
                          ${active ? 'border-blue-500 bg-blue-50 dark:bg-blue-900/20' : 'border-gray-200 dark:border-gray-700 hover:bg-gray-50 dark:hover:bg-gray-800'}
                        `}
                      >
                        <div className="font-semibold text-gray-900 dark:text-gray-100">
                          {t(`dag.templateName.${template.metadata.key}` as const)}
                        </div>
                        <div className="text-[10px] text-gray-500 mt-0.5 line-clamp-2">
                          {t(`dag.templateDesc.${template.metadata.key}` as const)}
                        </div>
                      </button>
                    );
                  })}
                </div>
              </div>
            ))}
          </div>
        </div>

        {/* 参数区域 */}
        <div className="border-t border-gray-200 dark:border-gray-700 pt-3">
          {noInputs && (
            <p className="text-gray-500">
              {t('dag.templateNoInputsHint')}
            </p>
          )}

          {twoTable && (
            <div className="space-y-4">
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <TableSelect
                  label={t('dag.templateReceiverTable')}
                  nodeId={form.receiverNodeId}
                  nodes={projectNodes}
                  tableId={form.receiverTableId}
                  tables={receiverTables}
                  onNodeChange={(id) => {
                    setField('receiverNodeId', id);
                    setField('receiverTableId', '');
                    setField('receiverKey', '');
                  }}
                  onTableChange={(id) => {
                    setField('receiverTableId', id);
                    setField('receiverKey', '');
                  }}
                />
                <TableSelect
                  label={t('dag.templateSenderTable')}
                  nodeId={form.senderNodeId}
                  nodes={projectNodes}
                  tableId={form.senderTableId}
                  tables={senderTables}
                  onNodeChange={(id) => {
                    setField('senderNodeId', id);
                    setField('senderTableId', '');
                    setField('senderKey', '');
                  }}
                  onTableChange={(id) => {
                    setField('senderTableId', id);
                    setField('senderKey', '');
                  }}
                />
              </div>

              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <ColumnSelect
                  label={t('dag.receiverKey')}
                  value={form.receiverKey}
                  columns={receiverTables.find((t) => t.tableId === form.receiverTableId)?.columns || []}
                  onChange={(v) => setField('receiverKey', v as string)}
                />
                <ColumnSelect
                  label={t('dag.senderKey')}
                  value={form.senderKey}
                  columns={senderTables.find((t) => t.tableId === form.senderTableId)?.columns || []}
                  onChange={(v) => setField('senderKey', v as string)}
                />
              </div>

              {needsFeatures && (
                <ColumnSelect
                  label={t('dag.featureColumns')}
                  value={form.featureColumns}
                  columns={allColumns}
                  onChange={(v) => setField('featureColumns', v as string[])}
                  multiple
                />
              )}
              {needsLabel && (
                <ColumnSelect
                  label={t('dag.labelColumn')}
                  value={form.labelColumn}
                  columns={allColumns}
                  onChange={(v) => setField('labelColumn', v as string)}
                />
              )}
              {needsPred && (
                <div>
                  <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">
                    {t('dag.predictionName')}
                  </label>
                  <input
                    type="text"
                    value={form.predictionName}
                    onChange={(e) => setField('predictionName', e.target.value)}
                    className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
                  />
                </div>
              )}
            </div>
          )}

          {singleTable && (
            <div className="space-y-4">
              <TableSelect
                label={t('dag.templateSingleTable')}
                nodeId={form.nodeId}
                nodes={projectNodes}
                tableId={form.tableId}
                tables={singleTables}
                onNodeChange={(id) => {
                  setField('nodeId', id);
                  setField('tableId', '');
                }}
                onTableChange={(id) => setField('tableId', id)}
              />

              {selectedTemplate.metadata.key === 'sanitization' && (
                <ColumnSelect
                  label={t('dag.sanitizationColumns')}
                  value={form.sanitizationColumns}
                  columns={singleTables.find((t) => t.tableId === form.tableId)?.columns || []}
                  onChange={(v) => setField('sanitizationColumns', v as string[])}
                  multiple
                />
              )}

              {(selectedTemplate.metadata.key === 'kAnonymity' || selectedTemplate.metadata.key === 'lDiversity') && (
                <>
                  <ColumnSelect
                    label={t('dag.qiColumns')}
                    value={form.qiColumns}
                    columns={singleTables.find((t) => t.tableId === form.tableId)?.columns || []}
                    onChange={(v) => setField('qiColumns', v as string[])}
                    multiple
                  />
                  <ColumnSelect
                    label={t('dag.saColumns')}
                    value={form.saColumns}
                    columns={singleTables.find((t) => t.tableId === form.tableId)?.columns || []}
                    onChange={(v) => setField('saColumns', v as string[])}
                    multiple
                  />
                </>
              )}

              {selectedTemplate.metadata.key === 'localDifferentialPrivacy' && (
                <ColumnSelect
                  label={t('dag.queryColumn')}
                  value={form.queryColumn}
                  columns={singleTables.find((t) => t.tableId === form.tableId)?.columns || []}
                  onChange={(v) => setField('queryColumn', v as string)}
                />
              )}
            </div>
          )}
        </div>
      </div>
    </Modal>
  );
};

TemplateWizard.displayName = 'TemplateWizard';
