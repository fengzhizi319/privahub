/**
 * DAG 缩略预览（结果血缘 / 模型提交链路共用）：按节点坐标等比绘制，高亮指定节点与其间连线。
 */
import React from 'react';

export interface PreviewNode {
  id: string;
  name: string;
  x: number;
  y: number;
}

export interface PreviewEdge {
  id: string;
  source: string;
  target: string;
}

export const DagPreview: React.FC<{ nodes: PreviewNode[]; edges: PreviewEdge[]; highlight: Set<string>; focusId?: string; className?: string }> = ({
  nodes,
  edges,
  highlight,
  focusId,
  className = 'w-full max-h-64',
}) => {
  if (nodes.length === 0) return null;
  const minX = Math.min(...nodes.map((n) => n.x));
  const minY = Math.min(...nodes.map((n) => n.y));
  const maxX = Math.max(...nodes.map((n) => n.x + 144));
  const maxY = Math.max(...nodes.map((n) => n.y + 64));
  const w = maxX - minX + 40;
  const h = maxY - minY + 40;
  const pos = (n: PreviewNode) => ({ x: n.x - minX + 20, y: n.y - minY + 20 });
  return (
    <svg viewBox={`0 0 ${w} ${h}`} className={`${className} bg-gray-50 dark:bg-gray-800 rounded-lg`} role="img" aria-label="dag preview">
      {edges.map((e) => {
        const s = nodes.find((n) => n.id === e.source);
        const t = nodes.find((n) => n.id === e.target);
        if (!s || !t) return null;
        const a = pos(s);
        const b = pos(t);
        const on = highlight.has(s.id) && highlight.has(t.id);
        return <line key={e.id} x1={a.x + 144} y1={a.y + 32} x2={b.x} y2={b.y + 32} stroke={on ? '#3b82f6' : '#9ca3af'} strokeOpacity={on ? 1 : 0.35} strokeWidth={on ? 3 : 2} />;
      })}
      {nodes.map((n) => {
        const p = pos(n);
        const on = highlight.has(n.id);
        const focus = n.id === focusId;
        return (
          <g key={n.id} opacity={on ? 1 : 0.35} data-node-id={n.id}>
            <rect x={p.x} y={p.y} width={144} height={64} rx={8} fill={focus ? '#bfdbfe' : on ? '#dbeafe' : '#e5e7eb'} stroke={focus ? '#1d4ed8' : on ? '#3b82f6' : '#9ca3af'} strokeWidth={focus ? 3 : 2} />
            <text x={p.x + 72} y={p.y + 36} textAnchor="middle" fontSize="12" fill="#111827">
              {n.name.length > 14 ? `${n.name.slice(0, 14)}…` : n.name}
            </text>
          </g>
        );
      })}
    </svg>
  );
};
