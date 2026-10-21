/**
 * `sf.report` 解析工具（对应旧版 dag-result/result-report-types.ts + vis/utils.ts）。
 *
 * Report proto（secretflow_spec v1 report.proto）：
 *   Report { name, desc, tabs: Tab[] }
 *   Tab { name, desc, divs: Div[] }
 *   Div { name, desc, children: Div.Child[] }
 *   Div.Child { type: 'descriptions'|'table'|'div', descriptions?, table?, div? }
 *   Descriptions { name, desc, items: { name, desc, type, value: Attribute }[] }
 *   Table { name, desc, headers: { name, desc, type }[], rows: { name, desc, items: Attribute[] }[] }
 *
 * Value(Attribute) 编码：f / i64 / s / b / fs / i64s / ss / bs（i64 在 JSON 中为字符串）。
 */

export type ReportValueType =
  | 'float'
  | 'int'
  | 'bool'
  | 'str'
  | 'AT_FLOAT'
  | 'AT_INT'
  | 'AT_STRING'
  | 'AT_BOOL'
  | 'AT_FLOATS'
  | 'AT_INTS'
  | 'AT_STRINGS'
  | 'AT_BOOLS'
  | string;

export interface ReportValue {
  f?: number | string;
  i64?: number | string;
  s?: string;
  b?: boolean;
  fs?: Array<number | string>;
  i64s?: Array<number | string>;
  ss?: string[];
  bs?: boolean[];
}

export interface ReportDescriptionItem {
  name: string;
  desc?: string;
  type: ReportValueType;
  value: ReportValue;
}

export interface ReportDescriptions {
  name?: string;
  desc?: string;
  items: ReportDescriptionItem[];
}

export interface ReportTableHeader {
  name: string;
  desc?: string;
  type: ReportValueType;
}

export interface ReportTable {
  name?: string;
  desc?: string;
  headers: ReportTableHeader[];
  rows: Array<{ name?: string; desc?: string; items: ReportValue[] }>;
}

export type ReportDivChild =
  | { type: 'descriptions'; descriptions: ReportDescriptions }
  | { type: 'table'; table: ReportTable }
  | { type: 'div'; div: ReportDiv };

export interface ReportDiv {
  name?: string;
  desc?: string;
  children: ReportDivChild[];
}

export interface ReportTab {
  name?: string;
  desc?: string;
  divs: ReportDiv[];
}

/** 类型 → Attribute 字段（旧版 TypeMap，扩展列表类型）。 */
export const VALUE_KEY_BY_TYPE: Record<string, keyof ReportValue> = {
  float: 'f',
  int: 'i64',
  bool: 'b',
  str: 's',
  AT_FLOAT: 'f',
  AT_INT: 'i64',
  AT_STRING: 's',
  AT_BOOL: 'b',
  AT_FLOATS: 'fs',
  AT_INTS: 'i64s',
  AT_STRINGS: 'ss',
  AT_BOOLS: 'bs',
};

export type CellValue = string | number | boolean;

/**
 * 解码单个 Value：优先按声明类型取字段，取不到时按字段存在性兜底。
 * i64 字符串会转为数字（超出安全整数时保留字符串）。
 */
export function decodeValue(value: ReportValue | undefined, type?: ReportValueType): CellValue | CellValue[] | undefined {
  if (!value || typeof value !== 'object') return undefined;
  const key = type ? VALUE_KEY_BY_TYPE[type] : undefined;
  const toNumber = (v: number | string | undefined): number | string | undefined => {
    if (v === undefined || v === null) return undefined;
    if (typeof v === 'number') return v;
    const n = Number(v);
    if (Number.isNaN(n)) return v;
    return Number.isSafeInteger(n) || !/^-?\d+$/.test(v) ? n : v;
  };
  const read = (k: keyof ReportValue): CellValue | CellValue[] | undefined => {
    const raw = value[k];
    if (raw === undefined || raw === null) return undefined;
    switch (k) {
      case 'f':
      case 'i64':
        return toNumber(raw as number | string);
      case 'fs':
      case 'i64s':
        return (raw as Array<number | string>).map((x) => toNumber(x) ?? '');
      default:
        return raw as CellValue | CellValue[];
    }
  };
  if (key) {
    const v = read(key);
    if (v !== undefined) return v;
    // 部分组件把 int 写在 f 字段（或反之），按类型族兜底。
  }
  for (const k of ['s', 'f', 'i64', 'b', 'ss', 'fs', 'i64s', 'bs'] as Array<keyof ReportValue>) {
    const v = read(k);
    if (v !== undefined) return v;
  }
  return undefined;
}

