/**
 * DAG 结果可视化组件集（ResultVisualization）。
 *
 * 对应迁移需求"DAG 结果可视化增强"。原前端 packages/dag/src/vis/ 下有 11+ 可视化组件
 * （output-table-result、binning、corr-matrix、feature-import、mpc-stats-info 等），
 * 新前端之前仅有基础 table/JSON 渲染。本文件实现一组自包含、零外部依赖的可视化组件：
 *
 * - ResultVisualization：根据 output.type 自动分发到合适的渲染器；
 * - OutputTable：增强表格（分页、列排序、CSV 导出）；
 * - StatsChart：SVG 柱状图 / 折线图（用于特征重要性、统计信息等）；
 * - KeyValuePanel：键值对面板（用于模型指标、评估结果等）；
 * - CorrelationHeatmap：相关性矩阵热力图（SVG 实现）。
 *
 * 该组件位于 `@secretpad/dag-next` 包内，文案通过 `labels` 注入，保持可复用性。
 */
import React, { useMemo, useState } from 'react';
import { Button } from '@secretpad/design-system';
import { NodeResultView } from './result/result-view';
import type { NodeResultViewProps } from './result/result-view';
import { resultKindOf } from './result/output';

// ─── Types ────────────────────────────────────────────────────────────────────

/** 可视化组件文案标签。 */
export interface ResultVisualizationLabels {
  noOutput?: string;
  exportCsv?: string;
  page?: string;
  of?: string;
  prev?: string;
  next?: string;
  sortAsc?: string;
  sortDesc?: string;
  rows?: string;
  columns?: string;
  key?: string;
  value?: string;
  chartTitle?: string;
  heatmapTitle?: string;
}

export interface OutputTableProps {
  rows: Record<string, unknown>[];
  labels?: ResultVisualizationLabels;
  pageSize?: number;
  title?: string;
}

export interface StatsChartProps {
  data: Array<{ label: string; value: number }>;
  type?: 'bar' | 'line';
  title?: string;
  color?: string;
  height?: number;
}

export interface KeyValuePanelProps {
  data: Record<string, unknown>;
  title?: string;
  labels?: ResultVisualizationLabels;
}

export interface CorrelationHeatmapProps {
  matrix: number[][];
  labels?: string[];
  title?: string;
}

export interface ResultVisualizationProps {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any -- 旧格式输出为任意 JSON，各分支按字段读取
  output: Record<string, any> | null;
  labels?: ResultVisualizationLabels;
  /** 输出锚点 ID（DistData 结果展示用）。 */
  outputId?: string;
  codeName?: string;
  downloadMode?: NodeResultViewProps['downloadMode'];
  actions?: NodeResultViewProps['actions'];
}

// ─── Utility ──────────────────────────────────────────────────────────────────

function safeJsonStringify(value: unknown, fallback = '{}'): string {
  try {
    return JSON.stringify(value ?? null, null, 2);
  } catch {
    return fallback;
  }
}

