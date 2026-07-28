/**
 * 数据上传组件。
 *
 * 旧前端 `DataController` 提供 `data/upload` 能力，允许用户将本地文件上传到
 * 指定 Kuscia 节点，上传成功后会返回数据源名称、真实文件名与数据源类型，
 * 后续可基于该数据源创建数据表。
 *
 * 本组件提供独立的文件上传弹窗，支持选择节点、选择文件、显示上传结果。
 */
import React, { useState } from 'react';
import { useMutation } from '@tanstack/react-query';
import { Button, Modal, toast } from '@secretpad/design-system';
import type { Node } from '@secretpad/api-client';
import { apiClient } from '@secretpad/api-client';
import { useTranslation } from '../../shared/lib/i18n';

interface DataUploadModalProps {
  isOpen: boolean;
  onClose: () => void;
  nodes: Node[];
  defaultNodeId?: string;
  onUploaded?: (result: { nodeId: string; datasource?: string; realName?: string; name?: string }) => void;
}

export const DataUploadModal: React.FC<DataUploadModalProps> = ({
  isOpen,
  onClose,
  nodes,
  defaultNodeId,
  onUploaded,
}) => {
  const { t } = useTranslation();
  const [selectedNodeId, setSelectedNodeId] = useState(defaultNodeId || '');
  const [file, setFile] = useState<File | null>(null);
  const [result, setResult] = useState<{ name?: string; realName?: string; datasource?: string; datasourceType?: string } | null>(null);

  const uploadMutation = useMutation({
    mutationFn: async () => {
      if (!selectedNodeId || !file) throw new Error(t('dataUpload.missingFileOrNode'));
      return apiClient.uploadData(selectedNodeId, file);
    },
    onSuccess: (res) => {
      setResult(res);
      toast.success(t('dataUpload.success', { name: res.realName || res.name || file?.name || '' }));
      onUploaded?.({ nodeId: selectedNodeId, datasource: res.datasource, realName: res.realName, name: res.name });
    },
    onError: (e) => toast.error(e instanceof Error ? e.message : String(e)),
  });

  const handleClose = () => {
    setFile(null);
    setResult(null);
    setSelectedNodeId(defaultNodeId || '');
    onClose();
  };

  const isValid = !!selectedNodeId && !!file;

  return (
    <Modal
      isOpen={isOpen}
      onClose={handleClose}
      title={t('dataUpload.title')}
      width="max-w-lg"
      footer={
        <>
          <Button variant="ghost" onClick={handleClose}>
            {t('common.close')}
          </Button>
          <Button
            variant="primary"
            onClick={() => uploadMutation.mutate()}
            loading={uploadMutation.isPending}
            disabled={!isValid}
          >
            {t('dataUpload.upload')}
          </Button>
        </>
      }
    >
      <div className="space-y-4 text-xs">
        <div>
          <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">
            {t('dataTables.nodeSelect')}
          </label>
          <select
            value={selectedNodeId}
            onChange={(e) => setSelectedNodeId(e.target.value)}
            className="w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 focus:outline-none focus:border-blue-500"
          >
            <option value="">-</option>
            {nodes.map((n) => (
              <option key={n.nodeId} value={n.nodeId}>
                {n.nodeName} ({n.nodeId})
              </option>
            ))}
          </select>
        </div>

        <div>
          <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">
            {t('dataUpload.file')}
          </label>
          <input
            type="file"
            onChange={(e) => setFile(e.target.files?.[0] || null)}
            className="block w-full text-xs text-gray-900 dark:text-gray-100 file:mr-4 file:py-2 file:px-4 file:rounded-lg file:border-0 file:text-xs file:font-semibold file:bg-blue-50 dark:file:bg-blue-950 file:text-blue-700 dark:file:text-blue-400 hover:file:bg-blue-100 dark:hover:file:bg-blue-900"
          />
          {file && (
            <div className="mt-2 text-gray-500">
              {file.name} ({(file.size / 1024).toFixed(1)} KB)
            </div>
          )}
        </div>

        {result && (
          <div className="p-3 rounded-lg bg-green-50 dark:bg-green-950/30 border border-green-200 dark:border-green-900 text-green-700 dark:text-green-400 space-y-1">
            <div className="font-semibold">{t('dataUpload.result')}</div>
            <div>{t('dataUpload.resultName')}: {result.name || '-'}</div>
            <div>{t('dataUpload.resultRealName')}: {result.realName || '-'}</div>
            <div>{t('dataUpload.resultDatasource')}: {result.datasource || '-'}</div>
            <div>{t('dataUpload.resultType')}: {result.datasourceType || '-'}</div>
          </div>
        )}
      </div>
    </Modal>
  );
};

DataUploadModal.displayName = 'DataUploadModal';