export function formatCell(v: CellValue | CellValue[] | undefined): string {
  if (v === undefined || v === null) return '-';
  if (Array.isArray(v)) return v.map(String).join(', ');
  return String(v);
}

/** Tab 名映射（旧版 getTabsName）。 */
const TAB_NAME_ZH: Record<string, string> = {
  SummaryReport: '总评估表',
  eq_frequent_bin_report: '等频',
  eq_range_bin_report: '等宽',
  head_report: '头表',
  eq_range_bin_tbl: '等宽',
};

export function getTabName(name: string | undefined, index: number, locale: 'zh' | 'en' = 'zh'): string {
  if (!name) return locale === 'zh' ? `输出表${index + 1}` : `Table ${index + 1}`;
  return locale === 'zh' ? TAB_NAME_ZH[name] || name : name;
}

/**
 * 解析 tabs：后端可能返回
 * - 数组（Java：`tabs: Tab[]`）；
 * - JSON 字符串；
 * - 完整 Report 对象 `{ name, tabs }`；
 * - DistData `{ meta: { tabs } }`。
 */
export function parseReportTabs(raw: unknown): ReportTab[] {
  let data: unknown = raw;
  if (typeof data === 'string') {
    try {
      data = JSON.parse(data);
    } catch {
      return [];
    }
  }
  if (Array.isArray(data)) {
    return data
      .map((t) => (typeof t === 'string' ? safeParse(t) : t))
      .filter((t): t is ReportTab => !!t && typeof t === 'object' && Array.isArray((t as ReportTab).divs));
  }
  if (data && typeof data === 'object') {
    const obj = data as Record<string, unknown>;
    if (Array.isArray(obj.tabs)) return parseReportTabs(obj.tabs);
    if (obj.meta && typeof obj.meta === 'object') return parseReportTabs((obj.meta as Record<string, unknown>).tabs);
  }
  return [];
}

function safeParse(s: string): unknown {
  try {
    return JSON.parse(s);
  } catch {
    return null;
  }
}

/** 扁平化后的表格。 */
export interface FlatTable {
  kind: 'table' | 'descriptions';
  columns: Array<{ name: string; type: string }>;
  rows: CellValue[][];
}

/**
 * 旧版 modifyDataStructure：取 tab 的首个 div 首个 child 决定形态：
 * - descriptions：汇总所有 div 的 descriptions.items → 两列（name / value）；
 * - table：首列补 `name`（行名），scql_analysis 不补。
 */
export function tabToFlatTable(tab: ReportTab, codeName?: string): FlatTable {
  const first = tab?.divs?.[0]?.children?.[0];
  if (first?.type === 'descriptions') {
    const items: ReportDescriptionItem[] = [];
    (tab.divs || []).forEach((div) =>
      (div.children || []).forEach((c) => {
        if (c.type === 'descriptions' && c.descriptions?.items) items.push(...c.descriptions.items);
      }),
    );
    return {
      kind: 'descriptions',
      columns: [
        { name: 'name', type: 'str' },
        { name: 'value', type: 'str' },
      ],
      rows: items.map((it) => [it.name, scalarize(decodeValue(it.value, it.type))]),
    };
  }
  if (first?.type === 'table') {
    const t = first.table;
    const withName = codeName !== 'stats/scql_analysis';
    const columns = [...(withName ? [{ name: 'name', type: 'str' }] : []), ...(t.headers || []).map((h) => ({ name: h.name, type: String(h.type) }))];
    const rows = (t.rows || []).map((row, i) => {
      const cells = (row.items || []).map((item, idx) => scalarize(decodeValue(item, t.headers?.[idx]?.type)));
      return withName ? [row.name ?? String(i), ...cells] : cells;
    });
    return { kind: 'table', columns, rows };
  }
  return { kind: 'table', columns: [], rows: [] };
}

