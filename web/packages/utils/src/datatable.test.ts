import { describe, expect, it } from 'vitest';
import {
  SQL_KEYWORDS,
  buildSchemaTemplateCsv,
  formatFailedNodes,
  formatNullStrs,
  isScqlUnfriendly,
  isSqlKeyword,
  parseNullStrs,
  parseSchemaCsv,
  scqlWarningIndexes,
  toSchemaColType,
  validateDisplayName,
  validateSchemaFields,
} from './datatable';
import { parseCsvPreview } from './csv';

describe('sql keywords', () => {
  it('ports the legacy MySQL keyword list', () => {
    expect(SQL_KEYWORDS.length).toBeGreaterThan(700);
    expect(SQL_KEYWORDS).toContain('SELECT');
    expect(SQL_KEYWORDS).toContain('ZONE');
  });

  it('matches case-insensitively and ignores empty input', () => {
    expect(isSqlKeyword('select')).toBe(true);
    expect(isSqlKeyword('Table')).toBe(true);
    expect(isSqlKeyword('age')).toBe(false);
    expect(isSqlKeyword('')).toBe(false);
    expect(isSqlKeyword(undefined)).toBe(false);
  });

  it('flags hyphenated names as SCQL-unfriendly', () => {
    expect(isScqlUnfriendly('user-id')).toBe(true);
    expect(isScqlUnfriendly('order')).toBe(true);
    expect(isScqlUnfriendly('user_id')).toBe(false);
  });
});

describe('validateSchemaFields', () => {
  const f = (featureName: string, featureType = 'int', featureDescription = '') => ({
    featureName,
    featureType,
    featureDescription,
  });

  it('accepts a valid schema', () => {
    expect(validateSchemaFields([f('id'), f('x1', 'float'), f('x-2', 'str')])).toEqual([]);
  });

  it('reports empty, pattern, length, duplicate, type and description issues', () => {
    const issues = validateSchemaFields([
      f(''),
      f('1abc'),
      f('a'.repeat(65)),
      f('dup'),
      f('dup'),
      f('ok', ''),
      f('desc', 'int', 'x'.repeat(201)),
    ]);
    const kinds = issues.map((i) => `${i.index}:${i.kind}`);
    expect(kinds).toEqual([
      '0:nameRequired',
      '1:namePattern',
      '2:nameTooLong',
      '3:duplicate',
      '4:duplicate',
      '5:typeRequired',
      '6:descTooLong',
    ]);
  });

  it('flags SCQL warnings separately', () => {
    expect(scqlWarningIndexes([f('id'), f('select'), f('a-b')])).toEqual([1, 2]);
  });

  it('maps inferred types onto editor types', () => {
    expect(toSchemaColType('int')).toBe('int');
    expect(toSchemaColType('bool')).toBe('str');
    expect(toSchemaColType('integer')).toBe('int');
  });
});

describe('schema template csv', () => {
  it('round-trips through parseSchemaCsv', () => {
    const text = buildSchemaTemplateCsv();
    const parsed = parseSchemaCsv(parseCsvPreview(text, 100));
    expect(parsed?.fields[0]).toEqual({ featureName: 'id1', featureType: 'str', featureDescription: '' });
    expect(parsed?.fields[1]).toEqual({ featureName: 'x1', featureType: 'int', featureDescription: '描述' });
    expect(parsed?.fields).toHaveLength(11);
  });

  it('rejects a wrong header and dedups names', () => {
    expect(parseSchemaCsv({ header: ['a', 'b'], rows: [] })).toBeNull();
    const r = parseSchemaCsv({
      header: ['特征名称', '特征类型', '特征描述'],
      rows: [
        ['a', 'integer', ''],
        ['a', 'float', ''],
        ['b', 'weird', 'd'],
      ],
    });
    expect(r).toEqual({
      fields: [
        { featureName: 'a', featureType: 'int', featureDescription: '' },
        { featureName: 'b', featureType: '', featureDescription: 'd' },
      ],
      duplicates: 1,
    });
  });
});

describe('names and null strings', () => {
  it('validates display names', () => {
    expect(validateDisplayName('')).toBe('required');
    expect(validateDisplayName('表_a-1')).toBeNull();
    expect(validateDisplayName('a b')).toBe('pattern');
    expect(validateDisplayName('a'.repeat(33))).toBe('tooLong');
  });

  it('parses legacy null string text', () => {
    expect(parseNullStrs('""')).toEqual(['']);
    expect(parseNullStrs('"", "NA", "-999"')).toEqual(['', 'NA', '-999']);
    expect(parseNullStrs('')).toEqual([]);
    expect(parseNullStrs('NA')).toBeNull();
    expect(formatNullStrs(['', 'NA'])).toBe('"","NA"');
  });
});

describe('formatFailedNodes', () => {
  it('turns the failedCreatedNodes map into a list', () => {
    expect(formatFailedNodes({ alice: 'timeout', bob: { code: 1 } })).toEqual([
      { nodeId: 'alice', message: 'timeout' },
      { nodeId: 'bob', message: '{"code":1}' },
    ]);
    expect(formatFailedNodes(undefined)).toEqual([]);
    expect(formatFailedNodes(null)).toEqual([]);
  });
});
