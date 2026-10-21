/**
 * DAG 执行记录时间线组件（ExecutionTimeline）。
 *
 * 对应原版 `modules/pipeline-record-list/record-list.tsx` 中的运行记录列表。
 * 原版使用 Ant Design List + Progress 展示作业执行历史，本组件实现一个
 * 自包含、零外部依赖的时间线视图：
 *
 * - 垂直时间线布局，每条记录显示状态图标、时间、耗时、进度；
 * - 支持 Running/Succeed/Failed/Stopped 四种状态；
 * - 支持点击选中记录、停止运行中的作业；
 * - 支持分页加载；
 * - 文案通过 labels 注入，保持可复用性。
 */
import React, { useState } from 'react';
import { NODE_STATUS } from './status';

/* -------------------------------------------------------------------------- */
/* 类型定义                                                                    */
/* -------------------------------------------------------------------------- */

/** 作业执行状态。 */
/** 记录状态：后端 job 状态（dag-next/status）+ 'PENDING'（未开始）。 */
export type JobStatus = typeof NODE_STATUS.RUNNING | typeof NODE_STATUS.SUCCEED | typeof NODE_STATUS.FAILED | typeof NODE_STATUS.STOPPED | 'PENDING';

/** 单条执行记录。 */
export interface ExecutionRecord {
  jobId: string;
  status: JobStatus;
  /** 创建时间（ISO 字符串或时间戳）。 */
  gmtCreate: string;
  /** 完成时间（可选）。 */
  gmtFinished?: string;
  /** 总任务数。 */
  taskCount: number;
  /** 已完成任务数。 */
  finishedTaskCount: number;
  /** 错误信息（可选）。 */
  errMsg?: string;
}

/** 分页信息。 */
export interface PaginationInfo {
  current: number;
  pageSize: number;
  total: number;
}

export interface ExecutionTimelineLabels {
  /** 标题。 */
  title?: string;
  /** 无记录占位。 */
  empty?: string;
  /** 加载中。 */
  loading?: string;
  /** 停止按钮。 */
  stop?: string;
  /** 确认停止。 */
  confirmStop?: string;
  /** 耗时前缀。 */
  duration?: string;
  /** 进度。 */
  progress?: string;
  /** 上一页。 */
  prev?: string;
  /** 下一页。 */
  next?: string;
  /** 错误信息。 */
  error?: string;
}

export interface ExecutionTimelineProps {
  /** 执行记录列表。 */
  records: ExecutionRecord[];
  /** 分页信息。 */
  pagination?: PaginationInfo;
  /** 当前选中的 jobId。 */
  selectedJobId?: string;
  /** 是否加载中。 */
  loading?: boolean;
  /** 文案标签。 */
  labels?: ExecutionTimelineLabels;
  /** 选中记录回调。 */
  onSelect?: (jobId: string) => void;
  /** 停止作业回调。 */
  onStop?: (jobId: string) => void | Promise<void>;
  /** 分页变化回调。 */
  onPageChange?: (page: number) => void;
}

/* -------------------------------------------------------------------------- */
/* 工具函数                                                                    */
/* -------------------------------------------------------------------------- */

/** 格式化时间戳为可读字符串。 */
function formatTime(value: string): string {
  if (!value) return '-';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  });
}

/** 计算耗时。 */
function getDuration(start: string, end?: string): string {
  if (!start) return '-';
  const startTime = new Date(start).getTime();
  const endTime = end ? new Date(end).getTime() : Date.now();
  if (Number.isNaN(startTime) || Number.isNaN(endTime)) return '-';

  const diff = Math.max(0, endTime - startTime);
  const hours = Math.floor(diff / 3600000);
  const minutes = Math.floor((diff % 3600000) / 60000);
  const seconds = Math.floor((diff % 60000) / 1000);

  if (hours > 0) return `${hours}h ${minutes}m ${seconds}s`;
  if (minutes > 0) return `${minutes}m ${seconds}s`;
  return `${seconds}s`;
}