function scalarize(v: CellValue | CellValue[] | undefined): CellValue {
  if (v === undefined) return '-';
  if (Array.isArray(v)) return v.map(String).join(', ');
  return v;
}

/** 相关系数矩阵：返回 [行名, 列名, 值] 记录（旧版 transformCorrMatrixData）。 */
export function tabToCorrMatrix(tab: ReportTab): { labels: string[]; matrix: number[][] } {
  const first = tab?.divs?.[0]?.children?.[0];
  if (!first || first.type !== 'table') return { labels: [], matrix: [] };
  const headers = first.table.headers || [];
  const colLabels = headers.map((h) => h.name);
  const rowLabels = (first.table.rows || []).map((r, i) => r.name ?? colLabels[i] ?? String(i));
  const matrix = (first.table.rows || []).map((r) =>
    (r.items || []).map((item, idx) => {
      const v = decodeValue(item, headers[idx]?.type);
      const n = typeof v === 'number' ? v : Number(v);
      return Number.isNaN(n) ? 0 : n;
    }),
  );
  const labels = rowLabels.length === colLabels.length ? colLabels : rowLabels;
  return { labels, matrix };
}

/** PVA（prediction_bias_eval）：按 interval 取 avg_prediction / avg_label / bias 三条折线。 */
export function tabToPvaSeries(tab: ReportTab): Array<{ label: string; avg_prediction: number; avg_label: number; bias: number }> {
  const flat = tabToFlatTable(tab);
  if (flat.kind !== 'table') return [];
  const idx = (n: string) => flat.columns.findIndex((c) => c.name === n);
  const iInterval = idx('interval');
  const iPred = idx('avg_prediction');
  const iLabel = idx('avg_label');
  const iBias = idx('bias');
  return flat.rows.map((r, i) => ({
    label: String(iInterval >= 0 ? r[iInterval] : r[0] ?? i),
    avg_prediction: Number(iPred >= 0 ? r[iPred] : NaN),
    avg_label: Number(iLabel >= 0 ? r[iLabel] : NaN),
    bias: Number(iBias >= 0 ? r[iBias] : NaN),
  }));
}

/** 回归评估残差直方图：第 1 行为分箱边界，第 2 行为计数（旧版 transformRegressionData）。 */
export function tabToHistogram(tab: ReportTab): Array<{ name: string; count: number }> {
  const first = tab?.divs?.[0]?.children?.[0];
  if (!first || first.type !== 'table') return [];
  const { headers = [], rows = [] } = first.table;
  if (rows.length < 2) return [];
  const bounds = rows[0].items || [];
  const counts = rows[1].items || [];
  const out: Array<{ name: string; count: number }> = [];
  bounds.forEach((item, index) => {
    if (index === 0) return;
    const type = headers[index]?.type;
    const left = formatCell(decodeValue(bounds[index - 1], type));
    const right = formatCell(decodeValue(item, type));
    const c = decodeValue(counts[index - 1], type);
    out.push({ name: `[${left},${right})`, count: Number(c) || 0 });
  });
  return out;
}

/** 字典序比较（旧版 lexicographicalOrder：字母前缀 + 数字后缀自然排序）。 */
export function lexicographicalOrder(a: string, b: string): number {
  const parse = (item: string): [string, number] => {
    const m = /(^[a-zA-Z]*)(\d*)$/.exec(item) || [];
    return [m[1] ?? '', Number(m[2] ?? 0)];
  };
  const [sa, na] = parse(String(a));
  const [sb, nb] = parse(String(b));
  if (!sa && !sb) return String(a).toLowerCase().localeCompare(String(b).toLowerCase());
  const cmp = sa.toLowerCase().localeCompare(sb.toLowerCase());
  return cmp === 0 ? na - nb : cmp;
}

