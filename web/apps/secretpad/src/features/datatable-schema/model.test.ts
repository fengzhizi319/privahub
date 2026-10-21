import { describe, expect, it } from 'vitest';
import { isSchemaValid, readCsvHead, schemaFromCsvPreview, toDatatableSchema, toTableColumns } from './model';

describe('schema editor model', () => {
  it('infers the schema from a CSV header and sample rows', () => {
    const fields = schemaFromCsvPreview({
      header: ['id', 'age', 'score', 'flag'],
      rows: [
        ['a1', '12', '1.5', 'true'],
        ['a2', '', '2', 'false'],
      ],
    });
    expect(fields.map((f) => `${f.featureName}:${f.featureType}`)).toEqual(['id:str', 'age:int', 'score:float', 'flag:str']);
  });

  it('reads only the head of a file and drops a truncated trailing line', async () => {
    const text = 'id,age\n1,2\n3,44444';
    const preview = await readCsvHead(new Blob([text]), 16);
    expect(preview.header).toEqual(['id', 'age']);
    expect(preview.rows).toEqual([['1', '2']]);
  });

  it('maps rows to Java column shapes', () => {
    const fields = [{ featureName: ' id ', featureType: 'str', featureDescription: '' }];
    expect(toTableColumns(fields)).toEqual([{ colName: 'id', colType: 'str', colComment: '' }]);
    expect(toDatatableSchema(fields)).toEqual([{ featureName: 'id', featureType: 'str', featureDescription: '' }]);
  });

  it('validates the schema', () => {
    expect(isSchemaValid([])).toBe(false);
    expect(isSchemaValid([{ featureName: 'id', featureType: 'str', featureDescription: '' }])).toBe(true);
    expect(
      isSchemaValid([
        { featureName: 'id', featureType: 'str', featureDescription: '' },
        { featureName: 'id', featureType: 'int', featureDescription: '' },
      ]),
    ).toBe(false);
  });
});
