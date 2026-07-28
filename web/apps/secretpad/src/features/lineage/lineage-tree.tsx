/**
 * 树形血缘可视化组件（Lineage Tree）。
 *
 * 对应旧前端 `data-table-tree` 模块：以“从左到右”的树形图展示数据资产的
 * 血缘关系（如：数据源 → 数据表 → 已授权项目 → 项目下产物）。
 *
 * 设计要点：
 * 1. 数据模型为通用的 `LineageNode`（含 icon / 标题 / 副标题 / 徽标 /
 *    明细键值对 / 子节点），由调用方把业务数据组装成树，组件本身不感知业务；
 * 2. 采用纯 CSS（flex + 绝对定位连接线）绘制父子连线，无需 SVG 或第三方
 *    图表库，保持零依赖、可离线渲染；
 * 3. 连接线规则：父节点右侧引出水平短线，连接到子节点列的竖直主干线；
 *    竖直主干线由每个子节点行各自绘制“上半段/下半段”拼接而成——
 *    首个子节点只画下半段、末个子节点只画上半段、中间子节点画整段，
 *    从而精确地让主干线只 spanning 于“首个子节点中心 ~ 末个子节点中心”；
 * 4. 整棵树包裹在横向滚动容器中，子节点较多时可左右拖动查看。
 */
import React from 'react';

/** 血缘树节点的徽标色调（映射到 Tailwind 颜色类）。 */
export type LineageBadgeTone = 'blue' | 'green' | 'amber' | 'purple' | 'gray';

/** 徽标色调 → Tailwind 类名映射。 */
const BADGE_TONE_CLASS: Record<LineageBadgeTone, string> = {
  blue: 'bg-blue-50 dark:bg-blue-950/60 text-blue-600 dark:text-blue-400 border-blue-200 dark:border-blue-900',
  green: 'bg-emerald-50 dark:bg-emerald-950/60 text-emerald-600 dark:text-emerald-400 border-emerald-200 dark:border-emerald-900',
  amber: 'bg-amber-50 dark:bg-amber-950/60 text-amber-600 dark:text-amber-400 border-amber-200 dark:border-amber-900',
  purple: 'bg-purple-50 dark:bg-purple-950/60 text-purple-600 dark:text-purple-400 border-purple-200 dark:border-purple-900',
  gray: 'bg-gray-100 dark:bg-gray-800 text-gray-500 dark:text-gray-400 border-gray-200 dark:border-gray-700',
};

/** 节点明细行（键值对），展示在卡片下方的次级信息。 */
export interface LineageDetail {
  /** 明细标签（如“关联键”“授权时间”）。 */
  label: string;
  /** 明细值。 */
  value: string;
}

/** 血缘树节点。 */
export interface LineageNode {
  /** 唯一键（用于 React key）。 */
  id: string;
  /** 节点图标（emoji 或字符）。 */
  icon?: string;
  /** 节点主标题（如数据表名 / 项目名）。 */
  title: string;
  /** 节点副标题（如 ID / 类型说明），等宽字体展示。 */
  subtitle?: string;
  /** 徽标文本（如计算模式 / 数据源类型）。 */
  badge?: string;
  /** 徽标色调。 */
  badgeTone?: LineageBadgeTone;
  /** 明细键值对列表。 */
  details?: LineageDetail[];
  /** 子节点列表。 */
  children?: LineageNode[];
}

/** 连接线的统一颜色类（抽出常量保证父子连线颜色一致）。 */
const LINE_CLASS = 'bg-gray-300 dark:bg-gray-700';

/**
 * 单个节点卡片。
 *
 * 紧凑布局：图标 + 标题 + 徽标在首行，副标题（等宽）在次行，
 * 明细键值对以“标签: 值”形式列在下方。
 */
