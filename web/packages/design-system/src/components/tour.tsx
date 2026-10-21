import React, { useEffect, useState } from 'react';

/**
 * Tour 轻量引导（零依赖）。
 *
 * - steps[i].target 为 CSS 选择器（推荐 `[data-tour="xxx"]`），找到时高亮并在其旁显示卡片；
 *   找不到时卡片居中显示；
 * - 受控：open / onClose；current 步由组件内部维护，onFinish 在最后一步完成时调用；
 * - 可配合 `isTourSeen / markTourSeen` 做“仅首次自动展示”。
 */
export interface TourStep {
  target?: string;
  title: React.ReactNode;
  content?: React.ReactNode;
}

export interface TourProps {
  open: boolean;
  steps: TourStep[];
  onClose: () => void;
  onFinish?: () => void;
  labels?: { prev?: string; next?: string; finish?: string; skip?: string };
}

interface Rect {
  top: number;
  left: number;
  width: number;
  height: number;
}

function measure(target?: string): Rect | null {
  if (!target || typeof document === 'undefined') return null;
  let el: Element | null;
  try {
    el = document.querySelector(target);
  } catch {
    return null;
  }
  if (!el) return null;
  const r = el.getBoundingClientRect();
  if (!r.width && !r.height) return null;
  return { top: r.top, left: r.left, width: r.width, height: r.height };
}

export const Tour: React.FC<TourProps> = ({ open, steps, onClose, onFinish, labels = {} }) => {
  const [index, setIndex] = useState(0);
  const [rect, setRect] = useState<Rect | null>(null);
  const step = steps[index];

  useEffect(() => {
    if (open) setIndex(0);
  }, [open]);

  useEffect(() => {
    if (!open || !step) return;
    const update = () => setRect(measure(step.target));
    update();
    const timer = window.setInterval(update, 300);
    window.addEventListener('resize', update);
    return () => {
      window.clearInterval(timer);
      window.removeEventListener('resize', update);
    };
  }, [open, step]);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [open, onClose]);

  if (!open || !step) return null;
  const last = index === steps.length - 1;
  const viewport =
    typeof window !== 'undefined'
      ? { width: window.innerWidth, height: window.innerHeight }
      : { width: 1200, height: 800 };
  const cardWidth = Math.min(320, viewport.width - 24);
  const cardStyle: React.CSSProperties = rect
    ? {
        top: Math.min(rect.top + rect.height + 12, viewport.height - 200),
        left: Math.max(12, Math.min(rect.left, viewport.width - cardWidth - 12)),
      }
    : { top: '40%', left: '50%', transform: 'translate(-50%, -50%)' };
  // 超高/超宽目标（如移动端列表表格）只高亮视口内可见部分。
  const highlight = rect
    ? {
        top: Math.max(8, rect.top - 4),
        left: Math.max(8, rect.left - 4),
        right: Math.min(viewport.width - 8, rect.left + rect.width + 4),
        bottom: Math.min(viewport.height - 8, rect.top + rect.height + 4),
      }
    : null;

  return (
    <div className="fixed inset-0 z-[60] pointer-events-none" role="dialog" aria-modal="false" aria-label="tour">
      {highlight && (
        <div
          className="absolute rounded-lg ring-4 ring-blue-500/70 shadow-[0_0_0_9999px_rgba(0,0,0,0.45)] transition-all"
          style={{
            top: highlight.top,
            left: highlight.left,
            width: Math.max(0, highlight.right - highlight.left),
            height: Math.max(0, highlight.bottom - highlight.top),
          }}
        />
      )}
      {!rect && <div className="absolute inset-0 bg-black/40" />}
      <div className="absolute w-[calc(100vw-1.5rem)] max-w-80 max-h-[calc(100dvh-2rem)] overflow-y-auto pointer-events-auto rounded-xl bg-white dark:bg-gray-900 border border-gray-200 dark:border-gray-700 shadow-2xl p-4 text-sm" style={cardStyle}>
        <div className="flex items-start justify-between gap-2">
          <div className="font-semibold text-gray-900 dark:text-gray-100">{step.title}</div>
          <span className="text-xs text-gray-400">
            {index + 1}/{steps.length}
          </span>
        </div>
        {step.content && <div className="mt-2 text-xs text-gray-600 dark:text-gray-300 leading-relaxed">{step.content}</div>}
        <div className="mt-3 flex items-center justify-between">
          <button type="button" className="text-xs text-gray-400 hover:text-gray-600" onClick={onClose}>
            {labels.skip ?? 'Skip'}
          </button>
          <div className="flex gap-2">
            {index > 0 && (
              <button type="button" className="px-2.5 py-1 text-xs rounded-lg border border-gray-200 dark:border-gray-700" onClick={() => setIndex((i) => i - 1)}>
                {labels.prev ?? 'Prev'}
              </button>
            )}
            <button
              type="button"
              className="px-2.5 py-1 text-xs rounded-lg bg-blue-600 text-white"
              onClick={() => {
                if (last) {
                  onFinish?.();
                  onClose();
                } else setIndex((i) => i + 1);
              }}
            >
              {last ? labels.finish ?? 'Done' : labels.next ?? 'Next'}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
};

export function isTourSeen(key: string): boolean {
  try {
    return localStorage.getItem(`secretpad-tour-${key}`) === '1';
  } catch {
    return false;
  }
}

export function markTourSeen(key: string): void {
  try {
    localStorage.setItem(`secretpad-tour-${key}`, '1');
  } catch {
    /* storage unavailable */
  }
}
