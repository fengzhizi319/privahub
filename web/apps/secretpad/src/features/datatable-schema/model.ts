/**
 * Schema-editor helpers shared by the upload flow (`data/create`) and the
 * datatable registration wizard (`datatable/create`).
 */
import {
  inferColumnType,
  parseCsvPreview,
  toSchemaColType,
  validateSchemaFields,
  buildSchemaTemplateCsv,
} from '@secretpad/utils';
import type { CsvPreview, SchemaField, SchemaFieldIssue } from '@secretpad/utils';
import type { DatatableSchemaJava, TableColumnJava } from '@secretpad/api-client';

export type { SchemaField, SchemaFieldIssue };

export const emptySchemaField = (): SchemaField => ({ featureName: '', featureType: '', featureDescription: '' });

/** Bytes read from the head of a CSV to parse its header and sample rows. */
export const CSV_HEAD_BYTES = 64 * 1024;

/** Read and parse the first `bytes` of a (CSV) file. The last, possibly truncated, line is dropped. */
export async function readCsvHead(file: Blob, bytes = CSV_HEAD_BYTES, sampleRows = 20): Promise<CsvPreview> {
  const blob = file.slice(0, bytes);
  const text = typeof blob.text === 'function' ? await blob.text() : await new Response(blob).text();
  const truncated = file.size > bytes;
  const safe = truncated ? text.slice(0, Math.max(text.lastIndexOf('\n'), 0)) || text : text;
  return parseCsvPreview(safe, sampleRows);
}

/** Initial schema rows from a CSV header, with types inferred from sample rows. */
export function schemaFromCsvPreview(preview: CsvPreview, nullStrs: string[] = ['']): SchemaField[] {
  return preview.header.map((col, i) => ({
    featureName: col,
    featureType: toSchemaColType(inferColumnType(preview.rows.map((r) => r[i] ?? ''), nullStrs)),
    featureDescription: '',
  }));
}

/** Map editor rows to Java `TableColumnVO[]` (datatable/create `columns`). */
export function toTableColumns(fields: SchemaField[]): TableColumnJava[] {
  return fields.map((f) => ({
    colName: f.featureName.trim(),
    colType: f.featureType,
    colComment: f.featureDescription || '',
  }));
}

/** Map editor rows to Java `DatatableSchema[]` (data/create `datatableSchema`). */
export function toDatatableSchema(fields: SchemaField[]): DatatableSchemaJava[] {
  return fields.map((f) => ({
    featureName: f.featureName.trim(),
    featureType: f.featureType,
    featureDescription: f.featureDescription || '',
  }));
}

/** Schema is submittable: at least one row and no blocking issues. */
export function isSchemaValid(fields: SchemaField[]): boolean {
  return fields.length > 0 && validateSchemaFields(fields).length === 0;
}

/** Trigger a browser download of the legacy schema template (`示例文件.csv`). */
export function downloadSchemaTemplate(fileName = '示例文件.csv'): void {
  const blob = new Blob([buildSchemaTemplateCsv()], { type: 'text/csv;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = fileName;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 0);
}
