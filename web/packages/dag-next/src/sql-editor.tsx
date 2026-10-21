/**
 * 轻量级 SQL 编辑器组件（SqlEditor）。
 *
 * 对应原版 `config-item-render/default-sql-editor.tsx` 和
 * `custom-render/scql-editor/scql-editor-content/code-editor/index.tsx`。
 * 原版使用 Monaco Editor（~2MB），本组件实现一个零外部依赖的轻量替代：
 *
 * - 行号显示
 * - 基础 SQL 语法高亮（关键字、字符串、注释、数字）
 * - Tab 缩进支持
 * - 全屏切换
 * - 基础 SQL 格式化（关键字大写 + 缩进）
 * - 暗色主题，与 dag-next 整体风格一致
 * - 文案通过 labels 注入
 */
import React, { useRef, useState, useCallback, useMemo } from 'react';

/* -------------------------------------------------------------------------- */
/* 类型定义                                                                    */
/* -------------------------------------------------------------------------- */

export interface SqlEditorLabels {
  /** 全屏。 */
  fullscreen?: string;
  /** 退出全屏。 */
  exitFullscreen?: string;
  /** 格式化。 */
  format?: string;
  /** 复制。 */
  copy?: string;
  /** 已复制。 */
  copied?: string;
  /** 占位文本。 */
  placeholder?: string;
  /** 行号。 */
  lines?: string;
}

export interface SqlEditorProps {
  /** SQL 文本值。 */
  value: string;
  /** 值变化回调。 */
  onChange?: (value: string) => void;
  /** 是否只读。 */
  readOnly?: boolean;
  /** 编辑器高度（px），默认 200。 */
  height?: number;
  /** 文案标签。 */
  labels?: SqlEditorLabels;
  /** 自定义类名。 */
  className?: string;
}

/* -------------------------------------------------------------------------- */
/* SQL 关键字与语法高亮                                                        */
/* -------------------------------------------------------------------------- */

const SQL_KEYWORDS = new Set([
  'SELECT', 'FROM', 'WHERE', 'INSERT', 'INTO', 'VALUES', 'UPDATE', 'SET',
  'DELETE', 'CREATE', 'TABLE', 'DROP', 'ALTER', 'INDEX', 'VIEW', 'JOIN',
  'LEFT', 'RIGHT', 'INNER', 'OUTER', 'FULL', 'CROSS', 'ON', 'AND', 'OR',
  'NOT', 'IN', 'EXISTS', 'BETWEEN', 'LIKE', 'IS', 'NULL', 'AS', 'ORDER',
  'BY', 'GROUP', 'HAVING', 'LIMIT', 'OFFSET', 'UNION', 'ALL', 'DISTINCT',
  'CASE', 'WHEN', 'THEN', 'ELSE', 'END', 'ASC', 'DESC', 'COUNT', 'SUM',
  'AVG', 'MIN', 'MAX', 'CAST', 'COALESCE', 'IF', 'GRANT', 'REVOKE',
  'PRIMARY', 'KEY', 'FOREIGN', 'REFERENCES', 'CONSTRAINT', 'DEFAULT',
  'CHECK', 'UNIQUE', 'WITH', 'RECURSIVE', 'EXPLAIN', 'ANALYZE',
]);

/** SQL 令牌类型。 */
type TokenType = 'keyword' | 'string' | 'comment' | 'number' | 'function' | 'plain';

interface Token {
  text: string;
  type: TokenType;
}

