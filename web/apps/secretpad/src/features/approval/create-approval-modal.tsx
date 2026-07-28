import React, { useEffect, useMemo, useState } from 'react';
import { useMutation } from '@tanstack/react-query';
import { Modal, Button, toast } from '@secretpad/design-system';
import { apiClient } from '@secretpad/api-client';
import { useTranslation } from '../../shared/lib/i18n';

/**
 * 审批主动创建 Modal。
 *
 * 对应后端 `POST /api/v1alpha1/approval/create`（`ApprovalController#createApproval`）。
 * 后端通过 `voteType` 外部属性决定 `voteConfig` 的反序列化目标子类，因此这里
 * 按 voteType 声明式地渲染不同的配置字段，并在提交时组装成对应的 voteConfig 对象。
 */

/** 后端支持的四种审批类型（与 `CreateApprovalRequest` 的 `@OneOfType` 保持一致）。 */
const VOTE_TYPES = ['NODE_ROUTE', 'TEE_DOWNLOAD', 'PROJECT_CREATE', 'PROJECT_ARCHIVE'] as const;
type VoteType = (typeof VOTE_TYPES)[number];

/** 单个配置字段的声明式定义。 */
interface FieldDef {
  /** voteConfig 中的键名（与后端 VoteConfig 子类字段一一对应）。 */
  key: string;
  /** i18n 标签键。 */
  i18nKey: string;
  /** 是否必填（仅做前端提示，最终校验在后端）。 */
  required?: boolean;
  /** 是否为列表字段：输入以逗号分隔，提交时拆分为字符串数组（如 participants）。 */
  isList?: boolean;
}

/**
 * 各审批类型对应的 voteConfig 字段定义。
 * 字段顺序即渲染顺序；键名严格对齐后端各 VoteConfig 子类的属性名。
 */
const VOTE_TYPE_FIELDS: Record<VoteType, FieldDef[]> = {
  // NodeRouteVoteConfig: srcNodeId / desNodeId / srcNodeAddr / desNodeAddr
  NODE_ROUTE: [
    { key: 'srcNodeId', i18nKey: 'approval.field.srcNodeId', required: true },
    { key: 'desNodeId', i18nKey: 'approval.field.desNodeId', required: true },
    { key: 'srcNodeAddr', i18nKey: 'approval.field.srcNodeAddr' },
    { key: 'desNodeAddr', i18nKey: 'approval.field.desNodeAddr' },
  ],
  // TeeDownLoadVoteConfig: taskID / resourceID / jobID / resourceType / projectID / graphID
  TEE_DOWNLOAD: [
    { key: 'projectID', i18nKey: 'approval.field.projectID', required: true },
    { key: 'jobID', i18nKey: 'approval.field.jobID' },
    { key: 'taskID', i18nKey: 'approval.field.taskID' },
    { key: 'graphID', i18nKey: 'approval.field.graphID' },
    { key: 'resourceID', i18nKey: 'approval.field.resourceID' },
    { key: 'resourceType', i18nKey: 'approval.field.resourceType' },
  ],
  // ProjectCreateApprovalConfig: projectId / participants(List) / participantNodeInstVOS
  PROJECT_CREATE: [
    { key: 'projectId', i18nKey: 'approval.field.projectId', required: true },
    { key: 'participants', i18nKey: 'approval.field.participants', isList: true },
  ],
  // ProjectArchiveConfig: projectId
  PROJECT_ARCHIVE: [{ key: 'projectId', i18nKey: 'approval.field.projectId', required: true }],
};

/** 统一的输入框样式（与消息页既有表单风格保持一致）。 */
const INPUT_CLASS =
  'w-full p-2.5 rounded-lg bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-900 dark:text-gray-100 text-xs focus:outline-none focus:border-blue-500';

/**
 * 根据 voteType 与表单值组装 voteConfig。
 * - 跳过空值，避免向后端传入空字符串字段；
 * - 列表字段按逗号拆分为数组。
 */
function buildVoteConfig(voteType: VoteType, form: Record<string, string>): Record<string, unknown> {
  const fields = VOTE_TYPE_FIELDS[voteType] ?? [];
  const config: Record<string, unknown> = {};
  for (const field of fields) {
    const raw = (form[field.key] ?? '').trim();
    if (!raw) continue;
    config[field.key] = field.isList
      ? raw.split(',').map((s) => s.trim()).filter(Boolean)
      : raw;
  }
  return config;
}

