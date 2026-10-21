/**
 * 节点结果渲染（对应旧版 modules/dag-result/** 与 result-details 中的报告部分）。
 *
 * - 按 DistData 类型分派：table / model / rule / report / serving / read_data；
 * - report：Tabs → divs → children（descriptions / table / div 递归），警告折叠、生成时间、Tab 名映射；
 * - codeName 专用可视化注册表（tab 级与 report 级）：相关系数热力图、PVA、groupby 透视、回归评估、
 *   全表统计、PSI、分箱、特征重要性（sgb_train）、数据集采样（data_filter/sample）；
 * - 整份报告与表结果字段表可全屏（ChartFrame）；groupby 透视支持行 / 列 / 值拖拽配置；
 * - table：字段 / 类型 / 节点筛选，按参与方分行，下载 / 预览 / TEE 申请；
 * - model / rule：路径复制、查看结果、下载 / TEE 申请。
 *
 * 样式采用中性半透明色，保证在深色画布与浅色页面中均可读。
 */
import React, { useEffect, useMemo, useRef, useState } from 'react';
import type { NormalizedOutput, OutputRow } from './output';
import { downloadDisabledReason, normalizeOutput } from './output';
import type { CellValue, FlatTable, PivotConfig, ReportDiv, ReportTab } from './report';
import {
  decodeValue,
  defaultPivotConfig,
  downloadText,
  featureImportanceSeries,
  formatCell,
  formatDpCell,
  formatTimestamp,
  getTabName,
  lexicographicalOrder,
  pivotFlatTable,
  sampleSummary,
  sampleSummaryCsv,
  statsPsiFullCsv,
  tabToCorrMatrix,
  tabToFlatTable,
  tabToHistogram,
  tabToPvaSeries,
  toCsv,
} from './report';

/* -------------------------------------------------------------------------- */
/* 公共类型                                                                     */
/* -------------------------------------------------------------------------- */

export interface ResultActions {
  /** 直接下载（AUTONOMY / 非 TEE）。 */
  onDownload?: (row: OutputRow, output: NormalizedOutput) => void | Promise<void>;
  /** 预览数据。 */
  onPreview?: (row: OutputRow, output: NormalizedOutput) => void;
  /** TEE 模式下发起下载审批（TEE_DOWNLOAD）。 */
  onApplyTeeDownload?: (row: OutputRow, output: NormalizedOutput) => void;
  /** 该输出能否发起 TEE 下载审批（resourceType 只能是 model / rule / table）；返回 false 时隐藏入口。 */
  canApplyTeeDownload?: (output: NormalizedOutput) => boolean;
  /** 内置节点：跳转查看节点结果。 */
  onViewNodeResult?: (row: OutputRow, output: NormalizedOutput) => void;
}

export interface ResultViewLabels {
  report?: string;
  table?: string;
  model?: string;
  rule?: string;
  generatedAt?: string;
  path?: string;
  download?: string;
  preview?: string;
  applyDownload?: string;
  viewResult?: string;
  exportData?: string;
  exportSchema?: string;
  field?: string;
  fieldType?: string;
  node?: string;
  search?: string;
  noResult?: string;
  noParty?: string;
  copy?: string;
  copied?: string;
  warning?: string;
  fullscreen?: string;
  exitFullscreen?: string;
  groupTable?: string;
  crossTable?: string;
  all?: string;
  copyFields?: string;
  tableView?: string;
  matrixView?: string;
  axisView?: string;
  features?: string;
  pivotRows?: string;
  pivotColumns?: string;
  pivotValues?: string;
  pivotFields?: string;
  pivotDragHint?: string;
  featureImportance?: string;
  sampleSummary?: string;
  sampleRate?: string;
}

const ZH_LABELS: Required<ResultViewLabels> = {
  report: '报告',
  table: '表',
  model: '模型',
  rule: '规则',
  generatedAt: '生成时间：',
  path: '路径：',
  download: '下载',
  preview: '预览',
  applyDownload: '申请下载',
  viewResult: '查看结果',
  exportData: '导出数据',
  exportSchema: '导出表结构',
  field: '字段',
  fieldType: '类型',
  node: '节点',
  search: '搜索',
  noResult: '暂无结果',
  noParty: '非数据参与方，无计算结果',
  copy: '复制',
  copied: '已复制',
  warning: 'Warning',
  fullscreen: '全屏',
  exitFullscreen: '退出全屏',
  groupTable: '分组结果表',
  crossTable: '交叉透视表',
  all: '全部',
  copyFields: '复制已选字段',
  tableView: '表格',
  matrixView: '色块矩阵',
  axisView: '色块坐标轴',
  features: '全部特征：',
  pivotRows: '行',
  pivotColumns: '列',
  pivotValues: '值',
  pivotFields: '待选字段',
  pivotDragHint: '拖拽字段到行 / 列 / 值，或用下拉切换区域',
  featureImportance: '特征重要性',
  sampleSummary: '采样汇总',
  sampleRate: '采样率',
};

export interface NodeResultViewProps {
  /** 原始输出（graph/node/output 返回值）或已规范化的输出。 */
  output: unknown;
  /** 输出锚点 ID（展示用）。 */
  outputId?: string;
  /** 组件 codeName（优先于 output.codeName）。 */
  codeName?: string;
  /** 下载方式：direct 直接下载，tee 走审批，none 不提供。 */
  downloadMode?: 'direct' | 'tee' | 'none';
  actions?: ResultActions;
  labels?: ResultViewLabels;
  locale?: 'zh' | 'en';
}

const box = 'rounded-lg border border-gray-500/30';
const muted = 'text-gray-500';
const linkBtn = 'text-blue-500 hover:underline disabled:opacity-40 disabled:no-underline disabled:cursor-not-allowed';

/* -------------------------------------------------------------------------- */
/* 可视化注册表                                                                 */
/* -------------------------------------------------------------------------- */

export interface TabVisProps {
  tab: ReportTab;
  tabs: ReportTab[];
  codeName: string;
  id: string;
  labels: Required<ResultViewLabels>;
}
export interface ReportVisProps {
  tabs: ReportTab[];
  codeName: string;
  id: string;
  labels: Required<ResultViewLabels>;
  locale: 'zh' | 'en';
}

type TabVis = (p: TabVisProps) => React.ReactNode;
type ReportVis = (p: ReportVisProps) => React.ReactNode;

const TAB_VIS = new Map<string, TabVis>();
const REPORT_VIS = new Map<string, ReportVis>();

/** 注册 codeName 专用可视化。scope=tab 替换单个 Tab 内容；scope=report 接管整个报告区域。 */
export function registerResultVisualization(codeName: string, scope: 'tab', render: TabVis): void;
export function registerResultVisualization(codeName: string, scope: 'report', render: ReportVis): void;
export function registerResultVisualization(codeName: string, scope: 'tab' | 'report', render: TabVis | ReportVis) {
  if (scope === 'tab') TAB_VIS.set(codeName, render as TabVis);
  else REPORT_VIS.set(codeName, render as ReportVis);
}

export function hasResultVisualization(codeName: string): boolean {
  return TAB_VIS.has(codeName) || REPORT_VIS.has(codeName);
}

/* -------------------------------------------------------------------------- */
/* 基础组件                                                                     */
/* -------------------------------------------------------------------------- */

const CopyText: React.FC<{ text: string; labels: Required<ResultViewLabels> }> = ({ text, labels }) => {
  const [copied, setCopied] = useState(false);
  return (
    <button
      type="button"
      className={linkBtn}
      onClick={() => {
        try {
          void navigator.clipboard?.writeText(text);
        } catch {
          /* ignore */
        }
        setCopied(true);
        setTimeout(() => setCopied(false), 1500);
      }}
    >
      {copied ? labels.copied : labels.copy}
    </button>
  );
};

