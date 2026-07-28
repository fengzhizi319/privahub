/**
 * DAG 日志查看器（LogViewer）—— Monaco 风格的轻量级实现。
 *
 * 对应迁移需求“DAG 日志 Monaco 查看器”。考虑到平台支持离线 / 内网部署，
 * 直接引入 `monaco-editor`（约 5MB+）会显著增大体积并带来 worker 加载问题，
 * 因此这里实现一个**自包含、零外部依赖**的 Monaco 风格日志查看器，提供
 * 与 Monaco 相近的核心阅读体验：
 *
 * - 行号列（只读、右对齐、与内容滚动同步）；
 * - 日志级别高亮（ERROR 红 / WARN 黄 / INFO 蓝 / DEBUG 灰）；
 * - 关键字搜索（实时过滤 + 匹配片段高亮）；
 * - 级别筛选（ALL / ERROR / WARN / INFO / DEBUG）；
 * - 自动滚动到底部（新日志到达时跟随，可开关）；
 * - 自动换行开关、字号增减、一键复制全部日志。
 *
 * 该组件位于 `@secretpad/dag-next` 包内，文案通过 `labels` 注入，保持可复用性。
 */
import React, { useEffect, useMemo, useRef, useState } from 'react';
import { Button } from '@secretpad/design-system';

/** 日志级别枚举（用于筛选与着色）。 */
type LogLevel = 'ERROR' | 'WARN' | 'INFO' | 'DEBUG';

/** 级别筛选选项（ALL 表示不过滤）。 */
type LevelFilter = 'ALL' | LogLevel;

/** 查看器文案标签。 */
export interface LogViewerLabels {
  searchPlaceholder?: string;
  copy?: string;
  copied?: string;
  wrap?: string;
  autoScroll?: string;
  empty?: string;
  lines?: string;
  zoomIn?: string;
  zoomOut?: string;
}

export interface LogViewerProps {
  /** 日志行数组（每个元素为一行原始文本）。 */
  logs: string[];
  /** 是否处于加载中。 */
  loading?: boolean;
  /** 空日志时的占位文案。 */
  emptyText?: string;
  labels?: LogViewerLabels;
}

/** 根据日志行内容推断其级别（按常见日志格式的关键字匹配）。 */
function detectLevel(line: string): LogLevel | null {
  const upper = line.toUpperCase();
  if (upper.includes('ERROR') || upper.includes('FATAL') || upper.includes('EXCEPTION') || upper.includes('TRACEBACK')) {
    return 'ERROR';
  }
  if (upper.includes('WARN')) return 'WARN';
  if (upper.includes('DEBUG') || upper.includes('TRACE')) return 'DEBUG';
  if (upper.includes('INFO')) return 'INFO';
  return null;
}

/** 各级别对应的文本颜色（Tailwind 类名）。 */
const LEVEL_COLOR: Record<LogLevel, string> = {
  ERROR: 'text-red-400',
  WARN: 'text-amber-300',
  INFO: 'text-sky-300',
  DEBUG: 'text-gray-500',
};

/** 级别筛选按钮的展示顺序。 */
const LEVEL_OPTIONS: LevelFilter[] = ['ALL', 'ERROR', 'WARN', 'INFO', 'DEBUG'];

/**
 * 渲染单行日志，并对搜索关键字做高亮切分。
 *
 * 将一行按关键字切成 [普通, 匹配, 普通, ...] 片段，匹配片段用黄色背景标注，
 * 其余部分保持级别着色。关键字为空时直接整行渲染。
 */
