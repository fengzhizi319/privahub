/**
 * commitlint 配置 —— 强制 Conventional Commits 提交信息规范。
 *
 * 该配置被 `.husky/commit-msg` 钩子调用：每次 `git commit` 时，
 * commitlint 会校验提交信息格式，不符合规范的提交将被拒绝。
 *
 * 规范示例：
 *   feat(web): 新增主题切换功能
 *   fix(api-client): 修复分页参数错误
 *   chore: 升级依赖
 *
 * 允许的 type：feat / fix / docs / style / refactor / perf / test /
 *             build / ci / chore / revert
 */
export default {
  // 继承社区通用的 Conventional Commits 规则集
  extends: ['@commitlint/config-conventional'],
  rules: {
    // type 必须为小写（如 feat 而非 Feat）
    'type-case': [2, 'always', 'lower-case'],
    // type 不得为空（即必须形如 feat: xxx）
    'type-empty': [2, 'never'],
    // subject（冒号后的描述）不得为空
    'subject-empty': [2, 'never'],
    // subject 不以句号结尾，保持简洁
    'subject-full-stop': [2, 'never', '.'],
    // header（type + subject）总长度上限 100，兼容中文描述
    'header-max-length': [2, 'always', 100],
  },
};