export interface CreateApprovalModalProps {
  isOpen: boolean;
  onClose: () => void;
  /** 当前登录用户的 ownerId，作为发起方 ID 的默认值。 */
  defaultInitiatorId: string;
  /** 创建成功后的回调（用于刷新消息列表等）。 */
  onCreated?: () => void;
}

export const CreateApprovalModal: React.FC<CreateApprovalModalProps> = ({
  isOpen,
  onClose,
  defaultInitiatorId,
  onCreated,
}) => {
  const { t } = useTranslation();

  const [voteType, setVoteType] = useState<VoteType>('NODE_ROUTE');
  const [initiatorId, setInitiatorId] = useState(defaultInitiatorId);
  const [form, setForm] = useState<Record<string, string>>({});

  // 每次打开 Modal 时重置表单，并同步最新的默认发起方 ID。
  useEffect(() => {
    if (isOpen) {
      setInitiatorId(defaultInitiatorId);
      setForm({});
    }
  }, [isOpen, defaultInitiatorId]);

  const fields = useMemo(() => VOTE_TYPE_FIELDS[voteType] ?? [], [voteType]);

  const createMutation = useMutation({
    mutationFn: () =>
      apiClient.createApproval({
        initiatorId: initiatorId.trim(),
        voteType,
        voteConfig: buildVoteConfig(voteType, form),
      }),
    onSuccess: () => {
      toast.success(t('approval.createSuccess'));
      onCreated?.();
      onClose();
    },
    onError: (e) => toast.error(e instanceof Error ? e.message : String(e)),
  });

  /** 前端轻量校验：发起方 ID 与所有必填字段非空才允许提交。 */
  const canSubmit = useMemo(() => {
    if (!initiatorId.trim()) return false;
    return fields.filter((f) => f.required).every((f) => (form[f.key] ?? '').trim().length > 0);
  }, [initiatorId, fields, form]);

  const handleFieldChange = (key: string, value: string) =>
    setForm((prev) => ({ ...prev, [key]: value }));

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={t('approval.createTitle')}
      width="max-w-xl"
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={createMutation.isPending}>
            {t('common.cancel')}
          </Button>
          <Button
            variant="primary"
            loading={createMutation.isPending}
            disabled={!canSubmit}
            onClick={() => createMutation.mutate()}
          >
            {t('approval.submit')}
          </Button>
        </>
      }
    >
      <div className="space-y-4 text-xs">
        {/* 审批类型选择：切换后动态渲染对应的 voteConfig 字段 */}
        <div>
          <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">
            {t('approval.voteType')}
          </label>
          <select
            value={voteType}
            onChange={(e) => {
              setVoteType(e.target.value as VoteType);
              setForm({}); // 切换类型时清空旧字段，避免残留无关键
            }}
            className={INPUT_CLASS}
          >
            {VOTE_TYPES.map((vt) => (
              <option key={vt} value={vt}>
                {t(`approval.voteTypeName.${vt}`)}
              </option>
            ))}
          </select>
        </div>

        {/* 发起方 ID：默认取当前登录用户的 ownerId */}
        <div>
          <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">
            {t('approval.initiatorId')} <span className="text-red-500">*</span>
          </label>
          <input
            value={initiatorId}
            onChange={(e) => setInitiatorId(e.target.value)}
            className={INPUT_CLASS}
            placeholder={t('approval.initiatorIdPlaceholder')}
          />
        </div>

        {/* 按 voteType 动态渲染 voteConfig 字段 */}
        {fields.map((field) => (
          <div key={`${voteType}-${field.key}`}>
            <label className="block font-semibold text-gray-700 dark:text-gray-300 mb-1">
              {t(field.i18nKey)} {field.required && <span className="text-red-500">*</span>}
            </label>
            <input
              value={form[field.key] ?? ''}
              onChange={(e) => handleFieldChange(field.key, e.target.value)}
              className={INPUT_CLASS}
              placeholder={field.isList ? t('approval.listPlaceholder') : undefined}
            />
          </div>
        ))}
      </div>
    </Modal>
  );
};
