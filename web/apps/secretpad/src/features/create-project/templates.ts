/**
 * Template helpers for the create-project wizard.
 *
 * Reuses the DAG template registry (`features/dag-templates`) and derives the
 * template build configs from the wizard's participant/table selection, the
 * same way the legacy `CreateProjectService.buildScenarioQuickConfigs` did for
 * the embedded alice/bob demo nodes (`id1` / `id2` keys, `y` label).
 */
import type { TableColumnConfigJava } from '@secretpad/api-client';
import {
  allTemplates,
  isSingleTableTemplate,
  isTwoTableTemplate,
  needsFeatureColumns,
  needsLabelColumn,
  needsPredictionName,
  templateByKey,
} from '../dag-templates/registry';
import type { AnyTemplateContribution } from '../dag-templates/types';

export const BLANK_TEMPLATE_KEY = 'blank';

/** Legacy `computeFuncList` (p2p-create-project/compute-func-data.ts). */
export enum ComputeFunc {
  DAG = 'DAG',
  PSI = 'PSI',
  ALL = 'ALL',
}

/** Only DAG is enabled in the legacy UI (PSI / ALL were commented out). */
export const COMPUTE_FUNC_OPTIONS: ComputeFunc[] = [ComputeFunc.DAG];

export type ComputeMode = 'MPC' | 'TEE';

/** Templates without explicit `computeModes` default to MPC (legacy rule); blank fits all modes. */
export function templateSupportsMode(template: AnyTemplateContribution, mode: ComputeMode): boolean {
  if (template.metadata.key === BLANK_TEMPLATE_KEY) return true;
  return (template.metadata.computeModes || ['MPC']).includes(mode);
}

export function templatesForMode(mode: ComputeMode): AnyTemplateContribution[] {
  return allTemplates.filter((t) => templateSupportsMode(t, mode));
}

/** Participant selection used to derive template configs. */
export interface ParticipantSelection {
  nodeId: string;
  /** Datatable selected for quick authorization (optional). */
  datatableId?: string;
}

/** Default join key of the embedded demo nodes. */
export function defaultJoinKey(nodeId: string): string {
  if (nodeId === 'alice') return 'id1';
  if (nodeId === 'bob') return 'id2';
  return '';
}

/**
 * Column configs sent with `project/datatable/add` for quick authorization.
 * Only the embedded demo nodes have known schemas (legacy behaviour); other
 * nodes are authorized without column configs and configured later.
 */
export function defaultTableConfigs(nodeId: string, templateKey: string): TableColumnConfigJava[] | undefined {
  const template = templateByKey(templateKey);
  if (!template || templateKey === BLANK_TEMPLATE_KEY || isSingleTableTemplate(template)) return undefined;
  const key = defaultJoinKey(nodeId);
  if (!key) return undefined;
  return [
    { colName: key, isAssociateKey: true, isProtection: true },
    { colName: 'y', isLabelKey: true },
  ];
}

/** Build the `configs` object passed to `template.build({graphId, configs})`. */
export function buildTemplateConfigs(templateKey: string, participants: ParticipantSelection[]): Record<string, unknown> {
  const template = templateByKey(templateKey);
  if (!template) return {};
  if (isTwoTableTemplate(template)) {
    const [receiver, sender] = participants;
    const configs: Record<string, unknown> = {
      receiverNodeId: receiver?.nodeId || '',
      senderNodeId: sender?.nodeId || '',
      receiverTableId: receiver?.datatableId || '',
      senderTableId: sender?.datatableId || '',
      receiverKey: receiver ? defaultJoinKey(receiver.nodeId) : '',
      senderKey: sender ? defaultJoinKey(sender.nodeId) : '',
    };
    if (needsFeatureColumns(template)) {
      configs.featureSelects = { ss: [] };
      const demo = receiver && defaultJoinKey(receiver.nodeId);
      if (needsLabelColumn(template)) configs.labelSelects = { s: demo ? 'y' : '' };
    }
    if (needsPredictionName(template)) configs.pred = { s: 'pred' };
    return configs;
  }
  if (isSingleTableTemplate(template)) {
    const target = participants.find((p) => p.datatableId) || participants[0];
    const configs: Record<string, unknown> = {
      nodeId: target?.nodeId || '',
      tableId: target?.datatableId || '',
    };
    switch (templateKey) {
      case 'kAnonymity':
      case 'lDiversity':
        configs.qiCols = [];
        configs.saCols = [];
        break;
      case 'localDifferentialPrivacy':
        configs.queryCol = target?.nodeId === 'alice' ? 'age' : '';
        break;
      case 'sanitization':
        configs.sanitizationCols = [];
        break;
    }
    return configs;
  }
  return {};
}
