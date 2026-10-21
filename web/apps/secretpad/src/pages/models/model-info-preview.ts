/**
 * model/info → 模型链路预览（旧 model-detail.view：graphDetailVO 画缩略图，
 * modelGraphDetail 中的 graphNodeId 高亮；servingDetails 给出各节点模型路径）。
 */
import { normalizeGraphDetail, type ModelPackInfoVO } from '@secretpad/api-client';
import { mapGraphToDAG } from '../dag/adapters';

export function modelInfoPreview(info: ModelPackInfoVO | null | undefined) {
  const graph = info?.graphDetailVO ? normalizeGraphDetail(info.graphDetailVO) : null;
  const dag = mapGraphToDAG(graph);
  const modelIds = new Set(info?.modelGraphDetail || []);
  const highlighted = dag.nodes.filter((n) => modelIds.has(n.id)).map((n) => n.id);
  // 旧版未命中时整张图正常显示；这里同样退化为全部高亮，避免整图变灰。
  const highlight = new Set(highlighted.length ? highlighted : dag.nodes.map((n) => n.id));
  const modelPaths = (info?.servingDetails || []).map((d) => ({
    nodeId: d.nodeId || '',
    nodeName: d.nodeName || d.nodeId || '',
    sourcePath: d.sourcePath || '',
  }));
  return { graphName: graph?.name, nodes: dag.nodes, edges: dag.edges, highlight, modelPaths };
}