/** 获取状态图标和颜色。 */
function getStatusVisual(status: JobStatus): { icon: string; color: string; bgColor: string } {
  switch (status) {
    case NODE_STATUS.SUCCEED:
      return { icon: '✓', color: 'text-green-400', bgColor: 'bg-green-500' };
    case NODE_STATUS.FAILED:
      return { icon: '✕', color: 'text-red-400', bgColor: 'bg-red-500' };
    case NODE_STATUS.STOPPED:
      return { icon: '⏹', color: 'text-orange-400', bgColor: 'bg-orange-500' };
    case NODE_STATUS.RUNNING:
      return { icon: '▶', color: 'text-cyan-400', bgColor: 'bg-cyan-500' };
    default:
      return { icon: '○', color: 'text-gray-400', bgColor: 'bg-gray-500' };
  }
}

/* -------------------------------------------------------------------------- */
/* 子组件                                                                      */
/* -------------------------------------------------------------------------- */

/** 状态图标。 */
const StatusIcon: React.FC<{ status: JobStatus; progress?: number }> = ({ status, progress }) => {
  const visual = getStatusVisual(status);

  if (status === NODE_STATUS.RUNNING && progress !== undefined) {
    // 运行中显示进度环
    const size = 16;
    const strokeWidth = 2;
    const radius = (size - strokeWidth) / 2;
    const circumference = 2 * Math.PI * radius;
    const offset = circumference * (1 - progress / 100);

    return (
      <svg width={size} height={size} className="animate-spin-slow">
        <circle
          cx={size / 2}
          cy={size / 2}
          r={radius}
          fill="none"
          stroke="#374151"
          strokeWidth={strokeWidth}
        />
        <circle
          cx={size / 2}
          cy={size / 2}
          r={radius}
          fill="none"
          stroke="#06b6d4"
          strokeWidth={strokeWidth}
          strokeDasharray={circumference}
          strokeDashoffset={offset}
          strokeLinecap="round"
          transform={`rotate(-90 ${size / 2} ${size / 2})`}
        />
      </svg>
    );
  }

  return (
    <span
      className={`inline-flex items-center justify-center w-4 h-4 rounded-full text-[10px] font-bold text-white ${visual.bgColor}`}
    >
      {visual.icon}
    </span>
  );
};

/** 单条记录项。 */
const RecordItem: React.FC<{
  record: ExecutionRecord;
  isSelected: boolean;
  labels: ExecutionTimelineLabels;
  onSelect?: (jobId: string) => void;
  onStop?: (jobId: string) => void | Promise<void>;
}> = ({ record, isSelected, labels, onSelect, onStop }) => {
  const [isStopping, setIsStopping] = useState(false);
  const progress = record.taskCount > 0 ? Math.round((record.finishedTaskCount / record.taskCount) * 100) : 0;

  const handleStop = async (e: React.MouseEvent) => {
    e.stopPropagation();
    if (!onStop || isStopping) return;
    setIsStopping(true);
    try {
      await onStop(record.jobId);
    } finally {
      setIsStopping(false);
    }
  };

  return (
    <div
      onClick={() => onSelect?.(record.jobId)}
      className={`relative pl-6 pb-4 cursor-pointer group ${isSelected ? 'bg-gray-800/50 -mx-2 px-2 rounded-lg' : ''}`}
    >
      {/* 时间线连接线 */}
      <div className="absolute left-[7px] top-5 bottom-0 w-px bg-gray-700" />

      {/* 状态图标 */}
      <div className="absolute left-0 top-1">
        <StatusIcon status={record.status} progress={progress} />
      </div>

      {/* 内容 */}
      <div className="ml-2">
        <div className="flex items-center justify-between gap-2">
          <span className="text-[11px] font-mono text-gray-300 truncate">{record.jobId.slice(0, 12)}...</span>
          <span className={`text-[10px] font-medium ${getStatusVisual(record.status).color}`}>
            {record.status}
          </span>
        </div>

        <div className="flex items-center gap-3 mt-1 text-[10px] text-gray-500">
          <span>{formatTime(record.gmtCreate)}</span>
          <span>
            {labels.duration ?? '耗时'}: {getDuration(record.gmtCreate, record.gmtFinished)}
          </span>
        </div>

        {/* 进度条（运行中） */}
        {record.status === NODE_STATUS.RUNNING && (
          <div className="mt-1.5 flex items-center gap-2">
            <div className="flex-1 h-1 bg-gray-700 rounded-full overflow-hidden">
              <div
                className="h-full bg-cyan-500 rounded-full transition-all duration-500"
                style={{ width: `${progress}%` }}
              />
            </div>
            <span className="text-[9px] text-cyan-400">{progress}%</span>
          </div>
        )}

        {/* 错误信息 */}
        {record.status === NODE_STATUS.FAILED && record.errMsg && (
          <div className="mt-1 text-[9px] text-red-400 bg-red-950/30 px-2 py-1 rounded truncate">
            {labels.error ?? '错误'}: {record.errMsg}
          </div>
        )}

        {/* 停止按钮（运行中） */}
        {record.status === NODE_STATUS.RUNNING && onStop && (
          <button
            onClick={handleStop}
            disabled={isStopping}
            className="mt-1.5 px-2 py-0.5 text-[9px] text-red-400 border border-red-800 rounded hover:bg-red-950/50 disabled:opacity-50 opacity-0 group-hover:opacity-100 transition-opacity"
          >
            {isStopping ? '...' : labels.stop ?? '停止'}
          </button>
        )}
      </div>
    </div>
  );
};

