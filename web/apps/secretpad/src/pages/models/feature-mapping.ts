/**
 * Model publish helpers (legacy model-manager/model-release):
 * feature auto-matching and `CreateModelServingRequest.partyConfigs` building.
 */
import type { ModelPartyConfigJava } from '@secretpad/api-client';

export const MOCK_FEATURE_SERVICE = 'mock';

/** Offline (in-model) feature → online feature mapping row. */
export interface FeatureMapItem {
  into: string;
  online?: string;
}

export type MatchStatus = 'default' | 'success' | 'error';

/** Same-name match (legacy handleMatch): matched when an online feature has the same name. */
export function autoMatchFeatures(offline: string[], online: string[]): FeatureMapItem[] {
  const set = new Set(online);
  return offline.map((into) => ({ into, online: set.has(into) ? into : undefined }));
}

/** Mock feature service: every offline feature maps to itself. */
export function mockMatchFeatures(offline: string[]): FeatureMapItem[] {
  return offline.map((into) => ({ into, online: into }));
}

/** Update one mapping row (manual selection). */
export function setFeatureMapping(items: FeatureMapItem[], into: string, online?: string): FeatureMapItem[] {
  return items.map((i) => (i.into === into ? { into, online: online || undefined } : i));
}

export function unmatchedCount(items: FeatureMapItem[]): number {
  return items.filter((i) => !i.online).length;
}

export function matchStatus(nodeId: string | undefined, featureTableId: string | undefined, items: FeatureMapItem[]): MatchStatus {
  if (!nodeId || !featureTableId) return 'default';
  return unmatchedCount(items) > 0 ? 'error' : 'success';
}

/** Online features not yet used by another row (legacy onlineOptionsFilter). */
export function availableOnlineFeatures(online: string[], items: FeatureMapItem[], current?: string): string[] {
  const used = new Set(items.map((i) => i.online).filter(Boolean) as string[]);
  return online.filter((o) => o === current || !used.has(o));
}

export interface PublishRow {
  nodeId?: string;
  featureTableId?: string;
  items: FeatureMapItem[];
}

export interface ResourceConfig {
  minCpu: number;
  maxCpu: number;
  /** Gi */
  minMemory: number;
  /** Gi */
  maxMemory: number;
}

export const DEFAULT_RESOURCE: ResourceConfig = { minCpu: 0, maxCpu: 0, minMemory: 0, maxMemory: 0 };

/** min ≤ max and within legacy InputNumber bounds (cpu 0–1000, memory 0–10000 Gi). */
export function validateResource(r: ResourceConfig): 'cpu' | 'memory' | null {
  const inRange = (v: number, max: number) => Number.isFinite(v) && v >= 0 && v <= max;
  if (!inRange(r.minCpu, 1000) || !inRange(r.maxCpu, 1000) || r.minCpu > r.maxCpu) return 'cpu';
  if (!inRange(r.minMemory, 10000) || !inRange(r.maxMemory, 10000) || r.minMemory > r.maxMemory) return 'memory';
  return null;
}

/** Build partyConfigs exactly like the legacy model-release handleOk. */
export function buildPartyConfigs(rows: PublishRow[], resources: Record<string, ResourceConfig>): ModelPartyConfigJava[] {
  return rows
    .filter((r) => r.nodeId)
    .map((r) => {
      const res = resources[r.nodeId!] || DEFAULT_RESOURCE;
      return {
        nodeId: r.nodeId!,
        featureTableId: r.featureTableId,
        isMock: r.featureTableId === MOCK_FEATURE_SERVICE,
        features: r.items.map((i) => ({ offlineName: i.into, onlineName: i.online })),
        resources: [
          {
            minCPU: `${res.minCpu}`,
            maxCPU: `${res.maxCpu}`,
            minMemory: `${res.minMemory}Gi`,
            maxMemory: `${res.maxMemory}Gi`,
          },
        ],
      };
    });
}

/** Publish allowed only when every row matched successfully (legacy submitDisabled). */
export function canPublish(rows: PublishRow[]): boolean {
  return rows.length > 0 && rows.every((r) => matchStatus(r.nodeId, r.featureTableId, r.items) === 'success');
}
