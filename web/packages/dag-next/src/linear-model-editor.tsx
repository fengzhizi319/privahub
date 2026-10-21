/**
 * 线性模型参数修改（旧版 linear-model-parameters-modification）：
 * 表格编辑各特征权重与截距（bias），支持导出 / 上传 JSON、重置，保存为 linear_model_pb2 格式。
 */
import React, { useEffect, useRef, useState } from 'react';
import type { LinearModelData } from './custom-serializers';
import { modelModificationsUnSerializer } from './custom-serializers';
import { downloadText } from './result/report';

export interface LinearModelEditorProps {
  data: LinearModelData | null;
  readOnly?: boolean;
  onSave?: (data: LinearModelData) => void | Promise<void>;
  source?: 'upstream' | 'latest';
  onSourceChange?: (source: 'upstream' | 'latest') => void;
}

export const LinearModelEditor: React.FC<LinearModelEditorProps> = ({ data, readOnly, onSave, source, onSourceChange }) => {
  const [current, setCurrent] = useState<LinearModelData | null>(data);
  const [filter, setFilter] = useState('');
  const [saving, setSaving] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);
  useEffect(() => setCurrent(data), [data]);
  if (!current) return <div className="text-gray-500 text-center py-8 text-xs">No model parameters</div>;
  const rows = (current.featureWeights || []).filter((r) => !filter || r.featureName.toLowerCase().includes(filter.toLowerCase()));
  const btn = 'px-2 py-1 text-[10px] rounded border border-gray-700 text-gray-300 disabled:opacity-30';
  return (
    <div className="space-y-2 text-xs text-gray-200">
      {!readOnly && (
        <div className="flex flex-wrap items-center gap-1.5">
          <input value={filter} onChange={(e) => setFilter(e.target.value)} placeholder="特征名" className="w-24 px-1 py-0.5 rounded bg-gray-800 border border-gray-700 text-[10px]" />
          <button type="button" className={btn} onClick={() => setCurrent(data)}>
            🔄 重置
          </button>
          <button type="button" className={btn} onClick={() => downloadText('model-params.json', JSON.stringify(current, null, 2), 'application/json')}>
            📥 导出
          </button>
          <button type="button" className={btn} onClick={() => fileRef.current?.click()}>
            📤 上传
          </button>
          <input
            ref={fileRef}
            type="file"
            accept=".json"
            className="hidden"
            onChange={(e) => {
              const f = e.target.files?.[0];
              if (!f) return;
              const reader = new FileReader();
              reader.onload = () => {
                try {
                  const parsed = modelModificationsUnSerializer(JSON.parse(String(reader.result)));
                  if (parsed?.featureWeights) setCurrent(parsed);
                } catch {
                  /* ignore */
                }
              };
              reader.readAsText(f);
              e.target.value = '';
            }}
          />
          {onSourceChange && (
            <select aria-label="params source" value={source ?? 'upstream'} onChange={(e) => onSourceChange(e.target.value as 'upstream' | 'latest')} className="px-1 py-0.5 text-[10px] rounded bg-gray-800 border border-gray-700">
              <option value="upstream">上游输出</option>
              <option value="latest">最新结果</option>
            </select>
          )}
          <div className="flex-1" />
          <button
            type="button"
            className="px-2 py-1 text-[10px] rounded border border-cyan-600 bg-cyan-700/30 text-cyan-300"
            disabled={saving}
            onClick={async () => {
              setSaving(true);
              try {
                await onSave?.(current);
              } finally {
                setSaving(false);
              }
            }}
          >
            💾 {saving ? '...' : '保存'}
          </button>
        </div>
      )}
      <label className="flex items-center gap-2">
        <span className="text-gray-400">bias</span>
        <input
          type="number"
          step="any"
          disabled={readOnly}
          value={current.bias ?? 0}
          onChange={(e) => setCurrent({ ...current, bias: Number(e.target.value) })}
          className="w-32 px-1 py-0.5 rounded bg-gray-800 border border-gray-700"
        />
      </label>
      <div className="max-h-80 overflow-auto rounded border border-gray-800">
        <table className="w-full text-[11px]">
          <thead>
            <tr className="bg-gray-800/60 text-left text-gray-400">
              <th className="px-2 py-1">feature</th>
              <th className="px-2 py-1">party</th>
              <th className="px-2 py-1">weight</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((r) => (
              <tr key={`${r.featureName}-${r.party}`} className="border-t border-gray-800">
                <td className="px-2 py-1">{r.featureName}</td>
                <td className="px-2 py-1">{r.party}</td>
                <td className="px-2 py-1">
                  <input
                    type="number"
                    step="any"
                    disabled={readOnly}
                    value={r.featureWeight}
                    onChange={(e) =>
                      setCurrent({
                        ...current,
                        featureWeights: current.featureWeights.map((x) => (x.featureName === r.featureName && x.party === r.party ? { ...x, featureWeight: Number(e.target.value) } : x)),
                      })
                    }
                    className="w-28 px-1 py-0.5 rounded bg-gray-800 border border-gray-700"
                  />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
};
