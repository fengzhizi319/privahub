import { act, renderHook } from '@testing-library/react';
import { beforeEach, describe, expect, it } from 'vitest';
import { PAGE_TOUR_KEYS } from './steps';
import { useDagResultTour } from './use-page-tour';
import { useGuideTourStore } from './store';

describe('useDagResultTour (legacy DAGGuideTourTwo)', () => {
  beforeEach(() => {
    localStorage.clear();
    useGuideTourStore.getState().stop();
  });

  it('opens only after the first DAG tour is done and a run succeeded, then never again', () => {
    localStorage.setItem(PAGE_TOUR_KEYS.dagOne, '1');
    const { result, rerender } = renderHook(({ first }) => useDagResultTour(first), { initialProps: { first: false } });
    expect(result.current.open).toBe(false);
    act(() => result.current.markRunSucceeded());
    expect(result.current.open).toBe(true);
    act(() => result.current.close());
    expect(result.current.open).toBe(false);
    expect(localStorage.getItem(PAGE_TOUR_KEYS.dagTwo)).toBe('1');
    rerender({ first: false });
    expect(result.current.open).toBe(false);
  });

  it('stays closed while the first tour has not been finished', () => {
    const { result, rerender } = renderHook(({ first }) => useDagResultTour(first), { initialProps: { first: true } });
    act(() => result.current.markRunSucceeded());
    expect(result.current.open).toBe(false);
    rerender({ first: false });
    // DAGGuideTourOne 仍未写入（用户还没关掉第一段）→ 不弹。
    expect(result.current.open).toBe(false);
  });
});
