/**
 * 新用户引导页（Onboarding Guide）。
 *
 * 对应旧前端 `guide` / `guide-pipeline` / `guide-node` 模块：为首次使用平台的
 * 用户提供一条“从零到跑通第一个隐私计算任务”的分步引导路径。
 *
 * 设计要点：
 * - 以“步骤卡片”形式组织核心上手流程：注册节点 → 导入数据表 → 创建项目 →
 *   编排 DAG → 运行并查看结果；
 * - 每个步骤可手动勾选“已完成”，进度持久化到 localStorage，刷新不丢失；
 * - 每个步骤提供“前往”按钮直接跳转对应功能页，降低学习成本；
 * - 顶部展示整体完成进度条，全部完成后给出祝贺态。
 */
import React, { useEffect, useState } from 'react';
import { useNavigate } from '@tanstack/react-router';
import { Card, Button, Badge } from '@secretpad/design-system';
import { useTranslation } from '../../shared/lib/i18n';

/** localStorage 中保存引导进度的键名。 */
const GUIDE_PROGRESS_KEY = 'secretpad-guide-progress';

/** 引导步骤定义。 */
interface GuideStep {
  /** 唯一键，用于持久化完成状态。 */
  key: string;
  /** 图标。 */
  icon: string;
  /** 步骤标题 i18n 键。 */
  titleKey: string;
  /** 步骤描述 i18n 键。 */
  descKey: string;
  /** “前往”按钮跳转的目标路由。 */
  target: string;
  /** “前往”按钮文案 i18n 键。 */
  actionKey: string;
}

/** 引导步骤列表（按上手顺序排列）。 */
const GUIDE_STEPS: GuideStep[] = [
  {
    key: 'node',
    icon: '🖥️',
    titleKey: 'guide.stepNodeTitle',
    descKey: 'guide.stepNodeDesc',
    target: '/nodes',
    actionKey: 'guide.goNodes',
  },
  {
    key: 'data',
    icon: '🗄️',
    titleKey: 'guide.stepDataTitle',
    descKey: 'guide.stepDataDesc',
    target: '/data-tables',
    actionKey: 'guide.goDataTables',
  },
  {
    key: 'project',
    icon: '📁',
    titleKey: 'guide.stepProjectTitle',
    descKey: 'guide.stepProjectDesc',
    target: '/projects',
    actionKey: 'guide.goProjects',
  },
  {
    key: 'dag',
    icon: '⚡',
    titleKey: 'guide.stepDagTitle',
    descKey: 'guide.stepDagDesc',
    target: '/dag',
    actionKey: 'guide.goDag',
  },
  {
    key: 'result',
    icon: '📦',
    titleKey: 'guide.stepResultTitle',
    descKey: 'guide.stepResultDesc',
    target: '/results',
    actionKey: 'guide.goResults',
  },
];

/** 从 localStorage 读取已完成步骤的键集合。 */
function loadProgress(): Set<string> {
  try {
    const raw = localStorage.getItem(GUIDE_PROGRESS_KEY);
    if (!raw) return new Set();
    const arr = JSON.parse(raw);
    return new Set(Array.isArray(arr) ? arr : []);
  } catch {
    return new Set();
  }
}

/** 将已完成步骤集合写回 localStorage。 */
function saveProgress(done: Set<string>): void {
  try {
    localStorage.setItem(GUIDE_PROGRESS_KEY, JSON.stringify([...done]));
  } catch {
    // 存储失败（如隐私模式）时静默忽略，引导仍可在内存中正常使用。
  }
}

