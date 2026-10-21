/**
 * Privacy scene catalogue → DAG template mapping (legacy `modules/privacy-scenes`).
 */
import { templateByKey } from '../../features/dag-templates/registry';
import { toValidProjectName } from '../../features/create-project/validation';
import type { ComputeMode, CreateProjectPreset } from '../../features/create-project';

export interface PrivacyScene {
  key: string;
  tags: string[];
  /** Template key in the DAG template registry. */
  templateKey: string;
}

export const scenes: PrivacyScene[] = [
  { key: 'psi', tags: ['PSI', 'Privacy Set Intersection'], templateKey: 'psi' },
  { key: 'mpcRisk', tags: ['MPC', 'LR', 'WOE'], templateKey: 'risk' },
  { key: 'tee', tags: ['TEE', 'Trusted Execution'], templateKey: 'tee' },
  { key: 'classification', tags: ['Data Classification', 'L1-L5'], templateKey: 'dataClassification' },
  { key: 'sanitization', tags: ['Masking', 'Data Sanitization'], templateKey: 'sanitization' },
  { key: 'kAnonymity', tags: ['K-Anonymity', 'Anonymization'], templateKey: 'kAnonymity' },
  { key: 'lDiversity', tags: ['L-Diversity', 'Anonymization'], templateKey: 'lDiversity' },
  { key: 'localDp', tags: ['Local DP', 'Differential Privacy'], templateKey: 'localDifferentialPrivacy' },
  { key: 'dpQuery', tags: ['DP Query', 'Differential Privacy'], templateKey: 'differentialPrivacy' },
  { key: 'queryObfuscation', tags: ['Query Obfuscation', 'Privacy'], templateKey: 'queryObfuscation' },
  { key: 'federatedLearning', tags: ['MPC', 'SS-SGD', 'Train → Predict → Evaluate'], templateKey: 'logisticRegression' },
];

/** Default demo participants (legacy quick create used alice + bob). */
export const DEFAULT_SCENE_NODES = ['alice', 'bob'];

function timestamp(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${pad(d.getMonth() + 1)}${pad(d.getDate())}${pad(d.getHours())}${pad(d.getMinutes())}`;
}

/**
 * Wizard preset for a scene, or null when the deployment supports none of the
 * template's compute modes. MPC is preferred when available (legacy default).
 */
export function scenePreset(
  scene: PrivacyScene,
  title: string,
  platform: { supportsMpc: boolean; supportsTee: boolean },
  now: Date = new Date()
): CreateProjectPreset | null {
  const template = templateByKey(scene.templateKey);
  const modes = scene.templateKey === 'blank' ? ['MPC', 'TEE'] : template?.metadata.computeModes || ['MPC'];
  let computeMode: ComputeMode | null = null;
  if (modes.includes('MPC') && platform.supportsMpc) computeMode = 'MPC';
  else if (modes.includes('TEE') && platform.supportsTee) computeMode = 'TEE';
  if (!computeMode) return null;
  const suffix = `-${timestamp(now)}`;
  const base = toValidProjectName(title).slice(0, 32 - suffix.length);
  return {
    templateKey: scene.templateKey,
    computeMode,
    nodeIds: [...DEFAULT_SCENE_NODES],
    name: `${base}${suffix}`,
  };
}
