/**
 * Lightweight step tour host (no third-party deps).
 *
 * Mounted once in AppLayout. When the tour is open it:
 * - navigates to the current step's route,
 * - highlights `[data-tour="<target>"]` if such an element appears on the page
 *   (polled briefly because pages are lazy-loaded), and
 * - renders a floating card (bottom-right) with prev / next / skip.
 */
import React, { useEffect, useState } from 'react';
import { useNavigate, useRouterState } from '@tanstack/react-router';
import { Button } from '@secretpad/design-system';
import { useTranslation } from '@/shared/lib/i18n';
import { useGuideTourStore } from './store';
import { GUIDE_TOUR_STEPS } from './steps';

interface Rect {
  top: number;
  left: number;
  width: number;
  height: number;
}

function findTarget(target?: string): Element | null {
  if (!target || typeof document === 'undefined') return null;
  return document.querySelector(`[data-tour="${target}"]`);
}

export const GuideTourHost: React.FC = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  const { open, step, next, prev, finish } = useGuideTourStore();
  const current = open ? GUIDE_TOUR_STEPS[step] : undefined;
  const [rect, setRect] = useState<Rect | null>(null);

  // Navigate to the step's route.
  useEffect(() => {
    if (current && pathname !== current.route) {
      navigate({ to: current.route });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [current?.key]);

  // Track the highlighted element (poll while the lazy page mounts, then follow scroll/resize).
  useEffect(() => {
    setRect(null);
    if (!current?.target) return;
    let el: Element | null = null;
    const measure = () => {
      el = el && document.contains(el) ? el : findTarget(current.target);
      if (!el) {
        setRect(null);
        return;
      }
      const r = el.getBoundingClientRect();
      setRect({ top: r.top, left: r.left, width: r.width, height: r.height });
    };
    measure();
    let tries = 0;
    const timer = window.setInterval(() => {
      tries += 1;
      measure();
      if (tries > 20) window.clearInterval(timer);
    }, 250);
    const firstScroll = window.setTimeout(() => {
      const target = findTarget(current.target);
      if (target && 'scrollIntoView' in target) (target as HTMLElement).scrollIntoView?.({ block: 'center', behavior: 'smooth' });
    }, 300);
    window.addEventListener('resize', measure);
    window.addEventListener('scroll', measure, true);
    return () => {
      window.clearInterval(timer);
      window.clearTimeout(firstScroll);
      window.removeEventListener('resize', measure);
      window.removeEventListener('scroll', measure, true);
    };
  }, [current?.target, current?.key, pathname]);

  // Esc skips the tour.
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') finish();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [open, finish]);

  if (!current) return null;
  const isLast = step === GUIDE_TOUR_STEPS.length - 1;

  return (
    <>
      {rect && (
        <div
          aria-hidden
          className="fixed z-[60] pointer-events-none rounded-lg ring-4 ring-blue-500/70 transition-all duration-200"
          style={{
            top: rect.top - 6,
            left: rect.left - 6,
            width: rect.width + 12,
            height: rect.height + 12,
            boxShadow: '0 0 0 9999px rgba(0,0,0,0.35)',
          }}
        />
      )}
      <div
        role="dialog"
        aria-label={t(current.titleKey)}
        className="fixed z-[61] bottom-6 right-6 w-80 max-w-[calc(100vw-2rem)] rounded-xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900 shadow-2xl p-4 text-xs"
      >
        <div className="flex items-center justify-between mb-2">
          <span className="font-mono text-[10px] text-gray-400">
            {t('guideTour.progress', { current: step + 1, total: GUIDE_TOUR_STEPS.length })}
          </span>
          <button className="text-gray-400 hover:text-gray-600 dark:hover:text-gray-200" onClick={finish} aria-label="close">
            ✕
          </button>
        </div>
        <h4 className="text-sm font-semibold text-gray-900 dark:text-gray-100">{t(current.titleKey)}</h4>
        <p className="mt-1.5 text-gray-600 dark:text-gray-300 leading-relaxed">{t(current.descKey)}</p>
        <div className="mt-2 flex gap-1">
          {GUIDE_TOUR_STEPS.map((s, i) => (
            <span key={s.key} className={`h-1 flex-1 rounded-full ${i <= step ? 'bg-blue-500' : 'bg-gray-200 dark:bg-gray-700'}`} />
          ))}
        </div>
        <div className="mt-3 flex items-center justify-between">
          <button className="text-gray-400 hover:text-blue-500 underline underline-offset-2" onClick={finish}>
            {t('guideTour.skip')}
          </button>
          <div className="flex items-center gap-2">
            {step > 0 && (
              <Button size="sm" variant="ghost" onClick={prev}>
                {t('guideTour.prev')}
              </Button>
            )}
            <Button size="sm" variant="primary" onClick={next}>
              {isLast ? t('guideTour.finish') : t('guideTour.next')}
            </Button>
          </div>
        </div>
      </div>
    </>
  );
};
