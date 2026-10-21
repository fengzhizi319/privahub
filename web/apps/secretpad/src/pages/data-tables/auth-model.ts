/**
 * Per-column project authorisation model (legacy `ProjectTableField`).
 * Java `TableColumnConfigParam{colName,isAssociateKey,isGroupKey,isLabelKey,isProtection}`.
 */
import type { DatatableColumnConfigJava, DatatableRowJava, TableColumnJava } from '@secretpad/api-client';

export type ColumnConfig = Required<Pick<DatatableColumnConfigJava, 'colName'>> &
  Omit<DatatableColumnConfigJava, 'colName'> & { colType?: string; colComment?: string };

export type ColumnConfigKey = 'isAssociateKey' | 'isGroupKey' | 'isLabelKey' | 'isProtection';

/** Built-in demo tables which the legacy UI never allowed to delete / batch select. */
export const EMBEDDED_SHEETS = ['alice.csv', 'bob.csv'];

/** Merge the table schema with saved configs (configs win; unknown saved columns are dropped). */
export function mergeColumnConfigs(schema: TableColumnJava[] = [], configs: DatatableColumnConfigJava[] = []): ColumnConfig[] {
  const byName = new Map(configs.map((c) => [c.colName, c]));
  return schema.map((col) => {
    const saved = byName.get(col.colName);
    return {
      colName: col.colName || '',
      colType: col.colType ?? saved?.colType,
      colComment: col.colComment ?? saved?.colComment,
      isAssociateKey: !!saved?.isAssociateKey,
      isGroupKey: !!saved?.isGroupKey,
      isLabelKey: !!saved?.isLabelKey,
      isProtection: !!saved?.isProtection,
    };
  });
}

/**
 * Toggle one flag with the legacy coupling rules:
 * - associate key ↔ group key are mutually exclusive, and turning either on turns protection on;
 * - toggling protection clears associate / group key;
 * - label key is independent.
 */
export function applyColumnConfigChange(fields: ColumnConfig[], colName: string, key: ColumnConfigKey, value: boolean): ColumnConfig[] {
  return fields.map((f) => {
    if (f.colName !== colName) return f;
    switch (key) {
      case 'isAssociateKey':
        return { ...f, isAssociateKey: value, isGroupKey: false, isProtection: value ? true : f.isProtection };
      case 'isGroupKey':
        return { ...f, isGroupKey: value, isAssociateKey: false, isProtection: value ? true : f.isProtection };
      case 'isProtection':
        return { ...f, isProtection: value, isGroupKey: false, isAssociateKey: false };
      case 'isLabelKey':
        return { ...f, isLabelKey: value };
      default:
        return f;
    }
  });
}

/** Strip display-only fields before sending configs to the backend. */
export function toConfigParams(fields: ColumnConfig[]): DatatableColumnConfigJava[] {
  return fields.map((f) => ({
    colName: f.colName,
    isAssociateKey: !!f.isAssociateKey,
    isGroupKey: !!f.isGroupKey,
    isLabelKey: !!f.isLabelKey,
    isProtection: !!f.isProtection,
  }));
}

/** Deletion is blocked for authorised or built-in tables. */
export function datatableDeleteBlock(row: DatatableRowJava): 'authorized' | 'embedded' | null {
  if (EMBEDDED_SHEETS.includes(row.datatableName || '')) return 'embedded';
  if ((row.authProjects || []).length > 0) return 'authorized';
  return null;
}

/** Push-to-TEE applies only to local (CSV) tables. */
export function canPushToTee(row: DatatableRowJava): boolean {
  return !['HTTP', 'OSS', 'MYSQL', 'ODPS'].includes(row.datasourceType || '');
}

/** Summarise `Promise.allSettled` results of a batch operation. */
export function summarizeBatch<T>(
  rows: T[],
  results: PromiseSettledResult<unknown>[],
  nameOf: (row: T) => string,
): { success: number; failures: { name: string; message: string }[] } {
  let success = 0;
  const failures: { name: string; message: string }[] = [];
  results.forEach((r, i) => {
    if (r.status === 'fulfilled') success += 1;
    else failures.push({ name: nameOf(rows[i]), message: r.reason instanceof Error ? r.reason.message : String(r.reason ?? '') });
  });
  return { success, failures };
}
