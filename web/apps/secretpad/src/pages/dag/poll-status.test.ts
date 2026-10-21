import { describe, expect, it } from 'vitest';
import type { GraphStatusJava } from '@secretpad/api-client';
import { anyFailed, shouldPollStatus } from './adapters';

const st = (finished: boolean, ...statuses: string[]): GraphStatusJava => ({
  finished,
  jobId: 'j1',
  nodes: statuses.map((status, i) => ({ graphNodeId: `g-node-${i + 1}`, status })),
});

describe('shouldPollStatus', () => {
  it('polls while a node is running or queued in an unfinished job', () => {
    expect(shouldPollStatus(st(false, 'SUCCEED', 'RUNNING'), false)).toBe(true);
    expect(shouldPollStatus(st(false, 'SUCCEED', 'INITIALIZED'), false)).toBe(true);
    expect(shouldPollStatus(st(true, 'RUNNING'), false)).toBe(true);
  });

  // Regression (system test 2026-09-25): a failed job left downstream nodes
  // INITIALIZED, so the canvas polled forever and never reported the failure.
  it('stops once the job finished even if nodes were never started', () => {
    const s = st(true, 'SUCCEED', 'FAILED', 'INITIALIZED');
    expect(shouldPollStatus(s, false)).toBe(false);
    expect(shouldPollStatus(s, true)).toBe(false);
    expect(anyFailed(s)).toBe(true);
  });

  it('keeps polling after a run request until the new job reports', () => {
    expect(shouldPollStatus(undefined, true)).toBe(true);
    expect(shouldPollStatus(st(false, 'STAGING'), true)).toBe(true);
    expect(shouldPollStatus(st(true, 'SUCCEED'), false)).toBe(false);
  });
});