/** 简易 SQL 词法分析器：将 SQL 文本拆分为带类型的令牌数组。 */
function tokenizeSql(line: string): Token[] {
  const tokens: Token[] = [];
  let i = 0;

  while (i < line.length) {
    // 单行注释 --
    if (line[i] === '-' && line[i + 1] === '-') {
      tokens.push({ text: line.slice(i), type: 'comment' });
      break;
    }
    // 字符串（单引号）
    if (line[i] === "'") {
      let end = i + 1;
      while (end < line.length && line[end] !== "'") {
        if (line[end] === '\\') end++; // 跳过转义
        end++;
      }
      end = Math.min(end + 1, line.length);
      tokens.push({ text: line.slice(i, end), type: 'string' });
      i = end;
      continue;
    }
    // 数字
    if (/\d/.test(line[i]) && (i === 0 || /[\s,()=<>!]/.test(line[i - 1]))) {
      let end = i;
      while (end < line.length && /[\d.]/.test(line[end])) end++;
      tokens.push({ text: line.slice(i, end), type: 'number' });
      i = end;
      continue;
    }
    // 标识符/关键字
    if (/[a-zA-Z_]/.test(line[i])) {
      let end = i;
      while (end < line.length && /[a-zA-Z0-9_.]/.test(line[end])) end++;
      const word = line.slice(i, end);
      const upper = word.toUpperCase();
      if (SQL_KEYWORDS.has(upper)) {
        tokens.push({ text: word, type: 'keyword' });
      } else if (end < line.length && line[end] === '(') {
        tokens.push({ text: word, type: 'function' });
      } else {
        tokens.push({ text: word, type: 'plain' });
      }
      i = end;
      continue;
    }
    // 其他字符（运算符、空格等）
    let end = i + 1;
    while (end < line.length && !/[a-zA-Z_\d'"]/.test(line[end]) && !(line[end] === '-' && line[end + 1] === '-')) {
      end++;
    }
    tokens.push({ text: line.slice(i, end), type: 'plain' });
    i = end;
  }

  return tokens;
}

/** 令牌类型 → Tailwind 颜色类名。 */
const TOKEN_COLORS: Record<TokenType, string> = {
  keyword: 'text-blue-400 font-semibold',
  string: 'text-green-400',
  comment: 'text-gray-500 italic',
  number: 'text-amber-400',
  function: 'text-purple-400',
  plain: 'text-gray-200',
};

/* -------------------------------------------------------------------------- */
/* SQL 格式化                                                                  */
/* -------------------------------------------------------------------------- */

/** 简易 SQL 格式化：关键字大写，主要子句换行缩进。 */
function formatSql(sql: string): string {
  const majorClauses = ['SELECT', 'FROM', 'WHERE', 'JOIN', 'LEFT JOIN', 'RIGHT JOIN',
    'INNER JOIN', 'OUTER JOIN', 'GROUP BY', 'ORDER BY', 'HAVING', 'LIMIT',
    'UNION', 'INSERT INTO', 'VALUES', 'UPDATE', 'SET', 'DELETE FROM', 'CREATE TABLE'];

  let formatted = sql.replace(/\s+/g, ' ').trim();

  // 关键字大写
  formatted = formatted.replace(/\b([a-zA-Z_]+)\b/g, (match) => {
    const upper = match.toUpperCase();
    return SQL_KEYWORDS.has(upper) ? upper : match;
  });

  // 主要子句前换行
  for (const clause of majorClauses) {
    const regex = new RegExp(`\\b(${clause.replace(/\s+/g, '\\s+')})\\b`, 'gi');
    formatted = formatted.replace(regex, '\n$1');
  }

  // 缩进子句内容
  const lines = formatted.split('\n').filter((l) => l.trim());
  return lines.map((line) => {
    const trimmed = line.trim();
    const isMajor = majorClauses.some((c) => trimmed.toUpperCase().startsWith(c));
    return isMajor ? trimmed : '  ' + trimmed;
  }).join('\n');
}

/* -------------------------------------------------------------------------- */
/* 组件                                                                        */
/* -------------------------------------------------------------------------- */

export const SqlEditor: React.FC<SqlEditorProps> = ({
  value,
  onChange,
  readOnly = false,
  height = 200,
  labels,
  className = '',
}) => {
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [copied, setCopied] = useState(false);

  const lines = useMemo(() => value.split('\n'), [value]);

  /** 高亮渲染：将每行拆分为带颜色的 span。 */
  const highlightedLines = useMemo(() => {
    return lines.map((line, idx) => {
      const tokens = tokenizeSql(line);
      return (
        <div key={idx} className="flex">
          <span className="inline-block w-8 text-right pr-2 text-gray-600 select-none flex-shrink-0 text-[11px] leading-5">
            {idx + 1}
          </span>
          <span className="flex-1 whitespace-pre leading-5">
            {tokens.map((token, ti) => (
              <span key={ti} className={TOKEN_COLORS[token.type]}>{token.text}</span>
            ))}
            {line === '' && '\u00A0'}
          </span>
        </div>
      );
    });
  }, [lines]);

  /** Tab 键支持：插入两个空格。 */
  const handleKeyDown = useCallback((e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Tab') {
      e.preventDefault();
      if (readOnly) return;
      const textarea = e.currentTarget;
      const start = textarea.selectionStart;
      const end = textarea.selectionEnd;
      const newValue = value.slice(0, start) + '  ' + value.slice(end);
      onChange?.(newValue);
      // 恢复光标位置
      requestAnimationFrame(() => {
        textarea.selectionStart = textarea.selectionEnd = start + 2;
      });
    }
  }, [value, onChange, readOnly]);

  /** 复制内容到剪贴板。 */
  const handleCopy = useCallback(() => {
    navigator.clipboard.writeText(value).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    });
  }, [value]);

  /** 格式化 SQL。 */
  const handleFormat = useCallback(() => {
    if (readOnly) return;
    onChange?.(formatSql(value));
  }, [value, onChange, readOnly]);

  const containerClass = isFullscreen
    ? 'fixed inset-0 z-50 flex flex-col bg-gray-950'
    : `relative flex flex-col rounded-lg border border-gray-700 bg-gray-950 overflow-hidden ${className}`;

  return (
    <div className={containerClass}>
      {/* 工具栏 */}
      <div className="flex items-center justify-between px-2 py-1 bg-gray-900 border-b border-gray-800 text-[10px]">
        <span className="text-gray-500 font-mono">SQL</span>
        <div className="flex items-center gap-1">
          {!readOnly && (
            <button
              onClick={handleFormat}
              className="px-1.5 py-0.5 rounded text-gray-400 hover:text-blue-400 hover:bg-gray-800 transition-colors"
              title={labels?.format ?? 'Format SQL'}
            >
              ✨ {labels?.format ?? 'Format'}
            </button>
          )}
          <button
            onClick={handleCopy}
            className="px-1.5 py-0.5 rounded text-gray-400 hover:text-green-400 hover:bg-gray-800 transition-colors"
            title={labels?.copy ?? 'Copy'}
          >
            {copied ? '✓' : '📋'} {copied ? (labels?.copied ?? 'Copied') : (labels?.copy ?? 'Copy')}
          </button>
          <button
            onClick={() => setIsFullscreen((f) => !f)}
            className="px-1.5 py-0.5 rounded text-gray-400 hover:text-amber-400 hover:bg-gray-800 transition-colors"
            title={isFullscreen ? (labels?.exitFullscreen ?? 'Exit Fullscreen') : (labels?.fullscreen ?? 'Fullscreen')}
          >
            {isFullscreen ? '🡐' : '⛶'}
          </button>
        </div>
      </div>

      {/* 编辑区域：高亮层 + 透明 textarea 叠加 */}
      <div
        className="relative flex-1 overflow-auto font-mono text-xs"
        style={{ height: isFullscreen ? undefined : height }}
      >
        {/* 语法高亮背景层 */}
        <div
          className="absolute inset-0 p-2 pointer-events-none overflow-hidden"
          aria-hidden="true"
        >
          {highlightedLines}
        </div>

        {/* 透明编辑层 */}
        <textarea
          ref={textareaRef}
          value={value}
          onChange={(e) => onChange?.(e.target.value)}
          onKeyDown={handleKeyDown}
          readOnly={readOnly}
          spellCheck={false}
          placeholder={labels?.placeholder ?? 'SELECT * FROM ...'}
          className="absolute inset-0 w-full h-full p-2 pl-10 bg-transparent text-transparent caret-white resize-none outline-none font-mono text-xs leading-5 placeholder-gray-600"
          style={{ tabSize: 2 }}
        />
      </div>

      {/* 状态栏 */}
      <div className="flex items-center justify-between px-2 py-0.5 bg-gray-900 border-t border-gray-800 text-[10px] text-gray-600">
        <span>{lines.length} {labels?.lines ?? 'lines'}</span>
        <span>{value.length} chars</span>
      </div>
    </div>
  );
};
