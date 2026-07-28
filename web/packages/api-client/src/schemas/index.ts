import { z } from 'zod';

// User Schema (returned by /api/login)
export const UserSchema = z.object({
  ownerId: z.string(),
  name: z.string(),
  role: z.enum(['ADMIN', 'DEVELOPER', 'GUEST']).optional(),
  token: z.string(),
  platformType: z.string(),
  platformNodeId: z.string().optional(),
  ownerType: z.string().optional(),
  deployMode: z.string().optional(),
  apiResources: z.array(z.string()).optional(),
});
export type User = z.infer<typeof UserSchema>;

// Platform Schema
export const PlatformSchema = z.object({
  platformType: z.enum(['CENTER', 'EDGE', 'AUTONOMY', 'P2P', 'TEST']),
  nodeId: z.string().optional(),
});
export type Platform = z.infer<typeof PlatformSchema>;

// Node Schema (from /api/v1alpha1/node/*)
export const NodeSchema = z.object({
  nodeId: z.string(),
  nodeName: z.string(),
  controlNodeId: z.string().optional(),
  masterNodeId: z.string().optional(),
  type: z.string(),
  nodeStatus: z.string(),
  netAddress: z.string().optional(),
  description: z.string().optional(),
  gmtCreate: z.string(),
  mode: z.number().optional(),
  nodeAuthenticationCode: z.string().optional(),
  token: z.string().optional(),
  tokenStatus: z.string().optional(),
  // legacy optional fields kept for local mock fallbacks
  ip: z.string().optional(),
  cpu: z.number().optional(),
  memory: z.number().optional(),
  createTime: z.string().optional(),
  status: z.string().optional(),
  name: z.string().optional(),
});
export type Node = z.infer<typeof NodeSchema>;

// Project participant node
export const ProjectNodeSchema = z.object({
  nodeId: z.string(),
  nodeName: z.string().optional(),
  nodeType: z.string().optional(),
});
export type ProjectNode = z.infer<typeof ProjectNodeSchema>;

// Project Schema (from /api/v1alpha1/project/*)
export const ProjectSchema = z.object({
  projectId: z.string(),
  projectName: z.string(),
  description: z.string().optional(),
  computeMode: z.string(),
  nodes: z.array(ProjectNodeSchema).default([]),
  status: z.string(),
  jobCount: z.number().default(0),
  gmtCreate: z.string(),
  // legacy fallbacks
  name: z.string().optional(),
  createTime: z.string().optional(),
});
export type Project = z.infer<typeof ProjectSchema>;

// DataTable column (frontend normalized shape)
export const DataTableColumnSchema = z.object({
  name: z.string(),
  type: z.string(),
  comment: z.string().optional(),
  classification: z.string().optional(),
});
export type DataTableColumn = z.infer<typeof DataTableColumnSchema>;

// Raw backend table column shape
export const BackendTableColumnSchema = z.object({
  colName: z.string().optional(),
  colType: z.string().optional(),
  colComment: z.string().optional(),
  classification: z.string().optional(),
});
export type BackendTableColumn = z.infer<typeof BackendTableColumnSchema>;

// DataTable Schema (from /api/v1alpha1/datatable/*)
export const DataTableSchema = z.object({
  tableName: z.string(),
  tableId: z.string(),
  nodeId: z.string(),
  nodeName: z.string().optional(),
  datasourceId: z.string().optional(),
  datasourceType: z.string().optional(),
  relativeUri: z.string().optional(),
  columns: z.array(DataTableColumnSchema).default([]),
  rowCount: z.number().default(0),
  status: z.string(),
  gmtCreate: z.string().optional(),
  description: z.string().optional(),
  // legacy fallbacks
  datatableVO: z.any().optional(),
  table: z.any().optional(),
  createTime: z.string().optional(),
});
export type DataTable = z.infer<typeof DataTableSchema>;

// DataSource related node
export const DataSourceNodeSchema = z.object({
  nodeId: z.string(),
  nodeName: z.string().optional(),
  status: z.string().optional(),
});
export type DataSourceNode = z.infer<typeof DataSourceNodeSchema>;