/** 差分隐私 count 查询结果取整（旧版 formatDpResult）。 */
export function formatDpCell(value: CellValue, row: Record<string, CellValue>, codeName?: string): CellValue {
  if (codeName === 'privacy/differential_privacy' && row.query_type === 'count') {
    const n = typeof value === 'number' ? value : Number(value);
    if (!Number.isNaN(n)) return Math.round(n);
  }
  return value;
}

/** stats_psi 全量 CSV：汇总表每行后附上对应特征明细（旧版 getFullCsvDataForStatsPSI）。 */
export function statsPsiFullCsv(tabs: ReportTab[]): CellValue[][] {
  if (tabs.length === 0) return [];
  const detail = new Map<string, FlatTable>();
  tabs.slice(1).forEach((t) => detail.set(t.name ?? '', tabToFlatTable(t)));
  const out: CellValue[][] = [[
    'name (feature)',
    'feature',
    'PSI (feature)',
    'name (Label)',
    'Label',
    'PSI (Label)',
    'Base Ratio',
    'Test Ratio',
  ]];
  const summary = tabToFlatTable(tabs[0]);
  summary.rows.forEach((row) => {
    out.push(row);
    const key = String(row[1]);
    (detail.get(key)?.rows || []).forEach((r) => out.push(['', '', '', ...r]));
  });
  return out;
}

/**
 * 特征重要性序列：表含 importance 列时取该列（标签取 feature / name 列）；
 * `allowDescriptions` 时，descriptions 形态（sgb_train 报告：每个 tab 一种
 * importance 类型，items 为「特征 → 权重」）直接按 name / value 取值。按值降序取前 limit。
 */
export function featureImportanceSeries(
  flat: FlatTable,
  opts: { allowDescriptions?: boolean; limit?: number } = {},
): Array<{ label: string; value: number }> | null {
  const cols = flat.columns.map((c) => c.name.toLowerCase());
  let vi = cols.findIndex((c) => c.includes('importance'));
  let li = cols.findIndex((c, i) => i !== vi && (c.includes('feature') || c === 'name'));
  if (vi < 0) {
    if (!opts.allowDescriptions || flat.kind !== 'descriptions') return null;
    li = 0;
    vi = 1;
  }
  const data = flat.rows
    .map((r) => ({ label: String(r[li >= 0 ? li : 0]), value: Number(r[vi]) }))
    .filter((d) => !Number.isNaN(d.value))
    .sort((a, b) => b.value - a.value)
    .slice(0, opts.limit ?? 30);
  return data.length ? data : null;
}

/** 数据集采样（data_filter/sample，分层采样）每个分层 tab 的汇总行。 */
export interface SampleSummaryRow {
  name: string;
  numBefore: number;
  numAfter: number;
  sampleRate: number;
}

/** 汇总各 tab 的 num_before_sample / num_after_sample / sample_rate（旧版 getFullCsvDataForSample）。 */
export function sampleSummary(tabs: ReportTab[]): SampleSummaryRow[] {
  return tabs.map((tab, i) => {
    const flat = tabToFlatTable(tab);
    const get = (k: string) => {
      const row = flat.kind === 'descriptions' ? flat.rows.find((r) => r[0] === k) : undefined;
      const n = Number(row?.[1]);
      return Number.isNaN(n) ? 0 : n;
    };
    return { name: tab.name || String(i), numBefore: get('num_before_sample'), numAfter: get('num_after_sample'), sampleRate: get('sample_rate') };
  });
}

