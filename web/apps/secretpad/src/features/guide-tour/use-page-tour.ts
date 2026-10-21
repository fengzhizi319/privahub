/**
 * 页面首访 tour（旧版 dag-guide-tour / dag-record-guide-tour / project-list / data-manager 的 Tour）：
 * 条件满足（ready）且对应 localStorage 键未写入时自动打开；关闭即写入键，之后不再自动弹出。
 * 全局引导（GuideTourHost）运行时不自动弹出，避免两个 tour 叠加。
 */
import { useCallback, useEffect, useState } from 'react';
import { useGuideTourStore } from './store';
import { PAGE_TOUR_KEYS, finishPageTour, isPageTourDone, type PageTourKey } from './steps';

export function usePageTour(key: PageTourKey, ready: boolean) {
  const globalOpen = useGuideTourStore((s) => s.open);
  const [open, setOpen] = useState(false);
  useEffect(() => {
    if (ready && !globalOpen && !isPageTourDone(key)) setOpen(true);
  }, [key, ready, globalOpen]);
  const close = useCallback(() => {
    setOpen(false);
    finishPageTour(key);
  }, [key]);
  return { open, close };
}

/**
 * 旧版 DAGGuideTourTwo：第一段 DAG tour 已结束，且本页首次有一次运行全部成功时，
 * 提示「点击组件查看结果」和「历史记录」（关闭即写入 DAGGuideTourTwo，不再弹出）。
 */
export function useDagResultTour(firstTourOpen: boolean) {
  const [succeeded, setSucceeded] = useState(false);
  const tour = usePageTour(PAGE_TOUR_KEYS.dagTwo, succeeded && !firstTourOpen && isPageTourDone(PAGE_TOUR_KEYS.dagOne));
  const markRunSucceeded = useCallback(() => setSucceeded(true), []);
  return { ...tour, markRunSucceeded };
}
