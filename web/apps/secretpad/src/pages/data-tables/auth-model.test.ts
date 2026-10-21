import { describe, expect, it } from 'vitest';
import {
  applyColumnConfigChange,
  canPushToTee,
  datatableDeleteBlock,
  mergeColumnConfigs,
  summarizeBatch,
  toConfigParams,
} from './auth-model';

const schema = [
  { colName: 'id', colType: 'str', colComment: '' },
  { colName: 'x', colType: 'int', colComment: 'c' },
];

describe('column auth config', () => {
  it('merges the schema with saved configs', () => {
    const merged = mergeColumnConfigs(schema, [{ colName: 'id', isAssociateKey: true, isProtection: true }, { colName: 'gone' }]);
    expect(merged).toEqual([
      { colName: 'id', colType: 'str', colComment: '', isAssociateKey: true, isGroupKey: false, isLabelKey: false, isProtection: true },
      { colName: 'x', colType: 'int', colComment: 'c', isAssociateKey: false, isGroupKey: false, isLabelKey: false, isProtection: false },
    ]);
  });

  it('applies the legacy coupling rules', () => {
    let f = mergeColumnConfigs(schema);
    f = applyColumnConfigChange(f, 'id', 'isAssociateKey', true);
    expect(f[0]).toMatchObject({ isAssociateKey: true, isGroupKey: false, isProtection: true });
    f = applyColumnConfigChange(f, 'id', 'isGroupKey', true);
    expect(f[0]).toMatchObject({ isAssociateKey: false, isGroupKey: true, isProtection: true });
    f = applyColumnConfigChange(f, 'id', 'isProtection', false);
    expect(f[0]).toMatchObject({ isAssociateKey: false, isGroupKey: false, isProtection: false });
    f = applyColumnConfigChange(f, 'x', 'isLabelKey', true);
    expect(f[1]).toMatchObject({ isLabelKey: true, isProtection: false });
    expect(toConfigParams(f)[1]).toEqual({ colName: 'x', isAssociateKey: false, isGroupKey: false, isLabelKey: true, isProtection: false });
  });
});

describe('row rules', () => {
  it('blocks deleting authorised and built-in tables', () => {
    expect(datatableDeleteBlock({ datatableName: 't', authProjects: [{ projectId: 'p' }] })).toBe('authorized');
    expect(datatableDeleteBlock({ datatableName: 'alice.csv' })).toBe('embedded');
    expect(datatableDeleteBlock({ datatableName: 't', authProjects: [] })).toBeNull();
  });

  it('allows push to TEE only for local tables', () => {
    expect(canPushToTee({ datasourceType: 'LOCAL' })).toBe(true);
    expect(canPushToTee({ datasourceType: 'OSS' })).toBe(false);
  });

  it('summarises batch results', () => {
    const r = summarizeBatch(
      [{ n: 'a' }, { n: 'b' }],
      [{ status: 'fulfilled', value: 1 }, { status: 'rejected', reason: new Error('boom') }],
      (x) => x.n,
    );
    expect(r).toEqual({ success: 1, failures: [{ name: 'b', message: 'boom' }] });
  });
});
