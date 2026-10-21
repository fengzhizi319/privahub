import { describe, it, expect } from 'vitest';
import { splitCsvLine, parseCsvPreview, inferColumnType, validateColumnNames } from './csv';

describe('csv helpers', () => {
  it('splits quoted fields with embedded delimiters and quotes', () => {
    expect(splitCsvLine('a,"b,c","d ""q""",e')).toEqual(['a', 'b,c', 'd "q"', 'e']);
  });

  it('parses header and sample rows, stripping BOM and blank lines', () => {
    const p = parseCsvPreview('﻿id,age,score\r\n1,20,0.5\n\n2,30,1.5\n', 1);
    expect(p.header).toEqual(['id', 'age', 'score']);
    expect(p.rows).toEqual([['1', '20', '0.5']]);
  });

  it('returns empty preview for empty text', () => {
    expect(parseCsvPreview('')).toEqual({ header: [], rows: [] });
  });

  it('infers column types', () => {
    expect(inferColumnType(['1', '-2', 'NULL'])).toBe('int');
    expect(inferColumnType(['1.5', '2', '3e5'])).toBe('float');
    expect(inferColumnType(['true', 'FALSE'])).toBe('bool');
    expect(inferColumnType(['a', '1'])).toBe('str');
    expect(inferColumnType(['', 'NA'])).toBe('str');
  });

  it('detects empty, invalid and duplicate column names', () => {
    const issues = validateColumnNames(['id', 'ID', '', 'bad name', 'ok_1']);
    expect(issues).toEqual([
      { kind: 'duplicate', index: 1, name: 'ID' },
      { kind: 'empty', index: 2 },
      { kind: 'invalidName', index: 3, name: 'bad name' },
    ]);
  });
});