const Tag: React.FC<{ children: React.ReactNode }> = ({ children }) => (
  <span className="px-1.5 py-0.5 rounded bg-blue-500/10 text-blue-500 text-[10px]">{children}</span>
);

const ResultHeader: React.FC<{ id?: string; tag: string; gmtCreate?: string; labels: Required<ResultViewLabels> }> = ({ id, tag, gmtCreate, labels }) => (
  <div className="space-y-1">
    <div className="flex items-center gap-2">
      <span className="font-semibold break-all">{id}</span>
      <Tag>{tag}</Tag>
    </div>
    {gmtCreate && (
      <div className={`text-[11px] ${muted}`}>
        {labels.generatedAt}
        {formatTimestamp(gmtCreate)}
      </div>
    )}
  </div>
);

/** 简易 Tabs。 */
export const SimpleTabs: React.FC<{ items: Array<{ key: string; label: string; content: React.ReactNode }> }> = ({ items }) => {
  const [active, setActive] = useState(items[0]?.key ?? '');
  const current = items.find((i) => i.key === active) ?? items[0];
  if (items.length === 0) return null;
  return (
    <div>
      <div className="flex flex-wrap gap-1 border-b border-gray-500/30 mb-2" role="tablist">
        {items.map((it) => (
          <button
            key={it.key}
            type="button"
            role="tab"
            aria-selected={it.key === current?.key}
            onClick={() => setActive(it.key)}
            className={`px-2 py-1 text-[11px] border-b-2 -mb-px ${it.key === current?.key ? 'border-blue-500 text-blue-500' : `border-transparent ${muted}`}`}
          >
            {it.label}
          </button>
        ))}
      </div>
      <div>{current?.content}</div>
    </div>
  );
};

interface SortableTableProps {
  columns: Array<{ name: string; type: string }>;
  rows: CellValue[][];
  codeName?: string;
  csvName?: string;
  labels: Required<ResultViewLabels>;
  /** 自定义 CSV 数据（stats_psi 全量导出）。 */
  csvRows?: CellValue[][];
  pageSize?: number;
}

/**
 * 通用报告表（旧版 vis/output-table）：列排序（int/str 字典序，其他数值序）、
 * 长文本省略、CSV 导出、>100 行分页、DP count 取整。
 */