function downloadCsv(rows: Record<string, unknown>[], columns: string[], filename = 'export.csv') {
  const escape = (v: unknown) => {
    const s = String(v ?? '');
    return s.includes(',') || s.includes('"') || s.includes('\n') ? `"${s.replace(/"/g, '""')}"` : s;
  };
  const header = columns.map(escape).join(',');
  const body = rows.map((row) => columns.map((col) => escape(row[col])).join(',')).join('\n');
  const csv = `${header}\n${body}`;
  const blob = new Blob([`\uFEFF${csv}`], { type: 'text/csv;charset=utf-8;' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}

// ─── OutputTable ──────────────────────────────────────────────────────────────

/**
 * 增强表格组件：支持分页、列排序、CSV 导出。
 * 对应原版 `output-table-result` 组件。
 */
export const OutputTable: React.FC<OutputTableProps> = ({ rows, labels = {}, pageSize = 20, title }) => {
  const [page, setPage] = useState(0);
  const [sortCol, setSortCol] = useState<string | null>(null);
  const [sortDir, setSortDir] = useState<'asc' | 'desc'>('asc');

  const columns = useMemo(() => (rows.length > 0 ? Object.keys(rows[0]) : []), [rows]);

  const sortedRows = useMemo(() => {
    if (!sortCol) return rows;
    const sorted = [...rows].sort((a, b) => {
      const av = a[sortCol];
      const bv = b[sortCol];
      const an = Number(av);
      const bn = Number(bv);
      if (!Number.isNaN(an) && !Number.isNaN(bn)) {
        return sortDir === 'asc' ? an - bn : bn - an;
      }
      const as = String(av ?? '');
      const bs = String(bv ?? '');
      return sortDir === 'asc' ? as.localeCompare(bs) : bs.localeCompare(as);
    });
    return sorted;
  }, [rows, sortCol, sortDir]);

  const totalPages = Math.max(1, Math.ceil(sortedRows.length / pageSize));
  const pageRows = sortedRows.slice(page * pageSize, (page + 1) * pageSize);

  const handleSort = (col: string) => {
    if (sortCol === col) {
      setSortDir((d) => (d === 'asc' ? 'desc' : 'asc'));
    } else {
      setSortCol(col);
      setSortDir('asc');
    }
    setPage(0);
  };

  return (
    <div className="space-y-2">
      {title && <div className="text-[11px] font-medium text-gray-300">{title}</div>}
      <div className="flex items-center justify-between">
        <span className="text-[10px] text-gray-500">
          {sortedRows.length} {labels.rows ?? 'rows'} × {columns.length} {labels.columns ?? 'cols'}
        </span>
        <Button variant="ghost" size="sm" onClick={() => downloadCsv(sortedRows, columns)}>
          {labels.exportCsv ?? 'Export CSV'}
        </Button>
      </div>
      <div className="overflow-auto max-h-80 border border-gray-800 rounded">
        <table className="w-full text-left text-[10px]">
          <thead className="bg-gray-900 text-gray-400 sticky top-0">
            <tr>
              {columns.map((col) => (
                <th
                  key={col}
                  className="p-2 border-b border-gray-800 cursor-pointer select-none hover:text-gray-200 whitespace-nowrap"
                  onClick={() => handleSort(col)}
                >
                  {col}
                  {sortCol === col && <span className="ml-1">{sortDir === 'asc' ? '↑' : '↓'}</span>}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {pageRows.map((row, idx) => (
              <tr key={idx} className="border-b border-gray-800/50 last:border-0 hover:bg-gray-800/30">
                {columns.map((col) => (
                  <td key={col} className="p-2 text-gray-300 whitespace-nowrap max-w-[200px] truncate">
                    {String(row[col] ?? '')}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {totalPages > 1 && (
        <div className="flex items-center justify-between text-[10px] text-gray-500">
          <span>
            {labels.page ?? 'Page'} {page + 1} {labels.of ?? 'of'} {totalPages}
          </span>
          <div className="flex gap-1">
            <Button variant="ghost" size="sm" onClick={() => setPage((p) => Math.max(0, p - 1))} disabled={page <= 0}>
              {labels.prev ?? '‹'}
            </Button>
            <Button variant="ghost" size="sm" onClick={() => setPage((p) => Math.min(totalPages - 1, p + 1))} disabled={page >= totalPages - 1}>
              {labels.next ?? '›'}
            </Button>
          </div>
        </div>
      )}
    </div>
  );
};

// ─── StatsChart ───────────────────────────────────────────────────────────────

/**
 * SVG 柱状图 / 折线图组件。
 * 对应原版 `feature-import`、`mpc-pva-chart`、`mpc-stats-info` 等图表组件。
 */
export const StatsChart: React.FC<StatsChartProps> = ({ data, type = 'bar', title, color = '#3b82f6', height = 200 }) => {
  if (data.length === 0) return null;

  const width = 400;
  const padding = { top: 20, right: 20, bottom: 40, left: 50 };
  const chartW = width - padding.left - padding.right;
  const chartH = height - padding.top - padding.bottom;

  const maxVal = Math.max(...data.map((d) => d.value), 0.001);
  const minVal = Math.min(...data.map((d) => d.value), 0);
  const range = maxVal - minVal || 1;

  const barWidth = Math.max(4, Math.min(40, chartW / data.length - 4));
  const gap = (chartW - barWidth * data.length) / (data.length + 1);

  const scaleY = (v: number) => chartH - ((v - minVal) / range) * chartH;

  // Y-axis ticks
  const tickCount = 5;
  const ticks = Array.from({ length: tickCount + 1 }, (_, i) => minVal + (range * i) / tickCount);

  // Line path
  const linePath = data
    .map((d, i) => {
      const x = padding.left + gap + i * (barWidth + gap) + barWidth / 2;
      const y = padding.top + scaleY(d.value);
      return `${i === 0 ? 'M' : 'L'}${x},${y}`;
    })
    .join(' ');

  return (
    <div className="space-y-1">
      {title && <div className="text-[11px] font-medium text-gray-300">{title}</div>}
      <svg viewBox={`0 0 ${width} ${height}`} className="w-full" style={{ maxHeight: height }}>
        {/* Grid lines */}
        {ticks.map((tick, i) => {
          const y = padding.top + scaleY(tick);
          return (
            <g key={i}>
              <line x1={padding.left} y1={y} x2={width - padding.right} y2={y} stroke="#374151" strokeWidth="0.5" strokeDasharray="2,2" />
              <text x={padding.left - 4} y={y + 3} textAnchor="end" fill="#9ca3af" fontSize="8">
                {tick >= 1000 ? `${(tick / 1000).toFixed(1)}k` : Number.isInteger(tick) ? tick : tick.toFixed(2)}
              </text>
            </g>
          );
        })}

        {/* Bars or Line */}
        {type === 'bar'
          ? data.map((d, i) => {
              const x = padding.left + gap + i * (barWidth + gap);
              const barH = ((d.value - minVal) / range) * chartH;
              const y = padding.top + chartH - barH;
              return (
                <g key={i}>
                  <rect x={x} y={y} width={barWidth} height={Math.max(1, barH)} fill={color} rx="1" opacity="0.85">
                    <title>{`${d.label}: ${d.value}`}</title>
                  </rect>
                  {data.length <= 15 && (
                    <text x={x + barWidth / 2} y={height - padding.bottom + 12} textAnchor="middle" fill="#9ca3af" fontSize="7" transform={`rotate(-30, ${x + barWidth / 2}, ${height - padding.bottom + 12})`}>
                      {d.label.length > 8 ? `${d.label.slice(0, 8)}…` : d.label}
                    </text>
                  )}
                </g>
              );
            })
          : <>
              <path d={linePath} fill="none" stroke={color} strokeWidth="1.5" />
              {data.map((d, i) => {
                const x = padding.left + gap + i * (barWidth + gap) + barWidth / 2;
                const y = padding.top + scaleY(d.value);
                return (
                  <circle key={i} cx={x} cy={y} r="2.5" fill={color}>
                    <title>{`${d.label}: ${d.value}`}</title>
                  </circle>
                );
              })}
              {data.length <= 15 &&
                data.map((d, i) => {
                  const x = padding.left + gap + i * (barWidth + gap) + barWidth / 2;
                  return (
                    <text key={i} x={x} y={height - padding.bottom + 12} textAnchor="middle" fill="#9ca3af" fontSize="7" transform={`rotate(-30, ${x}, ${height - padding.bottom + 12})`}>
                      {d.label.length > 8 ? `${d.label.slice(0, 8)}…` : d.label}
                    </text>
                  );
                })}
            </>}

        {/* Axes */}
        <line x1={padding.left} y1={padding.top} x2={padding.left} y2={padding.top + chartH} stroke="#6b7280" strokeWidth="1" />
        <line x1={padding.left} y1={padding.top + chartH} x2={width - padding.right} y2={padding.top + chartH} stroke="#6b7280" strokeWidth="1" />
      </svg>
    </div>
  );
};

// ─── KeyValuePanel ────────────────────────────────────────────────────────────

/**
 * 键值对面板：用于展示模型指标、评估结果等结构化数据。
 * 对应原版 `regression-evaluation` 中的指标表部分。
 */
export const KeyValuePanel: React.FC<KeyValuePanelProps> = ({ data, title, labels = {} }) => {
  const entries = Object.entries(data);
  if (entries.length === 0) return null;

  return (
    <div className="space-y-1">
      {title && <div className="text-[11px] font-medium text-gray-300">{title}</div>}
      <div className="border border-gray-800 rounded overflow-hidden">
        <table className="w-full text-[10px]">
          <thead className="bg-gray-900 text-gray-400">
            <tr>
              <th className="p-2 text-left border-b border-gray-800 w-1/2">{labels.key ?? 'Key'}</th>
              <th className="p-2 text-left border-b border-gray-800">{labels.value ?? 'Value'}</th>
            </tr>
          </thead>
          <tbody>
            {entries.map(([key, value]) => (
              <tr key={key} className="border-b border-gray-800/50 last:border-0">
                <td className="p-2 text-gray-400 font-mono">{key}</td>
                <td className="p-2 text-gray-200 font-mono">
                  {typeof value === 'object' ? safeJsonStringify(value) : String(value ?? '')}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
};

// ─── CorrelationHeatmap ───────────────────────────────────────────────────────

/**
 * 相关性矩阵热力图（SVG 实现）。
 * 对应原版 `corr-matrix` 组件。
 */
export const CorrelationHeatmap: React.FC<CorrelationHeatmapProps> = ({ matrix, labels: axisLabels = [], title }) => {
  const n = matrix.length;
  if (n === 0) return null;

  const cellSize = Math.max(16, Math.min(32, 300 / n));
  const labelW = 60;
  const size = labelW + n * cellSize + 10;

  const colorScale = (v: number): string => {
    // -1 (blue) → 0 (gray) → 1 (red)
    const clamped = Math.max(-1, Math.min(1, v));
    if (clamped >= 0) {
      const intensity = Math.round(clamped * 200);
      return `rgb(${80 + intensity}, ${60 - Math.round(clamped * 30)}, ${60 - Math.round(clamped * 30)})`;
    }
    const intensity = Math.round(-clamped * 200);
    return `rgb(${60 - Math.round(-clamped * 30)}, ${60 - Math.round(-clamped * 30)}, ${80 + intensity})`;
  };

  return (
    <div className="space-y-1">
      {title && <div className="text-[11px] font-medium text-gray-300">{title}</div>}
      <div className="overflow-auto">
        <svg viewBox={`0 0 ${size} ${size}`} style={{ width: size, height: size }}>
          {/* Column labels */}
          {axisLabels.map((label, i) => (
            <text
              key={`col-${i}`}
              x={labelW + i * cellSize + cellSize / 2}
              y={labelW - 4}
              textAnchor="start"
              fill="#9ca3af"
              fontSize="7"
              transform={`rotate(-45, ${labelW + i * cellSize + cellSize / 2}, ${labelW - 4})`}
            >
              {label.length > 8 ? `${label.slice(0, 8)}…` : label}
            </text>
          ))}
          {/* Row labels */}
          {axisLabels.map((label, i) => (
            <text key={`row-${i}`} x={labelW - 4} y={labelW + i * cellSize + cellSize / 2 + 3} textAnchor="end" fill="#9ca3af" fontSize="7">
              {label.length > 8 ? `${label.slice(0, 8)}…` : label}
            </text>
          ))}
          {/* Cells */}
          {matrix.map((row, ri) =>
            row.map((val, ci) => (
              <rect
                key={`${ri}-${ci}`}
                x={labelW + ci * cellSize}
                y={labelW + ri * cellSize}
                width={cellSize - 1}
                height={cellSize - 1}
                fill={colorScale(val)}
                rx="1"
              >
                <title>{`${axisLabels[ri] ?? ri} × ${axisLabels[ci] ?? ci}: ${val.toFixed(3)}`}</title>
              </rect>
            )),
          )}
        </svg>
      </div>
    </div>
  );
};

// ─── ResultVisualization ──────────────────────────────────────────────────────

/**
 * 结果可视化入口组件：根据 output 结构自动选择渲染方式。
 *
 * 支持的 output 格式：
 * - `{ type: 'table', meta: { rows: [...] } }` → OutputTable
 * - `{ type: 'chart', meta: { data: [...], chartType } }` → StatsChart
 * - `{ type: 'heatmap', meta: { matrix, labels } }` → CorrelationHeatmap
 * - `{ type: 'metrics', meta: { ... } }` → KeyValuePanel
 * - `{ tabs: { ... } }` → 多 Tab 渲染（每 tab 递归）
 * - 其他 → JSON 预览
 */
export const ResultVisualization: React.FC<ResultVisualizationProps> = ({ output, labels = {}, outputId, codeName, downloadMode, actions }) => {
  if (!output) {
    return <span className="text-gray-500 text-[11px]">{labels.noOutput ?? 'No output'}</span>;
  }

  // SecretPad / SecretFlow 结果：按 DistData 类型（table/model/rule/report/sf.*）分派。
  const distKind = resultKindOf(typeof output.type === 'string' ? output.type : undefined);
  if (distKind !== 'unknown' || Array.isArray(output.tabs) || typeof output.tabs === 'string') {
    return (
      <NodeResultView
        output={output}
        outputId={outputId}
        codeName={codeName}
        downloadMode={downloadMode}
        actions={actions}
        labels={labels.noOutput ? { noResult: labels.noOutput } : undefined}
      />
    );
  }

  // Table type
  if (output.type === 'table' && output.meta && Array.isArray(output.meta.rows)) {
    return (
      <div className="space-y-1">
        <div className="text-gray-500 text-[10px]">type: table · codeName: {output.codeName ?? '-'}</div>
        <OutputTable rows={output.meta.rows} labels={labels} title={output.name} />
      </div>
    );
  }

  // Chart type (feature importance, PVA, stats)
  if (output.type === 'chart' && output.meta && Array.isArray(output.meta.data)) {
    const chartData = (output.meta.data as Array<{ label?: string; name?: string; value?: number }>).map((d) => ({
      label: d.label ?? d.name ?? '',
      value: d.value ?? 0,
    }));
    return (
      <div className="space-y-1">
        <div className="text-gray-500 text-[10px]">type: chart · codeName: {output.codeName ?? '-'}</div>
        <StatsChart data={chartData} type={output.meta.chartType === 'line' ? 'line' : 'bar'} title={output.name} color={output.meta.color} />
      </div>
    );
  }

  // Heatmap type (correlation matrix)
  if (output.type === 'heatmap' && output.meta && Array.isArray(output.meta.matrix)) {
    return (
      <div className="space-y-1">
        <div className="text-gray-500 text-[10px]">type: heatmap · codeName: {output.codeName ?? '-'}</div>
        <CorrelationHeatmap matrix={output.meta.matrix} labels={output.meta.labels ?? []} title={output.name ?? labels.heatmapTitle} />
      </div>
    );
  }

  // Metrics / key-value type
  if (output.type === 'metrics' && output.meta && typeof output.meta === 'object') {
    return (
      <div className="space-y-1">
        <div className="text-gray-500 text-[10px]">type: metrics · codeName: {output.codeName ?? '-'}</div>
        <KeyValuePanel data={output.meta} title={output.name} labels={labels} />
      </div>
    );
  }

  // Tabs type (multi-tab output)
  if (output.tabs && typeof output.tabs === 'object') {
    const tabs = Object.entries(output.tabs as Record<string, unknown>);
    return (
      <div className="space-y-3">
        {tabs.map(([name, content]) => (
          <div key={name}>
            <div className="text-gray-400 text-[10px] mb-1 font-medium">{name}</div>
            {typeof content === 'object' && content !== null && (content as { type?: unknown }).type ? (
              <ResultVisualization output={content as Record<string, unknown>} labels={labels} />
            ) : (
              <div className="p-2 rounded bg-gray-900 border border-gray-800 font-mono text-[10px] text-gray-300 overflow-auto whitespace-pre-wrap">
                {typeof content === 'string' ? content : safeJsonStringify(content)}
              </div>
            )}
          </div>
        ))}
      </div>
    );
  }

  // Array of items → try to render as table
  if (Array.isArray(output.data) && output.data.length > 0 && typeof output.data[0] === 'object') {
    return <OutputTable rows={output.data} labels={labels} title={output.name ?? 'Data'} />;
  }

  // Key-value object with numeric values → auto chart
  if (typeof output === 'object' && !output.type && !output.tabs) {
    const entries = Object.entries(output).filter(([, v]) => typeof v === 'number');
    if (entries.length >= 3 && entries.length <= 30) {
      const chartData = entries.map(([k, v]) => ({ label: k, value: v as number }));
      return (
        <div className="space-y-3">
          <StatsChart data={chartData} title={labels.chartTitle ?? 'Statistics'} />
          <KeyValuePanel data={output} labels={labels} />
        </div>
      );
    }
  }

  // Fallback: JSON preview
  return (
    <pre className="p-2 rounded bg-gray-900 border border-gray-800 font-mono text-[10px] text-gray-300 max-h-96 overflow-auto whitespace-pre-wrap">
      {safeJsonStringify(output)}
    </pre>
  );
};
