/**
 * Binning 修改器 —— 对应原版 binning-modification/index.tsx
 *
 * 提供 WOE 分箱结果的可视化编辑：
 * - 表格展示各特征的分箱详情（区间、WOE 值、样本数）
 * - 合并选中分箱（Merge）
 * - 撤销/重做操作
 * - 重置到初始状态
 * - 导出 CSV
 * - 保存修改后的分箱规则
 *
 * 零外部依赖，Tailwind CSS 样式。
 */
import React, { useCallback, useRef, useState } from 'react';
import { binModificationsUnSerializer } from './custom-serializers';

/* -------------------------------------------------------------------------- */
/* 类型定义（对应原版 types.ts）                                               */
/* -------------------------------------------------------------------------- */

/** 单个分箱。 */
export interface Bin {
  key: string;
  label: string;
  markForMerge: boolean;
  totalCount: number;
  woe?: number;
  order?: number;
}

/** 特征分箱记录。 */
export interface BinningRecord {
  key: string;
  partyName: string;
  feature: string;
  type: string;
  isWoe: boolean;
  binCount: number;
  iv?: number;
  bins: Bin[];
}

/** 分箱数据（含模型哈希）。 */
export interface BinningData {
  modelHash: string;
  variableBins: BinningRecord[];
}

/** 组件文案标签。 */
export interface BinningModificationLabels {
  title?: string;
  feature?: string;
  type?: string;
  binCount?: string;
  iv?: string;
  binLabel?: string;
  woe?: string;
  count?: string;
  merge?: string;
  undo?: string;
  redo?: string;
  reset?: string;
  export?: string;
  save?: string;
  noData?: string;
  party?: string;
}

/** 组件 Props。 */
export interface BinningModificationProps {
  /** 分箱数据。 */
  data: BinningData | null;
  /** 只读模式。 */
  readOnly?: boolean;
  /** 保存回调（返回序列化后的分箱数据）。 */
  onSave?: (data: BinningData) => void | Promise<void>;
  /** 文案标签。 */
  labels?: BinningModificationLabels;
  /**
   * 合并（旧版语义）：把选中分箱标记 markForMerge 后交由宿主保存配置并执行节点，
   * 引擎计算合并后的新分箱（WOE 需重新计算，不能在前端近似）。未提供时回退为本地合并。
   */
  onMerge?: (data: BinningData) => void | Promise<void>;
  /** 数据来源：upstream 上游输出 / latest 最新保存的结果。 */
  source?: 'upstream' | 'latest';
  onSourceChange?: (source: 'upstream' | 'latest') => void;
}

/* -------------------------------------------------------------------------- */
/* 工具函数                                                                    */
/* -------------------------------------------------------------------------- */

/** 深拷贝分箱数据。 */
function cloneData(data: BinningData): BinningData {
  return JSON.parse(JSON.stringify(data));
}

/** 合并选中的分箱（对应原版 merge-operation）。 */
function mergeBins(record: BinningRecord, selectedKeys: Set<string>): BinningRecord {
  const selectedBins = record.bins.filter((b) => selectedKeys.has(b.key) && b.label !== 'ELSE');
  if (selectedBins.length < 2) return record;

  // 计算合并后的区间标签
  const bounds = selectedBins.map((b) => {
    const match = b.label.match(/^\(([^,]+),\s*([^\]]+)\]$/);
    if (match) return { left: parseFloat(match[1]), right: parseFloat(match[2]) };
    return null;
  }).filter(Boolean) as Array<{ left: number; right: number }>;

  if (bounds.length === 0) return record;

  const minLeft = Math.min(...bounds.map((b) => b.left));
  const maxRight = Math.max(...bounds.map((b) => b.right));
  const mergedLabel = `(${minLeft}, ${maxRight}]`;
  const mergedCount = selectedBins.reduce((sum, b) => sum + b.totalCount, 0);
  const mergedWoe = selectedBins[0].woe !== undefined
    ? selectedBins.reduce((sum, b) => sum + (b.woe || 0), 0) / selectedBins.length
    : undefined;

  const mergedBin: Bin = {
    key: `merged-${Date.now().toString(36)}`,
    label: mergedLabel,
    markForMerge: false,
    totalCount: mergedCount,
    woe: mergedWoe,
    order: selectedBins[0].order,
  };

  const remainingBins = record.bins.filter((b) => !selectedKeys.has(b.key) || b.label === 'ELSE');
  const insertIdx = remainingBins.findIndex((b) => b.label === 'ELSE');
  if (insertIdx >= 0) {
    remainingBins.splice(insertIdx, 0, mergedBin);
  } else {
    remainingBins.push(mergedBin);
  }

  return { ...record, binCount: remainingBins.filter((b) => b.label !== 'ELSE').length, bins: remainingBins };
}

