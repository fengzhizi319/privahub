/**
 * Interactive guide tour steps (legacy `modules/guide-tour` keys:
 * ProjectListTour → CreateProjectTour → DatatableAuthTour → DAGGuideTour → RecordGuideTour).
 *
 * Each step has a route to navigate to and an optional `data-tour` target to
 * highlight when the element exists on that page.
 */
export interface GuideTourStep {
  key: string;
  route: string;
  /** `data-tour="<target>"` element to highlight (optional). */
  target?: string;
  titleKey: string;
  descKey: string;
}

export const GUIDE_TOUR_STEPS: GuideTourStep[] = [
  { key: 'projectList', route: '/projects', target: 'project-list', titleKey: 'guideTour.projectList.title', descKey: 'guideTour.projectList.desc' },
  { key: 'createProject', route: '/projects', target: 'project-create', titleKey: 'guideTour.createProject.title', descKey: 'guideTour.createProject.desc' },
  { key: 'datatableAuth', route: '/data-tables', target: 'datatable-auth', titleKey: 'guideTour.datatableAuth.title', descKey: 'guideTour.datatableAuth.desc' },
  { key: 'dag', route: '/dag', target: 'dag-canvas', titleKey: 'guideTour.dag.title', descKey: 'guideTour.dag.desc' },
  { key: 'records', route: '/results', target: 'results-list', titleKey: 'guideTour.records.title', descKey: 'guideTour.records.desc' },
];

export const GUIDE_TOUR_DONE_KEY = 'secretpad-guide-tour-done';

export function clampStep(step: number, total = GUIDE_TOUR_STEPS.length): number {
  if (!Number.isFinite(step) || step < 0) return 0;
  return Math.min(Math.floor(step), total - 1);
}

export function isTourDone(): boolean {
  try {
    return localStorage.getItem(GUIDE_TOUR_DONE_KEY) === '1';
  } catch {
    return false;
  }
}

export function markTourDone(): void {
  try {
    localStorage.setItem(GUIDE_TOUR_DONE_KEY, '1');
  } catch {
    // storage unavailable — ignore
  }
}

export function resetTourDone(): void {
  try {
    localStorage.removeItem(GUIDE_TOUR_DONE_KEY);
  } catch {
    // ignore
  }
  // 旧版 GuideTourService.reset()：重开引导时各页面首访 tour 也重新提示。
  resetPageTours();
}

/**
 * 各页面「首次访问自动弹出」的 tour（旧版 GuideTourService 的 localStorage 键，保持同名）。
 * 未设置键 = 尚未提示过 → 页面满足条件时自动打开；关闭后写入 '1'。
 */
export const PAGE_TOUR_KEYS = {
  projectList: 'ProjectListTour',
  createProject: 'CreateProjectTour',
  datatableAuth: 'DatatableAuthTour',
  dagOne: 'DAGGuideTourOne',
  dagTwo: 'DAGGuideTourTwo',
  record: 'RecordGuideTour',
} as const;
export type PageTourKey = (typeof PAGE_TOUR_KEYS)[keyof typeof PAGE_TOUR_KEYS];

export function isPageTourDone(key: PageTourKey): boolean {
  try {
    return !!localStorage.getItem(key);
  } catch {
    // 无 storage 时视为已提示，避免反复弹出。
    return true;
  }
}

export function finishPageTour(key: PageTourKey): void {
  try {
    localStorage.setItem(key, '1');
  } catch {
    // ignore
  }
}

export function resetPageTours(): void {
  for (const key of Object.values(PAGE_TOUR_KEYS)) {
    try {
      localStorage.removeItem(key);
    } catch {
      // ignore
    }
  }
}

/** 旧版 finishAll：跳过全部页面 tour。 */
export function finishAllPageTours(): void {
  for (const key of Object.values(PAGE_TOUR_KEYS)) finishPageTour(key);
}
