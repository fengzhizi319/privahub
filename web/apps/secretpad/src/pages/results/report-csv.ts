/** Report output → CSV (ported from legacy result-manager.service `reportToCsv`). */
type ReportValue = { s?: string; f?: number; i64?: string | number; b?: boolean };
type ReportTable = { headers: { name: string; type: string }[]; rows: { name?: string; items: ReportValue[] }[] };
type ReportChild =
  | { type: 'table'; table: ReportTable }
  | { type: 'descriptions'; descriptions: { items: { name: string; type?: string; value: ReportValue }[] } }
  | { type: 'div'; div: { children: ReportChild[] } };
export type ReportTab = { name?: string; divs: { children: ReportChild[] }[] };

function valueToString(value: ReportValue | undefined, type: string): string {
  if (!value) return '';
  switch (type) {
    case 's':
    case 'AT_STRING':
      return value.s ?? '';
    case 'f':
    case 'AT_FLOAT':
      return value.f !== undefined ? String(value.f) : '';
    case 'i64':
    case 'AT_INT':
      return value.i64 !== undefined ? String(value.i64) : '';
    case 'b':
    case 'AT_BOOL':
      return value.b !== undefined ? String(value.b) : '';
    default:
      return value.s ?? String(value.f ?? value.i64 ?? value.b ?? '');
  }
}

function escapeCsv(value: string): string {
  return /[",\n]/.test(value) ? `"${value.replace(/"/g, '""')}"` : value;
}

function appendChild(child: ReportChild, lines: string[]) {
  if (child.type === 'table') {
    const table = child.table;
    if (!table?.headers?.length) return;
    lines.push(table.headers.map((h) => escapeCsv(h.name)).join(','));
    (table.rows || []).forEach((row) =>
      lines.push(row.items.map((item, i) => escapeCsv(valueToString(item, table.headers[i]?.type || 's'))).join(',')),
    );
  } else if (child.type === 'descriptions') {
    // 按条目声明类型取值；无类型时取第一个非空字段（原先固定按 s 取，数值条目会导出为空）。
    child.descriptions.items.forEach((item) => lines.push(`${escapeCsv(item.name)},${escapeCsv(valueToString(item.value, item.type || ''))}`));
  } else if (child.type === 'div') {
    child.div.children.forEach((c) => appendChild(c, lines));
  }
}

export function reportToCsv(tabs: ReportTab[]): string {
  const lines: string[] = [];
  tabs.forEach((tab, idx) => {
    if (idx > 0) lines.push('');
    if (tab.name) lines.push(escapeCsv(tab.name));
    (tab.divs || []).forEach((div) => (div.children || []).forEach((c) => appendChild(c, lines)));
  });
  return lines.join('\n');
}

/** 报告 CSV（带 BOM，Excel 可直接打开）；结果列表与结果详情共用。 */
export function reportCsvBlob(tabs: ReportTab[]): Blob {
  return new Blob(['﻿', reportToCsv(tabs)], { type: 'text/csv;charset=utf-8;' });
}