/** 导出为 CSV（对应原版 export-data/csv-export）。 */
function exportCsv(data: BinningData) {
  const lines: string[] = ['feature,party,type,bin_label,woe,count,mark_for_merge'];
  for (const record of data.variableBins) {
    for (const bin of record.bins) {
      lines.push(`${record.feature},${record.partyName},${record.type},"${bin.label}",${bin.woe ?? ''},${bin.totalCount},${bin.markForMerge}`);
    }
  }
  const blob = new Blob([lines.join('\n')], { type: 'text/csv' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `binning-${data.modelHash.slice(0, 8)}.csv`;
  a.click();
  URL.revokeObjectURL(url);
}

/* -------------------------------------------------------------------------- */
/* 主组件                                                                      */
/* -------------------------------------------------------------------------- */

/**
 * Binning 修改器：表格展示 + 合并 + 撤销/重做 + 导出 + 保存。
 */
export const BinningModification: React.FC<BinningModificationProps> = ({
  data,
  readOnly = false,
  onSave,
  labels = {},
  onMerge,
  source,
  onSourceChange,
}) => {
  const [defaultWoe, setDefaultWoe] = useState('');
  const fileRef = useRef<HTMLInputElement>(null);
  const [currentData, setCurrentData] = useState<BinningData | null>(data);
  const undoStackRef = useRef<BinningData[]>([]);
  const redoStackRef = useRef<BinningData[]>([]);
  const [canUndo, setCanUndo] = useState(false);
  const [canRedo, setCanRedo] = useState(false);
  const [selectedBins, setSelectedBins] = useState<Map<string, Set<string>>>(new Map());
  const [expandedFeature, setExpandedFeature] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  // 同步外部数据变化
  React.useEffect(() => {
    setCurrentData(data);
    undoStackRef.current = [];
    redoStackRef.current = [];
    setCanUndo(false);
    setCanRedo(false);
    setSelectedBins(new Map());
  }, [data]);

  const pushHistory = useCallback(() => {
    if (!currentData) return;
    undoStackRef.current.push(cloneData(currentData));
    if (undoStackRef.current.length > 30) undoStackRef.current.shift();
    redoStackRef.current = [];
    setCanUndo(true);
    setCanRedo(false);
  }, [currentData]);

  const handleUndo = () => {
    if (undoStackRef.current.length === 0 || !currentData) return;
    redoStackRef.current.push(cloneData(currentData));
    setCurrentData(undoStackRef.current.pop()!);
    setCanUndo(undoStackRef.current.length > 0);
    setCanRedo(true);
  };

  const handleRedo = () => {
    if (redoStackRef.current.length === 0 || !currentData) return;
    undoStackRef.current.push(cloneData(currentData));
    setCurrentData(redoStackRef.current.pop()!);
    setCanRedo(redoStackRef.current.length > 0);
    setCanUndo(true);
  };

  const handleReset = () => {
    if (!data) return;
    pushHistory();
    setCurrentData(cloneData(data));
    setSelectedBins(new Map());
  };

  const handleMerge = async () => {
    if (!currentData) return;
    pushHistory();
    const newData = cloneData(currentData);
    if (onMerge) {
      // 旧版：标记 markForMerge → 保存配置 → 执行单节点，由引擎输出合并后的分箱。
      newData.variableBins = newData.variableBins.map((record) => {
        const keys = selectedBins.get(record.key);
        if (!keys || keys.size < 2) return record;
        return { ...record, bins: record.bins.map((b) => (keys.has(b.key) && b.label !== 'ELSE' ? { ...b, markForMerge: true } : b)) };
      });
      setCurrentData(newData);
      setSelectedBins(new Map());
      await onMerge(newData);
      return;
    }
    newData.variableBins = newData.variableBins.map((record) => {
      const keys = selectedBins.get(record.key);
      if (keys && keys.size >= 2) {
        return mergeBins(record, keys);
      }
      return record;
    });
    setCurrentData(newData);
    setSelectedBins(new Map());
  };

  /** 编辑默认 WOE：设置所有 WOE 特征 ELSE 分箱的填充值（旧版 edit-default-woe）。 */
  const handleApplyDefaultWoe = () => {
    if (!currentData || defaultWoe === '' || Number.isNaN(Number(defaultWoe))) return;
    pushHistory();
    const woe = Number(defaultWoe);
    const next = cloneData(currentData);
    next.variableBins = next.variableBins.map((r) => (r.isWoe ? { ...r, bins: r.bins.map((b) => (b.label === 'ELSE' ? { ...b, woe } : b)) } : r));
    setCurrentData(next);
  };

  /** 上传分箱规则（JSON：表单格式或引擎 variableBins 格式）。 */
  const handleUpload = (file: File) => {
    const reader = new FileReader();
    reader.onload = () => {
      try {
        const parsed = JSON.parse(String(reader.result));
        const first = parsed?.variableBins?.[0];
        const next: BinningData | undefined = first && 'featureName' in first ? (binModificationsUnSerializer(parsed) as BinningData | undefined) : (parsed as BinningData);
        if (!next?.variableBins) return;
        pushHistory();
        setCurrentData(next);
      } catch {
        /* 非法文件忽略 */
      }
    };
    reader.readAsText(file);
  };

  const handleSave = async () => {
    if (!currentData) return;
    setSaving(true);
    try {
      await onSave?.(currentData);
    } finally {
      setSaving(false);
    }
  };

  const toggleBinSelection = (recordKey: string, binKey: string) => {
    setSelectedBins((prev) => {
      const next = new Map(prev);
      const set = new Set(next.get(recordKey) || []);
      if (set.has(binKey)) {
        set.delete(binKey);
      } else {
        set.add(binKey);
      }
      next.set(recordKey, set);
      return next;
    });
  };

  if (!currentData || currentData.variableBins.length === 0) {
    return (
      <div className="p-4 text-xs text-gray-500 text-center">
        {labels.noData ?? '暂无分箱数据'}
      </div>
    );
  }

  const btnCls = 'px-2 py-1 text-[10px] rounded border transition-colors disabled:opacity-30';

  return (
    <div className="flex flex-col h-full bg-gray-900 text-gray-200">
      {/* 工具栏（对应原版 toolbar/index.tsx） */}
      {!readOnly && (
        <div className="flex items-center gap-1.5 px-3 py-2 border-b border-gray-800 flex-wrap">
          <button className={`${btnCls} border-gray-700 hover:border-cyan-500 text-gray-300`} onClick={() => void handleMerge()} title={labels.merge ?? '合并选中分箱'}>
            🔗 {labels.merge ?? '合并'}
          </button>
          <button className={`${btnCls} border-gray-700 hover:border-cyan-500 text-gray-300`} onClick={handleUndo} disabled={!canUndo} title={labels.undo ?? '撤销'}>
            ↩️
          </button>
          <button className={`${btnCls} border-gray-700 hover:border-cyan-500 text-gray-300`} onClick={handleRedo} disabled={!canRedo} title={labels.redo ?? '重做'}>
            ↪️
          </button>
          <button className={`${btnCls} border-gray-700 hover:border-yellow-500 text-gray-300`} onClick={handleReset} title={labels.reset ?? '重置'}>
            🔄 {labels.reset ?? '重置'}
          </button>
          <button className={`${btnCls} border-gray-700 hover:border-green-500 text-gray-300`} onClick={() => exportCsv(currentData)} title={labels.export ?? '导出 CSV'}>
            📥 {labels.export ?? '导出'}
          </button>
          <button className={`${btnCls} border-gray-700 hover:border-green-500 text-gray-300`} onClick={() => fileRef.current?.click()} title="上传分箱">
            📤 上传
          </button>
          <input
            ref={fileRef}
            type="file"
            accept=".json"
            className="hidden"
            onChange={(e) => {
              const f = e.target.files?.[0];
              if (f) handleUpload(f);
              e.target.value = '';
            }}
          />
          <span className="inline-flex items-center gap-1">
            <input
              value={defaultWoe}
              onChange={(e) => setDefaultWoe(e.target.value)}
              placeholder="默认WOE"
              className="w-16 px-1 py-0.5 text-[10px] rounded bg-gray-800 border border-gray-700"
            />
            <button className={`${btnCls} border-gray-700 text-gray-300`} onClick={handleApplyDefaultWoe}>
              ✔
            </button>
          </span>
          {onSourceChange && (
            <select
              aria-label="binning source"
              value={source ?? 'upstream'}
              onChange={(e) => onSourceChange(e.target.value as 'upstream' | 'latest')}
              className="px-1 py-0.5 text-[10px] rounded bg-gray-800 border border-gray-700"
            >
              <option value="upstream">上游输出</option>
              <option value="latest">最新结果</option>
            </select>
          )}
          <div className="flex-1" />
          <button className={`${btnCls} border-cyan-600 bg-cyan-700/30 hover:bg-cyan-600/40 text-cyan-300 font-medium`} onClick={handleSave} disabled={saving}>
            💾 {saving ? '...' : (labels.save ?? '保存')}
          </button>
        </div>
      )}

      {/* 特征列表（可展开） */}
      <div className="flex-1 overflow-y-auto">
        {currentData.variableBins.map((record) => {
          const isExpanded = expandedFeature === record.key;
          const selected = selectedBins.get(record.key) || new Set();
          return (
            <div key={record.key} className="border-b border-gray-800/50">
              {/* 特征行 */}
              <div
                className="flex items-center gap-2 px-3 py-2 hover:bg-gray-800/50 cursor-pointer"
                onClick={() => setExpandedFeature(isExpanded ? null : record.key)}
              >
                <span className="text-[10px] text-gray-500">{isExpanded ? '▼' : '▶'}</span>
                <span className="text-xs font-medium text-gray-200 flex-1">{record.feature}</span>
                <span className="text-[10px] text-gray-500">{record.partyName}</span>
                <span className="text-[10px] px-1.5 py-0.5 bg-gray-800 rounded text-gray-400">{record.type}</span>
                <span className="text-[10px] text-gray-500">{record.binCount} bins</span>
                {record.isWoe && record.iv !== undefined && (
                  <span className="text-[10px] text-cyan-400">IV={record.iv.toFixed(4)}</span>
                )}
              </div>

              {/* 展开的分箱表格（对应原版 binning-table + expanded-table） */}
              {isExpanded && (
                <div className="px-3 pb-2">
                  <table className="w-full text-[10px]">
                    <thead>
                      <tr className="text-gray-500 border-b border-gray-800">
                        {!readOnly && <th className="py-1 w-6">✓</th>}
                        <th className="py-1 text-left">{labels.binLabel ?? '区间'}</th>
                        {record.isWoe && <th className="py-1 text-right">{labels.woe ?? 'WOE'}</th>}
                        <th className="py-1 text-right">{labels.count ?? '样本数'}</th>
                        <th className="py-1 text-center">合并</th>
                      </tr>
                    </thead>
                    <tbody>
                      {record.bins.map((bin) => (
                        <tr
                          key={bin.key}
                          className={`border-b border-gray-800/30 hover:bg-gray-800/30 ${bin.markForMerge ? 'bg-yellow-900/20' : ''} ${selected.has(bin.key) ? 'bg-cyan-900/20' : ''}`}
                        >
                          {!readOnly && (
                            <td className="py-1 text-center">
                              {bin.label !== 'ELSE' && (
                                <input
                                  type="checkbox"
                                  checked={selected.has(bin.key)}
                                  onChange={() => toggleBinSelection(record.key, bin.key)}
                                  className="w-3 h-3 accent-cyan-500"
                                />
                              )}
                            </td>
                          )}
                          <td className="py-1 text-gray-300 font-mono">{bin.label}</td>
                          {record.isWoe && <td className="py-1 text-right text-cyan-300">{bin.woe?.toFixed(4) ?? '-'}</td>}
                          <td className="py-1 text-right text-gray-400">{bin.totalCount.toLocaleString()}</td>
                          <td className="py-1 text-center">
                            {bin.markForMerge && <span className="text-yellow-400">●</span>}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
};
