import { describe, expect, it } from 'vitest';
import { canOperateSchedule, scheduleAction, shouldPollSubTasks, subTaskActions } from './helpers';

const now = new Date('2026-03-01T00:00:00').getTime();

describe('periodic task helpers', () => {
  it('offline is disabled while a sub task runs; offline tasks can be deleted', () => {
    expect(scheduleAction({ scheduleStats: 'UP' })).toBe('offline');
    expect(scheduleAction({ scheduleStats: 'UP', taskRunning: true })).toBe('offlineDisabled');
    expect(scheduleAction({ scheduleStats: 'DOWN' })).toBe('delete');
  });

  it('restricts operations to the owner on P2P platforms', () => {
    expect(canOperateSchedule({ owner: 'a' }, { isP2p: false, ownerId: 'b' })).toBe(true);
    expect(canOperateSchedule({ owner: 'a' }, { isP2p: true, ownerId: 'a' })).toBe(true);
    expect(canOperateSchedule({ owner: 'a' }, { isP2p: true, ownerId: 'b' })).toBe(false);
  });

  it('polls while sub tasks are pending / running / stopping', () => {
    expect(shouldPollSubTasks([{ scheduleTaskStatus: 'SUCCEED' }])).toBe(false);
    expect(shouldPollSubTasks([{ scheduleTaskStatus: 'TO_BE_RUN' }])).toBe(true);
  });

  it('offers rerun-failed (type 1) + rerun-all (type 0) for recent failures', () => {
    const a = subTaskActions({ scheduleTaskStatus: 'FAILED', scheduleTaskStartTime: '2026-02-20 10:00:00' }, now);
    expect(a.stop).toBe(false);
    expect(a.rerun.map((r) => r.type)).toEqual(['1', '0']);
    const reRan = subTaskActions(
      { scheduleTaskStatus: 'STOPPED', scheduleTaskStartTime: '2026-02-20 10:00:00', allReRun: true },
      now,
    );
    expect(reRan.rerun.map((r) => r.disabled)).toEqual([false, true]);
  });

  it('offers only rerun-all for successes and failures older than 30 days', () => {
    expect(subTaskActions({ scheduleTaskStatus: 'SUCCEED', scheduleTaskStartTime: '2026-02-28 10:00:00' }, now).rerun).toEqual([
      { type: '0', labelKey: 'periodicTasks.rerun', disabled: false },
    ]);
    const old = subTaskActions({ scheduleTaskStatus: 'FAILED', scheduleTaskStartTime: '2026-01-01 10:00:00' }, now);
    expect(old.rerun.map((r) => r.type)).toEqual(['0']);
  });

  it('allows stopping running tasks and nothing for pending ones', () => {
    expect(subTaskActions({ scheduleTaskStatus: 'RUNNING' }, now)).toEqual({ stop: true, rerun: [] });
    expect(subTaskActions({ scheduleTaskStatus: 'TO_BE_RUN' }, now)).toEqual({ stop: false, rerun: [] });
  });
});
