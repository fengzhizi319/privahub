/**
 * DAG 模板贡献系统类型定义。
 *
 * 设计目标：把旧前端 `modules/pipeline/templates/*` 中硬编码的 graph 构建逻辑
 * 迁移为新前端可扩展、可测试的纯函数模板。每个模板只负责根据用户输入
 * 生成 `GraphNodeInfo[] + GraphEdge[]`，再由通用 wizard 调用后端接口创建图。
 */
import type { GraphEdge, GraphNodeInfo } from '@secretpad/api-client';

/**
 * 模板在 UI 上的元信息。
 */
export interface TemplateMetadata {
  /** 唯一标识，也用于 i18n 键。 */
  key: string;
  /** 显示名称（i18n 键为 `dag.templateName.${key}`）。 */
  nameKey: string;
  /** 描述（i18n 键为 `dag.templateDesc.${key}`）。 */
  descKey: string;
  /** 适用计算模式，例如 ['MPC', 'TEE']；为空表示不限。 */
  computeModes?: string[];
  /** 模板分类，例如 'privacy' | 'ml' | 'tee' | 'basic'。 */
  category?: string;
}

/**
 * 模板构建输入。
 */
export interface TemplateBuildInput<T extends Record<string, unknown> = Record<string, unknown>> {
  /** 新建图 ID，节点/边 ID 都基于此前缀生成。 */
  graphId: string;
  /** 用户在向导中填写的参数。 */
  configs: T;
}

/**
 * 模板构建结果。
 */
export interface TemplateBuildResult {
  nodes: GraphNodeInfo[];
  edges: GraphEdge[];
}

/**
 * 模板贡献接口。所有模板必须实现该接口。
 */
export interface TemplateContribution<T extends Record<string, unknown> = Record<string, unknown>> {
  /** 模板元信息。 */
  metadata: TemplateMetadata;
  /**
   * 根据 graphId 和用户配置生成初始节点与边。
   * 注意：所有模板共用同一 ID 命名规则 `${graphId}-node-${idx}`，
   * 便于 wizard 在创建后统一选中并加载图。
   */
  build(input: TemplateBuildInput<T>): TemplateBuildResult;
}

/**
 * 任意模板贡献的联合类型（外部注册表使用）。
 */
export type AnyTemplateContribution = TemplateContribution<Record<string, unknown>>;

/**
 * 数据表选择配置，与旧前端 `datatable_selected` 属性兼容。
 */
export interface DataTableSelectedConfig {
  /** 数据表 ID。 */
  s: string;
  /** 是否缺失。 */
  is_na?: boolean;
}

/**
 * 列选择配置，与旧前端 `ss` / `s` 数组属性兼容。
 */
export type ColumnSelectConfig = { ss: string[]; is_na?: boolean } | { s: string; is_na?: boolean };

/**
 * 双表关联模板配置（PSI / Risk / TEE 等）。
 * 继承 `Record<string, unknown>` 以满足 `TemplateContribution<T>` 的泛型约束。
 */
export interface TwoTableTemplateConfig extends Record<string, unknown> {
  receiverNodeId?: string;
  senderNodeId?: string;
  receiverTableId?: string;
  senderTableId?: string;
  /** 关联键；快速配置可传多个（旧版 Form.List 关联键1..n）。 */
  receiverKey?: string | string[];
  senderKey?: string | string[];
  receiverPartition?: string;
  senderPartition?: string;
  /** PSI 结果接收方（旧版 receiverPSI）；缺省为两方都接收。 */
  receiverParties?: string[];
  /** 特征列（PSI 模板用于全表统计节点）。 */
  featureSelects?: { ss: string[] };
}

/**
 * 单表模板配置（隐私组件）。
 */
export interface SingleTableTemplateConfig extends Record<string, unknown> {
  nodeId?: string;
  tableId?: string;
  partition?: string;
}

/**
 * K-匿名 / L-多样性模板配置。
 */
export interface KAnonymityTemplateConfig extends SingleTableTemplateConfig {
  qiCols: string[];
  saCols: string[];
}

/**
 * 本地差分隐私模板配置。
 */
export interface LocalDifferentialPrivacyTemplateConfig extends SingleTableTemplateConfig {
  queryCol: string;
}

/**
 * 数据脱敏模板配置。
 */
export interface SanitizationTemplateConfig extends SingleTableTemplateConfig {
  sanitizationCols: string[];
}

/**
 * 风险建模模板配置。
 */
export interface RiskTemplateConfig extends TwoTableTemplateConfig {
  featureSelects: { ss: string[] };
  /** 标签列；旧版快速配置为 `{ ss: [label] }`，向导为 `{ s }`，两者均可。 */
  labelSelects: { s?: string; ss?: string[] };
  pred: { s: string };
  /** 预测结果接收方（旧版 receiver）；缺省为 receiverNodeId。 */
  predictReceiver?: string;
}

/**
 * TEE 建模模板配置。
 */
export interface TeeTemplateConfig extends TwoTableTemplateConfig {
  featureSelects: { ss: string[] };
  labelSelects: { s?: string; ss?: string[] };
  /** 以下为旧版 quick-config-risk-tee 字段。 */
  trainIdSelect?: string[];
  predictIdSelect?: string[];
  saveId?: boolean;
  saveLabel?: boolean;
  /** 评估节点的标签列名 / 预测得分列名（默认 labelSelects / pred）。 */
  label?: string;
  score?: string;
}

/** 单值或数组 → 去空数组。 */
export const toList = (v: string | string[] | undefined | null): string[] => (Array.isArray(v) ? v : v ? [v] : []).filter(Boolean);
/** 标签列：兼容 `{ s }` 与 `{ ss: [..] }`。 */
export const labelOf = (v: { s?: string; ss?: string[] } | undefined): string => v?.s || v?.ss?.[0] || '';

/**
 * 引导式模板配置占位类型。
 *
 * 引导式模板参数已预填充，无需用户在向导中填写，因此配置对象为空。
 */
export type GuideTemplateConfig = Record<string, unknown>;