const HighlightedLine: React.FC<{ line: string; keyword: string; colorClass: string }> = ({ line, keyword, colorClass }) => {
  // 无关键字时直接渲染整行。
  if (!keyword) {
    return <span className={colorClass}>{line}</span>;
  }
  // 按关键字（不区分大小写）切分，保留分隔符。
  const lower = line.toLowerCase();
  const kw = keyword.toLowerCase();
  const parts: React.ReactNode[] = [];
  let cursor = 0;
  let idx = lower.indexOf(kw);
  let key = 0;
  while (idx !== -1) {
    if (idx > cursor) parts.push(<span key={key++} className={colorClass}>{line.slice(cursor, idx)}</span>);
    parts.push(
      <mark key={key++} className="bg-yellow-500/40 text-yellow-100 rounded-sm px-0.5">
        {line.slice(idx, idx + kw.length)}
      </mark>
    );
    cursor = idx + kw.length;
    idx = lower.indexOf(kw, cursor);
  }
  if (cursor < line.length) parts.push(<span key={key} className={colorClass}>{line.slice(cursor)}</span>);
  return <>{parts}</>;
};

export const LogViewer: React.FC<LogViewerProps> = ({ logs, loading = false, emptyText, labels = {} }) => {
  // 搜索关键字。
  const [keyword, setKeyword] = useState('');
  // 级别筛选。
  const [levelFilter, setLevelFilter] = useState<LevelFilter>('ALL');
  // 是否自动滚动到底部。
  const [autoScroll, setAutoScroll] = useState(true);
  // 是否自动换行。
  const [wrap, setWrap] = useState(false);
  // 字号（px），可通过 +/- 调整。
  const [fontSize, setFontSize] = useState(11);
  // 复制成功提示的短暂状态。
  const [copied, setCopied] = useState(false);

  // 滚动容器引用，用于自动滚动到底部。
  const scrollRef = useRef<HTMLDivElement>(null);

  /**
   * 过滤后的日志行（带原始行号）。
   *
   * 先按级别过滤，再按关键字过滤；保留原始行号以便在行号列中展示真实位置。
   * 使用 useMemo 避免每次渲染重复遍历大日志数组。
   */
  const filteredLines = useMemo(() => {
    const kw = keyword.trim().toLowerCase();
    const result: Array<{ no: number; text: string; level: LogLevel | null }> = [];
    for (let i = 0; i < logs.length; i++) {
      const text = logs[i];
      const level = detectLevel(text);
      // 级别过滤：ALL 不过滤；否则仅保留匹配级别（无级别的行在筛选时视为不匹配）。
      if (levelFilter !== 'ALL' && level !== levelFilter) continue;
      // 关键字过滤（不区分大小写）。
      if (kw && !text.toLowerCase().includes(kw)) continue;
      result.push({ no: i + 1, text, level });
    }
    return result;
  }, [logs, keyword, levelFilter]);

  // 日志更新且开启自动滚动时，滚动到底部。
  useEffect(() => {
    if (autoScroll && scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [filteredLines, autoScroll]);

  /** 复制全部（未过滤）日志到剪贴板。 */
  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(logs.join('\n'));
      setCopied(true);
      // 2 秒后恢复“复制”文案。
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // 剪贴板不可用时静默忽略。
    }
  };

  const isEmpty = logs.length === 0;

  return (
    <div className="flex flex-col h-full min-h-0">
      {/* 工具栏：搜索 / 级别筛选 / 换行 / 自动滚动 / 字号 / 复制 */}
      <div className="flex flex-wrap items-center gap-2 pb-2">
        {/* 关键字搜索框 */}
        <input
          type="text"
          value={keyword}
          onChange={(e) => setKeyword(e.target.value)}
          placeholder={labels.searchPlaceholder ?? '搜索日志...'}
          className="flex-1 min-w-[140px] px-2.5 py-1.5 rounded-lg bg-gray-900 border border-gray-700 text-gray-200 text-[11px] font-mono focus:outline-none focus:border-blue-500"
        />

        {/* 级别筛选按钮组 */}
        <div className="flex items-center rounded-lg border border-gray-700 overflow-hidden">
          {LEVEL_OPTIONS.map((lv) => (
            <button
              key={lv}
              onClick={() => setLevelFilter(lv)}
              className={`px-2 py-1.5 text-[10px] font-mono transition-colors ${
                levelFilter === lv
                  ? 'bg-blue-600 text-white'
                  : 'bg-gray-900 text-gray-400 hover:text-gray-200'
              }`}
            >
              {lv}
            </button>
          ))}
        </div>

        {/* 自动换行开关 */}
        <button
          onClick={() => setWrap((v) => !v)}
          title={labels.wrap ?? '自动换行'}
          className={`px-2 py-1.5 rounded-lg text-[10px] font-mono border transition-colors ${
            wrap ? 'bg-blue-600 border-blue-600 text-white' : 'bg-gray-900 border-gray-700 text-gray-400 hover:text-gray-200'
          }`}
        >
          ⏎
        </button>

        {/* 自动滚动开关 */}
        <button
          onClick={() => setAutoScroll((v) => !v)}
          title={labels.autoScroll ?? '自动滚动到底部'}
          className={`px-2 py-1.5 rounded-lg text-[10px] font-mono border transition-colors ${
            autoScroll ? 'bg-blue-600 border-blue-600 text-white' : 'bg-gray-900 border-gray-700 text-gray-400 hover:text-gray-200'
          }`}
        >
          ⇩
        </button>

        {/* 字号减小 / 增大 */}
        <button
          onClick={() => setFontSize((s) => Math.max(9, s - 1))}
          title={labels.zoomOut ?? '缩小字号'}
          className="px-2 py-1.5 rounded-lg text-[10px] font-mono bg-gray-900 border border-gray-700 text-gray-400 hover:text-gray-200"
        >
          A-
        </button>
        <button
          onClick={() => setFontSize((s) => Math.min(18, s + 1))}
          title={labels.zoomIn ?? '放大字号'}
          className="px-2 py-1.5 rounded-lg text-[10px] font-mono bg-gray-900 border border-gray-700 text-gray-400 hover:text-gray-200"
        >
          A+
        </button>

        {/* 复制全部 */}
        <Button size="sm" variant="ghost" onClick={handleCopy} disabled={isEmpty}>
          {copied ? (labels.copied ?? '已复制 ✓') : (labels.copy ?? '复制')}
        </Button>
      </div>

      {/* 日志主体：左侧行号列 + 右侧内容列，二者共用同一滚动容器保证同步。 */}
      <div
        ref={scrollRef}
        className="flex-1 min-h-0 overflow-auto rounded-lg bg-gray-950 border border-gray-800"
      >
        {loading && <div className="p-3 text-[11px] text-gray-500 font-mono">...</div>}

        {!loading && isEmpty && (
          <div className="p-3 text-[11px] text-gray-500 font-mono">{emptyText ?? labels.empty ?? '暂无日志'}</div>
        )}

        {!loading && !isEmpty && (
          <table className="w-full border-collapse">
            <tbody>
              {filteredLines.map((line) => {
                // 无级别行使用默认灰色；有级别行按级别着色。
                const colorClass = line.level ? LEVEL_COLOR[line.level] : 'text-gray-300';
                return (
                  <tr key={line.no} className="hover:bg-gray-900/60">
                    {/* 行号列：右对齐、暗色、选中态高亮 */}
                    <td className="select-none text-right pr-3 pl-3 align-top text-gray-600 font-mono border-r border-gray-800/60 w-10 sticky left-0 bg-gray-950">
                      <span style={{ fontSize }}>{line.no}</span>
                    </td>
                    {/* 内容列：等宽字体，按 wrap 决定是否换行 */}
                    <td className="pl-3 pr-3 align-top font-mono">
                      <div
                        style={{ fontSize }}
                        className={`leading-relaxed ${wrap ? 'whitespace-pre-wrap break-all' : 'whitespace-pre'}`}
                      >
                        <HighlightedLine line={line.text} keyword={keyword.trim()} colorClass={colorClass} />
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>

      {/* 状态栏：匹配行数 / 总行数 */}
      <div className="flex items-center justify-between pt-1.5 text-[10px] text-gray-500 font-mono">
        <span>
          {labels.lines ?? '行数'}: {filteredLines.length} / {logs.length}
        </span>
        {keyword && <span>“{keyword}” → {filteredLines.length}</span>}
      </div>
    </div>
  );
};
