import { beforeEach, describe, expect, it } from 'vitest';
import { useGuideTourStore } from './store';
import { GUIDE_TOUR_DONE_KEY, GUIDE_TOUR_STEPS, clampStep, isTourDone } from './steps';

describe('guide tour store', () => {
  beforeEach(() => {
    localStorage.clear();
    useGuideTourStore.getState().stop();
  });

  it('starts, advances and finishes, persisting done flag', () => {
    const s = useGuideTourStore.getState();
    s.start();
    expect(useGuideTourStore.getState()).toMatchObject({ open: true, step: 0 });
    for (let i = 1; i < GUIDE_TOUR_STEPS.length; i++) {
      useGuideTourStore.getState().next();
      expect(useGuideTourStore.getState().step).toBe(i);
    }
    useGuideTourStore.getState().next();
    expect(useGuideTourStore.getState().open).toBe(false);
    expect(localStorage.getItem(GUIDE_TOUR_DONE_KEY)).toBe('1');
    expect(isTourDone()).toBe(true);
  });

  it('prev never goes below 0 and start resets done', () => {
    localStorage.setItem(GUIDE_TOUR_DONE_KEY, '1');
    useGuideTourStore.getState().start(2);
    expect(isTourDone()).toBe(false);
    useGuideTourStore.getState().prev();
    useGuideTourStore.getState().prev();
    useGuideTourStore.getState().prev();
    expect(useGuideTourStore.getState().step).toBe(0);
  });

  it('clamps steps', () => {
    expect(clampStep(-1)).toBe(0);
    expect(clampStep(99)).toBe(GUIDE_TOUR_STEPS.length - 1);
  });
});