// DataSource Schema (from /api/v1alpha1/datasource/*)
export const DataSourceSchema = z.object({
  datasourceId: z.string(),
  name: z.string(),
  type: z.string(),
  nodes: z.array(DataSourceNodeSchema).default([]),
  status: z.string().optional(),
  createTime: z.string().optional(),
  info: z.record(z.any()).optional(),
  // legacy fallbacks
  nodeId: z.string().optional(),
});
export type DataSource = z.infer<typeof DataSourceSchema>;

// Job Execution Schema (normalized for UI)
export const JobExecutionSchema = z.object({
  jobId: z.string(),
  projectId: z.string(),
  name: z.string(),
  status: z.enum(['RUNNING', 'SUCCEEDED', 'FAILED', 'PENDING']),
  duration: z.string().optional(),
  createTime: z.string(),
  errMsg: z.string().optional(),
  finished: z.boolean().optional(),
});
export type JobExecution = z.infer<typeof JobExecutionSchema>;

// Form input schemas (runtime validation helpers)
export const CreateNodeInputSchema = z.object({
  name: z.string().min(1).max(32),
  mode: z.number().int().min(0).max(2),
});
export type CreateNodeInput = z.infer<typeof CreateNodeInputSchema>;

export const UpdateNodeInputSchema = z.object({
  nodeId: z.string(),
  netAddress: z.string().regex(/^.+:\d+$/),
});
export type UpdateNodeInput = z.infer<typeof UpdateNodeInputSchema>;

export const CreateDataSourceInputSchema = z.object({
  ownerId: z.string(),
  nodeIds: z.array(z.string()),
  type: z.string(),
  name: z.string().min(1).max(32),
  info: z.record(z.any()),
});
export type CreateDataSourceInput = z.infer<typeof CreateDataSourceInputSchema>;

export const CreateDataTableInputSchema = z.object({
  ownerId: z.string(),
  nodeIds: z.array(z.string()),
  datatableName: z.string().min(1).max(32),
  datasourceId: z.string(),
  datasourceName: z.string(),
  datasourceType: z.string(),
  relativeUri: z.string(),
  columns: z.array(DataTableColumnSchema),
  desc: z.string().max(100).optional(),
});
export type CreateDataTableInput = z.infer<typeof CreateDataTableInputSchema>;


// Model / Serving Schema (from /api/v1alpha1/model/*)
export const ModelPackVOSchema = z.object({
  gmtCreate: z.string().optional(),
  modelDesc: z.string().optional(),
  modelId: z.string().optional(),
  modelName: z.string().optional(),
  modelStats: z.string().optional(),
  ownerId: z.string().optional(),
  servingId: z.string().optional(),
});
export type ModelPackVO = z.infer<typeof ModelPackVOSchema>;

export const ServingDetailSchema = z.object({
  endpoints: z.string().optional(),
  featureHttp: z.string().optional(),
  featureMappings: z.record(z.string()).optional(),
  isMock: z.boolean().optional(),
  nodeId: z.string().optional(),
  nodeName: z.string().optional(),
  sourcePath: z.string().optional(),
});
export type ServingDetail = z.infer<typeof ServingDetailSchema>;

// 模型发布详情 VO（来自 /api/v1alpha1/model/serving/detail）。
export const ServingDetailVOSchema = z.object({
  modelId: z.string().optional(),
  servingId: z.string().optional(),
  servingDetails: z.array(ServingDetailSchema).optional(),
});
export type ServingDetailVO = z.infer<typeof ServingDetailVOSchema>;

export const ModelPackInfoVOSchema = z.object({
  modelStats: z.string().optional(),
  servingDetails: z.array(ServingDetailSchema).optional(),
});
export type ModelPackInfoVO = z.infer<typeof ModelPackInfoVOSchema>;

// Message Schema (from /api/v1alpha1/message/*)
export const MessageVOSchema = z.object({
  createTime: z.string().optional(),
  messageName: z.string().optional(),
  status: z.string().optional(),
  type: z.string().optional(),
  voteID: z.string().optional(),
});
export type MessageVO = z.infer<typeof MessageVOSchema>;

