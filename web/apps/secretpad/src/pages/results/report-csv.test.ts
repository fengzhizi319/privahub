import { describe, expect, it } from 'vitest';
import { reportToCsv } from './report-csv';

describe('reportToCsv', () => {
  it('flattens tables and descriptions', () => {
    const csv = reportToCsv([
      {
        name: 'Tab,1',
        divs: [
          {
            children: [
              {
                type: 'table',
                table: {
                  headers: [
                    { name: 'k', type: 'AT_STRING' },
                    { name: 'v', type: 'AT_FLOAT' },
                  ],
                  rows: [{ items: [{ s: 'a' }, { f: 1.5 }] }],
                },
              },
              { type: 'descriptions', descriptions: { items: [{ name: 'auc', value: { s: '0.9' } }] } },
            ],
          },
        ],
      },
    ]);
    expect(csv).toBe('"Tab,1"\nk,v\na,1.5\nauc,0.9');
  });
});
