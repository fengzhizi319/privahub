import { create } from 'zustand';
import { GUIDE_TOUR_STEPS, clampStep, markTourDone, resetTourDone } from './steps';

/**
 * Global state of the interactive guide tour. The header's "reopen guide"
 * action and the guide page call `start()`; `GuideTourHost` (mounted once in
 * AppLayout) renders the current step.
 */
interface GuideTourState {
  open: boolean;
  step: number;
  start: (step?: number) => void;
  goTo: (step: number) => void;
  stop: () => void;
  /** Go to the next step, or finish (persist "done") after the last one. */
  next: () => void;
  prev: () => void;
  /** Close and persist "tour done" (localStorage `secretpad-guide-tour-done`). */
  finish: () => void;
}

export const useGuideTourStore = create<GuideTourState>((set, get) => ({
  open: false,
  step: 0,
  start: (step = 0) => {
    resetTourDone();
    set({ open: true, step: clampStep(step) });
  },
  goTo: (step) => set({ step: clampStep(step) }),
  stop: () => set({ open: false, step: 0 }),
  next: () => {
    const { step } = get();
    if (step >= GUIDE_TOUR_STEPS.length - 1) get().finish();
    else set({ step: step + 1 });
  },
  prev: () => set({ step: clampStep(get().step - 1) }),
  finish: () => {
    markTourDone();
    set({ open: false, step: 0 });
  },
}));
