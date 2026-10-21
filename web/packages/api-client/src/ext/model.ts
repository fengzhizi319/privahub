/**
 * Java-contract extension APIs: model management (ModelManagementController)
 * and the feature datasources used when publishing a model.
 */
import { javaPost } from './core';

/** Model status (`ModelPackVO.modelStats`). */
export const MODEL_STATS = ['INIT', 'PUBLISHING', 'PUBLISHED', 'PUBLISH_FAIL', 'OFFLINE', 'DISCARDED'] as const;
export type ModelStatsJava = (typeof MODEL_STATS)[number];

/** Java `ModelPackVO`. */
export interface ModelPackJava {
  modelId?: string;
  servingId?: string;
  modelName?: string;
  modelDesc?: string;
  modelStats?: string;
  gmtCreate?: string;
  ownerId?: string;
}

/** Java `QueryModelPageRequest`. */
export interface QueryModelPageRequestJava {
  page: number;
  size: number;
  sort?: Record<string, 'ASC' | 'DESC'>;
  projectId: string;
  searchKey?: string;
  modelStats?: string;
}

/** Java `ModelPackListVO`. */
export interface ModelPackListJava {
  modelPacks: ModelPackJava[];
  total: number;
  pageSize?: number;
  pageNum?: number;
  totalPage?: number;
}

/** Java `ModelPackDetailVO.Parties`. */
export interface ModelPartyJava {
  nodeId?: string;
  nodeName?: string;
  /** Offline (in-model) feature names. */
  columns?: string[];
}

/** Java `FeatureDataSourceVO`. */
export interface FeatureTableJava {
  nodeId?: string;
  featureTableId?: string;
  featureTableName?: string;
  columns?: { colName?: string; colType?: string; colComment?: string }[];
}

/** Java `CreateModelServingRequest.PartyConfig` (as sent by legacy model-release). */
export interface ModelPartyConfigJava {
  nodeId: string;
  featureTableId?: string;
  isMock: boolean;
  features: { offlineName: string; onlineName?: string }[];
  resources: { minCPU: string; maxCPU: string; minMemory: string; maxMemory: string }[];
}

/** Java `CreateModelServingRequest`. */
export interface CreateModelServingRequestJava {
  modelId: string;
  projectId: string;
  partyConfigs: ModelPartyConfigJava[];
}

/** Java `ServingDetailVO.ServingDetail`. */
export interface ServingDetailItemJava {
  nodeId?: string;
  nodeName?: string;
  endpoints?: string;
  featureHttp?: string;
  isMock?: boolean;
  sourcePath?: string;
  featureMappings?: Record<string, string>;
  resources?: { minCPU?: string; maxCPU?: string; minMemory?: string; maxMemory?: string }[];
}

/** Java `ServingDetailVO`. */
export interface ServingDetailJava {
  modelId?: string;
  servingId?: string;
  servingDetails?: ServingDetailItemJava[];
}

export async function pageModelsJava(req: QueryModelPageRequestJava): Promise<ModelPackListJava> {
  const data = await javaPost<Partial<ModelPackListJava> | null>('model/page', req);
  const modelPacks = Array.isArray(data?.modelPacks) ? data!.modelPacks! : [];
  return { ...data, modelPacks, total: Number(data?.total ?? modelPacks.length) || 0 };
}

export async function getModelPartiesJava(modelId: string, projectId: string): Promise<ModelPartyJava[]> {
  const data = await javaPost<{ parties?: ModelPartyJava[] } | null>('model/detail', { modelId, projectId });
  return Array.isArray(data?.parties) ? data!.parties! : [];
}

export async function listProjectFeatureTablesJava(projectId: string, nodeId: string): Promise<FeatureTableJava[]> {
  const data = await javaPost<FeatureTableJava[] | null>('feature_datasource/auth/list', { projectId, nodeId });
  return Array.isArray(data) ? data : [];
}

export function createModelServingJava(req: CreateModelServingRequestJava): Promise<unknown> {
  return javaPost('model/serving/create', req);
}

/** Take a published model offline. */
export function deleteModelServingJava(servingId: string): Promise<unknown> {
  return javaPost('model/serving/delete', { servingId });
}

export function getModelServingJava(servingId: string): Promise<ServingDetailJava> {
  return javaPost<ServingDetailJava>('model/serving/detail', { servingId }).then((d) => d || {});
}

export function discardModelJava(modelId: string): Promise<unknown> {
  return javaPost('model/discard', { modelId });
}

export function deleteModelJava(modelId: string, nodeId?: string): Promise<unknown> {
  return javaPost('model/delete', nodeId ? { modelId, nodeId } : { modelId });
}