// Scheduled / Periodic Task Schema (from /api/v1alpha1/scheduled/*)
export const PageScheduledVOSchema = z.object({
  createTime: z.string().optional(),
  creator: z.string().optional(),
  owner: z.string().optional(),
  ownerName: z.string().optional(),
  scheduleDesc: z.string().optional(),
  scheduleId: z.string().optional(),
  scheduleStats: z.string().optional(),
  taskRunning: z.boolean().optional(),
});
export type PageScheduledVO = z.infer<typeof PageScheduledVOSchema>;

// Graph / DAG Schema (from /api/v1alpha1/graph/*)
export const GraphMetaVOSchema = z.object({
  graphId: z.string().optional(),
  name: z.string().optional(),
  ownerId: z.string().optional(),
  projectId: z.string().optional(),
});
export type GraphMetaVO = z.infer<typeof GraphMetaVOSchema>;

export const GraphNodeDetailSchema = z.object({
  graphNodeId: z.string().optional(),
  codeName: z.string().optional(),
  label: z.string().optional(),
  status: z.string().optional(),
  x: z.number().optional(),
  y: z.number().optional(),
  progress: z.number().optional(),
  inputs: z.array(z.string()).optional(),
  outputs: z.array(z.string()).optional(),
  nodeDef: z.record(z.any()).optional(),
  parties: z.array(z.record(z.any())).optional(),
  results: z.array(z.record(z.any())).optional(),
});
export type GraphNodeDetail = z.infer<typeof GraphNodeDetailSchema>;

export const GraphNodeInfoSchema = z.object({
  graphNodeId: z.string().optional(),
  codeName: z.string().optional(),
  label: z.string().optional(),
  x: z.number().optional(),
  y: z.number().optional(),
  inputs: z.array(z.string()).optional(),
  outputs: z.array(z.string()).optional(),
  nodeDef: z.record(z.any()).optional(),
});
export type GraphNodeInfo = z.infer<typeof GraphNodeInfoSchema>;

export const GraphEdgeSchema = z.object({
  edgeId: z.string().optional(),
  source: z.string().optional(),
  target: z.string().optional(),
  sourceAnchor: z.string().optional(),
  targetAnchor: z.string().optional(),
});
export type GraphEdge = z.infer<typeof GraphEdgeSchema>;

export const GraphDataSourceConfigSchema = z.object({
  nodeId: z.string().optional(),
  dataSourceId: z.string().optional(),
});
export type GraphDataSourceConfig = z.infer<typeof GraphDataSourceConfigSchema>;

export const GraphDetailVOSchema = z.object({
  graphId: z.string().optional(),
  name: z.string().optional(),
  projectId: z.string().optional(),
  nodes: z.array(GraphNodeDetailSchema).default([]),
  edges: z.array(GraphEdgeSchema).default([]),
  maxParallelism: z.number().optional(),
  dataSourceConfig: z.array(GraphDataSourceConfigSchema).optional(),
});
export type GraphDetailVO = z.infer<typeof GraphDetailVOSchema>;

export const GraphNodeStatusVOSchema = z.object({
  graphNodeId: z.string().optional(),
  taskId: z.string().optional(),
  jobId: z.string().optional(),
  status: z.string().optional(),
  progress: z.number().optional(),
  parties: z.array(z.record(z.any())).optional(),
});
export type GraphNodeStatusVO = z.infer<typeof GraphNodeStatusVOSchema>;

export const GraphStatusSchema = z.object({
  finished: z.boolean().optional(),
  nodes: z.array(GraphNodeStatusVOSchema).optional(),
});
export type GraphStatus = z.infer<typeof GraphStatusSchema>;

export const GraphNodeTaskLogsVOSchema = z.object({
  status: z.string().optional(),
  logs: z.array(z.string()).optional(),
});
export type GraphNodeTaskLogsVO = z.infer<typeof GraphNodeTaskLogsVOSchema>;

export const GraphNodeOutputVOSchema = z.object({
  type: z.string().optional(),
  codeName: z.string().optional(),
  tabs: z.record(z.any()).optional(),
  meta: z.record(z.any()).optional(),
  jobId: z.string().optional(),
  taskId: z.string().optional(),
  graphID: z.string().optional(),
  warning: z.array(z.string()).optional(),
  gmtCreate: z.string().optional(),
  gmtModified: z.string().optional(),
});
export type GraphNodeOutputVO = z.infer<typeof GraphNodeOutputVOSchema>;

