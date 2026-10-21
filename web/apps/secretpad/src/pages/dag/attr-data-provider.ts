/**
 * 属性表单的真实数据提供器（替代旧的空实现 / 写死 alice、bob）：
 * - 列：上游输出表 schema（graph/node/output meta.rows.fields），上游为样本表时取
 *   project/datatable/get 的列配置；上游未运行时回溯到最上游样本表；
 * - 表：项目已授权数据表（project/get nodes[].datatables）；
 * - 模型：model/page；
 * - 参与方：项目节点；scql task_initiator 取上游输出的参与方；
 * - FeatureColumnConfig：上游输出表及特征。
 */
import type { AttrDataProvider, ColumnOption, DAGEdge, DAGNode, OptionItem } from '@secretpad/dag-next';
import { directUpstream, normalizeOutput, parseAnchor } from '@secretpad/dag-next';
import type { ProjectDetailJava } from '@secretpad/api-client';

export interface ProviderDeps {
  projectId: string;
  graphId: string;
  project?: ProjectDetailJava | null;
  fetchOutput: (graphNodeId: string, outputId: string) => Promise<Record<string, unknown> | null>;
  fetchTableColumns: (req: { nodeId: string; datatableId: string }) => Promise<Array<{ colName: string; colType?: string }>>;
  fetchModels?: () => Promise<OptionItem[]>;
}

const READ_DATA = 'read_data/datatable';

/** 样本表节点选中的数据表 ID（nodeDef.attrs[0].s，兼容 attrPaths 查找）。 */
export function selectedDatatableId(node: DAGNode | undefined): string | undefined {
  const def = node?.nodeDef as { attrPaths?: string[]; attrs?: Array<{ s?: string }> } | undefined;
  if (!def?.attrs) return undefined;
  const idx = def.attrPaths?.indexOf('datatable_selected') ?? -1;
  return (idx >= 0 ? def.attrs[idx]?.s : def.attrs[0]?.s) || undefined;
}

function ownerOfTable(project: ProjectDetailJava | null | undefined, datatableId: string): string | undefined {
  return project?.nodes.find((n) => n.datatables.some((t) => t.datatableId === datatableId))?.nodeId;
}

export function createAttrDataProvider(node: DAGNode, nodes: DAGNode[], edges: DAGEdge[], deps: ProviderDeps): AttrDataProvider {
  const cache = new Map<string, Promise<unknown>>();
  const memo = <T,>(key: string, fn: () => Promise<T>): Promise<T> => {
    if (!cache.has(key)) cache.set(key, fn().catch(() => undefined as unknown as T));
    return cache.get(key) as Promise<T>;
  };
  const byId = (id: string) => nodes.find((n) => n.id === id);

  /** 某个节点某个输出锚点的列（含参与方）。 */
  const columnsOfOutput = (upstream: DAGNode, anchor: string | undefined, depth = 0): Promise<ColumnOption[]> =>
    memo(`cols:${upstream.id}:${anchor}`, async () => {
      if (upstream.codeName === READ_DATA) {
        const tableId = selectedDatatableId(upstream);
        const owner = tableId ? ownerOfTable(deps.project, tableId) : undefined;
        if (!tableId || !owner) return [];
        const cols = await deps.fetchTableColumns({ nodeId: owner, datatableId: tableId });
        return cols.map((c) => ({ name: c.colName, type: c.colType, party: owner }));
      }
      const outputId = anchor || upstream.outputs?.[0];
      if (outputId) {
        const out = normalizeOutput(await deps.fetchOutput(upstream.id, outputId));
        const cols = (out?.rows || []).flatMap((r) => r.fields.map((f, i) => ({ name: f, type: r.fieldTypes[i], party: r.nodeId })));
        if (cols.length > 0) return cols;
      }
      // 上游尚未运行：回溯其第一个输入（旧版 useCols 回退到输入样本表）。
      if (depth > 10) return [];
      const ups = directUpstream(upstream.id, edges);
      const results = await Promise.all(
        ups.map((u) => {
          const n = byId(u.nodeId);
          return n ? columnsOfOutput(n, u.sourceAnchor, depth + 1) : Promise.resolve([]);
        }),
      );
      return results.flat();
    });

  const upstreamOf = (inputIndex?: number) => {
    const ups = directUpstream(node.id, edges);
    if (inputIndex === undefined) return ups;
    return ups.filter((u) => u.inputIndex === inputIndex);
  };

  const dedupe = (cols: ColumnOption[]) => {
    const seen = new Set<string>();
    return cols.filter((c) => {
      const k = `${c.name}\u0000${c.party ?? ''}`;
      if (seen.has(k)) return false;
      seen.add(k);
      return true;
    });
  };

  return {
    fetchColumns: async (_path, ctx) => {
      const ups = upstreamOf(ctx?.inputIndex);
      const all = await Promise.all(
        ups.map((u) => {
          const n = byId(u.nodeId);
          return n ? columnsOfOutput(n, u.sourceAnchor) : Promise.resolve([] as ColumnOption[]);
        }),
      );
      return dedupe(all.flat());
    },
    fetchTables: async () =>
      (deps.project?.nodes || []).flatMap((n) =>
        n.datatables.map((t) => ({ id: t.datatableId, name: `${t.datatableName || t.datatableId} (${n.nodeName || n.nodeId})` })),
      ),
    fetchModels: deps.fetchModels,
    fetchParties: async (_path, ctx) => {
      if (ctx?.source === 'upstream') {
        const ups = upstreamOf();
        const parties = new Map<string, string>();
        for (const u of ups) {
          const n = byId(u.nodeId);
          if (!n) continue;
          if (n.codeName === READ_DATA) {
            const tableId = selectedDatatableId(n);
            const owner = tableId ? ownerOfTable(deps.project, tableId) : undefined;
            if (owner) parties.set(owner, deps.project?.nodes.find((p) => p.nodeId === owner)?.nodeName || owner);
            continue;
          }
          const outputId = u.sourceAnchor || n.outputs?.[0];
          if (!outputId) continue;
          const out = normalizeOutput(await deps.fetchOutput(n.id, outputId));
          out?.rows.forEach((r) => r.nodeId && parties.set(r.nodeId, r.nodeName || r.nodeId));
        }
        return [...parties.entries()].map(([id, name]) => ({ id, name }));
      }
      return (deps.project?.nodes || []).map((n) => ({ id: n.nodeId, name: n.nodeName || n.nodeId }));
    },
    fetchUpstreamFeatures: async () => {
      const ups = upstreamOf();
      const upstreamIsSample = ups[0] && byId(ups[0].nodeId)?.codeName === READ_DATA;
      const baseName = node.inputs?.[0] || ups[0]?.sourceAnchor || '';
      const features: Array<{ tableName: string; tableFeatures: string[]; nodeName: string }> = [];
      for (const u of ups) {
        const n = byId(u.nodeId);
        if (!n) continue;
        const cols = await columnsOfOutput(n, u.sourceAnchor);
        const byParty = new Map<string, string[]>();
        cols.forEach((c) => byParty.set(c.party || '', [...(byParty.get(c.party || '') || []), c.name]));
        byParty.forEach((feats, party) => {
          const tableId = upstreamIsSample ? selectedDatatableId(n) : baseName;
          features.push({
            tableName: `${party}_${tableId || parseAnchor(u.sourceAnchor)?.nodeId || ''}`.replace(/-/g, '_'),
            tableFeatures: feats,
            nodeName: deps.project?.nodes.find((p) => p.nodeId === party)?.nodeName || party,
          });
        });
      }
      return features;
    },
  };
}
