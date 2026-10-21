/**
 * Cross-node aggregation for the CENTER "all data" views (legacy
 * `all-data-sources` / `all-data-tables`). Pure and unit-tested.
 */
import type { DatasourceListResultJava, DatatableRowJava, Node } from '@secretpad/api-client';

export interface AggregatedDatasource {
  datasourceId?: string;
  name?: string;
  type?: string;
  nodeId?: string;
  nodeName?: string;
  status?: string;
  relatedDatas?: string[];
}

export type AggregatedDatatable = DatatableRowJava;

export interface Aggregated<T> {
  items: T[];
  /** Node names whose list request failed. */
  failed: string[];
}

/** Dedupe nodes by nodeId (the node list may contain duplicates after center/edge sync). */
export function uniqueNodes<T extends { nodeId: string }>(nodes: T[]): T[] {
  const seen = new Set<string>();
  return nodes.filter((n) => (seen.has(n.nodeId) ? false : (seen.add(n.nodeId), true)));
}

/** One row per (datasourceId, nodeId); status taken from the node's own entry. */
export function aggregateDatasources(
  nodes: Pick<Node, 'nodeId' | 'nodeName'>[],
  results: PromiseSettledResult<DatasourceListResultJava>[],
): Aggregated<AggregatedDatasource> {
  const map = new Map<string, AggregatedDatasource>();
  const failed: string[] = [];
  results.forEach((r, i) => {
    const node = nodes[i];
    if (r.status === 'rejected') {
      failed.push(node.nodeName || node.nodeId);
      return;
    }
    r.value.infos.forEach((s) => {
      const key = `${s.datasourceId}-${node.nodeId}`;
      if (!s.datasourceId || map.has(key)) return;
      const rel = s.nodes?.find((n) => n.nodeId === node.nodeId);
      map.set(key, {
        datasourceId: s.datasourceId,
        name: s.name,
        type: s.type,
        nodeId: node.nodeId,
        nodeName: rel?.nodeName || node.nodeName,
        status: rel?.status,
        relatedDatas: s.relatedDatas,
      });
    });
  });
  return { items: Array.from(map.values()), failed };
}

/** One row per (datatableId, nodeId, datasourceType). */
export function aggregateDatatables(
  nodes: Pick<Node, 'nodeId' | 'nodeName'>[],
  results: PromiseSettledResult<DatatableRowJava[]>[],
): Aggregated<AggregatedDatatable> {
  const map = new Map<string, AggregatedDatatable>();
  const failed: string[] = [];
  results.forEach((r, i) => {
    const node = nodes[i];
    if (r.status === 'rejected') {
      failed.push(node.nodeName || node.nodeId);
      return;
    }
    r.value.forEach((row) => {
      if (!row.datatableId) return;
      const nodeId = row.nodeId || node.nodeId;
      const key = `${row.datatableId}-${nodeId}-${row.datasourceType || 'LOCAL'}`;
      if (map.has(key)) return;
      map.set(key, { ...row, nodeId, nodeName: row.nodeName || node.nodeName });
    });
  });
  return { items: Array.from(map.values()), failed };
}

/** Case-insensitive name search + optional node filter. */
export function filterAggregated<T extends { nodeId?: string }>(
  items: T[],
  search: string,
  nodeId: string,
  nameOf: (i: T) => string | undefined,
): T[] {
  const q = search.trim().toLowerCase();
  return items.filter((i) => (!nodeId || i.nodeId === nodeId) && (!q || (nameOf(i) || '').toLowerCase().includes(q)));
}
