/**
 * Create-project orchestration (port of legacy `CreateProjectService.createProject`).
 *
 * Sequence (Java contract):
 * 1. `project/create` {name, description, computeMode, teeNodeId, computeFunc} → {projectId}
 * 2. `project/node/add` for each participant (an EDGE account on CENTER is added
 *    automatically by the backend, so its own node is skipped — legacy rule)
 * 3. `project/inst/add` for each distinct participant institution — CENTER only
 *    (AUTONOMY/P2P projects go through the approval flow on /p2p/projects)
 * 4. `project/datatable/add` for each selected datatable (quick authorization)
 * 5. if a template was chosen: `graph/create` + `graph/update` with the template topology
 *
 * Step 1 failing aborts; later failures are collected as warnings so the UI
 * can report partial success while still navigating to the new project.
 */
import {
  addProjectDatatableJava,
  addProjectInstJava,
  addProjectNodeJava,
  apiClient,
  createProjectJava,
} from '@secretpad/api-client';
import type {
  AddProjectDatatableRequestJava,
  CreateProjectRequestJava,
  CreateProjectVOJava,
  GraphEdge,
  GraphNodeInfo,
} from '@secretpad/api-client';
import { templateByKey } from '../dag-templates/registry';
import { BLANK_TEMPLATE_KEY, buildTemplateConfigs, defaultTableConfigs } from './templates';
import type { ComputeMode, ParticipantSelection } from './templates';

export interface CreateProjectInput {
  name: string;
  description?: string;
  computeMode: ComputeMode;
  computeFunc?: string;
  teeNodeId?: string;
  /** Template key from the DAG template registry; undefined → no graph created. */
  templateKey?: string;
  /** Graph name for the template graph (defaults to the template name). */
  graphName?: string;
  participants: (ParticipantSelection & { instId?: string })[];
}

export interface CreateProjectContext {
  platformType: string;
  ownerType: string;
  ownerId: string;
}

export interface CreateProjectDeps {
  createProject: (body: CreateProjectRequestJava) => Promise<CreateProjectVOJava>;
  addNode: (projectId: string, nodeId: string) => Promise<void>;
  addInst: (projectId: string, instId: string) => Promise<void>;
  addDatatable: (body: AddProjectDatatableRequestJava) => Promise<void>;
  createGraph: (input: { projectId: string; name: string }) => Promise<string>;
  updateGraph: (projectId: string, graphId: string, nodes: GraphNodeInfo[], edges: GraphEdge[]) => Promise<void>;
}

export const defaultDeps: CreateProjectDeps = {
  createProject: createProjectJava,
  addNode: addProjectNodeJava,
  addInst: addProjectInstJava,
  addDatatable: addProjectDatatableJava,
  createGraph: (input) => apiClient.createGraph(input),
  updateGraph: (projectId, graphId, nodes, edges) => apiClient.updateGraph(projectId, graphId, nodes, edges),
};

export type CreateStep = 'node' | 'inst' | 'datatable' | 'graph';

export interface CreateWarning {
  step: CreateStep;
  target: string;
  message: string;
}

export interface CreateProjectResult {
  projectId: string;
  graphId?: string;
  warnings: CreateWarning[];
}

const EMBEDDED_INSTS = ['alice', 'bob'];

function msg(e: unknown): string {
  return e instanceof Error ? e.message : String(e);
}

/** Institutions to add via `project/inst/add` (CENTER only). */
export function resolveInstIds(input: CreateProjectInput, ctx: CreateProjectContext): string[] {
  if (ctx.platformType !== 'CENTER') return [];
  const ids = new Set<string>();
  input.participants.forEach((p) => {
    if (p.instId) ids.add(p.instId);
    else if (EMBEDDED_INSTS.includes(p.nodeId)) ids.add(p.nodeId);
  });
  return [...ids];
}

/** Nodes to add via `project/node/add`: dedupe, skip the EDGE account's own node. */
export function resolveNodeIds(input: CreateProjectInput, ctx: CreateProjectContext): string[] {
  const isEdgeOnCenter = ctx.platformType === 'CENTER' && ctx.ownerType === 'EDGE' && !!ctx.ownerId;
  const ids = [...new Set(input.participants.map((p) => p.nodeId).filter(Boolean))];
  return isEdgeOnCenter ? ids.filter((id) => id !== ctx.ownerId) : ids;
}

export async function createProjectWithSetup(
  input: CreateProjectInput,
  ctx: CreateProjectContext,
  deps: CreateProjectDeps = defaultDeps
): Promise<CreateProjectResult> {
  const warnings: CreateWarning[] = [];

  const created = await deps.createProject({
    name: input.name.trim(),
    description: (input.description || '').trim(),
    computeMode: input.computeMode,
    teeNodeId: input.computeMode === 'TEE' ? input.teeNodeId : undefined,
    computeFunc: input.computeFunc,
  });
  const projectId = created?.projectId;
  if (!projectId) throw new Error('project/create returned no projectId');

  // Nodes must be joined before datatables can be authorized, so run sequentially per phase.
  await Promise.all(
    resolveNodeIds(input, ctx).map((nodeId) =>
      deps.addNode(projectId, nodeId).catch((e) => {
        warnings.push({ step: 'node', target: nodeId, message: msg(e) });
      })
    )
  );

  await Promise.all(
    resolveInstIds(input, ctx).map((instId) =>
      deps.addInst(projectId, instId).catch((e) => {
        warnings.push({ step: 'inst', target: instId, message: msg(e) });
      })
    )
  );

  const templateKey = input.templateKey;
  await Promise.all(
    input.participants
      .filter((p) => p.datatableId)
      .map((p) =>
        deps
          .addDatatable({
            projectId,
            nodeId: p.nodeId,
            datatableId: p.datatableId!,
            configs: templateKey ? defaultTableConfigs(p.nodeId, templateKey) : undefined,
            teeNodeId: input.computeMode === 'TEE' ? input.teeNodeId : undefined,
          })
          .catch((e) => {
            warnings.push({ step: 'datatable', target: `${p.nodeId}/${p.datatableId}`, message: msg(e) });
          })
      )
  );

  let graphId: string | undefined;
  const template = templateKey ? templateByKey(templateKey) : undefined;
  if (template) {
    try {
      const name = (input.graphName || '').trim() || (templateKey === BLANK_TEMPLATE_KEY ? '自定义训练流' : `${input.name.trim()}-${templateKey}`);
      graphId = await deps.createGraph({ projectId, name });
      const configs = buildTemplateConfigs(template.metadata.key, input.participants);
      const { nodes, edges } = template.build({ graphId, configs: { ...configs, graphId } });
      if (nodes.length > 0) await deps.updateGraph(projectId, graphId, nodes, edges);
    } catch (e) {
      warnings.push({ step: 'graph', target: templateKey!, message: msg(e) });
    }
  }

  return { projectId, graphId, warnings };
}
