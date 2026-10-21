/**
 * 模板快速配置（旧版 template-quick-config/quick-config-drawer + GraphService.saveTemplateQuickConfig）：
 * 抽屉表单值 → 模板配置 → `template.build({ graphId, configs })` → graph/update 全量覆盖当前画布。
 */
import type { QuickConfigValues, TemplateType } from '@secretpad/dag-next';
import type { ProjectDetailJava } from '@secretpad/api-client';

export const QUICK_CONFIG_TEMPLATES: Array<{ type: TemplateType; templateKey: string }> = [
  { type: 'PSI', templateKey: 'psi' },
  { type: 'PSI_TEE', templateKey: 'psiTee' },
  { type: 'RISK', templateKey: 'risk' },
  { type: 'TEE', templateKey: 'tee' },
  { type: 'K_ANONYMITY', templateKey: 'kAnonymity' },
  { type: 'L_DIVERSITY', templateKey: 'lDiversity' },
  { type: 'DIFFERENTIAL_PRIVACY', templateKey: 'differentialPrivacy' },
  { type: 'LOCAL_DIFFERENTIAL_PRIVACY', templateKey: 'localDifferentialPrivacy' },
  { type: 'SANITIZATION', templateKey: 'sanitization' },
];

function ownerOf(project: ProjectDetailJava | null | undefined, tableId?: unknown): string | undefined {
  if (!tableId) return undefined;
  return project?.nodes.find((n) => n.datatables.some((t) => t.datatableId === tableId))?.nodeId;
}

type S = { s?: string } | undefined;
type SS = { ss?: string[] } | undefined;
const sOf = (v: unknown): string | undefined => (v as S)?.s || undefined;
const ssOf = (v: unknown): string[] => ((v as SS)?.ss || []).filter(Boolean);

/**
 * 快速配置表单值（旧版 Form 值形状：dataTableReceiver{s}、receiverKey{ss}…）→ 模板 build 所需 configs。
 * 旧版模板直接消费这些字段，新版模板以 receiverTableId / receiverKey 等命名，在此统一换算。
 */
export function quickConfigToTemplateConfigs(type: TemplateType, values: QuickConfigValues, project?: ProjectDetailJava | null): Record<string, unknown> {
  switch (type) {
    case 'PSI':
    case 'PSI_TEE':
    case 'RISK':
    case 'TEE': {
      const receiverTableId = sOf(values.dataTableReceiver);
      const senderTableId = sOf(values.dataTableSender);
      const configs: Record<string, unknown> = {
        receiverTableId,
        senderTableId,
        receiverPartition: values.dataTableReceiverPartition || undefined,
        senderPartition: values.dataTableSenderPartition || undefined,
        receiverKey: ssOf(values.receiverKey),
        senderKey: ssOf(values.senderKey),
        receiverNodeId: ownerOf(project, receiverTableId),
        senderNodeId: ownerOf(project, senderTableId),
      };
      if (values.featureSelects) configs.featureSelects = { ss: ssOf(values.featureSelects) };
      if (values.receiverPSI) configs.receiverParties = ssOf(values.receiverPSI);
      if (type === 'RISK') {
        configs.labelSelects = { ss: ssOf(values.labelSelects) };
        configs.pred = { s: sOf(values.pred) || 'pred' };
        configs.predictReceiver = ssOf(values.receiver)[0];
      }
      if (type === 'TEE') {
        configs.labelSelects = { ss: ssOf(values.labelSelect) };
        configs.trainIdSelect = ssOf(values.trainIdSelect);
        configs.predictIdSelect = ssOf(values.predictIdSelect);
        configs.saveId = !!values.saveId;
        configs.saveLabel = !!values.saveLabel;
        configs.label = ssOf(values.label)[0];
        configs.score = ssOf(values.score)[0];
      }
      return configs;
    }
    default: {
      const tableId = sOf(values.dataTable);
      const configs: Record<string, unknown> = { tableId, nodeId: ownerOf(project, tableId) };
      if (type === 'K_ANONYMITY' || type === 'L_DIVERSITY') {
        configs.qiCols = (values.qiCols as string[] | undefined) ?? [];
        configs.saCols = (values.saCols as string[] | undefined) ?? [];
      }
      if (type === 'DIFFERENTIAL_PRIVACY' || type === 'LOCAL_DIFFERENTIAL_PRIVACY') configs.queryCol = sOf(values.queryCol);
      if (type === 'SANITIZATION') configs.sanitizationCols = (values.sanitizationCols as string[] | undefined) ?? [];
      return configs;
    }
  }
}