// Component Schema (from /api/v1alpha1/component/*)
export const ComponentSummaryDefSchema = z.object({
  desc: z.string().optional(),
  domain: z.string().optional(),
  name: z.string().optional(),
  version: z.string().optional(),
});
export type ComponentSummaryDef = z.infer<typeof ComponentSummaryDefSchema>;

export const ComponentDefSchema = z.object({
  domain: z.string().optional(),
  name: z.string().optional(),
  version: z.string().optional(),
  desc: z.string().optional(),
  inputs: z.array(z.record(z.any())).optional(),
  outputs: z.array(z.record(z.any())).optional(),
  attrs: z.array(z.record(z.any())).optional(),
});
export type ComponentDef = z.infer<typeof ComponentDefSchema>;

export const CompListVOSchema = z.object({
  name: z.string().optional(),
  version: z.string().optional(),
  desc: z.string().optional(),
  comps: z.array(ComponentSummaryDefSchema).optional(),
});
export type CompListVO = z.infer<typeof CompListVOSchema>;

// ======================================================================
// Extended VOs for full migration (project detail / job / p2p / inst /
// nodeRoute / user / scheduled / data / datasource / approval)
// ======================================================================

// Party vote info (project approval voting)
export const PartyVoteInfoSchema = z.object({
  partyId: z.string().optional(),
  partyName: z.string().optional(),
  action: z.string().optional(),
  reason: z.string().optional(),
});
export type PartyVoteInfo = z.infer<typeof PartyVoteInfoSchema>;

export const ProjectInstSchema = z.object({
  instId: z.string().optional(),
  instName: z.string().optional(),
});
export type ProjectInst = z.infer<typeof ProjectInstSchema>;

export const ProjectDatatableBaseSchema = z.object({
  datatableId: z.string().optional(),
  datatableName: z.string().optional(),
});
export type ProjectDatatableBase = z.infer<typeof ProjectDatatableBaseSchema>;

export const ProjectNodeVOSchema = z.object({
  nodeId: z.string().optional(),
  nodeName: z.string().optional(),
  nodeType: z.string().optional(),
  datatables: z.array(ProjectDatatableBaseSchema).optional(),
});
export type ProjectNodeVO = z.infer<typeof ProjectNodeVOSchema>;

// Full ProjectVO (from /api/v1alpha1/project/get & p2p/project/list)
export const ProjectVOSchema = z.object({
  projectId: z.string(),
  projectName: z.string().optional(),
  description: z.string().optional(),
  nodes: z.array(ProjectNodeVOSchema).optional(),
  insts: z.array(ProjectInstSchema).optional(),
  graphCount: z.number().optional(),
  jobCount: z.number().optional(),
  gmtCreate: z.string().optional(),
  computeMode: z.string().optional(),
  teeNodeId: z.string().optional(),
  status: z.string().optional(),
  initiator: z.string().optional(),
  initiatorName: z.string().optional(),
  computeFunc: z.string().optional(),
  voteId: z.string().optional(),
  partyVoteInfos: z.array(PartyVoteInfoSchema).optional(),
});
export type ProjectVO = z.infer<typeof ProjectVOSchema>;

// Project job VO (from /api/v1alpha1/project/job/get & scheduled/info)
export const ProjectJobVOSchema = z.object({
  jobId: z.string().optional(),
  status: z.string().optional(),
  errMsg: z.string().optional(),
  gmtCreate: z.string().optional(),
  gmtModified: z.string().optional(),
  gmtFinished: z.string().optional(),
  finished: z.boolean().optional(),
  graph: GraphDetailVOSchema.optional(),
});
export type ProjectJobVO = z.infer<typeof ProjectJobVOSchema>;

// Message detail VO (from /api/v1alpha1/message/detail)
export const MessageDetailVOSchema = z.object({
  messageName: z.string().optional(),
  type: z.string().optional(),
  status: z.string().optional(),
});
export type MessageDetailVO = z.infer<typeof MessageDetailVOSchema>;

// Model export package response (from /api/v1alpha1/model/pack)
export const ModelExportPackageResponseSchema = z.object({
  modelId: z.string().optional(),
  jobId: z.string().optional(),
});
export type ModelExportPackageResponse = z.infer<typeof ModelExportPackageResponseSchema>;