export const GuidePage: React.FC = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();

  // 已完成步骤集合（初始从 localStorage 恢复）。
  const [done, setDone] = useState<Set<string>>(() => loadProgress());

  // 进度变化时持久化。
  useEffect(() => {
    saveProgress(done);
  }, [done]);

  /** 切换某步骤的完成状态。 */
  const toggleDone = (key: string) => {
    setDone((prev) => {
      const next = new Set(prev);
      if (next.has(key)) {
        next.delete(key);
      } else {
        next.add(key);
      }
      return next;
    });
  };

  const doneCount = GUIDE_STEPS.filter((s) => done.has(s.key)).length;
  const total = GUIDE_STEPS.length;
  const percent = Math.round((doneCount / total) * 100);
  const allDone = doneCount === total;

  return (
    <div className="space-y-6 max-w-3xl mx-auto">
      {/* 头部：标题与总进度 */}
      <div className="bg-white dark:bg-gray-900 p-6 rounded-xl border border-gray-200 dark:border-gray-800">
        <div className="flex items-center justify-between">
          <div>
            <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">{t('guide.title')}</h2>
            <p className="text-xs text-gray-500 mt-1">{t('guide.subtitle')}</p>
          </div>
          <div className="text-right">
            <div className="text-2xl font-bold text-blue-600">{percent}%</div>
            <div className="text-[11px] text-gray-400">
              {t('guide.progress', { done: doneCount, total })}
            </div>
          </div>
        </div>
        {/* 进度条 */}
        <div className="mt-4 w-full bg-gray-100 dark:bg-gray-800 rounded-full h-2 overflow-hidden">
          <div
            className="bg-gradient-to-r from-blue-500 to-purple-500 h-2 rounded-full transition-all duration-500"
            style={{ width: `${percent}%` }}
          />
        </div>
        {/* 全部完成祝贺 */}
        {allDone && (
          <div className="mt-4 p-3 rounded-lg bg-emerald-50 dark:bg-emerald-950/40 border border-emerald-200 dark:border-emerald-900 text-emerald-700 dark:text-emerald-300 text-xs flex items-center gap-2">
            <span className="text-base">🎉</span>
            {t('guide.allDone')}
          </div>
        )}
      </div>

      {/* 步骤卡片列表 */}
      <div className="space-y-4">
        {GUIDE_STEPS.map((step, idx) => {
          const isDone = done.has(step.key);
          return (
            <Card key={step.key} bodyClassName="p-5">
              <div className="flex items-start gap-4">
                {/* 步骤序号 / 完成态 */}
                <div
                  className={`w-10 h-10 rounded-xl flex items-center justify-center text-lg flex-shrink-0 transition-colors ${
                    isDone
                      ? 'bg-emerald-100 dark:bg-emerald-950/60'
                      : 'bg-blue-50 dark:bg-blue-950/50'
                  }`}
                >
                  {isDone ? '✅' : step.icon}
                </div>

                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="text-[10px] font-mono text-gray-400">
                      {t('guide.stepIndex', { index: idx + 1 })}
                    </span>
                    <h3 className="font-semibold text-sm text-gray-900 dark:text-gray-100">
                      {t(step.titleKey)}
                    </h3>
                    {isDone && (
                      <Badge status="success">
                        <span className="text-[10px]">{t('guide.done')}</span>
                      </Badge>
                    )}
                  </div>
                  <p className="text-xs text-gray-500 mt-1.5 leading-relaxed">{t(step.descKey)}</p>

                  <div className="flex items-center gap-3 mt-3">
                    {/* 前往对应功能页 */}
                    <Button size="sm" variant="primary" onClick={() => navigate({ to: step.target })}>
                      {t(step.actionKey)} →
                    </Button>
                    {/* 手动标记完成 / 撤销 */}
                    <button
                      onClick={() => toggleDone(step.key)}
                      className="text-[11px] text-gray-400 hover:text-blue-500 transition-colors underline underline-offset-2"
                    >
                      {isDone ? t('guide.markUndone') : t('guide.markDone')}
                    </button>
                  </div>
                </div>
              </div>
            </Card>
          );
        })}
      </div>

      {/* 底部：跳转隐私场景体验 */}
      <Card bodyClassName="p-5">
        <div className="flex items-center justify-between">
          <div>
            <h3 className="font-semibold text-sm text-gray-900 dark:text-gray-100">{t('guide.exploreTitle')}</h3>
            <p className="text-xs text-gray-500 mt-1">{t('guide.exploreDesc')}</p>
          </div>
          <Button variant="outline" size="sm" onClick={() => navigate({ to: '/privacy-scenes' })}>
            🛡️ {t('guide.exploreAction')}
          </Button>
        </div>
      </Card>
    </div>
  );
};