const NodeCard: React.FC<{ node: LineageNode }> = ({ node }) => (
  <div className="min-w-[180px] max-w-[240px] rounded-lg border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-850 shadow-sm px-3 py-2 flex-shrink-0">
    {/* 首行：图标 + 标题 + 徽标 */}
    <div className="flex items-center gap-2">
      {node.icon && <span className="text-base leading-none">{node.icon}</span>}
      <span className="font-semibold text-xs text-gray-900 dark:text-gray-100 truncate">{node.title}</span>
      {node.badge && (
        <span
          className={`ml-auto px-1.5 py-0.5 rounded border text-[10px] font-semibold whitespace-nowrap ${
            BADGE_TONE_CLASS[node.badgeTone ?? 'gray']
          }`}
        >
          {node.badge}
        </span>
      )}
    </div>
    {/* 副标题（ID / 类型） */}
    {node.subtitle && <div className="text-[10px] text-gray-400 font-mono mt-0.5 truncate">{node.subtitle}</div>}
    {/* 明细键值对 */}
    {node.details && node.details.length > 0 && (
      <div className="mt-1.5 space-y-0.5 border-t border-gray-100 dark:border-gray-800 pt-1.5">
        {node.details.map((d, i) => (
          <div key={i} className="flex items-start gap-1 text-[10px] leading-snug">
            <span className="text-gray-400 whitespace-nowrap">{d.label}:</span>
            <span className="text-gray-600 dark:text-gray-300 break-all">{d.value}</span>
          </div>
        ))}
      </div>
    )}
  </div>
);

/**
 * 递归渲染一个节点及其子树。
 *
 * 布局：`[节点卡片] — [水平短线] — [子节点列]`。
 * 子节点列中每个子节点行绘制自己的连接线段，拼接成完整的主干线。
 */
const TreeBranch: React.FC<{ node: LineageNode }> = ({ node }) => {
  const children = node.children ?? [];
  const hasChildren = children.length > 0;

  return (
    <div className="flex items-center">
      {/* 当前节点卡片 */}
      <NodeCard node={node} />

      {hasChildren && (
        <>
          {/* 父节点右侧引出的水平短线，连接到子节点列的竖直主干线 */}
          <div className={`w-6 h-px ${LINE_CLASS} flex-shrink-0`} />

          {/* 子节点列：竖直方向排布 */}
          <div className="flex flex-col">
            {children.map((child, i) => {
              const isFirst = i === 0;
              const isLast = i === children.length - 1;
              return (
                <div key={child.id} className="flex items-stretch">
                  {/* 连接线单元格：宽 6（24px），内含竖直主干线段 + 水平短线 */}
                  <div className="relative w-6 flex-shrink-0">
                    {/* 竖直主干上半段：非首个子节点才绘制（从行顶部到行中心） */}
                    {!isFirst && <div className={`absolute left-0 top-0 h-1/2 w-px ${LINE_CLASS}`} />}
                    {/* 竖直主干下半段：非末个子节点才绘制（从行中心到行底部） */}
                    {!isLast && <div className={`absolute left-0 bottom-0 h-1/2 w-px ${LINE_CLASS}`} />}
                    {/* 水平短线：从竖直主干连接到子节点卡片左缘（位于行中心） */}
                    <div className={`absolute left-0 top-1/2 w-full h-px ${LINE_CLASS}`} />
                  </div>
                  {/* 子节点（递归），上下留白避免卡片紧贴 */}
                  <div className="py-1.5">
                    <TreeBranch node={child} />
                  </div>
                </div>
              );
            })}
          </div>
        </>
      )}
    </div>
  );
};

/** LineageTree 组件属性。 */
export interface LineageTreeProps {
  /** 树根节点。 */
  root: LineageNode;
  /** 空数据提示文案（当 root 无子节点且需要提示时由调用方决定，此处仅渲染树）。 */
  className?: string;
}

/**
 * 树形血缘可视化入口组件。
 *
 * 外层提供横向滚动容器，内部递归渲染 `TreeBranch`。
 */
export const LineageTree: React.FC<LineageTreeProps> = ({ root, className = '' }) => (
  <div className={`overflow-x-auto pb-2 ${className}`}>
    <div className="inline-block min-w-full">
      <TreeBranch node={root} />
    </div>
  </div>
);