// Model party path response (from /api/v1alpha1/model/modelPartyPath)
export const ModelPartyPathResponseSchema = z.object({
  nodeId: z.string().optional(),
  nodeName: z.string().optional(),
  dataSources: z.array(z.record(z.any())).optional(),
});
export type ModelPartyPathResponse = z.infer<typeof ModelPartyPathResponseSchema>;

// Model pack detail VO (from /api/v1alpha1/model/detail)
export const ModelPartyColumnsSchema = z.object({
  nodeId: z.string().optional(),
  nodeName: z.string().optional(),
  columns: z.array(z.string()).optional(),
});
export const ModelPackDetailVOSchema = z.object({
  parties: z.array(ModelPartyColumnsSchema).optional(),
});
export type ModelPackDetailVO = z.infer<typeof ModelPackDetailVOSchema>;

// Node instance / datatable sub-VOs
export const NodeInstanceDTOSchema = z.object({
  name: z.string().optional(),
  status: z.string().optional(),
  version: z.string().optional(),
  lastHeartbeatTime: z.string().optional(),
  lastTransitionTime: z.string().optional(),
});
export type NodeInstanceDTO = z.infer<typeof NodeInstanceDTOSchema>;

export const NodeDatatableVOSchema = z.object({
  datatableId: z.string().optional(),
  datatableName: z.string().optional(),
});
export type NodeDatatableVO = z.infer<typeof NodeDatatableVOSchema>;

// Node route VO (from /api/v1alpha1/nodeRoute/*)
export const NodeRouterVOSchema = z.object({
  routeId: z.string().optional(),
  srcNodeId: z.string().optional(),
  dstNodeId: z.string().optional(),
  srcNode: z.record(z.any()).optional(),
  dstNode: z.record(z.any()).optional(),
  srcNetAddress: z.string().optional(),
  dstNetAddress: z.string().optional(),
  status: z.string().optional(),
  gmtCreate: z.string().optional(),
  gmtModified: z.string().optional(),
  isProjectJobRunning: z.boolean().optional(),
  routeType: z.string().optional(),
});
export type NodeRouterVO = z.infer<typeof NodeRouterVOSchema>;

// Node results VO (from /api/v1alpha1/node/result/*)
export const NodeResultsVOSchema = z.object({
  domainDataId: z.string().optional(),
  datasourceId: z.string().optional(),
  datasourceType: z.string().optional(),
  productName: z.string().optional(),
  datatableType: z.string().optional(),
  sourceProjectId: z.string().optional(),
  sourceProjectName: z.string().optional(),
  relativeUri: z.string().optional(),
  jobId: z.string().optional(),
  trainFlow: z.string().optional(),
  pullFromTeeStatus: z.string().optional(),
  pullFromTeeErrMsg: z.string().optional(),
  gmtCreate: z.string().optional(),
  computeMode: z.string().optional(),
});
export type NodeResultsVO = z.infer<typeof NodeResultsVOSchema>;

export const NodeAllResultsVOSchema = z.object({
  nodeResultsVO: NodeResultsVOSchema.optional(),
  nodeId: z.string().optional(),
  nodeName: z.string().optional(),
});
export type NodeAllResultsVO = z.infer<typeof NodeAllResultsVOSchema>;

export const AllNodeResultsListVOSchema = z.object({
  nodeAllResultsVOList: z.array(NodeAllResultsVOSchema).optional(),
  totalNodeResultNums: z.number().optional(),
});
export type AllNodeResultsListVO = z.infer<typeof AllNodeResultsListVOSchema>;

export const NodeResultDetailVOSchema = z.object({
  nodeResultsVO: NodeResultsVOSchema.optional(),
  graphDetailVO: GraphDetailVOSchema.optional(),
  tableColumnVOList: z.array(BackendTableColumnSchema).optional(),
  output: GraphNodeOutputVOSchema.optional(),
  datasource: z.string().optional(),
});
export type NodeResultDetailVO = z.infer<typeof NodeResultDetailVOSchema>;

