/**
 * DAG 组件解释器（Component Interpreter）。
 *
 * 对应旧前端 `component-interpreter` / `component-tree` 模块：在算子库面板中
 * 为每个算子提供“ℹ️”入口，点击后弹出该算子的完整定义解释，包括：
 * - 算子描述（中英双语由后端 `component/i18n` 与 `desc` 提供）；
 * - 输入 / 输出端口（名称、允许的数据类型、描述）；
 * - 可配置属性（复用 `AttributeForm` 的只读模式，展示默认值与说明）。
 *
 * 该组件位于 `@secretpad/dag-next` 包内，文案通过 `labels` 注入，保持可复用性。
 */
import React, { useEffect, useState } from 'react';
import { Modal, Badge } from '@secretpad/design-system';
import { AttributeForm } from './attribute-form';
import type { DAGComponentDef } from './index';

/** 输入/输出端口的解释信息。 */
export interface IoPortMeta {
  name?: string;
  desc?: string;
  /** 允许的数据类型列表（来自 IoDef.types）。 */
  types?: string[];
}

/** 组件元数据（由宿主通过 onGetComponentDef 返回）。 */
export interface InterpreterMetadata {
  desc?: string;
  version?: string;
  domain?: string;
  inputs?: IoPortMeta[];
  outputs?: IoPortMeta[];
  attrs?: Array<Record<string, unknown>>;
}

/** 解释器文案标签。 */
export interface ComponentInterpreterLabels {
  title?: string;
  description?: string;
  inputs?: string;
  outputs?: string;
  attributes?: string;
  version?: string;
  domain?: string;
  loading?: string;
  noDefinition?: string;
  allowedTypes?: string;
  close?: string;
}

export interface ComponentInterpreterProps {
  /** 当前解释的算子；为 null 时关闭。 */
  component: DAGComponentDef | null;
  /** 拉取组件定义的回调（宿主复用 DAG 画布的 onGetComponentDef）。 */
  fetchMetadata: (component: DAGComponentDef) => Promise<InterpreterMetadata | null>;
  onClose: () => void;
  labels?: ComponentInterpreterLabels;
}

/** 渲染单个输入/输出端口。 */
const IoPortRow: React.FC<{ port: IoPortMeta; index: number; allowedTypesLabel: string }> = ({ port, index, allowedTypesLabel }) => (
  <div className="p-2 rounded bg-gray-900 border border-gray-800 space-y-1">
    <div className="flex items-center justify-between">
      <span className="font-mono text-[11px] text-blue-300">{port.name || `port-${index}`}</span>
      {port.types && port.types.length > 0 && (
        <span className="text-[9px] text-gray-500">
          {allowedTypesLabel}: {port.types.join(' | ')}
        </span>
      )}
    </div>
    {port.desc && <div className="text-gray-400 text-[10px] leading-relaxed">{port.desc}</div>}
  </div>
);

/**
 * 组件解释器主组件。
 *
 * 打开时按 component 的 domain/name 拉取完整定义并解释展示；
 * 属性部分以只读 `AttributeForm` 呈现，帮助用户在拖入算子前理解各参数含义。
 */
export const ComponentInterpreter: React.FC<ComponentInterpreterProps> = ({ component, fetchMetadata, onClose, labels = {} }) => {
  const [metadata, setMetadata] = useState<InterpreterMetadata | null>(null);
  const [loading, setLoading] = useState(false);

  // 算子变化时重新拉取定义。
  useEffect(() => {
    if (!component) {
      setMetadata(null);
      return;
    }
    let cancelled = false;
    setLoading(true);
    fetchMetadata(component)
      .then((meta) => {
        if (!cancelled) setMetadata(meta);
      })
      .catch(() => {
        if (!cancelled) setMetadata(null);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [component, fetchMetadata]);

  if (!component) return null;

  const codeName = `${component.domain}/${component.name}`;
  const hasPorts = (metadata?.inputs && metadata.inputs.length > 0) || (metadata?.outputs && metadata.outputs.length > 0);

  return (
    <Modal
      isOpen={!!component}
      onClose={onClose}
      title={`${labels.title ?? '组件解释'} · ${component.name}`}
      footer={null}
    >
      <div className="space-y-4 text-xs max-h-[65vh] overflow-y-auto pr-1">
        {/* 基本信息：codeName / domain / version */}
        <div className="flex flex-wrap items-center gap-2">
          <Badge status="default">
            <span className="font-mono">{codeName}</span>
          </Badge>
          {metadata?.domain && (
            <Badge status="default">
              {labels.domain ?? 'Domain'}: {metadata.domain}
            </Badge>
          )}
          {metadata?.version && (
            <Badge status="default">
              {labels.version ?? 'Version'}: {metadata.version}
            </Badge>
          )}
        </div>

        {/* 加载中占位 */}
        {loading && <div className="text-gray-400">{labels.loading ?? '加载组件定义中...'}</div>}

        {/* 定义加载失败或为空 */}
        {!loading && !metadata && <div className="text-gray-500">{labels.noDefinition ?? '暂无组件定义'}</div>}

        {metadata && (
          <>
            {/* 描述 */}
            {metadata.desc && (
              <div>
                <div className="text-gray-500 text-[10px] font-semibold uppercase tracking-wide mb-1">
                  {labels.description ?? '描述'}
                </div>
                <div className="p-2.5 rounded bg-gray-900 border border-gray-800 text-gray-300 text-[11px] leading-relaxed">
                  {metadata.desc}
                </div>
              </div>
            )}

            {/* 输入端口 */}
            {metadata.inputs && metadata.inputs.length > 0 && (
              <div>
                <div className="text-gray-500 text-[10px] font-semibold uppercase tracking-wide mb-1">
                  {labels.inputs ?? '输入'} ({metadata.inputs.length})
                </div>
                <div className="space-y-1.5">
                  {metadata.inputs.map((port, idx) => (
                    <IoPortRow key={idx} port={port} index={idx} allowedTypesLabel={labels.allowedTypes ?? '类型'} />
                  ))}
                </div>
              </div>
            )}

            {/* 输出端口 */}
            {metadata.outputs && metadata.outputs.length > 0 && (
              <div>
                <div className="text-gray-500 text-[10px] font-semibold uppercase tracking-wide mb-1">
                  {labels.outputs ?? '输出'} ({metadata.outputs.length})
                </div>
                <div className="space-y-1.5">
                  {metadata.outputs.map((port, idx) => (
                    <IoPortRow key={idx} port={port} index={idx} allowedTypesLabel={labels.allowedTypes ?? '类型'} />
                  ))}
                </div>
              </div>
            )}

            {/* 无端口提示 */}
            {!hasPorts && <div className="text-gray-600 text-[10px]">—</div>}

            {/* 可配置属性（只读表单，展示默认值与说明） */}
            {metadata.attrs && metadata.attrs.length > 0 && (
              <div>
                <div className="text-gray-500 text-[10px] font-semibold uppercase tracking-wide mb-1">
                  {labels.attributes ?? '可配置属性'} ({metadata.attrs.length})
                </div>
                <div className="p-3 rounded bg-gray-950 border border-gray-800">
                  <AttributeForm defs={metadata.attrs} nodeDef={undefined} readOnly labels={{ advanced: labels.attributes ?? '可配置属性' }} />
                </div>
              </div>
            )}
          </>
        )}
      </div>
    </Modal>
  );
};
