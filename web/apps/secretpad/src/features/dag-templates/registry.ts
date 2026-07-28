/**
 * DAG 模板注册表。
 *
 * 设计说明：
 * - 模板按 category 分组（basic / privacy / ml），便于 wizard 按分类展示。
 * - 任何新增模板只需在此导入并加入 `allTemplates` 数组即可自动被 UI 发现。
 * - 运行时可通过 `templateByKey` 快速查找模板并调用 `build(...)`。
 */
import type { AnyTemplateContribution, TemplateContribution } from './types';
import { blankTemplate } from './templates/blank';
import { dataClassificationTemplate } from './templates/data-classification';
import { differentialPrivacyTemplate } from './templates/differential-privacy';
import { kAnonymityTemplate } from './templates/k-anonymity';
import { lDiversityTemplate } from './templates/l-diversity';
import { localDifferentialPrivacyTemplate } from './templates/local-differential-privacy';
import { psiTemplate } from './templates/psi';
import { queryObfuscationTemplate } from './templates/query-obfuscation';
import { riskTemplate } from './templates/risk';
import { sanitizationTemplate } from './templates/sanitization';
import { teeTemplate } from './templates/tee';

/** 所有已注册的模板。 */
export const allTemplates: AnyTemplateContribution[] = [
  blankTemplate,
  psiTemplate,
  dataClassificationTemplate,
  sanitizationTemplate,
  kAnonymityTemplate,
  lDiversityTemplate,
  localDifferentialPrivacyTemplate,
  queryObfuscationTemplate,
  differentialPrivacyTemplate,
  riskTemplate,
  teeTemplate,
];

/** 模板分类顺序与显示分组键。 */
export const templateCategories = ['basic', 'privacy', 'ml'] as const;

/** 分类的 i18n 键前缀。 */
export const categoryNameKey: Record<(typeof templateCategories)[number], string> = {
  basic: 'dag.templateCategory.basic',
  privacy: 'dag.templateCategory.privacy',
  ml: 'dag.templateCategory.ml',
};

/** 按 category 分组的模板。 */
export function templatesByCategory(): Record<string, AnyTemplateContribution[]> {
  const grouped: Record<string, AnyTemplateContribution[]> = {};
  allTemplates.forEach((t) => {
    const cat = t.metadata.category || 'basic';
    if (!grouped[cat]) grouped[cat] = [];
    grouped[cat].push(t);
  });
  // 保持 categories 定义的顺序
  const ordered: Record<string, AnyTemplateContribution[]> = {};
  templateCategories.forEach((cat) => {
    if (grouped[cat]) ordered[cat] = grouped[cat];
  });
  return ordered;
}

/** 根据 key 查找模板。 */
export function templateByKey(key: string): AnyTemplateContribution | undefined {
  return allTemplates.find((t) => t.metadata.key === key);
}

/**
 * 类型守卫：判断模板是否需要双表输入。
 *
 * 当前通过 key 判定；未来可在 TemplateMetadata 增加显式字段。
 */
export function isTwoTableTemplate(template: AnyTemplateContribution): boolean {
  return ['psi', 'risk', 'tee'].includes(template.metadata.key);
}

/**
 * 类型守卫：判断模板是否需要单表输入。
 *
 * 包含除空白/双表/查询混淆之外的所有隐私组件模板。
 */
export function isSingleTableTemplate(template: AnyTemplateContribution): boolean {
  const singleKeys = [
    'dataClassification',
    'sanitization',
    'kAnonymity',
    'lDiversity',
    'localDifferentialPrivacy',
    'differentialPrivacy',
  ];
  return singleKeys.includes(template.metadata.key);
}

/**
 * 类型守卫：判断模板是否需要列多选（特征列）。
 */
export function needsFeatureColumns(template: AnyTemplateContribution): boolean {
  return ['risk', 'tee'].includes(template.metadata.key);
}

/**
 * 类型守卫：判断模板是否需要单选标签列。
 */
export function needsLabelColumn(template: AnyTemplateContribution): boolean {
  return ['risk', 'tee'].includes(template.metadata.key);
}

/**
 * 类型守卫：判断模板是否需要指定预测列名。
 */
export function needsPredictionName(template: AnyTemplateContribution): boolean {
  return template.metadata.key === 'risk';
}

/**
 * 类型守卫：判断模板是否完全不需要参数（直接创建即可）。
 */
export function needsNoInputs(template: AnyTemplateContribution): boolean {
  return template.metadata.key === 'blank' || template.metadata.key === 'queryObfuscation';
}

/** 窄化模板为具体泛型类型（ wizard 内部使用）。 */
export function asTyped<T extends Record<string, unknown>>(
  template: AnyTemplateContribution
): TemplateContribution<T> {
  return template as TemplateContribution<T>;
}

export type TemplateCategory = (typeof templateCategories)[number];
