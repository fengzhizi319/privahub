import { act, renderHook } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { useGraphEditor } from './use-graph-editor';
import type { GraphDetailJava } from '@secretpad/api-client';

const graph = (id: string) => ({ graphId:id, nodes:[{graphNodeId:`${id}-node-1`,codeName:'read_data/datatable',x:80,y:60,inputs:[],outputs:[]}],edges:[] }) as unknown as GraphDetailJava;
afterEach(() => vi.useRealTimers());

it('switching graphs cancels the old timer and replaces every node and edge', async () => {
  vi.useFakeTimers();
  const saveA=vi.fn(async()=>{}), saveB=vi.fn(async()=>{});
  const a=graph('a'), b=graph('b');
  const {result,rerender}=renderHook(({key,server,save})=>useGraphEditor({graphKey:key,server,save,defs:{},debounceMs:100}),
    {initialProps:{key:'a',server:a,save:saveA}});
  act(()=>result.current.onGraphChange([{...result.current.current().nodes[0],x:123}],[]));
  rerender({key:'b',server:b,save:saveB});
  expect(result.current.current().nodes.map(n=>n.id)).toEqual(['b-node-1']);
  await act(async()=>vi.advanceTimersByTimeAsync(200));
  expect(saveA).not.toHaveBeenCalled();
  expect(saveB).not.toHaveBeenCalled();
});

it.each(['resolve','reject'])('late %s from A cannot change dirty graph B', async outcome => {
  let complete!:()=>void;
  const pending=new Promise<void>((resolve,reject)=>{ complete=()=>outcome==='resolve'?resolve():reject(new Error('old graph failure')); });
  const saveA=vi.fn(()=>pending), saveB=vi.fn(async()=>{}), onError=vi.fn();
  const a=graph('a'), b=graph('b');
  const {result,rerender}=renderHook(({key,server,save})=>useGraphEditor({graphKey:key,server,save,defs:{},onSaveError:onError,debounceMs:60000}),
    {initialProps:{key:'a',server:a,save:saveA}});
  act(()=>result.current.onGraphChange([{...result.current.current().nodes[0],x:123}],[]));
  let flush!:Promise<void>;
  act(()=>{flush=result.current.flush();});
  rerender({key:'b',server:b,save:saveB});
  act(()=>result.current.onGraphChange([{...result.current.current().nodes[0],x:456}],[]));
  await act(async()=>{complete();await flush;});
  expect(result.current.saveState).toBe('dirty');
  expect(result.current.current().nodes.map(n=>n.id)).toEqual(['b-node-1']);
  expect(onError).not.toHaveBeenCalled();
  await act(async()=>result.current.flush());
  expect(saveB).toHaveBeenCalledWith([expect.objectContaining({id:'b-node-1',x:456})],[]);
});

it('never exposes the old canvas during the first render of a new graph', () => {
  const frames: string[][] = [];
  const save = vi.fn(async () => {});
  const { rerender } = renderHook(({ key, server }) => {
    const editor = useGraphEditor({ graphKey: key, server, save, defs: {} });
    frames.push(editor.initialNodes.map(node => node.id));
    return editor;
  }, { initialProps: { key: 'a', server: graph('a') } });
  frames.length = 0;
  rerender({ key: 'b', server: graph('b') });
  expect(frames.length).toBeGreaterThan(0);
  expect(frames.every(ids => ids.length === 1 && ids[0] === 'b-node-1')).toBe(true);
});