export const SortableTable: React.FC<SortableTableProps> = ({ columns, rows, codeName, csvName = 'export', labels, csvRows, pageSize = 100 }) => {
  const [sort, setSort] = useState<{ col: number; dir: 1 | -1 } | null>(null);
  const [page, setPage] = useState(0);
  const ellipsis = columns.length > 3;
  const displayRows = useMemo(() => {
    const objRows = rows.map((r) => {
      const o: Record<string, CellValue> = {};
      columns.forEach((c, i) => (o[c.name] = r[i]));
      return o;
    });
    const formatted = rows.map((r, ri) => r.map((v) => formatDpCell(v, objRows[ri], codeName)));
    if (!sort) return formatted;
    const type = columns[sort.col]?.type;
    const lexi = type === 'int' || type === 'str' || type === 'AT_INT' || type === 'AT_STRING';
    return [...formatted].sort((a, b) => {
      const x = a[sort.col];
      const y = b[sort.col];
      const cmp = lexi ? lexicographicalOrder(String(x), String(y)) : Number(x) - Number(y);
      return (Number.isNaN(cmp) ? String(x).localeCompare(String(y)) : cmp) * sort.dir;
    });
  }, [rows, columns, sort, codeName]);
  const pages = Math.max(1, Math.ceil(displayRows.length / pageSize));
  const visible = displayRows.length > pageSize ? displayRows.slice(page * pageSize, (page + 1) * pageSize) : displayRows;
  const exportCsv = () => downloadText(`${csvName}.csv`, toCsv(csvRows ?? [columns.map((c) => c.name), ...displayRows]));
  return (
    <div className="space-y-1">
      <div className="flex justify-end">
        <button type="button" className={`${linkBtn} text-[11px]`} onClick={exportCsv}>
          ⬇ {labels.exportData}
        </button>
      </div>
      <div className={`${box} overflow-auto`}>
        <table className="w-full text-[11px]">
          <thead>
            <tr className="bg-gray-500/10">
              {columns.map((c, i) => (
                <th
                  key={`${c.name}-${i}`}
                  className={`px-2 py-1 text-left font-semibold whitespace-nowrap ${c.name !== 'name' ? 'cursor-pointer select-none' : ''}`}
                  onClick={() => {
                    if (c.name === 'name') return;
                    setSort((s) => (s?.col === i ? (s.dir === 1 ? { col: i, dir: -1 } : null) : { col: i, dir: 1 }));
                  }}
                >
                  {c.name}
                  {sort?.col === i ? (sort.dir === 1 ? ' ▲' : ' ▼') : ''}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {visible.map((r, ri) => (
              <tr key={ri} className="border-t border-gray-500/20">
                {r.map((v, ci) => {
                  const text = String(v ?? '');
                  return (
                    <td key={ci} className="px-2 py-1 whitespace-nowrap" title={text}>
                      {ellipsis && text.length > 12 ? `${text.slice(0, 6)}...${text.slice(-6)}` : text}
                    </td>
                  );
                })}
              </tr>
            ))}
            {visible.length === 0 && (
              <tr>
                <td colSpan={Math.max(1, columns.length)} className={`px-2 py-3 text-center ${muted}`}>
                  {labels.noResult}
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
      {displayRows.length > pageSize && (
        <div className="flex items-center justify-end gap-2 text-[11px]">
          <button type="button" className={linkBtn} disabled={page === 0} onClick={() => setPage((p) => p - 1)}>
            ‹
          </button>
          <span>
            {page + 1}/{pages}
          </span>
          <button type="button" className={linkBtn} disabled={page >= pages - 1} onClick={() => setPage((p) => p + 1)}>
            ›
          </button>
        </div>
      )}
    </div>
  );
};

/** 渲染 descriptions（键值网格）。 */
const DescriptionsBlock: React.FC<{ items: Array<{ name: string; desc?: string; type: string; value: unknown }>; title?: string }> = ({ items, title }) => (
  <div className="space-y-1">
    {title && <div className="font-medium text-[11px]">{title}</div>}
    <div className={`${box} grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)] text-[11px]`}>
      {items.map((it, i) => (
        <React.Fragment key={`${it.name}-${i}`}>
          <div className="px-2 py-1 bg-gray-500/10 border-b border-gray-500/20 break-all" title={it.desc}>
            {it.name}
          </div>
          <div className="px-2 py-1 border-b border-gray-500/20 break-all">{formatCell(decodeValue(it.value as never, it.type))}</div>
        </React.Fragment>
      ))}
    </div>
  </div>
);

/** 递归渲染 Div（descriptions / table / div）。 */
export const ReportDivView: React.FC<{ div: ReportDiv; codeName?: string; id: string; labels: Required<ResultViewLabels>; depth?: number }> = ({
  div,
  codeName,
  id,
  labels,
  depth = 0,
}) => (
  <div className={`space-y-2 ${depth > 0 ? 'pl-2 border-l border-gray-500/30' : ''}`}>
    {div.name && <div className="font-semibold text-[11px]">{div.name}</div>}
    {div.desc && <div className={`text-[10px] ${muted}`}>{div.desc}</div>}
    {(div.children || []).map((child, i) => {
      if (child.type === 'descriptions' && child.descriptions) {
        return <DescriptionsBlock key={i} items={child.descriptions.items || []} title={child.descriptions.name} />;
      }
      if (child.type === 'table' && child.table) {
        const t = child.table;
        const flat = tabToFlatTable({ divs: [{ children: [child] }] }, codeName);
        return (
          <div key={i} className="space-y-1">
            {t.name && <div className="font-medium text-[11px]">{t.name}</div>}
            <SortableTable columns={flat.columns} rows={flat.rows} codeName={codeName} csvName={t.name || id} labels={labels} />
          </div>
        );
      }
      if (child.type === 'div' && child.div) {
        return <ReportDivView key={i} div={child.div} codeName={codeName} id={id} labels={labels} depth={depth + 1} />;
      }
      return null;
    })}
  </div>
);

/** 默认 Tab 渲染：单一表 / 描述时走扁平表（支持排序导出），否则递归渲染结构。 */
const DefaultTabView: React.FC<TabVisProps> = ({ tab, tabs, codeName, id, labels }) => {
  const divs = tab.divs || [];
  const simple = divs.length > 0 && divs.every((d) => (d.children || []).every((c) => c.type !== 'div'));
  const firstType = divs[0]?.children?.[0]?.type;
  if (simple && (firstType === 'descriptions' || (divs.length === 1 && divs[0].children?.length === 1))) {
    const flat = tabToFlatTable(tab, codeName);
    const importance = featureImportanceSeries(flat);
    const csvRows = codeName === 'stats/stats_psi' ? statsPsiFullCsv(tabs) : undefined;
    return (
      <div className="space-y-2">
        {tab.desc && <div className={`text-[10px] ${muted}`}>{tab.desc}</div>}
        {importance && (
          <ChartFrame labels={labels}>
            <HBarChart data={importance} />
          </ChartFrame>
        )}
        <SortableTable columns={flat.columns} rows={flat.rows} codeName={codeName} csvName={id} labels={labels} csvRows={csvRows} />
      </div>
    );
  }
  return (
    <div className="space-y-3">
      {tab.desc && <div className={`text-[10px] ${muted}`}>{tab.desc}</div>}
      {divs.map((div, i) => (
        <ReportDivView key={i} div={div} codeName={codeName} id={id} labels={labels} />
      ))}
    </div>
  );
};

/* -------------------------------------------------------------------------- */
/* 图表（零依赖 SVG）                                                            */
/* -------------------------------------------------------------------------- */

/**
 * 图表外框：标题栏右侧放置操作（导出等）与「全屏 / 退出全屏」（旧版 useFullscreen）。
 * 优先用浏览器 Fullscreen API，不可用时退化为固定定位覆盖层。
 */
export const ChartFrame: React.FC<{
  title?: string;
  labels: Pick<Required<ResultViewLabels>, 'fullscreen' | 'exitFullscreen'>;
  extra?: React.ReactNode;
  /** 可传函数，按是否全屏调整内容（如全屏时取消表格最大高度）。 */
  children: React.ReactNode | ((full: boolean) => React.ReactNode);
}> = ({
  title,
  labels,
  extra,
  children,
}) => {
  const ref = useRef<HTMLDivElement>(null);
  const [full, setFull] = useState(false);
  useEffect(() => {
    const onChange = () => {
      if (typeof document !== 'undefined' && !document.fullscreenElement) setFull(false);
    };
    document.addEventListener?.('fullscreenchange', onChange);
    return () => document.removeEventListener?.('fullscreenchange', onChange);
  }, []);
  const enter = () => {
    const el = ref.current;
    setFull(true);
    if (el?.requestFullscreen) el.requestFullscreen().catch(() => undefined);
  };
  const exit = () => {
    setFull(false);
    if (typeof document !== 'undefined' && document.fullscreenElement) document.exitFullscreen?.().catch(() => undefined);
  };
  return (
    <div ref={ref} className={full ? 'fixed inset-0 z-[100] overflow-auto bg-white dark:bg-gray-950 p-6 space-y-2' : 'space-y-2'}>
      <div className="flex items-center gap-3 text-[11px]">
        {title && <span className="font-semibold">{title}</span>}
        <span className="flex-1" />
        {extra}
        <button type="button" className={linkBtn} onClick={full ? exit : enter}>
          {full ? `⤡ ${labels.exitFullscreen}` : `⤢ ${labels.fullscreen}`}
        </button>
      </div>
      {typeof children === 'function' ? children(full) : children}
    </div>
  );
};

const niceTicks = (min: number, max: number, count = 5): number[] => {
  if (!Number.isFinite(min) || !Number.isFinite(max)) return [];
  if (min === max) return [min];
  const step = (max - min) / (count - 1);
  return Array.from({ length: count }, (_, i) => min + step * i);
};
const fmtTick = (v: number) => (Math.abs(v) >= 1000 || Number.isInteger(v) ? String(Math.round(v)) : v.toFixed(Math.abs(v) < 1 ? 3 : 2));

const PALETTE = ['#3b82f6', '#22c55e', '#f59e0b', '#ef4444', '#a855f7', '#06b6d4'];

export const HBarChart: React.FC<{ data: Array<{ label: string; value: number }>; height?: number; color?: string }> = ({ data, color = PALETTE[0] }) => {
  if (data.length === 0) return null;
  const max = Math.max(...data.map((d) => Math.abs(d.value)), 1e-9);
  const rowH = 16;
  const labelW = 110;
  const width = 360;
  return (
    <svg width="100%" viewBox={`0 0 ${width} ${data.length * rowH + 4}`} className="max-w-full" role="img">
      {data.map((d, i) => {
        const w = (Math.abs(d.value) / max) * (width - labelW - 50);
        return (
          <g key={`${d.label}-${i}`} transform={`translate(0, ${i * rowH + 2})`}>
            <text x={labelW - 4} y={rowH / 2 + 3} textAnchor="end" fontSize="9" fill="currentColor">
              {d.label.length > 16 ? `${d.label.slice(0, 16)}…` : d.label}
            </text>
            <rect x={labelW} y={2} width={Math.max(1, w)} height={rowH - 4} fill={d.value < 0 ? PALETTE[3] : color} rx={2}>
              <title>{`${d.label}: ${d.value}`}</title>
            </rect>
            <text x={labelW + w + 4} y={rowH / 2 + 3} fontSize="9" fill="currentColor">
              {Number.isInteger(d.value) ? d.value : d.value.toFixed(4)}
            </text>
          </g>
        );
      })}
    </svg>
  );
};

/** 折线图（旧版 G2 line：图例、网格、共享 tooltip + 十字辅助线）。 */
export const MultiLineChart: React.FC<{ labels: string[]; series: Array<{ name: string; values: number[] }>; height?: number }> = ({ labels, series, height = 260 }) => {
  const [hover, setHover] = useState<number | null>(null);
  const width = 520;
  const pad = { l: 48, r: 12, t: 12, b: 44 };
  const all = series.flatMap((s) => s.values).filter((v) => !Number.isNaN(v));
  if (all.length === 0 || labels.length === 0) return null;
  const min = Math.min(...all, 0);
  const max = Math.max(...all, 1e-9);
  const innerW = width - pad.l - pad.r;
  const x = (i: number) => pad.l + (labels.length === 1 ? innerW / 2 : (i / (labels.length - 1)) * innerW);
  const y = (v: number) => pad.t + (1 - (v - min) / (max - min || 1)) * (height - pad.t - pad.b);
  const onMove = (e: React.MouseEvent<SVGRectElement>) => {
    const box = e.currentTarget.getBoundingClientRect();
    const px = ((e.clientX - box.left) / (box.width || 1)) * innerW;
    const idx = labels.length === 1 ? 0 : Math.round((px / innerW) * (labels.length - 1));
    setHover(Math.max(0, Math.min(labels.length - 1, idx)));
  };
  return (
    <div className="relative">
      <svg width="100%" viewBox={`0 0 ${width} ${height}`} role="img" aria-label="line chart">
        {niceTicks(min, max).map((v, i) => (
          <g key={i}>
            <line x1={pad.l} x2={width - pad.r} y1={y(v)} y2={y(v)} stroke="currentColor" opacity={0.12} />
            <text x={pad.l - 4} y={y(v) + 3} fontSize="8" textAnchor="end" fill="currentColor">
              {fmtTick(v)}
            </text>
          </g>
        ))}
        <line x1={pad.l} y1={height - pad.b} x2={width - pad.r} y2={height - pad.b} stroke="currentColor" opacity={0.3} />
        {labels.map((l, i) =>
          i % Math.ceil(labels.length / 10) === 0 ? (
            <text key={i} x={x(i)} y={height - pad.b + 12} fontSize="8" textAnchor="end" fill="currentColor" transform={`rotate(-30, ${x(i)}, ${height - pad.b + 12})`}>
              {l.length > 14 ? `${l.slice(0, 14)}…` : l}
            </text>
          ) : null,
        )}
        {series.map((s, si) => (
          <polyline
            key={s.name}
            fill="none"
            stroke={PALETTE[si % PALETTE.length]}
            strokeWidth={2.5}
            points={s.values.map((v, i) => (Number.isNaN(v) ? '' : `${x(i)},${y(v)}`)).filter(Boolean).join(' ')}
          />
        ))}
        {hover !== null && (
          <g>
            <line x1={x(hover)} x2={x(hover)} y1={pad.t} y2={height - pad.b} stroke="currentColor" opacity={0.4} strokeDasharray="3 3" />
            {series.map((s, si) =>
              Number.isNaN(s.values[hover]) ? null : <circle key={s.name} cx={x(hover)} cy={y(s.values[hover])} r={3.5} fill={PALETTE[si % PALETTE.length]} />,
            )}
          </g>
        )}
        <rect x={pad.l} y={pad.t} width={innerW} height={height - pad.t - pad.b} fill="transparent" onMouseMove={onMove} onMouseLeave={() => setHover(null)} data-testid="chart-hover" />
      </svg>
      {hover !== null && (
        <div
          role="tooltip"
          className="absolute top-2 pointer-events-none rounded border border-gray-500/30 bg-white/95 dark:bg-gray-900/95 px-2 py-1 text-[10px] shadow"
          style={{ left: `${Math.min(70, (x(hover) / width) * 100)}%` }}
        >
          <div className="font-semibold mb-0.5">{labels[hover]}</div>
          {series.map((s, si) => (
            <div key={s.name} className="flex items-center gap-1">
              <span className="w-2 h-2 rounded-full" style={{ background: PALETTE[si % PALETTE.length] }} />
              {s.name}: {Number.isNaN(s.values[hover]) ? '-' : s.values[hover]}
            </div>
          ))}
        </div>
      )}
      <div className="flex gap-3 text-[10px] justify-center">
        {series.map((s, si) => (
          <span key={s.name} className="inline-flex items-center gap-1">
            <span className="w-2 h-2 rounded-full" style={{ background: PALETTE[si % PALETTE.length] }} />
            {s.name}
          </span>
        ))}
      </div>
    </div>
  );
};

/** 纵向柱状图（旧版回归评估残差分布：y 轴计数、柱最小高度 1px、悬停提示）。 */
export const ColumnChart: React.FC<{ data: Array<{ label: string; value: number }>; height?: number; color?: string }> = ({ data, height = 260, color = PALETTE[0] }) => {
  if (data.length === 0) return null;
  const width = 520;
  const pad = { l: 44, r: 8, t: 12, b: 20 };
  const max = Math.max(...data.map((d) => d.value), 1e-9);
  const innerW = width - pad.l - pad.r;
  const innerH = height - pad.t - pad.b;
  const bw = innerW / data.length;
  const y = (v: number) => pad.t + innerH - (v / max) * innerH;
  return (
    <svg width="100%" viewBox={`0 0 ${width} ${height}`} role="img" aria-label="column chart">
      {niceTicks(0, max).map((v, i) => (
        <g key={i}>
          <line x1={pad.l} x2={width - pad.r} y1={y(v)} y2={y(v)} stroke="currentColor" opacity={0.12} />
          <text x={pad.l - 4} y={y(v) + 3} fontSize="8" textAnchor="end" fill="currentColor">
            {fmtTick(v)}
          </text>
        </g>
      ))}
      {data.map((d, i) => {
        const h = d.value > 0 ? Math.max(1, (d.value / max) * innerH) : 0;
        return (
          <rect key={i} x={pad.l + i * bw + bw * 0.1} y={pad.t + innerH - h} width={Math.max(1, bw * 0.8)} height={h} fill={color}>
            <title>{`${d.label}: ${d.value}`}</title>
          </rect>
        );
      })}
      <line x1={pad.l} y1={pad.t + innerH} x2={width - pad.r} y2={pad.t + innerH} stroke="currentColor" opacity={0.3} />
    </svg>
  );
};

const Heatmap: React.FC<{ labels: string[]; matrix: number[][] }> = ({ labels, matrix }) => {
  const cell = Math.max(10, Math.min(28, Math.floor(360 / Math.max(1, labels.length))));
  const lw = 70;
  const size = lw + cell * labels.length;
  const color = (v: number) => {
    const t = Math.max(-1, Math.min(1, v));
    return t >= 0 ? `rgba(239,68,68,${0.15 + 0.85 * t})` : `rgba(59,130,246,${0.15 + 0.85 * -t})`;
  };
  return (
    <svg width="100%" viewBox={`0 0 ${size} ${size + 26}`} className="max-w-[560px]" role="img" aria-label="correlation heatmap">
      {labels.map((l, i) => (
        <text key={`c${i}`} x={lw + i * cell + cell / 2} y={lw - 4} fontSize="8" textAnchor="start" fill="currentColor" transform={`rotate(-45, ${lw + i * cell + cell / 2}, ${lw - 4})`}>
          {l.length > 10 ? `${l.slice(0, 10)}…` : l}
        </text>
      ))}
      {labels.map((l, i) => (
        <text key={`r${i}`} x={lw - 4} y={lw + i * cell + cell / 2 + 3} fontSize="8" textAnchor="end" fill="currentColor">
          {l.length > 10 ? `${l.slice(0, 10)}…` : l}
        </text>
      ))}
      {matrix.map((row, ri) =>
        row.map((v, ci) => (
          <rect key={`${ri}-${ci}`} x={lw + ci * cell} y={lw + ri * cell} width={cell - 1} height={cell - 1} fill={color(v)}>
            <title>{`${labels[ri]} × ${labels[ci]}: ${v.toFixed(4)}`}</title>
          </rect>
        )),
      )}
      {/* 色阶图例：-1 → 0 → 1 */}
      <defs>
        <linearGradient id="corr-legend" x1="0" x2="1" y1="0" y2="0">
          <stop offset="0%" stopColor="rgb(59,130,246)" />
          <stop offset="50%" stopColor="rgba(200,200,200,0.3)" />
          <stop offset="100%" stopColor="rgb(239,68,68)" />
        </linearGradient>
      </defs>
      <rect x={lw} y={size + 6} width={Math.max(60, cell * labels.length)} height={6} fill="url(#corr-legend)" />
      <text x={lw} y={size + 22} fontSize="8" fill="currentColor">-1</text>
      <text x={lw + Math.max(60, cell * labels.length) / 2} y={size + 22} fontSize="8" textAnchor="middle" fill="currentColor">0</text>
      <text x={lw + Math.max(60, cell * labels.length)} y={size + 22} fontSize="8" textAnchor="end" fill="currentColor">1</text>
    </svg>
  );
};

/**
 * 色块坐标轴（旧版 corr-matrix/axis-chart，G2 point + square）：两个分类轴为特征，
 * 方块边长按 |r| 缩放（对角线不画），正相关绿色、负相关红色，透明度为 |r|。
 */
const CorrAxisChart: React.FC<{ labels: string[]; matrix: number[][] }> = ({ labels, matrix }) => {
  const n = Math.max(1, labels.length);
  const cell = Math.max(14, Math.min(40, Math.floor(480 / n)));
  const lw = 80;
  const width = lw + cell * n + 8;
  const height = cell * n + lw;
  const color = (v: number) => (v < 0 ? `rgba(255,0,0,${Math.min(1, -v)})` : `rgba(0,128,0,${Math.min(1, v)})`);
  const short = (l: string) => (l.length > 12 ? `${l.slice(0, 12)}…` : l);
  return (
    <svg width="100%" viewBox={`0 0 ${width} ${height}`} className="max-w-[640px]" role="img" aria-label="correlation axis chart">
      {labels.map((_, i) => (
        <line key={`g${i}`} x1={lw} x2={lw + cell * n} y1={i * cell + cell / 2} y2={i * cell + cell / 2} stroke="currentColor" opacity={0.08} />
      ))}
      <line x1={lw} x2={lw} y1={0} y2={cell * n} stroke="currentColor" opacity={0.2} />
      <line x1={lw} x2={lw + cell * n} y1={cell * n} y2={cell * n} stroke="currentColor" opacity={0.2} />
      {labels.map((l, i) => (
        <text key={`y${i}`} x={lw - 6} y={i * cell + cell / 2 + 3} fontSize="9" textAnchor="end" fill="currentColor">
          {short(l)}
        </text>
      ))}
      {labels.map((l, i) => (
        <text
          key={`x${i}`}
          x={lw + i * cell + cell / 2}
          y={cell * n + 10}
          fontSize="9"
          textAnchor="end"
          fill="currentColor"
          transform={`rotate(-45, ${lw + i * cell + cell / 2}, ${cell * n + 10})`}
        >
          {short(l)}
        </text>
      ))}
      {matrix.map((row, ri) =>
        row.map((v, ci) => {
          if (ri === ci || !Number.isFinite(v) || v === 0) return null;
          const side = Math.min(1, Math.abs(v)) * (cell - 2);
          return (
            <rect
              key={`${ri}-${ci}`}
              x={lw + ci * cell + (cell - side) / 2}
              y={ri * cell + (cell - side) / 2}
              width={side}
              height={side}
              fill={color(v)}
            >
              <title>{`${labels[ci]} × ${labels[ri]}: ${v.toFixed(4)}`}</title>
            </rect>
          );
        }),
      )}
    </svg>
  );
};

/* -------------------------------------------------------------------------- */
/* 专用可视化                                                                   */
/* -------------------------------------------------------------------------- */

/** 相关系数矩阵（stats/ss_pearsonr）：旧版 CorrMatrix —— 「全部」+ 特征标签多选（默认全选）、
 * 表格 / 色块矩阵 / 色块坐标轴三种视图、导出数据（CSV）、复制已选字段、全屏。 */
export const CorrMatrixVis: React.FC<TabVisProps> = ({ tab, id, labels }) => {
  const { labels: features, matrix } = useMemo(() => tabToCorrMatrix(tab), [tab]);
  const [selected, setSelected] = useState<string[] | null>(null);
  const [view, setView] = useState<'table' | 'matrix' | 'axis'>('table');
  const [copied, setCopied] = useState(false);
  const active = selected ?? features;
  const idx = features.map((f, i) => (active.includes(f) ? i : -1)).filter((i) => i >= 0);
  const sub = idx.map((r) => idx.map((c) => matrix[r]?.[c] ?? 0));
  const names = idx.map((i) => features[i]);
  const allOn = active.length === features.length;
  const toggle = (f: string) => setSelected(active.includes(f) ? active.filter((x) => x !== f) : [...active, f]);
  const tag = (on: boolean) => `px-1.5 py-0.5 rounded cursor-pointer border ${on ? 'border-blue-500 bg-blue-500/10 text-blue-500' : 'border-gray-500/30'}`;
  const copy = () => {
    void navigator.clipboard?.writeText(names.join(',')).catch(() => undefined);
    setCopied(true);
    setTimeout(() => setCopied(false), 5000);
  };
  return (
    <ChartFrame
      labels={labels}
      extra={
        <>
          {view === 'table' && (
            <button type="button" className={linkBtn} onClick={() => downloadText(`${id}-corr.csv`, toCsv([['', ...names], ...sub.map((r, i) => [names[i], ...r])]))}>
              ⬇ {labels.exportData}
            </button>
          )}
          <button type="button" className={linkBtn} onClick={copy}>
            {copied ? '✓' : '⧉'} {labels.copyFields}
          </button>
        </>
      }
    >
      <div className="text-[11px] font-semibold">{labels.features}</div>
      <div className="flex flex-wrap gap-1 text-[10px] max-h-28 overflow-y-auto">
        <button type="button" className={tag(allOn)} onClick={() => setSelected(allOn ? [] : features)}>
          {labels.all}
        </button>
        {features.map((f) => (
          <button key={f} type="button" title={f} className={tag(active.includes(f))} onClick={() => toggle(f)}>
            {f}
          </button>
        ))}
      </div>
      <div className="inline-flex rounded border border-gray-500/30 overflow-hidden text-[11px]">
        {(['table', 'matrix', 'axis'] as const).map((m) => (
          <button key={m} type="button" onClick={() => setView(m)} className={`px-2 py-0.5 ${view === m ? 'bg-blue-500 text-white' : ''}`}>
            {m === 'table' ? labels.tableView : m === 'matrix' ? labels.matrixView : labels.axisView}
          </button>
        ))}
      </div>
      {view === 'matrix' ? (
        <Heatmap labels={names} matrix={sub} />
      ) : view === 'axis' ? (
        <CorrAxisChart labels={names} matrix={sub} />
      ) : (
        <SortableTable columns={[{ name: '', type: 'str' }, ...names.map((n) => ({ name: n, type: 'float' }))]} rows={sub.map((r, i) => [names[i], ...r.map((v) => Number(v.toFixed(4)))])} labels={labels} csvName={`${id}-corr`} />
      )}
    </ChartFrame>
  );
};

/** PVA（ml.eval/prediction_bias_eval）。 */
export const PvaVis: React.FC<TabVisProps> = ({ tab, id, labels }) => {
  const data = useMemo(() => tabToPvaSeries(tab), [tab]);
  if (data.length === 0) return <DefaultTabView tab={tab} tabs={[tab]} codeName="" id={id} labels={labels} />;
  return (
    <ChartFrame
      labels={labels}
      extra={
        <button
          type="button"
          className={linkBtn}
          onClick={() =>
            downloadText('PVA.csv', toCsv([['interval', 'avg_prediction', 'avg_label', 'bias'], ...data.map((d) => [d.label, d.avg_prediction, d.avg_label, d.bias])]))
          }
        >
          ⬇ {labels.exportData}
        </button>
      }
    >
      <MultiLineChart
        height={320}
        labels={data.map((d) => d.label)}
        series={[
          { name: 'avg_prediction', values: data.map((d) => d.avg_prediction) },
          { name: 'avg_label', values: data.map((d) => d.avg_label) },
          { name: 'bias', values: data.map((d) => d.bias) },
        ]}
      />
    </ChartFrame>
  );
};

/** PSI（stats/stats_psi）：汇总表附 PSI 条形图，CSV 全量导出。 */
const PsiVis: React.FC<TabVisProps> = (props) => {
  const flat = tabToFlatTable(props.tab, props.codeName);
  const vi = flat.columns.findIndex((c) => /psi/i.test(c.name));
  const li = flat.columns.findIndex((c, i) => i !== vi && /feature|label|name/i.test(c.name));
  const data =
    vi >= 0
      ? flat.rows.map((r) => ({ label: String(r[li >= 0 ? li : 0]), value: Number(r[vi]) })).filter((d) => !Number.isNaN(d.value))
      : [];
  return (
    <div className="space-y-2">
      {data.length > 0 && (
        <ChartFrame labels={props.labels}>
          <HBarChart data={data.slice(0, 30)} color={PALETTE[2]} />
        </ChartFrame>
      )}
      <DefaultTabView {...props} />
    </div>
  );
};

/** 分箱（WOE / 等频 / 等宽）：含 woe 列时绘制 WOE 条形图。 */
const BinningVis: React.FC<TabVisProps> = (props) => {
  const flat = tabToFlatTable(props.tab, props.codeName);
  const wi = flat.columns.findIndex((c) => /woe/i.test(c.name));
  const li = flat.columns.findIndex((c) => /label|bin|interval|name/i.test(c.name));
  const data =
    wi >= 0
      ? flat.rows.map((r) => ({ label: String(r[li >= 0 ? li : 0]), value: Number(r[wi]) })).filter((d) => !Number.isNaN(d.value))
      : [];
  return (
    <div className="space-y-2">
      {data.length > 0 && (
        <ChartFrame labels={props.labels}>
          <HBarChart data={data} color={PALETTE[1]} />
        </ChartFrame>
      )}
      <DefaultTabView {...props} />
    </div>
  );
};

/** 全表统计（stats/table_statistics）：选择统计量绘制各特征条形图。 */
const TableStatsVis: React.FC<TabVisProps> = (props) => {
  const flat = tabToFlatTable(props.tab, props.codeName);
  const numericCols = flat.columns
    .map((c, i) => ({ ...c, i }))
    .filter((c) => c.i > 0 && flat.rows.some((r) => typeof r[c.i] === 'number'));
  const [metric, setMetric] = useState<number>(numericCols[0]?.i ?? -1);
  const data = metric >= 0 ? flat.rows.map((r) => ({ label: String(r[0]), value: Number(r[metric]) })).filter((d) => !Number.isNaN(d.value)) : [];
  return (
    <div className="space-y-2">
      {numericCols.length > 0 && (
        <select className="text-[11px] bg-transparent border border-gray-500/30 rounded px-1 py-0.5" value={metric} onChange={(e) => setMetric(Number(e.target.value))}>
          {numericCols.map((c) => (
            <option key={c.i} value={c.i}>
              {c.name}
            </option>
          ))}
        </select>
      )}
      {data.length > 0 && (
        <ChartFrame labels={props.labels}>
          <HBarChart data={data.slice(0, 40)} />
        </ChartFrame>
      )}
      <DefaultTabView {...props} />
    </div>
  );
};

/** groupby 透视表（stats/groupby_statistics）：分组结果表 / 交叉透视表切换。 */
export const GroupByPivotVis: React.FC<ReportVisProps> = ({ tabs, codeName, id, labels, locale }) => {
  const [mode, setMode] = useState<'group' | 'cross'>('group');
  return (
    <div className="space-y-2">
      <div className="inline-flex rounded border border-gray-500/30 overflow-hidden text-[11px]">
        {(['group', 'cross'] as const).map((m) => (
          <button key={m} type="button" onClick={() => setMode(m)} className={`px-2 py-0.5 ${mode === m ? 'bg-blue-500 text-white' : ''}`}>
            {m === 'group' ? labels.groupTable : labels.crossTable}
          </button>
        ))}
      </div>
      <SimpleTabs
        items={tabs.map((tab, i) => ({
          key: String(i),
          label: getTabName(tab.name, i, locale),
          content:
            mode === 'group' ? (
              <DefaultTabView tab={tab} tabs={tabs} codeName={codeName} id={id} labels={labels} />
            ) : (
              <PivotTable flat={tabToFlatTable(tab, codeName)} labels={labels} />
            ),
        }))}
      />
    </div>
  );
};

type PivotZone = keyof PivotConfig | 'fields';
const PIVOT_ZONES: PivotZone[] = ['rows', 'columns', 'values', 'fields'];

/** 把列 idx 移到目标区域（从原区域移除）；fields 即「不使用」。 */
export function movePivotField(cfg: PivotConfig, idx: number, to: PivotZone): PivotConfig {
  const next: PivotConfig = {
    rows: cfg.rows.filter((i) => i !== idx),
    columns: cfg.columns.filter((i) => i !== idx),
    values: cfg.values.filter((i) => i !== idx),
  };
  if (to !== 'fields') next[to] = [...next[to], idx];
  return next;
}

/**
 * 交叉透视（旧版 S2 PivotSheet + Switcher 的轻量实现）：字段可在「行 / 列 / 值 / 待选」
 * 之间拖拽（HTML5 DnD），每个字段也有下拉可切换区域（键盘可达）。首列行名不参与。
 */
export const PivotTable: React.FC<{ flat: FlatTable; labels: Required<ResultViewLabels> }> = ({ flat, labels }) => {
  const [cfg, setCfg] = useState<PivotConfig>(() => defaultPivotConfig(flat));
  const [over, setOver] = useState<PivotZone | null>(null);
  const skipFirst = flat.kind === 'table' && flat.columns[0]?.name === 'name';
  const all = flat.columns.map((c, i) => ({ ...c, i })).slice(skipFirst ? 1 : 0);
  const zoneOf = (i: number): PivotZone =>
    cfg.rows.includes(i) ? 'rows' : cfg.columns.includes(i) ? 'columns' : cfg.values.includes(i) ? 'values' : 'fields';
  const zoneLabel: Record<PivotZone, string> = {
    rows: labels.pivotRows,
    columns: labels.pivotColumns,
    values: labels.pivotValues,
    fields: labels.pivotFields,
  };
  const pivot = useMemo(() => (cfg.values.length > 0 ? pivotFlatTable(flat, cfg) : null), [flat, cfg]);
  const fieldsIn = (z: PivotZone) =>
    z === 'fields' ? all.filter((c) => zoneOf(c.i) === 'fields') : cfg[z].map((i) => all.find((c) => c.i === i)).filter((c): c is (typeof all)[number] => !!c);
  return (
    <div className="space-y-2">
      <div className={`text-[10px] ${muted}`}>{labels.pivotDragHint}</div>
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-2" data-testid="pivot-config">
        {PIVOT_ZONES.map((z) => (
          <div
            key={z}
            data-testid={`pivot-zone-${z}`}
            onDragOver={(e) => {
              e.preventDefault();
              setOver(z);
            }}
            onDragLeave={() => setOver((o) => (o === z ? null : o))}
            onDrop={(e) => {
              e.preventDefault();
              setOver(null);
              const idx = Number(e.dataTransfer.getData('text/plain'));
              if (!Number.isNaN(idx)) setCfg((c) => movePivotField(c, idx, z));
            }}
            className={`${box} p-1.5 min-h-[52px] space-y-1 ${over === z ? 'bg-blue-500/10 border-blue-500/60' : ''}`}
          >
            <div className="text-[10px] font-semibold text-gray-500">{zoneLabel[z]}</div>
            <div className="flex flex-wrap gap-1">
              {fieldsIn(z).map((c) => (
                <span
                  key={c.i}
                  draggable
                  onDragStart={(e) => e.dataTransfer.setData('text/plain', String(c.i))}
                  className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded bg-gray-500/10 text-[11px] cursor-move"
                >
                  {c.name}
                  <select
                    aria-label={`${c.name} ${labels.pivotFields}`}
                    className="bg-transparent text-[10px] text-gray-500"
                    value={z}
                    onChange={(e) => setCfg((cur) => movePivotField(cur, c.i, e.target.value as PivotZone))}
                  >
                    {PIVOT_ZONES.map((o) => (
                      <option key={o} value={o}>
                        {zoneLabel[o]}
                      </option>
                    ))}
                  </select>
                </span>
              ))}
            </div>
          </div>
        ))}
      </div>
      {pivot ? (
        <SortableTable columns={pivot.columns} rows={pivot.rows} labels={labels} csvName="pivot" />
      ) : (
        <SortableTable columns={flat.columns} rows={flat.rows} labels={labels} />
      )}
    </div>
  );
};

/** 回归评估（ml.eval/regression_eval）：首 Tab 指标表，其余为残差直方图。 */
export const RegressionVis: React.FC<ReportVisProps> = ({ tabs, codeName, id, labels, locale }) => (
  <SimpleTabs
    items={tabs.map((tab, i) => ({
      key: String(i),
      label: getTabName(tab.name, i, locale),
      content:
        i === 0 ? (
          <DefaultTabView tab={tab} tabs={tabs} codeName={codeName} id={id} labels={labels} />
        ) : (
          <ChartFrame
            labels={labels}
            extra={
              <button type="button" className={linkBtn} onClick={() => downloadText(`${id}-${i}.csv`, toCsv([['interval', 'count'], ...tabToHistogram(tab).map((d) => [d.name, d.count])]))}>
                ⬇ {labels.exportData}
              </button>
            }
          >
            <ColumnChart data={tabToHistogram(tab).map((d) => ({ label: d.name, value: d.count }))} />
          </ChartFrame>
        ),
    }))}
  />
);

/** 特征重要性（ml.train/sgb_train 报告：每个 tab 一种 importance 类型）：条形图 + 明细表。 */
export const FeatureImportanceVis: React.FC<TabVisProps> = (props) => {
  const data = featureImportanceSeries(tabToFlatTable(props.tab, props.codeName), { allowDescriptions: true });
  return (
    <div className="space-y-2">
      {data && (
        <ChartFrame title={props.labels.featureImportance} labels={props.labels}>
          <HBarChart data={data} color={PALETTE[4]} />
        </ChartFrame>
      )}
      <DefaultTabView {...props} />
    </div>
  );
};

/** 数据集采样（data_filter/sample 分层采样报告）：汇总表 + 采样率条形图 + 各分层明细，CSV 与旧版一致。 */
export const SampleVis: React.FC<ReportVisProps> = ({ tabs, codeName, id, labels, locale }) => {
  const summary = sampleSummary(tabs);
  const csv = sampleSummaryCsv(summary);
  return (
    <div className="space-y-3">
      <ChartFrame title={labels.sampleSummary} labels={labels}>
        <HBarChart data={summary.map((r) => ({ label: r.name, value: r.sampleRate }))} color={PALETTE[5]} />
        <SortableTable columns={csv[0].map((name, i) => ({ name: String(name), type: i === 0 ? 'str' : 'float' }))} rows={csv.slice(1)} csvName={`${id}-sample`} labels={labels} />
      </ChartFrame>
      <SimpleTabs
        items={tabs.map((tab, i) => ({
          key: String(i),
          label: getTabName(tab.name, i, locale),
          content: <DefaultTabView tab={tab} tabs={tabs} codeName={codeName} id={id} labels={labels} />,
        }))}
      />
    </div>
  );
};

registerResultVisualization('stats/ss_pearsonr', 'tab', (p) => <CorrMatrixVis {...p} />);
registerResultVisualization('ml.train/sgb_train', 'tab', (p) => <FeatureImportanceVis {...p} />);
registerResultVisualization('data_filter/sample', 'report', (p) => <SampleVis {...p} />);
registerResultVisualization('ml.eval/prediction_bias_eval', 'tab', (p) => <PvaVis {...p} />);
registerResultVisualization('stats/stats_psi', 'tab', (p) => <PsiVis {...p} />);
registerResultVisualization('stats/table_statistics', 'tab', (p) => <TableStatsVis {...p} />);
['feature/vert_woe_binning', 'feature/vert_binning', 'feature/binning', 'feature/binning_modifications'].forEach((c) =>
  registerResultVisualization(c, 'tab', (p) => <BinningVis {...p} />),
);
registerResultVisualization('stats/groupby_statistics', 'report', (p) => <GroupByPivotVis {...p} />);
registerResultVisualization('ml.eval/regression_eval', 'report', (p) => <RegressionVis {...p} />);

/* -------------------------------------------------------------------------- */
/* 各类型结果                                                                   */
/* -------------------------------------------------------------------------- */

export const ReportResult: React.FC<{ output: NormalizedOutput; id: string; codeName: string; labels: Required<ResultViewLabels>; locale: 'zh' | 'en' }> = ({
  output,
  id,
  codeName,
  labels,
  locale,
}) => {
  const tabs = output.tabs;
  const reportVis = REPORT_VIS.get(codeName);
  const tabVis = TAB_VIS.get(codeName);
  return (
    <div className="space-y-3">
      <ResultHeader id={id} tag={labels.report} gmtCreate={output.gmtCreate} labels={labels} />
      {output.warnings.length > 0 && (
        <details className="rounded bg-amber-500/10 border border-amber-500/30 px-2 py-1 text-[11px]">
          <summary className="cursor-pointer text-amber-600">
            {labels.warning} ({output.warnings.length})
          </summary>
          <ul className="list-disc pl-4 mt-1 space-y-0.5">
            {output.warnings.map((w, i) => (
              <li key={i}>{w}</li>
            ))}
          </ul>
        </details>
      )}
      {tabs.length === 0 ? (
        <div className={muted}>{labels.noResult}</div>
      ) : (
        <ChartFrame labels={labels}>
          {reportVis ? (
            reportVis({ tabs, codeName, id, labels, locale })
          ) : (
            <SimpleTabs
              items={tabs.map((tab, i) => ({
                key: String(i),
                label: getTabName(tab.name, i, locale),
                content: tabVis ? tabVis({ tab, tabs, codeName, id, labels }) : <DefaultTabView tab={tab} tabs={tabs} codeName={codeName} id={id} labels={labels} />,
              }))}
            />
          )}
        </ChartFrame>
      )}
    </div>
  );
};

interface RowActionsProps {
  row: OutputRow;
  output: NormalizedOutput;
  codeName: string;
  downloadMode: 'direct' | 'tee' | 'none';
  actions?: ResultActions;
  labels: Required<ResultViewLabels>;
  allowDownload: boolean;
}

const RowActions: React.FC<RowActionsProps> = ({ row, output, codeName, downloadMode, actions, labels, allowDownload }) => {
  const disabledReason = downloadDisabledReason(row.datasourceType, row.path);
  return (
    <span className="inline-flex gap-2 ml-2">
      {row.nodeType === 'embedded' && codeName !== 'read_data/datatable' && actions?.onViewNodeResult && (
        <button type="button" className={linkBtn} onClick={() => actions.onViewNodeResult?.(row, output)}>
          {labels.viewResult}
        </button>
      )}
      {allowDownload && actions?.onPreview && (
        <button type="button" className={linkBtn} onClick={() => actions.onPreview?.(row, output)}>
          {labels.preview}
        </button>
      )}
      {allowDownload && downloadMode === 'tee' && actions?.onApplyTeeDownload && actions.canApplyTeeDownload?.(output) !== false && (
        <button type="button" className={linkBtn} onClick={() => actions.onApplyTeeDownload?.(row, output)}>
          {labels.applyDownload}
        </button>
      )}
      {allowDownload && downloadMode === 'direct' && actions?.onDownload && (
        <button type="button" className={linkBtn} disabled={!!disabledReason} title={disabledReason} onClick={() => actions.onDownload?.(row, output)}>
          {labels.download}
        </button>
      )}
    </span>
  );
};

export const TableResult: React.FC<{
  output: NormalizedOutput;
  id: string;
  codeName: string;
  labels: Required<ResultViewLabels>;
  downloadMode: 'direct' | 'tee' | 'none';
  actions?: ResultActions;
}> = ({ output, id, codeName, labels, downloadMode, actions }) => {
  const [search, setSearch] = useState('');
  const [typeFilter, setTypeFilter] = useState('');
  const [nodeFilter, setNodeFilter] = useState('');
  const schema = useMemo(
    () =>
      output.rows.flatMap((r, ri) =>
        r.fields.map((f, i) => ({ field: f, fieldType: r.fieldTypes[i] ?? '', nodeId: r.nodeId ?? r.nodeName ?? `#${ri}` })),
      ),
    [output.rows],
  );
  const types = [...new Set(schema.map((s) => s.fieldType))].filter(Boolean);
  const nodes = [...new Set(schema.map((s) => s.nodeId))];
  const filtered = schema.filter(
    (s) =>
      (!search || s.field.toLowerCase().includes(search.toLowerCase())) &&
      (!typeFilter || s.fieldType === typeFilter) &&
      (!nodeFilter || s.nodeId === nodeFilter),
  );
  const inputCls = 'text-[11px] bg-transparent border border-gray-500/30 rounded px-1 py-0.5';
  return (
    <div className="space-y-3">
      <ResultHeader id={id} tag={labels.table} gmtCreate={output.gmtCreate} labels={labels} />
      <div className="space-y-1 text-[11px]">
        {output.rows.map((row, i) => (
          <div key={`${row.nodeId}-${i}`} className="break-all">
            <span className={muted}>
              {row.nodeName || row.nodeId || ''}
              {labels.path}
            </span>
            <span>{row.path || row.tableId || row.domainDataId || '-'}</span>
            <RowActions row={row} output={output} codeName={codeName} downloadMode={downloadMode} actions={actions} labels={labels} allowDownload={codeName !== 'read_data/datatable'} />
          </div>
        ))}
      </div>
      <ChartFrame labels={labels}>
        {(full) => (
          <div className="space-y-2">
            <div className="flex flex-wrap items-center gap-2">
              <input className={inputCls} placeholder={`${labels.search} ${labels.field}`} value={search} onChange={(e) => setSearch(e.target.value)} />
              <select className={inputCls} value={typeFilter} onChange={(e) => setTypeFilter(e.target.value)} aria-label={labels.fieldType}>
                <option value="">{labels.fieldType}</option>
                {types.map((t) => (
                  <option key={t} value={t}>
                    {t}
                  </option>
                ))}
              </select>
              <select className={inputCls} value={nodeFilter} onChange={(e) => setNodeFilter(e.target.value)} aria-label={labels.node}>
                <option value="">{labels.node}</option>
                {nodes.map((n) => (
                  <option key={n} value={n}>
                    {n}
                  </option>
                ))}
              </select>
              <button
                type="button"
                className={`${linkBtn} text-[11px] ml-auto`}
                onClick={() => downloadText(`${id}.csv`, toCsv([['field', 'fieldType', 'nodeId'], ...schema.map((s) => [s.field, s.fieldType, s.nodeId])]))}
              >
                ⬇ {labels.exportSchema}
              </button>
            </div>
            <div className={`${box} overflow-auto ${full ? '' : 'max-h-80'}`}>
              <table className="w-full text-[11px]">
                <thead>
                  <tr className="bg-gray-500/10 text-left">
                    <th className="px-2 py-1">{labels.field}</th>
                    <th className="px-2 py-1">{labels.fieldType}</th>
                    <th className="px-2 py-1">{labels.node}</th>
                  </tr>
                </thead>
                <tbody>
                  {filtered.map((s, i) => (
                    <tr key={`${s.nodeId}-${s.field}-${i}`} className="border-t border-gray-500/20">
                      <td className="px-2 py-1">{s.field}</td>
                      <td className="px-2 py-1">{(s.fieldType || '').toLowerCase() || ' - - '}</td>
                      <td className="px-2 py-1">{s.nodeId}</td>
                    </tr>
                  ))}
                  {filtered.length === 0 && (
                    <tr>
                      <td colSpan={3} className={`px-2 py-3 text-center ${muted}`}>
                        {labels.noResult}
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>
          </div>
        )}
      </ChartFrame>
    </div>
  );
};

export const PathResult: React.FC<{
  output: NormalizedOutput;
  id: string;
  codeName: string;
  tag: string;
  labels: Required<ResultViewLabels>;
  downloadMode: 'direct' | 'tee' | 'none';
  actions?: ResultActions;
}> = ({ output, id, codeName, tag, labels, downloadMode, actions }) => {
  // 读模型产出的模型不允许下载（旧版 isReadModal）。
  const allowDownload = codeName !== 'ml.predict/read_model';
  return (
    <div className="space-y-3">
      <ResultHeader id={id} tag={tag} gmtCreate={output.gmtCreate} labels={labels} />
      {output.rows.length === 0 && <div className={muted}>{labels.noResult}</div>}
      {output.rows.map((row, i) => (
        <div key={`${row.path}-${i}`} className={`${box} p-2 text-[11px] break-all`}>
          <span className={muted}>
            {row.nodeName || row.nodeId || ''}
            {labels.path}
          </span>
          <span>{row.path || row.tableId || '-'}</span>
          {row.path && (
            <span className="ml-2">
              <CopyText text={row.path} labels={labels} />
            </span>
          )}
          <RowActions row={row} output={output} codeName={codeName} downloadMode={downloadMode} actions={actions} labels={labels} allowDownload={allowDownload} />
        </div>
      ))}
    </div>
  );
};

/**
 * 结果入口：按 DistData 类型分派。无 type 时提示“非数据参与方”，scql 分析额外提示容器地址。
 */
export const NodeResultView: React.FC<NodeResultViewProps> = ({ output, outputId, codeName, downloadMode = 'none', actions, labels: labelsProp, locale = 'zh' }) => {
  const labels = { ...ZH_LABELS, ...(labelsProp || {}) } as Required<ResultViewLabels>;
  const normalized = useMemo(() => (isNormalized(output) ? output : normalizeOutput(output)), [output]);
  if (!normalized) return <div className={muted}>{labels.noResult}</div>;
  const cn = codeName || normalized.codeName || '';
  const id = outputId || '';
  switch (normalized.kind) {
    case 'report':
      return <ReportResult output={normalized} id={id} codeName={cn} labels={labels} locale={locale} />;
    case 'table':
      return <TableResult output={normalized} id={id} codeName={cn} labels={labels} downloadMode={downloadMode} actions={actions} />;
    case 'model':
    case 'serving':
      return <PathResult output={normalized} id={id} codeName={cn} tag={labels.model} labels={labels} downloadMode={downloadMode} actions={actions} />;
    case 'rule':
      return <PathResult output={normalized} id={id} codeName={cn} tag={labels.rule} labels={labels} downloadMode={downloadMode} actions={actions} />;
    case 'read_data':
      return <TableResult output={normalized} id={id} codeName="read_data/datatable" labels={labels} downloadMode="none" actions={actions} />;
    default:
      return (
        <div className="space-y-1 text-[11px]">
          <div>{labels.noParty}</div>
          {cn === 'stats/scql_analysis' && (
            <>
              <div>自定义scql分析组件请到接收方KUSCIA容器中查数据</div>
              <span className="break-all">{`地址：${normalized.jobId ?? ''}-${id}`}</span>
            </>
          )}
        </div>
      );
  }
};

function isNormalized(v: unknown): v is NormalizedOutput {
  return !!v && typeof v === 'object' && 'kind' in (v as object) && 'raw' in (v as object) && Array.isArray((v as NormalizedOutput).tabs);
}

/** 独立渲染报告 Tabs（result-details 中使用）。 */
export const ReportTabsView: React.FC<{ tabs: ReportTab[]; codeName: string; id: string; labels?: ResultViewLabels; locale?: 'zh' | 'en' }> = ({
  tabs,
  codeName,
  id,
  labels: labelsProp,
  locale = 'zh',
}) => {
  const labels = { ...ZH_LABELS, ...(labelsProp || {}) } as Required<ResultViewLabels>;
  const reportVis = REPORT_VIS.get(codeName);
  const tabVis = TAB_VIS.get(codeName);
  return (
    <ChartFrame labels={labels}>
      {reportVis ? (
        reportVis({ tabs, codeName, id, labels, locale })
      ) : (
        <SimpleTabs
          items={tabs.map((tab, i) => ({
            key: String(i),
            label: getTabName(tab.name, i, locale),
            content: tabVis ? tabVis({ tab, tabs, codeName, id, labels }) : <DefaultTabView tab={tab} tabs={tabs} codeName={codeName} id={id} labels={labels} />,
          }))}
        />
      )}
    </ChartFrame>
  );
};
