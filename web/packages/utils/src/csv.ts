/**
 * Lightweight CSV helpers for the data upload wizard (header parsing and
 * schema validation happen client-side before calling `data/create`).
 */

/** Split one CSV line honoring double-quoted fields and escaped quotes (""). */
export function splitCsvLine(line: string, delimiter = ','): string[] {
  const out: string[] = [];
  let cur = '';
  let inQuotes = false;
  for (let i = 0; i < line.length; i++) {
    const ch = line[i];
    if (inQuotes) {
      if (ch === '"') {
        if (line[i + 1] === '"') {
          cur += '"';
          i++;
        } else {
          inQuotes = false;
        }
      } else {
        cur += ch;
      }
    } else if (ch === '"') {
      inQuotes = true;
    } else if (ch === delimiter) {
      out.push(cur);
      cur = '';
    } else {
      cur += ch;
    }
  }
  out.push(cur);
  return out.map((s) => s.trim());
}

export interface CsvPreview {
  header: string[];
  rows: string[][];
}

/** Parse the header row plus up to `sampleRows` data rows from CSV text. Strips a UTF-8 BOM. */
export function parseCsvPreview(text: string, sampleRows = 20): CsvPreview {
  const clean = text.replace(/^\uFEFF/, '');
  const lines = clean.split(/\r?\n/).filter((l) => l.trim() !== '');
  if (lines.length === 0) return { header: [], rows: [] };
  const header = splitCsvLine(lines[0]);
  const rows = lines.slice(1, 1 + sampleRows).map((l) => splitCsvLine(l));
  return { header, rows };
}

export type InferredColType = 'int' | 'float' | 'bool' | 'str';

/** Guess a SecretFlow column type from sample values (empty / null tokens are ignored). */
export function inferColumnType(values: string[], nullStrs: string[] = ['', 'NULL', 'null', 'NA']): InferredColType {
  const vals = values.filter((v) => !nullStrs.includes(v));
  if (vals.length === 0) return 'str';
  if (vals.every((v) => /^[-+]?\d+$/.test(v))) return 'int';
  if (vals.every((v) => /^[-+]?(\d+\.?\d*|\.\d+)([eE][-+]?\d+)?$/.test(v))) return 'float';
  if (vals.every((v) => /^(true|false)$/i.test(v))) return 'bool';
  return 'str';
}

export type SchemaIssue =
  | { kind: 'empty'; index: number }
  | { kind: 'duplicate'; index: number; name: string }
  | { kind: 'invalidName'; index: number; name: string };

/** Column name rule used by the legacy platform: letters, digits, underscore, hyphen, CJK. */
const COLUMN_NAME_RE = /^[\w一-龥-]+$/;

/** Validate column names: non-empty, legal characters and no duplicates (case-insensitive). */
export function validateColumnNames(names: string[]): SchemaIssue[] {
  const issues: SchemaIssue[] = [];
  const seen = new Map<string, number>();
  names.forEach((raw, index) => {
    const name = raw.trim();
    if (!name) {
      issues.push({ kind: 'empty', index });
      return;
    }
    if (!COLUMN_NAME_RE.test(name)) issues.push({ kind: 'invalidName', index, name });
    const key = name.toLowerCase();
    if (seen.has(key)) issues.push({ kind: 'duplicate', index, name });
    else seen.set(key, index);
  });
  return issues;
}