/** 采样汇总 CSV（表头与旧版一致）。 */
export function sampleSummaryCsv(rows: SampleSummaryRow[]): CellValue[][] {
  return [['sample_name', 'num_before_sample', 'num_after_sample', 'sample_rate'], ...rows.map((r) => [r.name, r.numBefore, r.numAfter, r.sampleRate])];
}

/** 透视配置：各区域放置的列下标（FlatTable.columns 下标）。 */
export interface PivotConfig {
  rows: number[];
  columns: number[];
  values: number[];
}

/** 默认透视：字符串列前两个为行 / 列，其余第一个为值（与旧版 S2 默认一致）。忽略首列行名。 */
export function defaultPivotConfig(flat: FlatTable): PivotConfig {
  const idx = flat.columns.map((_, i) => i).slice(flat.kind === 'table' && flat.columns[0]?.name === 'name' ? 1 : 0);
  const dims = idx.filter((i) => flat.rows.some((r) => typeof r[i] === 'string'));
  const vals = idx.filter((i) => !dims.includes(i));
  return { rows: dims.slice(0, 1), columns: dims.slice(1, 2), values: vals.slice(0, 1) };
}

/**
 * 透视计算（旧版 S2 PivotSheet 的轻量实现）：行维度组合成行，列维度组合 × 值字段组合成列，
 * 同一格多条记录时数值求和（groupby 结果每个分组本就唯一）。
 */
export function pivotFlatTable(flat: FlatTable, cfg: PivotConfig): FlatTable {
  const name = (i: number) => flat.columns[i]?.name ?? String(i);
  const key = (r: CellValue[], dims: number[]) => dims.map((d) => String(r[d])).join(' / ');
  const rowKeys = [...new Set(flat.rows.map((r) => key(r, cfg.rows)))];
  const colKeys = cfg.columns.length ? [...new Set(flat.rows.map((r) => key(r, cfg.columns)))] : [''];
  const values = cfg.values.length ? cfg.values : [];
  const cells = new Map<string, CellValue>();
  flat.rows.forEach((r) => {
    const rk = key(r, cfg.rows);
    const ck = cfg.columns.length ? key(r, cfg.columns) : '';
    values.forEach((v) => {
      const k = `${rk}\u0000${ck}\u0000${v}`;
      const prev = cells.get(k);
      const cur = r[v];
      cells.set(k, typeof prev === 'number' && typeof cur === 'number' ? prev + cur : cur);
    });
  });
  const header = cfg.rows.length ? cfg.rows.map(name).join(' / ') : '';
  const cornerName = cfg.columns.length ? `${header} \\ ${cfg.columns.map(name).join(' / ')}` : header || '-';
  const columns = [
    { name: cornerName, type: 'str' },
    ...colKeys.flatMap((ck) =>
      values.map((v) => ({
        name: [ck, values.length > 1 || !ck ? name(v) : ''].filter(Boolean).join(' · '),
        type: flat.columns[v]?.type ?? 'float',
      })),
    ),
  ];
  const rows = rowKeys.map((rk) => [rk || '-', ...colKeys.flatMap((ck) => values.map((v) => cells.get(`${rk}\u0000${ck}\u0000${v}`) ?? '-'))]);
  return { kind: 'table', columns, rows };
}

export function toCsv(rows: CellValue[][]): string {
  return rows
    .map((r) =>
      r
        .map((c) => {
          const s = String(c ?? '').replace(/"/g, '');
          return /[",\n]/.test(s) ? `"${s}"` : s;
        })
        .join(','),
    )
    .join('\n');
}

export function downloadText(filename: string, text: string, mime = 'text/csv;charset=utf-8') {
  if (typeof document === 'undefined' || typeof URL === 'undefined' || !URL.createObjectURL) return;
  const blob = new Blob(['﻿', text], { type: mime });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

/** 格式化时间戳（旧版 formatTimestamp 把 UTC 转本地）。 */
export function formatTimestamp(ts?: string): string {
  if (!ts) return '';
  const d = new Date(ts);
  if (Number.isNaN(d.getTime())) return ts;
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}
