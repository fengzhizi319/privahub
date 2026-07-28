import { api } from './api';
import type { components } from './generated/secretpad';
import { z } from 'zod';
import type {
  Node,
  Project,
  DataTable,
  DataSource,
  JobExecution,
  User,
  CreateNodeInput,
  UpdateNodeInput,
  CreateDataSourceInput,
  CreateDataTableInput,
  DataTableColumn,
  BackendTableColumn,
  ModelPackVO,
  ModelPackInfoVO,
  MessageVO,
  PageScheduledVO,
  GraphMetaVO,
  GraphDetailVO,
  GraphNodeInfo,
  GraphEdge,
  GraphStatus,
  GraphNodeTaskLogsVO,
  GraphNodeOutputVO,
  CompListVO,
  ComponentDef,
  ProjectVO,
  ProjectJobVO,
  MessageDetailVO,
  ModelExportPackageResponse,
  ModelPartyPathResponse,
  ModelPackDetailVO,
  NodeRouterVO,
  AllNodeResultsListVO,
  NodeResultDetailVO,
  InstVO,
  InstTokenVO,
  ProjectParticipantsDetailVO,
  UserContextDTO,
  TaskPageScheduledVO,
  DatatableNodeVO,
  DatasourceDetailAggregateVO,
  DatasourceNodesVO,
  UploadDataResultVO,
  SyncDataDTO,
  PullStatusVO,
} from './schemas';
import {
  ProjectVOSchema,
  ProjectJobVOSchema,
  MessageDetailVOSchema,
  ModelExportPackageResponseSchema,
  ModelPartyPathResponseSchema,
  ModelPackDetailVOSchema,
  NodeRouterVOSchema,
  AllNodeResultsListVOSchema,
  NodeResultDetailVOSchema,
  InstVOSchema,
  InstTokenVOSchema,
  ProjectParticipantsDetailVOSchema,
  UserContextDTOSchema,
  DatatableNodeVOSchema,
  DatasourceDetailAggregateVOSchema,
  DatasourceNodesVOSchema,
  UploadDataResultVOSchema,
  SyncDataDTOSchema,
  PullStatusVOSchema,
  NodeSchema,
  pageResponseSchema,
} from './schemas';

type SecretPadResponse<T> = {
  status?: { code: number; msg?: string };
  data?: T;
};

function unwrap<T>(res: SecretPadResponse<T>): T {
  if (res.status && res.status.code !== 0) {
    throw new Error(res.status.msg || `API error ${res.status.code}`);
  }
  if (res.data === undefined || res.data === null) {
    throw new Error('API returned empty data');
  }
  return res.data;
}

function unwrapVoid(res: SecretPadResponse<unknown>): void {
  if (res.status && res.status.code !== 0) {
    throw new Error(res.status.msg || `API error ${res.status.code}`);
  }
}

function apiError(error: unknown): string {
  return String((error as any)?.message || error || 'Unknown error');
}

/**
 * Runtime-validate an API payload against a Zod schema.
 * Replaces bare `as unknown as` casts with real `safeParse` checks and throws
 * a descriptive error (including offending field paths) on contract violation.
 */
export function validated<S extends z.ZodTypeAny>(schema: S, data: unknown, context?: string): z.output<S> {
  const result = schema.safeParse(data);
  if (!result.success) {
    const issues = result.error.issues
      .slice(0, 5)
      .map((i) => `${i.path.join('.') || '(root)'}: ${i.message}`)
      .join('; ');
    throw new Error(`API schema validation failed${context ? ` [${context}]` : ''}: ${issues}`);
  }
  return result.data;
}

/** Unwrap a SecretPadResponse and runtime-validate its `data` payload. */
function unwrapValidated<S extends z.ZodTypeAny>(schema: S, res: unknown, context?: string): z.output<S> {
  return validated(schema, unwrap(res as SecretPadResponse<unknown>), context);
}

/** Read the stored auth token (shared with raw `fetch` for multipart uploads). */
function getStoredToken(): string | null {
  return typeof localStorage !== 'undefined' ? localStorage.getItem('secretpad-token') : null;
}

/** POST a multipart/form-data body via raw fetch (openapi-fetch is JSON-only). */
async function postMultipart<T>(url: string, formData: FormData, schema: z.ZodType<T>, context?: string): Promise<T> {
  const headers: Record<string, string> = { 'Trace-Id': `${Date.now().toString(36)}-up` };
  const token = getStoredToken();
  if (token) headers['User-Token'] = token;
  const response = await fetch(url, { method: 'POST', headers, body: formData });
  if (!response.ok) throw new Error(`Upload failed with HTTP ${response.status}`);
  const json = (await response.json()) as SecretPadResponse<unknown>;
  return unwrapValidated(schema, json, context);
}

function mapBackendColumn(col: BackendTableColumn): DataTableColumn {
  return {
    name: col.colName || '',
    type: col.colType || '',
    comment: col.colComment,
    classification: col.classification,
  };
}

/** Map a backend ProjectVO into the normalized frontend Project shape. */
function mapProjectVO(vo: ProjectVO): Project {
  return {
    projectId: vo.projectId,
    projectName: vo.projectName || '',
    name: vo.projectName || '',
    description: vo.description || '',
    computeMode: vo.computeMode || 'FL',
    nodes: (vo.nodes || []).map((n) => ({
      nodeId: n.nodeId || '',
      nodeName: n.nodeName,
      nodeType: n.nodeType,
    })),
    status: vo.status || 'ACTIVE',
    jobCount: vo.jobCount ?? 0,
    gmtCreate: vo.gmtCreate || '',
    createTime: vo.gmtCreate || '',
  } as Project;
}

function mapJobStatus(status: string): JobExecution['status'] {
  switch (status) {
    case 'RUNNING':
      return 'RUNNING';
    case 'SUCCEED':
      return 'SUCCEEDED';
    case 'FAILED':
    case 'STOPPED':
      return 'FAILED';
    default:
      return 'PENDING';
  }
}

function formatDuration(start?: string, end?: string): string {
  if (!start) return '-';
  const startMs = new Date(start).getTime();
  const endMs = end ? new Date(end).getTime() : Date.now();
  if (!startMs || Number.isNaN(endMs)) return '-';
  const diff = Math.max(0, Math.floor((endMs - startMs) / 1000));
  const hours = Math.floor(diff / 3600);
  const mins = Math.floor((diff % 3600) / 60);
  const secs = diff % 60;
  if (hours > 0) return `${hours}h ${mins}m ${secs}s`;
  if (mins > 0) return `${mins}m ${secs}s`;
  return `${secs}s`;
}

