/**
 * 本地数据上传（旧前端 `UploadTable` 的迁移版）。
 *
 * 流程：
 * 1. 选择节点与 CSV 文件（不支持含空格的文件名），客户端读取前 64KB 解析表头并推断字段类型；
 * 2. 调用 `data/upload` 上传文件，返回 `{name, realName, datasource, datasourceType}`；
 * 3. 编辑数据表名称、描述、空缺值与数据表结构（字段名 / 类型 / 描述）；
 * 4. 调用 `data/create`（Java `CreateDataRequest`）完成数据表注册。
 */
import React, { useEffect, useState } from 'react';
import { useMutation } from '@tanstack/react-query';
import { Button, FormField, Input, Modal, Select, Steps, Textarea, toast } from '@secretpad/design-system';
import type { Node, UploadDataResultVO } from '@secretpad/api-client';
import { apiClient, createDataJava } from '@secretpad/api-client';
import { DEFAULT_NULL_STRS_TEXT, parseNullStrs } from '@secretpad/utils';
import { useTranslation } from '@/shared/lib/i18n';
import { NullStrsField, SchemaEditor, readCsvHead, schemaFromCsvPreview } from '../datatable-schema';
import type { SchemaField } from '../datatable-schema';
import { buildCreateDataRequest, checkUploadFile, validateUploadForm } from './model';

interface DataUploadModalProps {
  isOpen: boolean;
  onClose: () => void;
  nodes: Node[];
  defaultNodeId?: string;
  /** Lock the node (node-context view / non-CENTER accounts). */
  fixedNodeId?: string;
  onUploaded?: (result: { nodeId: string; datasource?: string; realName?: string; name?: string }) => void;
  /** Called after `data/create` succeeds. */
  onCreated?: (datatableId: string) => void;
}