/* -------------------------------------------------------------------------- */
/* 主组件                                                                      */
/* -------------------------------------------------------------------------- */

/**
 * DAG 执行记录时间线。
 *
 * 展示作业执行历史的垂直时间线视图，支持状态图标、进度显示、
 * 停止作业、分页等功能。
 */
export const ExecutionTimeline: React.FC<ExecutionTimelineProps> = ({
  records,
  pagination,
  selectedJobId,
  loading = false,
  labels = {},
  onSelect,
  onStop,
  onPageChange,
}) => {
  const totalPages = pagination ? Math.ceil(pagination.total / pagination.pageSize) : 1;

  return (
    <div className="flex flex-col h-full">
      {/* 标题 */}
      <div className="text-xs font-semibold text-gray-300 mb-3 flex items-center gap-2">
        <span className="w-1 h-3 bg-blue-500 rounded-full" />
        {labels.title ?? '执行记录'}
        {pagination && (
          <span className="text-[10px] text-gray-500 font-normal">
            ({pagination.total})
          </span>
        )}
      </div>

      {/* 加载中 */}
      {loading && (
        <div className="text-[10px] text-gray-500 py-4 text-center">
          {labels.loading ?? '加载中...'}
        </div>
      )}

      {/* 空状态 */}
      {!loading && records.length === 0 && (
        <div className="text-[10px] text-gray-600 py-8 text-center">
          {labels.empty ?? '暂无执行记录'}
        </div>
      )}

      {/* 时间线列表 */}
      {!loading && records.length > 0 && (
        <div className="flex-1 overflow-y-auto pr-1">
          {records.map((record) => (
            <RecordItem
              key={record.jobId}
              record={record}
              isSelected={selectedJobId === record.jobId}
              labels={labels}
              onSelect={onSelect}
              onStop={onStop}
            />
          ))}
        </div>
      )}

      {/* 分页 */}
      {pagination && totalPages > 1 && (
        <div className="flex items-center justify-between mt-3 pt-2 border-t border-gray-800">
          <button
            onClick={() => onPageChange?.(pagination.current - 1)}
            disabled={pagination.current <= 1}
            className="px-2 py-1 text-[10px] text-gray-400 hover:text-gray-200 disabled:opacity-30 disabled:cursor-not-allowed"
          >
            ← {labels.prev ?? '上一页'}
          </button>
          <span className="text-[10px] text-gray-500">
            {pagination.current} / {totalPages}
          </span>
          <button
            onClick={() => onPageChange?.(pagination.current + 1)}
            disabled={pagination.current >= totalPages}
            className="px-2 py-1 text-[10px] text-gray-400 hover:text-gray-200 disabled:opacity-30 disabled:cursor-not-allowed"
          >
            {labels.next ?? '下一页'} →
          </button>
        </div>
      )}
    </div>
  );
};

ExecutionTimeline.displayName = 'ExecutionTimeline';
