/**
 * 各 DAG 模板实现导出。
 *
 * 每个文件对应一个旧前端 `pipeline-template-*.ts` 的迁移实现；
 * 新增模板时只需在此追加导出即可被注册表自动发现。
 */
export { blankTemplate } from './blank';
export { dataClassificationTemplate } from './data-classification';
export { differentialPrivacyTemplate } from './differential-privacy';
export { kAnonymityTemplate } from './k-anonymity';
export { lDiversityTemplate } from './l-diversity';
export { localDifferentialPrivacyTemplate } from './local-differential-privacy';
export { psiTemplate } from './psi';
export { queryObfuscationTemplate } from './query-obfuscation';
export { riskTemplate } from './risk';
export { sanitizationTemplate } from './sanitization';
export { teeTemplate } from './tee';