// Institution VOs (from /api/v1alpha1/inst/*)
export const InstVOSchema = z.object({
  instId: z.string().optional(),
  instName: z.string().optional(),
  localNodeId: z.string().optional(),
});
export type InstVO = z.infer<typeof InstVOSchema>;

export const InstTokenVOSchema = z.object({
  nodeId: z.string().optional(),
  nodeName: z.string().optional(),
  instToken: z.string().optional(),
  createTime: z.string().optional(),
  instTokenState: z.string().optional(),
});
export type InstTokenVO = z.infer<typeof InstTokenVOSchema>;

// P2P participants VOs (from /api/v1alpha1/p2p/project/participants)
export const NodeInstVOSchema = z.object({
  inviteeId: z.string().optional(),
  inviteeName: z.string().optional(),
  instId: z.string().optional(),
  instName: z.string().optional(),
});
export type NodeInstVO = z.infer<typeof NodeInstVOSchema>;

export const PartyVoteStatusSchema = z.object({
  participantID: z.string().optional(),
  participantName: z.string().optional(),
  action: z.string().optional(),
  reason: z.string().optional(),
});
export type PartyVoteStatus = z.infer<typeof PartyVoteStatusSchema>;

export const ProjectParticipantsDetailVOSchema = z.object({
  initiatorId: z.string().optional(),
  initiatorName: z.string().optional(),
  projectName: z.string().optional(),
  partyVoteStatuses: z.array(PartyVoteStatusSchema).optional(),
  computeMode: z.string().optional(),
  computeFunc: z.string().optional(),
  projectDesc: z.string().optional(),
  initiatorNodeId: z.string().optional(),
  initiatorNodeName: z.string().optional(),
  invitees: z.array(NodeInstVOSchema).optional(),
});
export type ProjectParticipantsDetailVO = z.infer<typeof ProjectParticipantsDetailVOSchema>;

/** 单个投票信息（某参与方的一次投票动作）。 */
export const VoteInfoSchema = z.object({
  voteID: z.string().optional(),
  nodeID: z.string().optional(),
  action: z.string().optional(),
  reason: z.string().optional(),
});
export type VoteInfo = z.infer<typeof VoteInfoSchema>;

/** 审批参与方（节点维度的投票状态与明细）。 */
export const ParticipantSchema = z.object({
  nodeID: z.string().optional(),
  nodeName: z.string().optional(),
  status: z.string().optional(),
  voteInfos: z.array(VoteInfoSchema).optional(),
});
export type Participant = z.infer<typeof ParticipantSchema>;

/** 审批状态轮询结果（`approval/pull/status` 返回值）。 */
export const PullStatusVOSchema = z.object({
  projectID: z.string().optional(),
  jobID: z.string().optional(),
  taskID: z.string().optional(),
  graphID: z.string().optional(),
  resourceID: z.string().optional(),
  resourceType: z.string().optional(),
  parties: z.array(ParticipantSchema).optional(),
});
export type PullStatusVO = z.infer<typeof PullStatusVOSchema>;

// User context DTO (from /api/v1alpha1/user/get)
export const UserContextDTOSchema = z.object({
  token: z.string().optional(),
  name: z.string().optional(),
  platformType: z.string().optional(),
  platformNodeId: z.string().optional(),
  ownerType: z.string().optional(),
  ownerId: z.string().optional(),
  projectIds: z.array(z.string()).optional(),
  apiResources: z.array(z.string()).optional(),
  virtualUserForNode: z.boolean().optional(),
  deployMode: z.string().optional(),
});
export type UserContextDTO = z.infer<typeof UserContextDTOSchema>;

// Scheduled task page VO (from /api/v1alpha1/scheduled/task/page)
export const TaskPageScheduledVOSchema = z.object({
  scheduleTaskId: z.string().optional(),
  scheduleTaskExpectStartTime: z.string().optional(),
  scheduleTaskStartTime: z.string().optional(),
  scheduleTaskEndTime: z.string().optional(),
  scheduleTaskStatus: z.string().optional(),
  allReRun: z.boolean().optional(),
});
export type TaskPageScheduledVO = z.infer<typeof TaskPageScheduledVOSchema>;

