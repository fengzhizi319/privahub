import { describe, expect, it } from 'vitest';
import { toJobPage } from './scheduled';

describe('toJobPage (project/job/list, scheduled/job/list)', () => {
  it('prefers the exact Go `total` over any pageTotal estimate', () => {
    // Real Go response for a project with 3 jobs.
    expect(toJobPage({ data: [1, 2, 3], pageSize: 10, pageTotal: 1, total: 3 }, 10)).toEqual({ list: [1, 2, 3], total: 3 });
    // Second page of a larger list: total stays exact.
    expect(toJobPage({ data: [11], pageSize: 5, pageTotal: 3, total: 11 }, 5).total).toBe(11);
  });

  it('Java shape without total: a single page is exactly the list length', () => {
    expect(toJobPage({ data: [1], pageSize: 10, pageTotal: 1 }, 10)).toEqual({ list: [1], total: 1 });
    expect(toJobPage({ data: [], pageSize: 10, pageTotal: 0 }, 10)).toEqual({ list: [], total: 0 });
  });

  it('Java shape without total and several pages: estimates pageTotal × pageSize', () => {
    expect(toJobPage({ data: [1, 2, 3, 4, 5], pageSize: 5, pageTotal: 3 }, 5).total).toBe(15);
  });
});