export const apiClient = {
  async login(name: string, passwordHash: string): Promise<User> {
    const { data, error } = await api.POST('/api/login', {
      body: { name, passwordHash },
    });
    if (error || !data) {
      throw new Error(
        (error as any)?.message || (data as unknown as SecretPadResponse<unknown>)?.status?.msg || 'Login failed'
      );
    }
    const payload = unwrap(data as unknown as SecretPadResponse<User>);
    if (payload.token) {
      localStorage.setItem('secretpad-token', payload.token);
    }
    return payload;
  },

  async logout(): Promise<void> {
    await api.POST('/api/logout').catch(() => undefined);
    localStorage.removeItem('secretpad-token');
  },

  async getNodes(): Promise<Node[]> {
    const { data, error } = await api.POST('/api/v1alpha1/node/list', { body: {} as any });
    if (error) throw new Error(apiError(error));
    return unwrap(data as unknown as SecretPadResponse<Node[]>).map((n) => ({
      ...n,
      name: n.nodeName,
      status: n.nodeStatus,
      createTime: n.gmtCreate,
    }));
  },

  async pageNodes(input: {
    page?: number;
    size?: number;
    search?: string;
    sort?: Record<string, string>;
  }): Promise<{ list: components['schemas']['NodeVO'][]; total: number }> {
    const { data, error } = await api.POST('/api/v1alpha1/node/page', {
      body: input as components['schemas']['PageNodeRequest'],
    });
    if (error) throw new Error(apiError(error));
    const payload = unwrap(
      data as unknown as SecretPadResponse<{ list?: components['schemas']['NodeVO'][]; total?: number }>
    );
    return { list: payload.list || [], total: payload.total || 0 };
  },

  async createNode(input: CreateNodeInput): Promise<string> {
    const { data, error } = await api.POST('/api/v1alpha1/node/create', {
      body: input as components['schemas']['CreateNodeRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrap(data as unknown as SecretPadResponse<string>);
  },

  async updateNode(input: UpdateNodeInput): Promise<string> {
    const { data, error } = await api.POST('/api/v1alpha1/node/update', {
      body: input as components['schemas']['UpdateNodeRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrap(data as unknown as SecretPadResponse<string>);
  },

  async deleteNode(nodeId: string): Promise<void> {
    const { data, error } = await api.POST('/api/v1alpha1/node/delete', {
      body: { nodeId } as components['schemas']['NodeIdRequest'],
    });
    if (error) throw new Error(apiError(error));
    unwrap(data as unknown as SecretPadResponse<unknown>);
  },

  async refreshNode(nodeId: string): Promise<Node> {
    const { data, error } = await api.POST('/api/v1alpha1/node/refresh', {
      body: { nodeId } as components['schemas']['NodeIdRequest'],
    });
    if (error) throw new Error(apiError(error));
    const node = unwrap(data as unknown as SecretPadResponse<Node>);
    return { ...node, name: node.nodeName, status: node.nodeStatus, createTime: node.gmtCreate };
  },

  async getNodeToken(nodeId: string): Promise<{ token?: string; tokenStatus?: string; lastTransitionTime?: string }> {
    const { data, error } = await api.POST('/api/v1alpha1/node/token', {
      body: { nodeId } as components['schemas']['NodeTokenRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrap(data as unknown as SecretPadResponse<{ token?: string; tokenStatus?: string; lastTransitionTime?: string }>);
  },

  async getProjects(): Promise<Project[]> {
    const { data, error } = await api.POST('/api/v1alpha1/project/list', { body: {} as any });
    if (error) throw new Error(apiError(error));
    return unwrap(data as unknown as SecretPadResponse<Project[]>).map((p) => ({
      ...p,
      name: p.projectName,
      createTime: p.gmtCreate,
      jobCount: p.jobCount ?? 0,
    }));
  },

  async getProjectDetail(id: string): Promise<Project | undefined> {
    const { data, error } = await api.POST('/api/v1alpha1/project/get', {
      body: { projectId: id } as components['schemas']['GetProjectRequest'],
    });
    if (error) throw new Error(apiError(error));
    const vo = unwrapValidated(ProjectVOSchema, data, 'project/get');
    return mapProjectVO(vo);
  },

  async createProject(data: Partial<Project>): Promise<Project> {
    const { data: res, error } = await api.POST('/api/v1alpha1/project/create', {
      body: {
        name: data.projectName || data.name || '',
        description: data.description || '',
        computeMode: data.computeMode || 'FL',
        teeNodeId: '',
      } as components['schemas']['CreateProjectRequest'],
    });
    if (error) throw new Error(apiError(error));
    const created = unwrap(res as unknown as SecretPadResponse<components['schemas']['CreateProjectVO']>);
    return {
      projectId: created.projectId || String(Date.now()),
      projectName: data.projectName || data.name || 'New Project',
      name: data.name || data.projectName || 'New Project',
      description: data.description || '',
      computeMode: data.computeMode || 'FL',
      nodes: data.nodes || [],
      status: 'ACTIVE',
      jobCount: 0,
      gmtCreate: new Date().toISOString(),
      createTime: new Date().toISOString(),
    } as Project;
  },

  async getDataSources(ownerId: string): Promise<DataSource[]> {
    const { data, error } = await api.POST('/api/v1alpha1/datasource/list', {
      body: { ownerId, page: 1, size: 1000 } as components['schemas']['DatasourceListRequest'],
    });
    if (error) throw new Error(apiError(error));
    const list = unwrap(
      data as unknown as SecretPadResponse<{ infos?: DataSource[]; list?: DataSource[] }>
    );
    return (list.infos || list.list || []).map((ds) => ({
      ...ds,
      nodeId: ds.nodes?.[0]?.nodeId || '',
      status: ds.status || 'Available',
    }));
  },

  async createDataSource(input: CreateDataSourceInput): Promise<DataSource> {
    const { data, error } = await api.POST('/api/v1alpha1/datasource/create', {
      body: {
        ownerId: input.ownerId,
        nodeIds: input.nodeIds,
        type: input.type,
        name: input.name,
        dataSourceInfo: input.info,
      } as components['schemas']['CreateDatasourceRequest'],
    });
    if (error) throw new Error(apiError(error));
    const created = unwrap(data as unknown as SecretPadResponse<components['schemas']['CreateDatasourceVO']>);
    return {
      datasourceId: created.datasourceId || '',
      name: input.name,
      type: input.type,
      nodes: input.nodeIds.map((id) => ({ nodeId: id })),
      status: 'Available',
      info: input.info,
    };
  },

  async deleteDataSource(ownerId: string, datasourceId: string, type: string): Promise<void> {
    const { data, error } = await api.POST('/api/v1alpha1/datasource/delete', {
      body: { ownerId, datasourceId, type } as components['schemas']['DeleteDatasourceRequest'],
    });
    if (error) throw new Error(apiError(error));
    unwrap(data as unknown as SecretPadResponse<unknown>);
  },

  async getDataTables(ownerId?: string): Promise<DataTable[]> {
    const { data, error } = await api.POST('/api/v1alpha1/datatable/list', {
      body: { pageSize: 1000, pageNumber: 1, ownerId } as components['schemas']['ListDatatableRequest'],
    });
    if (error) throw new Error(apiError(error));
    const payload = unwrap(
      data as unknown as SecretPadResponse<{ datatableNodeVOList?: DataTable[]; list?: DataTable[] }>
    );
    const raw = payload.datatableNodeVOList || payload.list || [];
    return raw.map((item) => {
      const vo = (item as any).datatableVO || (item as any).table || item;
      const backendColumns: BackendTableColumn[] = vo?.schema || vo?.columns || [];
      const columns: DataTableColumn[] = backendColumns.map((c) =>
        c.colName ? mapBackendColumn(c) : (c as unknown as DataTableColumn)
      );
      return {
        ...item,
        tableId: vo?.datatableId || (item as any).tableId || '',
        tableName: vo?.datatableName || (item as any).tableName || '',
        nodeId: item.nodeId || vo?.nodeId || '',
        nodeName: item.nodeName || vo?.nodeName || '',
        datasourceId: vo?.datasourceId || '',
        datasourceType: vo?.datasourceType || '',
        relativeUri: vo?.relativeUri || '',
        status: vo?.status || item.status || 'Available',
        columns,
        rowCount: vo?.rowCount || 0,
        createTime: vo?.gmtCreate || item.gmtCreate,
      } as DataTable;
    });
  },

  async createDataTable(input: CreateDataTableInput): Promise<DataTable> {
    const { data, error } = await api.POST('/api/v1alpha1/datatable/create', {
      body: {
        ownerId: input.ownerId,
        nodeIds: input.nodeIds,
        datatableName: input.datatableName,
        datasourceId: input.datasourceId,
        datasourceName: input.datasourceName,
        datasourceType: input.datasourceType,
        relativeUri: input.relativeUri,
        columns: input.columns,
        desc: input.desc || '',
      } as components['schemas']['CreateDatatableRequest'],
    });
    if (error) throw new Error(apiError(error));
    const created = unwrap(data as unknown as SecretPadResponse<components['schemas']['CreateDatatableVO']>);
    const first = created.dataTableNodeInfos?.[0];
    return {
      tableId: first?.domainDataId || String(Date.now()),
      tableName: input.datatableName,
      nodeId: first?.nodeId || input.ownerId,
      datasourceId: input.datasourceId,
      datasourceType: input.datasourceType,
      relativeUri: input.relativeUri,
      columns: input.columns,
      rowCount: 0,
      status: 'Available',
      gmtCreate: new Date().toISOString(),
      createTime: new Date().toISOString(),
    } as DataTable;
  },

  async deleteDataTable(input: { nodeId: string; datatableId: string; datasourceId?: string; datasourceType?: string; relativeUri?: string }): Promise<void> {
    const { data, error } = await api.POST('/api/v1alpha1/datatable/delete', {
      body: {
        nodeId: input.nodeId,
        datatableId: input.datatableId,
        datasourceId: input.datasourceId,
        datasourceType: input.datasourceType,
        relativeUri: input.relativeUri,
      } as components['schemas']['DeleteDatatableRequest'],
    });
    if (error) throw new Error(apiError(error));
    unwrap(data as unknown as SecretPadResponse<unknown>);
  },

  async getProjectJobs(projectId: string, pageSize = 10, pageNum = 1): Promise<JobExecution[]> {
    const { data, error } = await api.POST('/api/v1alpha1/project/job/list', {
      body: { projectId, pageSize, pageNum } as components['schemas']['ListProjectJobRequest'],
    });
    if (error) throw new Error(apiError(error));
    const payload = unwrap(
      data as unknown as SecretPadResponse<{ data?: any[]; pageSize?: number; pageTotal?: number }>
    );
    const raw = payload.data || [];
    return raw.map((j) => ({
      jobId: j.jobId || '',
      projectId,
      name: j.graph?.name || `Job ${j.jobId || ''}`,
      status: mapJobStatus(j.status),
      duration: formatDuration(j.gmtCreate, j.gmtFinished || (j.finished ? j.gmtModified : undefined)),
      createTime: j.gmtCreate,
      errMsg: j.errMsg,
      finished: j.finished,
    }));
  },

  async getJobs(limit = 10): Promise<JobExecution[]> {
    const projects = await this.getProjects();
    // Fetch per-project jobs concurrently (bounded) instead of a serial N+1 loop.
    const results = await Promise.all(
      projects.slice(0, 5).map((project) =>
        this.getProjectJobs(project.projectId, 10, 1).catch(() => [] as JobExecution[])
      )
    );
    return results
      .flat()
      .sort((a, b) => new Date(b.createTime || 0).getTime() - new Date(a.createTime || 0).getTime())
      .slice(0, limit);
  },

  async getModels(projectId: string, page = 1, size = 100): Promise<ModelPackVO[]> {
    const { data, error } = await api.POST('/api/v1alpha1/model/page', {
      body: { projectId, page, size } as components['schemas']['QueryModelPageRequest'],
    });
    if (error) throw new Error(apiError(error));
    const payload = unwrap(data as unknown as SecretPadResponse<{ modelPacks?: ModelPackVO[]; list?: ModelPackVO[] }>);
    return payload.modelPacks || payload.list || [];
  },

  async getModelInfo(modelId: string, projectId: string): Promise<ModelPackInfoVO> {
    const { data, error } = await api.POST('/api/v1alpha1/model/info', {
      body: { modelId, projectId } as components['schemas']['QueryModelDetailRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrap(data as unknown as SecretPadResponse<ModelPackInfoVO>);
  },

  async deleteModel(modelId: string, nodeId: string): Promise<void> {
    const { data, error } = await api.POST('/api/v1alpha1/model/delete', {
      body: { modelId, nodeId } as components['schemas']['DeleteModelPackRequest'],
    });
    if (error) throw new Error(apiError(error));
    unwrap(data as unknown as SecretPadResponse<unknown>);
  },

  async createModelServing(input: { modelId: string; projectId: string }): Promise<{ servingId?: string }> {
    const { data, error } = await api.POST('/api/v1alpha1/model/serving/create', {
      body: input as components['schemas']['CreateModelServingRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrap(data as unknown as SecretPadResponse<{ servingId?: string }>);
  },

  async deleteModelServing(servingId: string): Promise<void> {
    const { data, error } = await api.POST('/api/v1alpha1/model/serving/delete', {
      body: { servingId } as components['schemas']['DeleteModelServingRequest'],
    });
    if (error) throw new Error(apiError(error));
    unwrap(data as unknown as SecretPadResponse<unknown>);
  },

  async getModelServingDetail(servingId: string): Promise<components['schemas']['ServingDetailVO']> {
    const { data, error } = await api.POST('/api/v1alpha1/model/serving/detail', {
      body: { servingId } as components['schemas']['QueryModelServingRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrap(data as unknown as SecretPadResponse<components['schemas']['ServingDetailVO']>);
  },

  async getMessages(ownerId: string, page = 1, size = 100): Promise<MessageVO[]> {
    const { data, error } = await api.POST('/api/v1alpha1/message/list', {
      // isInitiator 在 OpenAPI 声明中是可选的，但后端反序列化时使用了 boolean
      // 基本类型，缺失会被解析为 null，触发 NPE。因此显式传入 false，表示当前
      // 用户作为消息参与方（非发起方）拉取待处理消息列表。
      body: { ownerId, page, size, isInitiator: false } as components['schemas']['MessageListRequest'],
    });
    if (error) throw new Error(apiError(error));
    const payload = unwrap(data as unknown as SecretPadResponse<{ messages?: MessageVO[]; list?: MessageVO[] }>);
    return payload.messages || payload.list || [];
  },

  async getPendingMessageCount(ownerId: string): Promise<number> {
    const { data, error } = await api.POST('/api/v1alpha1/message/pending', {
      body: { ownerId } as components['schemas']['MessagePendingCountRequest'],
    });
    if (error) throw new Error(apiError(error));
    const count = unwrap(data as unknown as SecretPadResponse<number | string>);
    return Number(count) || 0;
  },

  async getScheduledTasks(projectId: string, page = 1, size = 100): Promise<PageScheduledVO[]> {
    const { data, error } = await api.POST('/api/v1alpha1/scheduled/page', {
      body: { projectId, page, size } as components['schemas']['PageScheduledRequest'],
    });
    if (error) throw new Error(apiError(error));
    const payload = unwrap(data as unknown as SecretPadResponse<{ list?: PageScheduledVO[] }>);
    return payload.list || [];
  },

  async offlineScheduledTask(scheduleId: string): Promise<void> {
    const { data, error } = await api.POST('/api/v1alpha1/scheduled/offline', {
      body: { scheduleId } as components['schemas']['ScheduledOfflineRequest'],
    });
    if (error) throw new Error(apiError(error));
    unwrap(data as unknown as SecretPadResponse<unknown>);
  },

  async deleteScheduledTask(scheduleId: string): Promise<void> {
    const { data, error } = await api.POST('/api/v1alpha1/scheduled/del', {
      body: { scheduleId } as components['schemas']['ScheduledDelRequest'],
    });
    if (error) throw new Error(apiError(error));
    unwrap(data as unknown as SecretPadResponse<unknown>);
  },

  async getGraphs(projectId: string): Promise<GraphMetaVO[]> {
    const { data, error } = await api.POST('/api/v1alpha1/graph/list', {
      body: { projectId } as components['schemas']['ListGraphRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrap(data as unknown as SecretPadResponse<GraphMetaVO[]>);
  },

  async getGraphDetail(projectId: string, graphId: string): Promise<GraphDetailVO> {
    const { data, error } = await api.POST('/api/v1alpha1/graph/detail', {
      body: { projectId, graphId } as components['schemas']['GetGraphRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrap(data as unknown as SecretPadResponse<GraphDetailVO>);
  },

  async createGraph(input: { projectId: string; name: string; nodes?: any[]; edges?: any[] }): Promise<string> {
    const { data, error } = await api.POST('/api/v1alpha1/graph/create', {
      body: {
        projectId: input.projectId,
        name: input.name,
        nodes: input.nodes || [],
        edges: input.edges || [],
      } as components['schemas']['CreateGraphRequest'],
    });
    if (error) throw new Error(apiError(error));
    const created = unwrap(data as unknown as SecretPadResponse<{ graphId?: string }>);
    return created.graphId || String(Date.now());
  },

  async deleteGraph(projectId: string, graphId: string): Promise<void> {
    const { data, error } = await api.POST('/api/v1alpha1/graph/delete', {
      body: { projectId, graphId } as components['schemas']['DeleteGraphRequest'],
    });
    if (error) throw new Error(apiError(error));
    unwrap(data as unknown as SecretPadResponse<unknown>);
  },

  async startGraph(projectId: string, graphId: string, nodes: string[]): Promise<string> {
    const { data, error } = await api.POST('/api/v1alpha1/graph/start', {
      body: { projectId, graphId, nodes } as components['schemas']['StartGraphRequest'],
    });
    if (error) throw new Error(apiError(error));
    const res = unwrap(data as unknown as SecretPadResponse<{ jobId?: string }>);
    return res.jobId || '';
  },

  async getComponents(): Promise<CompListVO[]> {
    const { data, error } = await api.POST('/api/v1alpha1/component/list', { body: {} as any });
    if (error) throw new Error(apiError(error));
    const payload = unwrap(data as unknown as SecretPadResponse<CompListVO[]>);
    return payload || [];
  },

  async batchGetComponent(requests: { domain: string; name: string; version?: string; app?: string }[]): Promise<Record<string, ComponentDef>> {
    const { data, error } = await api.POST('/api/v1alpha1/component/batch', {
      body: requests as components['schemas']['GetComponentRequest'][],
    });
    if (error) throw new Error(apiError(error));
    const payload = unwrap(data as unknown as SecretPadResponse<Record<string, ComponentDef>>);
    return payload || {};
  },

  async listComponentI18n(): Promise<Record<string, string>> {
    const { data, error } = await api.POST('/api/v1alpha1/component/i18n', { body: {} as any });
    if (error) throw new Error(apiError(error));
    const payload = unwrap(data as unknown as SecretPadResponse<Record<string, string>>);
    return payload || {};
  },

  async updateGraph(
    projectId: string,
    graphId: string,
    nodes: GraphNodeInfo[],
    edges: GraphEdge[],
    options?: { maxParallelism?: number; dataSourceConfig?: components['schemas']['GraphDataSourceConfig'][] }
  ): Promise<void> {
    const { data, error } = await api.POST('/api/v1alpha1/graph/update', {
      body: {
        projectId,
        graphId,
        nodes,
        edges,
        maxParallelism: options?.maxParallelism,
        dataSourceConfig: options?.dataSourceConfig,
      } as components['schemas']['FullUpdateGraphRequest'],
    });
    if (error) throw new Error(apiError(error));
    unwrapVoid(data as unknown as SecretPadResponse<unknown>);
  },

  async updateGraphNode(projectId: string, graphId: string, node: GraphNodeInfo): Promise<void> {
    const { data, error } = await api.POST('/api/v1alpha1/graph/node/update', {
      body: { projectId, graphId, node } as components['schemas']['UpdateGraphNodeRequest'],
    });
    if (error) throw new Error(apiError(error));
    unwrapVoid(data as unknown as SecretPadResponse<unknown>);
  },

  async getGraphNodeStatus(projectId: string, graphId: string): Promise<GraphStatus> {
    const { data, error } = await api.POST('/api/v1alpha1/graph/node/status', {
      body: { projectId, graphId } as components['schemas']['ListGraphNodeStatusRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrap(data as unknown as SecretPadResponse<GraphStatus>);
  },

  async getGraphNodeLogs(projectId: string, graphId: string, graphNodeId: string): Promise<GraphNodeTaskLogsVO> {
    const { data, error } = await api.POST('/api/v1alpha1/graph/node/logs', {
      body: { projectId, graphId, graphNodeId } as components['schemas']['GraphNodeLogsRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrap(data as unknown as SecretPadResponse<GraphNodeTaskLogsVO>);
  },

  async getGraphNodeOutput(
    projectId: string,
    graphId: string,
    graphNodeId: string,
    outputId: string
  ): Promise<GraphNodeOutputVO> {
    const { data, error } = await api.POST('/api/v1alpha1/graph/node/output', {
      body: { projectId, graphId, graphNodeId, outputId } as components['schemas']['GraphNodeOutputRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrap(data as unknown as SecretPadResponse<GraphNodeOutputVO>);
  },

  async refreshGraphNodeMaxIndex(
    projectId: string,
    graphId: string,
    currentIndex?: number
  ): Promise<number> {
    const { data, error } = await api.POST('/api/v1alpha1/graph/node/max_index', {
      body: { projectId, graphId, currentIndex } as components['schemas']['GraphNodeMaxIndexRefreshRequest'],
    });
    if (error) throw new Error(apiError(error));
    const payload = unwrap(data as unknown as SecretPadResponse<{ maxIndex?: number }>);
    return payload.maxIndex || 0;
  },

  // ============================ project ============================

  async updateProject(input: { projectId: string; name?: string; description?: string }): Promise<void> {
    const { data, error } = await api.POST('/api/v1alpha1/project/update', {
      body: input as components['schemas']['UpdateProjectRequest'],
    });
    if (error) throw new Error(apiError(error));
    unwrapVoid(data as unknown as SecretPadResponse<unknown>);
  },

  async deleteProject(projectId: string): Promise<void> {
    const { data, error } = await api.POST('/api/v1alpha1/project/delete', {
      body: { projectId } as components['schemas']['GetProjectRequest'],
    });
    if (error) throw new Error(apiError(error));
    unwrapVoid(data as unknown as SecretPadResponse<unknown>);
  },

  async addProjectNode(projectId: string, nodeId: string): Promise<void> {
    const { data, error } = await api.POST('/api/v1alpha1/project/node/add', {
      body: { projectId, nodeId } as components['schemas']['AddNodeToProjectRequest'],
    });
    if (error) throw new Error(apiError(error));
    unwrapVoid(data as unknown as SecretPadResponse<unknown>);
  },

  async addProjectDatatable(input: {
    projectId: string;
    nodeId: string;
    datatableId: string;
    configs?: components['schemas']['TableColumnConfigParam'][];
    teeNodeId?: string;
    datasourceId?: string;
    type?: string;
  }): Promise<void> {
    const { data, error } = await api.POST('/api/v1alpha1/project/datatable/add', {
      body: input as components['schemas']['AddProjectDatatableRequest'],
    });
    if (error) throw new Error(apiError(error));
    unwrapVoid(data as unknown as SecretPadResponse<unknown>);
  },

  async deleteProjectDatatable(input: { projectId: string; nodeId: string; datatableId: string }): Promise<void> {
    const { data, error } = await api.POST('/api/v1alpha1/project/datatable/delete', {
      body: input as components['schemas']['DeleteProjectDatatableRequest'],
    });
    if (error) throw new Error(apiError(error));
    unwrapVoid(data as unknown as SecretPadResponse<unknown>);
  },

  async getProjectDatatable(input: {
    projectId: string;
    nodeId: string;
    datatableId: string;
    type?: string;
  }): Promise<DatatableNodeVO> {
    const { data, error } = await api.POST('/api/v1alpha1/project/datatable/get', {
      body: input as components['schemas']['GetProjectDatatableRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrapValidated(DatatableNodeVOSchema, data, 'project/datatable/get');
  },

  async getProjectJob(projectId: string, jobId: string): Promise<ProjectJobVO> {
    const { data, error } = await api.POST('/api/v1alpha1/project/job/get', {
      body: { projectId, jobId } as components['schemas']['GetProjectJobRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrapValidated(ProjectJobVOSchema, data, 'project/job/get');
  },

  async stopProjectJob(projectId: string, jobId: string): Promise<void> {
    const { data, error } = await api.POST('/api/v1alpha1/project/job/stop', {
      body: { projectId, jobId } as components['schemas']['StopProjectJobTaskRequest'],
    });
    if (error) throw new Error(apiError(error));
    unwrapVoid(data as unknown as SecretPadResponse<unknown>);
  },

  async getJobTaskLogs(input: { projectId: string; jobId: string; taskId: string }): Promise<GraphNodeTaskLogsVO> {
    const { data, error } = await api.POST('/api/v1alpha1/project/job/task/logs', {
      body: input as components['schemas']['GetProjectJobTaskLogRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrap(data as unknown as SecretPadResponse<GraphNodeTaskLogsVO>);
  },

  async getJobTaskOutput(input: {
    projectId: string;
    jobId: string;
    taskId: string;
    outputId: string;
  }): Promise<GraphNodeOutputVO> {
    const { data, error } = await api.POST('/api/v1alpha1/project/job/task/output', {
      body: input as components['schemas']['GetProjectJobTaskOutputRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrap(data as unknown as SecretPadResponse<GraphNodeOutputVO>);
  },

  async getTeeNodes(): Promise<Node[]> {
    const { data, error } = await api.POST('/api/v1alpha1/project/tee/list', { body: {} as never });
    if (error) throw new Error(apiError(error));
    const list = unwrap(data as unknown as SecretPadResponse<Node[] | { list?: Node[] }>);
    return Array.isArray(list) ? list : list.list || [];
  },

  async getProjectOutTables(projectId: string, graphId: string): Promise<components['schemas']['ProjectOutputVO']> {
    const { data, error } = await api.POST('/api/v1alpha1/project/getOutTable', {
      body: { projectId, graphId } as components['schemas']['GetProjectGraphRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrap(data as unknown as SecretPadResponse<components['schemas']['ProjectOutputVO']>);
  },

  async updateProjectTableConfig(input: {
    projectId: string;
    nodeId: string;
    datatableId: string;
    datasourceId?: string;
    teeNodeId?: string;
    type?: string;
    configs?: components['schemas']['TableColumnConfigParam'][];
  }): Promise<void> {
    const { data, error } = await api.POST('/api/v1alpha1/project/update/tableConfig', {
      body: input as components['schemas']['AddProjectDatatableRequest'],
    });
    if (error) throw new Error(apiError(error));
    unwrapVoid(data as unknown as SecretPadResponse<unknown>);
  },

  async getProjectDataSources(
    projectId: string
  ): Promise<components['schemas']['ProjectGraphDomainDataSourceVO'][]> {
    const { data, error } = await api.POST('/api/v1alpha1/project/datasource/list', {
      body: { projectId } as components['schemas']['GetProjectGraphDomainDataSourceRequest'],
    });
    if (error) throw new Error(apiError(error));
    const payload = unwrap(
      data as unknown as SecretPadResponse<components['schemas']['ProjectGraphDomainDataSourceVO'][] | { list?: components['schemas']['ProjectGraphDomainDataSourceVO'][] }>
    );
    return Array.isArray(payload) ? payload : payload.list || [];
  },

  async addProjectInst(projectId: string, instId: string): Promise<void> {
    const { data, error } = await api.POST('/api/v1alpha1/project/inst/add', {
      body: { projectId, instId } as components['schemas']['AddInstToProjectRequest'],
    });
    if (error) throw new Error(apiError(error));
    unwrapVoid(data as unknown as SecretPadResponse<unknown>);
  },

  // ============================ graph ============================

  async stopGraph(projectId: string, graphId: string, graphNodeId?: string): Promise<void> {
    const { data, error } = await api.POST('/api/v1alpha1/graph/stop', {
      body: { projectId, graphId, graphNodeId } as components['schemas']['StopGraphNodeRequest'],
    });
    if (error) throw new Error(apiError(error));
    unwrapVoid(data as unknown as SecretPadResponse<unknown>);
  },

  async renameGraph(projectId: string, graphId: string, name: string): Promise<void> {
    const { data, error } = await api.POST('/api/v1alpha1/graph/meta/update', {
      body: { projectId, graphId, name } as components['schemas']['UpdateGraphMetaRequest'],
    });
    if (error) throw new Error(apiError(error));
    unwrapVoid(data as unknown as SecretPadResponse<unknown>);
  },

  // ============================ message ============================

  async getMessageDetail(input: {
    ownerId: string;
    voteId: string;
    isInitiator: boolean;
    voteType: string;
    projectId?: string;
  }): Promise<MessageDetailVO> {
    const { data, error } = await api.POST('/api/v1alpha1/message/detail', {
      body: input as components['schemas']['MessageDetailRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrapValidated(MessageDetailVOSchema, data, 'message/detail');
  },

  async replyMessage(input: { voteId: string; voteParticipantId: string; action: string; reason?: string }): Promise<void> {
    const { data, error } = await api.POST('/api/v1alpha1/message/reply', {
      body: input as components['schemas']['VoteReplyRequest'],
    });
    if (error) throw new Error(apiError(error));
    unwrapVoid(data as unknown as SecretPadResponse<unknown>);
  },

  // ============================ model ============================

  async packModel(input: {
    projectId: string;
    graphId: string;
    trainId: string;
    modelName: string;
    modelDesc?: string;
    graphNodeOutPutId: string;
    modelPartyConfig: unknown[];
    modelComponent: unknown[];
  }): Promise<ModelExportPackageResponse> {
    const { data, error } = await api.POST('/api/v1alpha1/model/pack', {
      body: input as components['schemas']['ModelExportPackageRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrapValidated(ModelExportPackageResponseSchema, data, 'model/pack');
  },

  async discardModel(modelId: string): Promise<void> {
    const { data, error } = await api.POST('/api/v1alpha1/model/discard', {
      body: { modelId } as components['schemas']['DiscardModelPackRequest'],
    });
    if (error) throw new Error(apiError(error));
    unwrapVoid(data as unknown as SecretPadResponse<unknown>);
  },

  async getModelStatus(jobId: string, projectId: string): Promise<string> {
    const { data, error } = await api.POST('/api/v1alpha1/model/status', {
      body: { jobId, projectId } as components['schemas']['ModelExportStatusRequest'],
    });
    if (error) throw new Error(apiError(error));
    const payload = unwrap(data as unknown as SecretPadResponse<unknown>);
    return typeof payload === 'string' ? payload : (payload as { modelStats?: string })?.modelStats || '';
  },

  async getModelDetail(modelId: string, projectId: string): Promise<ModelPackDetailVO> {
    const { data, error } = await api.POST('/api/v1alpha1/model/detail', {
      body: { modelId, projectId } as components['schemas']['QueryModelDetailRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrapValidated(ModelPackDetailVOSchema, data, 'model/detail');
  },

  async getModelPartyPath(input: {
    projectId: string;
    graphNodeId: string;
    graphNodeOutPutId: string;
  }): Promise<ModelPartyPathResponse[]> {
    const { data, error } = await api.POST('/api/v1alpha1/model/modelPartyPath', {
      body: input as components['schemas']['ModelPartyPathRequest'],
    });
    if (error) throw new Error(apiError(error));
    return validated(z.array(ModelPartyPathResponseSchema), unwrap(data as unknown as SecretPadResponse<unknown>), 'model/modelPartyPath');
  },

  // ============================ data ============================

  async createData(input: {
    nodeId: string;
    name: string;
    tableName: string;
    datasourceType: string;
    datasourceName: string;
    realName?: string;
    description?: string;
    datatableSchema?: { colName?: string; colType?: string; colComment?: string }[];
    nullStrs?: string[];
  }): Promise<string> {
    const { data, error } = await api.POST('/api/v1alpha1/data/create', {
      body: input as components['schemas']['CreateDataRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrap(data as unknown as SecretPadResponse<string>);
  },

  async downloadData(input: { nodeId: string; domainDataId: string }): Promise<Blob> {
    const headers: Record<string, string> = { 'Content-Type': 'application/json' };
    const token = getStoredToken();
    if (token) headers['User-Token'] = token;
    const response = await fetch('/api/v1alpha1/data/download', {
      method: 'POST',
      headers,
      body: JSON.stringify(input),
    });
    if (!response.ok) throw new Error(`Download failed with HTTP ${response.status}`);
    return response.blob();
  },

  async uploadData(nodeId: string, file: File): Promise<UploadDataResultVO> {
    const formData = new FormData();
    formData.append('file', file);
    return postMultipart(
      `/api/v1alpha1/data/upload?Node-Id=${encodeURIComponent(nodeId)}`,
      formData,
      UploadDataResultVOSchema,
      'data/upload'
    );
  },

  async syncData(domainDataId: string, kusciaOriginSource = ''): Promise<SyncDataDTO> {
    const { data, error } = await api.POST('/api/v1alpha1/data/sync', {
      params: { header: { 'kuscia-origin-source': kusciaOriginSource } },
      body: domainDataId,
    });
    if (error) throw new Error(apiError(error));
    return unwrapValidated(SyncDataDTOSchema, data, 'data/sync');
  },

  // ============================ node ============================

  async getNode(nodeId: string): Promise<Node> {
    const { data, error } = await api.POST('/api/v1alpha1/node/get', {
      body: { nodeId } as components['schemas']['NodeIdRequest'],
    });
    if (error) throw new Error(apiError(error));
    const node = unwrapValidated(NodeSchema, data, 'node/get');
    return { ...node, name: node.nodeName, status: node.nodeStatus, createTime: node.gmtCreate };
  },

  async newNodeToken(nodeId: string): Promise<{ token?: string; tokenStatus?: string; lastTransitionTime?: string }> {
    const { data, error } = await api.POST('/api/v1alpha1/node/newToken', {
      body: { nodeId } as components['schemas']['NodeTokenRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrap(data as unknown as SecretPadResponse<{ token?: string; tokenStatus?: string; lastTransitionTime?: string }>);
  },

  async listNodeResults(input: {
    ownerId?: string;
    pageSize?: number;
    pageNumber?: number;
    nodeNamesFilter?: string[];
    kindFilters?: string[];
    nameFilter?: string;
    timeSortingRule?: string;
    teeNodeId?: string;
  }): Promise<AllNodeResultsListVO> {
    const { data, error } = await api.POST('/api/v1alpha1/node/result/list', {
      body: input as components['schemas']['ListNodeResultRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrapValidated(AllNodeResultsListVOSchema, data, 'node/result/list');
  },

  async getNodeResultDetail(input: {
    nodeId: string;
    domainDataId: string;
    dataType?: string;
    dataVendor?: string;
  }): Promise<NodeResultDetailVO> {
    const { data, error } = await api.POST('/api/v1alpha1/node/result/detail', {
      body: input as components['schemas']['GetNodeResultDetailRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrapValidated(NodeResultDetailVOSchema, data, 'node/result/detail');
  },

  // ============================ nodeRoute ============================

  async listNodeRoutes(input: {
    pageNumber?: number;
    pageSize?: number;
    dstNodeId?: string;
    srcNodeId?: string;
    routeType?: string;
  } = {}): Promise<{ data: NodeRouterVO[]; totalCount: number }> {
    const { data, error } = await api.POST('/api/v1alpha1/nodeRoute/page', {
      body: input as components['schemas']['PageNodeRouteRequest'],
    });
    if (error) throw new Error(apiError(error));
    const page = unwrapValidated(pageResponseSchema(NodeRouterVOSchema), data, 'nodeRoute/page');
    return { data: page.data || [], totalCount: page.totalCount || 0 };
  },

  async listRouteNodes(): Promise<Node[]> {
    const { data, error } = await api.POST('/api/v1alpha1/nodeRoute/listNode', { body: {} as any });
    if (error) throw new Error(apiError(error));
    return validated(z.array(NodeSchema), unwrap(data as unknown as SecretPadResponse<unknown>), 'nodeRoute/listNode').map((n) => ({
      ...n,
      name: n.nodeName,
      status: n.nodeStatus,
      createTime: n.gmtCreate,
    }));
  },

  async getNodeRoute(routerId: string): Promise<NodeRouterVO> {
    const { data, error } = await api.POST('/api/v1alpha1/nodeRoute/get', {
      body: { routerId } as components['schemas']['RouterIdRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrapValidated(NodeRouterVOSchema, data, 'nodeRoute/get');
  },

  async updateNodeRoute(input: { routerId: string; srcNetAddress?: string; dstNetAddress?: string }): Promise<string> {
    const { data, error } = await api.POST('/api/v1alpha1/nodeRoute/update', {
      body: input as components['schemas']['UpdateNodeRouterRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrap(data as unknown as SecretPadResponse<string>);
  },

  async deleteNodeRoute(routerId: string): Promise<void> {
    const { data, error } = await api.POST('/api/v1alpha1/nodeRoute/delete', {
      body: { routerId } as components['schemas']['RouterIdRequest'],
    });
    if (error) throw new Error(apiError(error));
    unwrapVoid(data as unknown as SecretPadResponse<unknown>);
  },

  async refreshNodeRoute(routerId: string): Promise<NodeRouterVO> {
    const { data, error } = await api.POST('/api/v1alpha1/nodeRoute/refresh', {
      body: { routerId } as components['schemas']['RouterIdRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrapValidated(NodeRouterVOSchema, data, 'nodeRoute/refresh');
  },

  // ============================ inst (institution) ============================

  async getInst(instId: string): Promise<InstVO> {
    const { data, error } = await api.POST('/api/v1alpha1/inst/get', {
      body: { instId } as components['schemas']['InstRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrapValidated(InstVOSchema, data, 'inst/get');
  },

  async listInstNodes(): Promise<Node[]> {
    const { data, error } = await api.POST('/api/v1alpha1/inst/node/list', { body: {} as any });
    if (error) throw new Error(apiError(error));
    return validated(z.array(NodeSchema), unwrap(data as unknown as SecretPadResponse<unknown>), 'inst/node/list').map((n) => ({
      ...n,
      name: n.nodeName,
      status: n.nodeStatus,
      createTime: n.gmtCreate,
    }));
  },

  async addInstNode(input: { name: string; mode?: number; netAddress?: string; description?: string }): Promise<InstTokenVO> {
    const { data, error } = await api.POST('/api/v1alpha1/inst/node/add', {
      body: input as components['schemas']['CreateNodeRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrapValidated(InstTokenVOSchema, data, 'inst/node/add');
  },

  async deleteInstNode(nodeId: string): Promise<void> {
    const { data, error } = await api.POST('/api/v1alpha1/inst/node/delete', {
      body: { nodeId } as components['schemas']['NodeIdRequest'],
    });
    if (error) throw new Error(apiError(error));
    unwrapVoid(data as unknown as SecretPadResponse<unknown>);
  },

  async getInstNodeToken(nodeId: string): Promise<InstTokenVO> {
    const { data, error } = await api.POST('/api/v1alpha1/inst/node/token', {
      body: { nodeId } as components['schemas']['NodeTokenRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrapValidated(InstTokenVOSchema, data, 'inst/node/token');
  },

  async newInstNodeToken(nodeId: string): Promise<InstTokenVO> {
    const { data, error } = await api.POST('/api/v1alpha1/inst/node/newToken', {
      body: { nodeId } as components['schemas']['NodeTokenRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrapValidated(InstTokenVOSchema, data, 'inst/node/newToken');
  },

  async registerInstNode(jsonData: string, files: { certFile?: File; keyFile?: File; token?: File }): Promise<void> {
    const formData = new FormData();
    if (files.certFile) formData.append('certFile', files.certFile);
    if (files.keyFile) formData.append('keyFile', files.keyFile);
    if (files.token) formData.append('token', files.token);
    const headers: Record<string, string> = {};
    const token = getStoredToken();
    if (token) headers['User-Token'] = token;
    const response = await fetch(`/api/v1alpha1/inst/node/register?json_data=${encodeURIComponent(jsonData)}`, {
      method: 'POST',
      headers,
      body: formData,
    });
    if (!response.ok) throw new Error(`Register node failed with HTTP ${response.status}`);
    const json = (await response.json()) as SecretPadResponse<unknown>;
    unwrapVoid(json);
  },

  // ============================ p2p ============================

  async createP2pNode(input: {
    name?: string;
    mode: number;
    masterNodeId?: string;
    certText: string;
    dstNodeId: string;
    srcNetAddress?: string;
    srcNodeId: string;
    dstNetAddress: string;
    dstInstId?: string;
    dstInstName?: string;
  }): Promise<string> {
    const { data, error } = await api.POST('/api/v1alpha1/p2p/node/create', {
      body: input as components['schemas']['P2pCreateNodeRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrap(data as unknown as SecretPadResponse<string>);
  },

  async deleteP2pNode(routerId: string): Promise<void> {
    const { data, error } = await api.POST('/api/v1alpha1/p2p/node/delete', {
      body: { routerId } as components['schemas']['RouterIdRequest'],
    });
    if (error) throw new Error(apiError(error));
    unwrapVoid(data as unknown as SecretPadResponse<unknown>);
  },

  async createP2pProject(input: {
    name: string;
    description?: string;
    computeMode: string;
    teeNodeId?: string;
    computeFunc?: string;
  }): Promise<{ projectId?: string }> {
    const { data, error } = await api.POST('/api/v1alpha1/p2p/project/create', {
      body: input as components['schemas']['CreateProjectRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrap(data as unknown as SecretPadResponse<{ projectId?: string }>);
  },

  async updateP2pProject(input: { projectId: string; name?: string; description?: string }): Promise<void> {
    const { data, error } = await api.POST('/api/v1alpha1/p2p/project/update', {
      body: input as components['schemas']['UpdateProjectRequest'],
    });
    if (error) throw new Error(apiError(error));
    unwrapVoid(data as unknown as SecretPadResponse<unknown>);
  },

  async listP2pProjects(): Promise<ProjectVO[]> {
    const { data, error } = await api.POST('/api/v1alpha1/p2p/project/list', { body: {} as any });
    if (error) throw new Error(apiError(error));
    return validated(z.array(ProjectVOSchema), unwrap(data as unknown as SecretPadResponse<unknown>), 'p2p/project/list');
  },

  async archiveP2pProject(projectId: string): Promise<ProjectVO[]> {
    const { data, error } = await api.POST('/api/v1alpha1/p2p/project/archive', {
      body: { projectId } as components['schemas']['ArchiveProjectRequest'],
    });
    if (error) throw new Error(apiError(error));
    return validated(z.array(ProjectVOSchema), unwrap(data as unknown as SecretPadResponse<unknown>), 'p2p/project/archive');
  },

  async getP2pParticipants(voteId: string): Promise<ProjectParticipantsDetailVO> {
    const { data, error } = await api.POST('/api/v1alpha1/p2p/project/participants', {
      body: { voteId } as components['schemas']['ProjectParticipantsRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrapValidated(ProjectParticipantsDetailVOSchema, data, 'p2p/project/participants');
  },

  // ============================ user ============================

  async getUser(): Promise<UserContextDTO> {
    const { data, error } = await api.POST('/api/v1alpha1/user/get', { body: {} as any });
    if (error) throw new Error(apiError(error));
    return unwrapValidated(UserContextDTOSchema, data, 'user/get');
  },

  async updatePassword(input: {
    name?: string;
    oldPasswordHash: string;
    newPasswordHash: string;
    confirmPasswordHash: string;
  }): Promise<boolean> {
    const { data, error } = await api.POST('/api/v1alpha1/user/updatePwd', {
      body: input as components['schemas']['UserUpdatePwdRequest'],
    });
    if (error) throw new Error(apiError(error));
    return Boolean(unwrap(data as unknown as SecretPadResponse<boolean>));
  },

  async resetNodeUserPassword(input: {
    nodeId: string;
    name: string;
    passwordHash: string;
    newPasswordHash: string;
  }): Promise<string> {
    const { data, error } = await api.POST('/api/v1alpha1/user/node/resetPassword', {
      body: input as components['schemas']['ResetNodeUserPwdRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrap(data as unknown as SecretPadResponse<string>);
  },

  async resetRemoteUserPassword(input: {
    nodeId: string;
    name: string;
    passwordHash: string;
    newPasswordHash: string;
  }): Promise<string> {
    const { data, error } = await api.POST('/api/v1alpha1/user/remote/resetPassword', {
      body: input as components['schemas']['ResetNodeUserPwdRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrap(data as unknown as SecretPadResponse<string>);
  },

  // ============================ scheduled ============================

  async createScheduledGraph(input: {
    scheduleId?: string;
    scheduleDesc?: string;
    cron: { startTime?: string; endTime?: string; scheduleCycle?: string; scheduleDate?: string; scheduleTime?: string };
    projectId: string;
    graphId: string;
    nodes: string[];
  }): Promise<void> {
    const { data, error } = await api.POST('/api/v1alpha1/scheduled/graph/create', {
      body: input as components['schemas']['ScheduledGraphCreateRequest'],
    });
    if (error) throw new Error(apiError(error));
    unwrapVoid(data as unknown as SecretPadResponse<unknown>);
  },

  async getScheduledId(projectId: string, graphId: string): Promise<string> {
    const { data, error } = await api.POST('/api/v1alpha1/scheduled/id', {
      body: { projectId, graphId } as components['schemas']['ScheduledIdRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrap(data as unknown as SecretPadResponse<string>);
  },

  async getScheduledInfo(scheduleId: string): Promise<ProjectJobVO> {
    const { data, error } = await api.POST('/api/v1alpha1/scheduled/info', {
      body: { scheduleId } as components['schemas']['ScheduledInfoRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrapValidated(ProjectJobVOSchema, data, 'scheduled/info');
  },

  async getScheduledTaskPage(scheduleId: string, page = 1, size = 100): Promise<TaskPageScheduledVO[]> {
    const { data, error } = await api.POST('/api/v1alpha1/scheduled/task/page', {
      body: { scheduleId, page, size } as components['schemas']['TaskPageScheduledRequest'],
    });
    if (error) throw new Error(apiError(error));
    const payload = unwrap(data as unknown as SecretPadResponse<{ list?: TaskPageScheduledVO[]; data?: TaskPageScheduledVO[] }>);
    return payload.list || payload.data || [];
  },

  async rerunScheduledTask(input: { scheduleId: string; scheduleTaskId: string; type?: string }): Promise<void> {
    const { data, error } = await api.POST('/api/v1alpha1/scheduled/task/rerun', {
      body: input as components['schemas']['TaskReRunScheduledRequest'],
    });
    if (error) throw new Error(apiError(error));
    unwrapVoid(data as unknown as SecretPadResponse<unknown>);
  },

  async stopScheduledTask(input: { scheduleId: string; scheduleTaskId: string }): Promise<void> {
    const { data, error } = await api.POST('/api/v1alpha1/scheduled/task/stop', {
      body: input as components['schemas']['TaskStopScheduledRequest'],
    });
    if (error) throw new Error(apiError(error));
    unwrapVoid(data as unknown as SecretPadResponse<unknown>);
  },

  async getScheduledOnceSuccess(projectId: string, graphId: string): Promise<boolean> {
    const { data, error } = await api.POST('/api/v1alpha1/scheduled/graph/once/success', {
      body: { projectId, graphId } as components['schemas']['ScheduledGraphOnceSuccessRequest'],
    });
    if (error) throw new Error(apiError(error));
    const result = unwrap(data as unknown as SecretPadResponse<boolean>);
    return Boolean(result);
  },

  async getScheduledJobs(input: {
    projectId: string;
    graphId: string;
    scheduleTaskId: string;
    pageNum?: number;
    pageSize?: number;
  }): Promise<components['schemas']['ProjectJobSummaryVO'][]> {
    const { data, error } = await api.POST('/api/v1alpha1/scheduled/job/list', {
      body: input as components['schemas']['ScheduleListProjectJobRequest'],
    });
    if (error) throw new Error(apiError(error));
    const payload = unwrap(
      data as unknown as SecretPadResponse<{ list?: components['schemas']['ProjectJobSummaryVO'][] }>
    );
    return payload.list || [];
  },

  async getScheduledTaskInfo(input: { scheduleId: string; scheduleTaskId: string }): Promise<ProjectJobVO> {
    const { data, error } = await api.POST('/api/v1alpha1/scheduled/task/info', {
      body: input as components['schemas']['TaskInfoScheduledRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrapValidated(ProjectJobVOSchema, data, 'scheduled/task/info');
  },

  // ============================ datatable ============================

  async getDataTable(input: {
    nodeId: string;
    datatableId: string;
    datasourceType?: string;
    type?: string;
    teeNodeId?: string;
  }): Promise<DatatableNodeVO> {
    const { data, error } = await api.POST('/api/v1alpha1/datatable/get', {
      body: input as components['schemas']['GetDatatableRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrapValidated(DatatableNodeVOSchema, data, 'datatable/get');
  },

  async pushDatatableToTee(input: {
    nodeId: string;
    datatableId: string;
    teeNodeId?: string;
    datasourceId?: string;
    relativeUri?: string;
  }): Promise<void> {
    const { data, error } = await api.POST('/api/v1alpha1/datatable/pushToTee', {
      body: input as components['schemas']['PushDatatableToTeeRequest'],
    });
    if (error) throw new Error(apiError(error));
    unwrapVoid(data as unknown as SecretPadResponse<unknown>);
  },

  // ============================ datasource ============================

  async getDataSourceDetail(ownerId: string, datasourceId: string, type: string): Promise<DatasourceDetailAggregateVO> {
    const { data, error } = await api.POST('/api/v1alpha1/datasource/detail', {
      body: { ownerId, datasourceId, type } as components['schemas']['DatasourceDetailRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrapValidated(DatasourceDetailAggregateVOSchema, data, 'datasource/detail');
  },

  async getDataSourceNodes(ownerId: string, datasourceId: string): Promise<DatasourceNodesVO> {
    const { data, error } = await api.POST('/api/v1alpha1/datasource/nodes', {
      body: { ownerId, datasourceId } as components['schemas']['DatasourceNodesRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrapValidated(DatasourceNodesVOSchema, data, 'datasource/nodes');
  },

  // ============================ approval ============================

  async createApproval(input: { initiatorId: string; voteType: string; voteConfig?: unknown }): Promise<unknown> {
    const { data, error } = await api.POST('/api/v1alpha1/approval/create', {
      body: input as components['schemas']['CreateApprovalRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrap(data as unknown as SecretPadResponse<unknown>);
  },

  async pullApprovalStatus(input: {
    projectID: string;
    jobID: string;
    taskID: string;
    resourceID: string;
    resourceType?: string;
  }): Promise<PullStatusVO> {
    const { data, error } = await api.POST('/api/v1alpha1/approval/pull/status', {
      body: input as components['schemas']['PullStatusRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrapValidated(PullStatusVOSchema, data, 'approval/pull/status');
  },

  // ============================ feature datasource ============================

  async createFeatureDatasource(input: {
    featureTableName: string;
    type: string;
    url: string;
    ownerId: string;
    nodeIds: string[];
    columns: components['schemas']['TableColumnVO'][];
    datasourceId?: string;
    desc?: string;
  }): Promise<void> {
    const { data, error } = await api.POST('/api/v1alpha1/feature_datasource/create', {
      body: input as components['schemas']['CreateFeatureDatasourceRequest'],
    });
    if (error) throw new Error(apiError(error));
    unwrapVoid(data as unknown as SecretPadResponse<unknown>);
  },

  async listFeatureDatasourceAuth(projectId: string, nodeId: string): Promise<components['schemas']['FeatureDataSourceVO'][]> {
    const { data, error } = await api.POST('/api/v1alpha1/feature_datasource/auth/list', {
      body: { projectId, nodeId } as components['schemas']['ListProjectFeatureDatasourceRequest'],
    });
    if (error) throw new Error(apiError(error));
    const list = unwrap(data as unknown as SecretPadResponse<components['schemas']['FeatureDataSourceVO'][]>);
    return Array.isArray(list) ? list : [];
  },

  // ============================ cloud log ============================

  async getCloudLogs(input: {
    projectId: string;
    jobId?: string;
    taskId?: string;
    graphNodeId?: string;
    nodeId?: string;
    queryParties?: boolean;
  }): Promise<components['schemas']['CloudGraphNodeTaskLogsVO']> {
    const { data, error } = await api.POST('/api/v1alpha1/cloud_log/sls', {
      body: input as components['schemas']['GraphNodeCloudLogsRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrap(data as unknown as SecretPadResponse<components['schemas']['CloudGraphNodeTaskLogsVO']>);
  },

  // ============================ vote sync ============================

  async createVoteSync(dbSyncRequests: components['schemas']['DbSyncRequest'][]): Promise<unknown> {
    const { data, error } = await api.POST('/api/v1alpha1/vote_sync/create', {
      body: { dbSyncRequests } as components['schemas']['VoteSyncRequest'],
    });
    if (error) throw new Error(apiError(error));
    return unwrap(data as unknown as SecretPadResponse<unknown>);
  },

  // ============================ version ============================

  async listComponentVersions(): Promise<components['schemas']['ComponentVersion']> {
    const { data, error } = await api.POST('/api/v1alpha1/version/list', { body: {} as never });
    if (error) throw new Error(apiError(error));
    return unwrap(data as unknown as SecretPadResponse<components['schemas']['ComponentVersion']>);
  },
};