// Cron config (for scheduled graph create)
export const CronSchema = z.object({
  startTime: z.string().optional(),
  endTime: z.string().optional(),
  scheduleCycle: z.string().optional(),
  scheduleDate: z.string().optional(),
  scheduleTime: z.string().optional(),
});
export type Cron = z.infer<typeof CronSchema>;

// Datatable VO (raw backend shape, from /api/v1alpha1/datatable/get)
export const TableColumnVOSchema = BackendTableColumnSchema;
export type TableColumnVO = z.infer<typeof TableColumnVOSchema>;

export const AuthProjectVOSchema = z.object({
  projectId: z.string().optional(),
  name: z.string().optional(),
  computeMode: z.string().optional(),
  gmtCreate: z.string().optional(),
  // 关联键列表（授权时指定的 associate key）。
  associateKeys: z.array(z.string()).optional(),
  // 标签列列表（授权时指定的 label key）。
  labelKeys: z.array(z.string()).optional(),
  // 分组键列表（授权时指定的 group key）。
  groupKeys: z.array(z.string()).optional(),
});
export type AuthProjectVO = z.infer<typeof AuthProjectVOSchema>;

export const DatatableVOSchema = z.object({
  datatableId: z.string().optional(),
  datatableName: z.string().optional(),
  status: z.string().optional(),
  pushToTeeStatus: z.string().optional(),
  pushToTeeErrMsg: z.string().optional(),
  datasourceId: z.string().optional(),
  datasourceType: z.string().optional(),
  datasourceName: z.string().optional(),
  nodeId: z.string().optional(),
  relativeUri: z.string().optional(),
  type: z.string().optional(),
  description: z.string().optional(),
  schema: z.array(TableColumnVOSchema).optional(),
  authProjects: z.array(AuthProjectVOSchema).optional(),
  nullStrs: z.array(z.string()).optional(),
});
export type DatatableVO = z.infer<typeof DatatableVOSchema>;

export const DatatableNodeVOSchema = z.object({
  datatableVO: DatatableVOSchema.optional(),
  nodeName: z.string().optional(),
  nodeId: z.string().optional(),
});
export type DatatableNodeVO = z.infer<typeof DatatableNodeVOSchema>;

// Datasource detail VOs (from /api/v1alpha1/datasource/detail & nodes)
export const DataSourceRelatedNodeSchema = z.object({
  nodeId: z.string().optional(),
  nodeName: z.string().optional(),
  status: z.string().optional(),
});
export type DataSourceRelatedNode = z.infer<typeof DataSourceRelatedNodeSchema>;

export const DatasourceDetailAggregateVOSchema = z.object({
  nodes: z.array(DataSourceRelatedNodeSchema).optional(),
  datasourceId: z.string().optional(),
  name: z.string().optional(),
  type: z.string().optional(),
  status: z.string().optional(),
  info: z.record(z.any()).optional(),
});
export type DatasourceDetailAggregateVO = z.infer<typeof DatasourceDetailAggregateVOSchema>;

export const DatasourceNodesVOSchema = z.object({
  nodes: z.array(DataSourceRelatedNodeSchema).optional(),
});
export type DatasourceNodesVO = z.infer<typeof DatasourceNodesVOSchema>;

// Upload data result VO (from /api/v1alpha1/data/upload)
export const UploadDataResultVOSchema = z.object({
  name: z.string().optional(),
  realName: z.string().optional(),
  datasource: z.string().optional(),
  datasourceType: z.string().optional(),
});
export type UploadDataResultVO = z.infer<typeof UploadDataResultVOSchema>;

// Sync data DTO (from /api/v1alpha1/data/sync)
export const SyncDataDTOSchema = z.object({
  tableName: z.string().optional(),
  lastUpdateTime: z.string().optional(),
  action: z.string().optional(),
  data: z.record(z.any()).optional(),
});
export type SyncDataDTO = z.infer<typeof SyncDataDTOSchema>;

// Generic page response wrapper (SecretPadPageResponse<T>)
export const pageResponseSchema = <T extends z.ZodTypeAny>(itemSchema: T) =>
  z.object({
    data: z.array(itemSchema).optional(),
    pageNumber: z.number().optional(),
    pageSize: z.number().optional(),
    totalCount: z.number().optional(),
    totalPage: z.number().optional(),
  });
