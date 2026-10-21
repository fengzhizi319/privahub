/**
 * Pure helpers for the projects list page (filtering, counts, permissions).
 */
import type { Project } from '@secretpad/api-client';

export type ModeFilter = 'ALL' | 'MPC' | 'TEE';

export function filterProjects<T extends Pick<Project, 'projectName' | 'computeMode'> & { name?: string; projectId?: string; description?: string; nodes?: { nodeId?: string; nodeName?: string }[] }>(
  projects: T[],
  search: string,
  mode: ModeFilter
): T[] {
  const q = search.trim().toLowerCase();
  return projects.filter((p) => {
    if (mode !== 'ALL' && (p.computeMode || 'MPC').toUpperCase() !== mode) return false;
    if (!q) return true;
    return [p.projectName, p.name, p.projectId, p.description, ...(p.nodes || []).flatMap(node => [node.nodeId, node.nodeName])].filter(Boolean).join(' ').toLowerCase().includes(q);
  });
}

/** `ProjectVO.graphCount` / `jobCount` (Java contract), tolerant of missing fields. */
export function projectCounts(project: object): { graphCount: number; jobCount: number } {
  const p = project as { graphCount?: unknown; jobCount?: unknown };
  return { graphCount: Number(p.graphCount ?? 0) || 0, jobCount: Number(p.jobCount ?? 0) || 0 };
}

/** Delete is enabled only when the typed text equals the project name exactly. */
export function isDeleteConfirmed(project: { projectName?: string; name?: string }, typed: string): boolean {
  const name = project.projectName || project.name || '';
  return !!name && typed === name;
}

export interface ProjectPlatformFlags {
  isCenter: boolean;
  isCenterAdmin: boolean;
  isAutonomy: boolean;
  isTest?: boolean;
}

/**
 * - create: CENTER (admin or EDGE account — legacy handles both) and AUTONOMY on its own platform
 * - manage (edit/delete/add node/add table/stop job): CENTER admin and AUTONOMY
 */
export function projectPermissions(p: ProjectPlatformFlags): { canCreate: boolean; canManage: boolean } {
  return {
    canCreate: p.isCenter || p.isAutonomy || !!p.isTest,
    canManage: p.isCenterAdmin || p.isAutonomy || !!p.isTest,
  };
}

export type ProjectGrouping = 'mode' | 'status';
/** Use explicit project fields, never infer business types from user-entered names. */
export function groupProjects<T extends { computeMode?: string; status?: string }>(projects: T[], by: ProjectGrouping): { key: string; projects: T[] }[] {
  const groups = new Map<string, T[]>();
  for (const project of projects) {
    const key = by === 'mode' ? (project.computeMode || 'MPC').toUpperCase() : (project.status || 'UNKNOWN').toUpperCase();
    const list = groups.get(key) || [];
    list.push(project);
    groups.set(key, list);
  }
  return [...groups].sort(([a], [b]) => a.localeCompare(b)).map(([key, items]) => ({ key, projects: items }));
}
