import { expect, it } from 'vitest';
import { mergeTableColumns } from './columns';
it('offers features from both parties, including labels held by the sender', () => {
  expect(mergeTableColumns([{name:'id'}, {name:'a1'}], [{name:'id'}, {name:'b1'}, {name:'y'}]).map(c=>c.name))
    .toEqual(['id','a1','b1','y']);
  expect(mergeTableColumns(undefined, [{name:'b1'}])).toEqual([{name:'b1'}]);
});
