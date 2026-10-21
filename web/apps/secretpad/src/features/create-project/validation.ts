/**
 * Pure validation rules for creating / editing a project.
 *
 * Mirrors the legacy `modules/create-project/create-project.view.tsx` antd rules:
 * - project name: required, max 32 chars, only Chinese / letters / digits / `_` / `-`
 * - description: optional, max 128 chars, same charset
 * - participant nodes: 2 ~ 10 ("最多可选十个，至少要两个节点才能创建一个项目")
 *
 * Validators return an i18n key (or null when valid) so the UI can translate.
 */

export const PROJECT_NAME_MAX = 32;
export const PROJECT_DESC_MAX = 128;
export const MIN_PARTICIPANTS = 2;
export const MAX_PARTICIPANTS = 10;

/** Chinese / English letters / digits / underscore / hyphen. */
export const PROJECT_NAME_PATTERN = /^[一-龥A-Za-z0-9\-_]+$/;

export function validateProjectName(name: string): string | null {
  const value = (name || '').trim();
  if (!value) return 'projectWizard.errors.nameRequired';
  if (value.length > PROJECT_NAME_MAX) return 'projectWizard.errors.nameTooLong';
  if (!PROJECT_NAME_PATTERN.test(value)) return 'projectWizard.errors.nameCharset';
  return null;
}

export function validateProjectDescription(desc: string | undefined): string | null {
  const value = (desc || '').trim();
  if (!value) return null;
  if (value.length > PROJECT_DESC_MAX) return 'projectWizard.errors.descTooLong';
  if (!PROJECT_NAME_PATTERN.test(value)) return 'projectWizard.errors.descCharset';
  return null;
}

export function validateParticipants(nodeIds: string[]): string | null {
  const count = new Set(nodeIds.filter(Boolean)).size;
  if (count < MIN_PARTICIPANTS) return 'projectWizard.errors.tooFewNodes';
  if (count > MAX_PARTICIPANTS) return 'projectWizard.errors.tooManyNodes';
  return null;
}

/** TEE projects must pick a TEE node (legacy took `teeNodeList[0]`). */
export function validateTeeNode(computeMode: string, teeNodeId: string | undefined): string | null {
  if (computeMode === 'TEE' && !teeNodeId) return 'projectWizard.errors.teeNodeRequired';
  return null;
}

/**
 * Sanitize a free-form title (e.g. privacy scene name + timestamp) into a
 * valid project name: strips disallowed chars and truncates to 32.
 */
export function toValidProjectName(raw: string): string {
  const cleaned = (raw || '')
    .replace(/\s+/g, '_')
    .replace(/[^一-龥A-Za-z0-9\-_]/g, '')
    .replace(/_+/g, '_');
  return cleaned.slice(0, PROJECT_NAME_MAX) || 'project';
}