export const DataUploadModal: React.FC<DataUploadModalProps> = ({
  isOpen,
  onClose,
  nodes,
  defaultNodeId,
  fixedNodeId,
  onUploaded,
  onCreated,
}) => {
  const { t } = useTranslation();
  const [step, setStep] = useState(0);
  const [nodeId, setNodeId] = useState(fixedNodeId || defaultNodeId || '');
  const [file, setFile] = useState<File | null>(null);
  const [parsing, setParsing] = useState(false);
  const [upload, setUpload] = useState<UploadDataResultVO | null>(null);
  const [tableName, setTableName] = useState('');
  const [description, setDescription] = useState('');
  const [nullStrsText, setNullStrsText] = useState(DEFAULT_NULL_STRS_TEXT);
  const [fields, setFields] = useState<SchemaField[]>([]);
  const [touched, setTouched] = useState(false);

  useEffect(() => {
    if (isOpen) setNodeId(fixedNodeId || defaultNodeId || '');
  }, [isOpen, fixedNodeId, defaultNodeId]);

  const reset = () => {
    setStep(0);
    setFile(null);
    setUpload(null);
    setTableName('');
    setDescription('');
    setNullStrsText(DEFAULT_NULL_STRS_TEXT);
    setFields([]);
    setTouched(false);
  };

  const handleClose = () => {
    reset();
    onClose();
  };

  const pickFile = async (f: File | null) => {
    setFile(null);
    setFields([]);
    if (!f) return;
    const issue = checkUploadFile(f);
    if (issue) {
      toast.error(t(`dataUpload.fileIssue.${issue}`));
      return;
    }
    setParsing(true);
    try {
      const preview = await readCsvHead(f);
      if (preview.header.length === 0) throw new Error('empty');
      setFields(schemaFromCsvPreview(preview, parseNullStrs(nullStrsText) ?? ['']));
      setFile(f);
    } catch {
      toast.error(t('dataUpload.parseError'));
    } finally {
      setParsing(false);
    }
  };

  const uploadMutation = useMutation({
    mutationFn: async () => {
      if (!nodeId || !file) throw new Error(t('dataUpload.missingFileOrNode'));
      return apiClient.uploadData(nodeId, file);
    },
    onSuccess: (res) => {
      setUpload(res);
      setStep(1);
      toast.success(t('dataUpload.success', { name: res.realName || res.name || file?.name || '' }));
      onUploaded?.({ nodeId, datasource: res.datasource, realName: res.realName, name: res.name });
    },
    onError: (e) => toast.error(t('dataUpload.uploadFailed', { message: e instanceof Error ? e.message : String(e) })),
  });

  const values = { tableName, description, nullStrsText, fields };
  const issues = validateUploadForm(values);
  const canSubmit = Object.keys(issues).length === 0;

  const createMutation = useMutation({
    mutationFn: () =>
      createDataJava(buildCreateDataRequest({ nodeId, fileName: file?.name || '', upload: upload || {}, values })),
    onSuccess: (id) => {
      toast.success(t('dataUpload.createSuccess', { name: tableName.trim() }));
      onCreated?.(id);
      handleClose();
    },
    onError: (e) => toast.error(e instanceof Error ? e.message : String(e)),
  });

  const submit = () => {
    setTouched(true);
    if (!canSubmit) return;
    createMutation.mutate();
  };

  const nodeOptions = nodes.map((n) => ({ value: n.nodeId, label: `${n.nodeName || n.nodeId} (${n.nodeId})` }));

  return (
    <Modal
      isOpen={isOpen}
      onClose={handleClose}
      title={t('dataUpload.title')}
      width="max-w-3xl"
      footer={
        <>
          <Button variant="ghost" onClick={handleClose}>
            {t('common.cancel')}
          </Button>
          {step === 0 ? (
            <Button
              variant="primary"
              onClick={() => uploadMutation.mutate()}
              loading={uploadMutation.isPending || parsing}
              disabled={!nodeId || !file}
            >
              {t('dataUpload.upload')}
            </Button>
          ) : (
            <Button variant="primary" onClick={submit} loading={createMutation.isPending} disabled={touched && !canSubmit}>
              {t('dataUpload.submit')}
            </Button>
          )}
        </>
      }
    >
      <div className="space-y-4 text-xs max-h-[70vh] overflow-y-auto pr-1">
        <Steps steps={[t('dataUpload.stepUpload'), t('dataUpload.stepSchema')]} current={step} />

        {step === 0 && (
          <>
            {!fixedNodeId && (
              <FormField label={t('dataTables.nodeSelect')} required>
                <Select
                  aria-label={t('dataTables.nodeSelect')}
                  value={nodeId}
                  placeholder="-"
                  options={nodeOptions}
                  onChange={setNodeId}
                />
              </FormField>
            )}
            <FormField label={t('dataUpload.file')} required help={t('dataUpload.fileHelp')}>
              <input
                type="file"
                accept=".csv"
                aria-label={t('dataUpload.file')}
                onChange={(e) => pickFile(e.target.files?.[0] || null)}
                className="block w-full text-xs text-gray-900 dark:text-gray-100 file:mr-4 file:py-2 file:px-4 file:rounded-lg file:border-0 file:text-xs file:font-semibold file:bg-blue-50 dark:file:bg-blue-950 file:text-blue-700 dark:file:text-blue-400"
              />
            </FormField>
            {file && (
              <div className="text-gray-500">
                {file.name} ({(file.size / 1024).toFixed(1)} KB) · {t('dataUpload.detectedColumns', { n: fields.length })}
              </div>
            )}
          </>
        )}

        {step === 1 && (
          <>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 p-3 rounded-lg bg-gray-50 dark:bg-gray-800">
              <div>
                <span className="text-gray-400">{t('dataUpload.dataFile')}：</span>
                <span className="font-mono">{file?.name}</span>
              </div>
              <div>
                <span className="text-gray-400">{t('dataUpload.resultRealName')}：</span>
                <span className="font-mono break-all">{upload?.realName || '-'}</span>
              </div>
              <div>
                <span className="text-gray-400">{t('dataUpload.resultDatasource')}：</span>
                {upload?.datasource || t('dataUpload.localDatasource')}
              </div>
              <div>
                <span className="text-gray-400">{t('dataUpload.resultType')}：</span>
                {upload?.datasourceType || 'LOCAL'}
              </div>
            </div>
            <FormField
              label={t('dataTables.nameLabel')}
              required
              error={touched && issues.tableName ? t(`dataCommon.nameIssue.${issues.tableName}`) : undefined}
            >
              <Input
                aria-label={t('dataTables.nameLabel')}
                value={tableName}
                maxLength={64}
                invalid={touched && !!issues.tableName}
                placeholder={t('dsForm.namePlaceholder')}
                onChange={(e) => setTableName(e.target.value)}
              />
            </FormField>
            <FormField label={t('dataCommon.descriptionOptional')}>
              <Textarea
                aria-label={t('dataCommon.descriptionOptional')}
                value={description}
                maxLength={200}
                rows={3}
                onChange={(e) => setDescription(e.target.value)}
              />
            </FormField>
            <NullStrsField value={nullStrsText} onChange={setNullStrsText} error={!!issues.nullStrs} />
            <SchemaEditor value={fields} onChange={setFields} showErrors={touched} editableRows={false} />
          </>
        )}
      </div>
    </Modal>
  );
};

DataUploadModal.displayName = 'DataUploadModal';
