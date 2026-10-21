/**
 * 节点输出（graph/node/output、project/job/task/output、node result detail.output）规范化。
 *
 * Java 契约（GraphNodeOutputVO）：
 *   { type: 'table'|'model'|'rule'|'report', codeName, gmtCreate, jobId, taskId, graphID,
 *     meta: { headers, rows: OutputResult[] }, tabs: Tab[], warning?: string[] }
 *   OutputResult { nodeId, nodeName?, path, tableId, dsId, type('embedded'...), datasourceType,
 *                  fields: 'a,b', fieldTypes: 'int,str' }
 * 旧 Go 形态：
 *   { graph_node_id, outputs[], type: 'table'|..., meta: { rows: [{tableId, domainDataId}], columns: [{name,type}], datasourceId }, tabs }
 * 同时兼容 DistData 类型名（sf.table.* / sf.model.* / sf.rule.* / sf.report / sf.serving.model / sf.read_data）。
 */
import { parseReportTabs } from './report';
import type { ReportTab } from './report';

export type ResultKind = 'table' | 'model' | 'rule' | 'report' | 'serving' | 'read_data' | 'unknown';

export interface OutputRow {
  nodeId?: string;
  nodeName?: string;
  path?: string;
  tableId?: string;
  domainDataId?: string;
  dsId?: string;
  /** 节点类型（embedded 表示内置节点，可跳转查看结果）。 */
  nodeType?: string;
  datasourceType?: string;
  fields: string[];
  fieldTypes: string[];
}

export interface NormalizedOutput {
  kind: ResultKind;
  /** 原始类型字符串（可能是 DistData 类型名）。 */
  rawType?: string;
  codeName?: string;
  gmtCreate?: string;
  jobId?: string;
  taskId?: string;
  graphId?: string;
  rows: OutputRow[];
  tabs: ReportTab[];
  warnings: string[];
  raw: Record<string, unknown>;
}

export function resultKindOf(type?: string | null): ResultKind {
  if (!type) return 'unknown';
  const t = String(type).toLowerCase();
  if (t === 'table' || t.startsWith('sf.table')) return 'table';
  if (t === 'sf.serving.model' || t === 'serving_model' || t === 'serving') return 'serving';
  if (t === 'model' || t.startsWith('sf.model')) return 'model';
  if (t === 'rule' || t.startsWith('sf.rule')) return 'rule';
  if (t === 'report' || t === 'sf.report') return 'report';
  if (t === 'read_data' || t === 'sf.read_data') return 'read_data';
  return 'unknown';
}

function str(v: unknown): string | undefined {
  if (v === undefined || v === null || v === '') return undefined;
  return String(v);
}

function splitList(v: unknown): string[] {
  if (Array.isArray(v)) return v.map(String);
  if (typeof v === 'string') return v === '' ? [] : v.split(',');
  return [];
}

export function normalizeOutput(raw: unknown): NormalizedOutput | null {
  if (!raw || typeof raw !== 'object') return null;
  const r = raw as Record<string, unknown>;
  const meta = (r.meta && typeof r.meta === 'object' ? r.meta : {}) as Record<string, unknown>;
  const rawType = str(r.type) ?? str((r.outputs as Array<{ type?: string }> | undefined)?.[0]?.type);
  const tabs = parseReportTabs(r.tabs);
  let kind = resultKindOf(rawType);
  if (kind === 'unknown' && tabs.length > 0) kind = 'report';

  // Go 形态：columns 在 meta 顶层。
  const metaColumns = Array.isArray(meta.columns) ? (meta.columns as Array<{ name?: string; type?: string }>) : [];
  const rowsRaw = Array.isArray(meta.rows) ? (meta.rows as Array<Record<string, unknown>>) : [];
  const rows: OutputRow[] = rowsRaw.map((row) => {
    let fields = splitList(row.fields);
    let fieldTypes = splitList(row.fieldTypes ?? row.field_types);
    if (fields.length === 0 && metaColumns.length > 0) {
      fields = metaColumns.map((c) => String(c.name ?? ''));
      fieldTypes = metaColumns.map((c) => String(c.type ?? ''));
    }
    return {
      nodeId: str(row.nodeId ?? row.node_id),
      nodeName: str(row.nodeName ?? row.node_name),
      path: str(row.path ?? row.relativeUri),
      tableId: str(row.tableId ?? row.table_id),
      domainDataId: str(row.domainDataId ?? row.domain_data_id ?? row.tableId),
      dsId: str(row.dsId ?? row.datasourceId ?? meta.datasourceId),
      nodeType: str(row.type),
      datasourceType: str(row.datasourceType),
      fields,
      fieldTypes,
    };
  });

  const warningsRaw = r.warning ?? r.warnings;
  const warnings = Array.isArray(warningsRaw) ? warningsRaw.map(String) : typeof warningsRaw === 'string' && warningsRaw ? [warningsRaw] : [];

  return {
    kind,
    rawType,
    codeName: str(r.codeName ?? r.code_name),
    gmtCreate: str(r.gmtCreate ?? r.gmt_create),
    jobId: str(r.jobId ?? r.job_id),
    taskId: str(r.taskId ?? r.task_id),
    graphId: str(r.graphID ?? r.graphId ?? r.graph_id),
    rows,
    tabs,
    warnings,
    raw: r,
  };
}

/** 数据源类型：OSS/ODPS/MYSQL 不支持直接下载（旧版 getDownloadBtnTitle）。 */
export const NON_DOWNLOADABLE_DATASOURCES = ['OSS', 'ODPS', 'MYSQL'];

export function downloadDisabledReason(datasourceType?: string, path?: string): string {
  switch ((datasourceType || '').toUpperCase()) {
    case 'OSS':
      return `OSS 文件不支持直接下载，请到 OSS 对应 bucket 的预设路径下找到文件下载，地址：${path ?? ''}`;
    case 'ODPS':
      return `ODPS 文件不支持直接下载，请到 ODPS 对应项目下找到文件下载，地址：${path ?? ''}`;
    case 'MYSQL':
      return `MYSQL 文件不支持直接下载，请到 MYSQL 对应的数据库下找到文件下载，地址：${path ?? ''}`;
    default:
      return '';
  }
}

/** 数据分类分级系统字段（result-details 中的 L1~L5 标注）。 */
export const CLASSIFICATION_COLUMNS: Record<string, string> = {
  __final_level__: '敏感等级 L1~L5',
  __needs_review__: '需人工复核',
  __tags_json__: '命中标签 JSON',
};

export function isClassificationTable(columns: Array<{ colName?: string; name?: string }>): boolean {
  return columns.some((c) => (c.colName ?? c.name) === '__final_level__');
}
