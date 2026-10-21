# DAG 模板库（dag-templates）

## 1. 设计目标

把旧前端 `modules/pipeline/templates/*.ts` 中硬编码、分散的 graph 构建逻辑迁移为新前端可扩展、可测试的纯函数模板系统：

- **模板即纯函数**：每个模板只负责根据 `graphId` 和用户配置返回 `GraphNodeInfo[] + GraphEdge[]`。
- **统一注册**：新增模板只需在 `registry.ts` 注册，UI 自动发现。
- **统一向导**：`TemplateWizard` 根据模板元信息动态渲染表单，避免为每个模板重复写弹窗。
- **与旧前端拓扑一致**：节点编号、边连接、nodeDef 属性顺序与旧前端 `pipeline-template-*.ts` 保持一致，确保后端可正常调度。

## 2. 目录结构

```
features/dag-templates/
├── index.ts                 # 公共导出入口
├── types.ts                 # 类型定义
├── builder.ts               # 节点/边/属性构建工具
├── registry.ts              # 模板注册表与类型守卫
├── use-template-wizard.ts   # 向导状态管理 Hook
├── template-wizard.tsx      # 通用模板向导 UI
└── templates/               # 各模板实现
    ├── index.ts
    ├── blank.ts
    ├── psi.ts
    ├── data-classification.ts
    ├── sanitization.ts
    ├── k-anonymity.ts
    ├── l-diversity.ts
    ├── local-differential-privacy.ts
    ├── differential-privacy.ts
    ├── query-obfuscation.ts
    ├── risk.ts
    └── tee.ts
```

## 3. 核心类型

- `TemplateMetadata`：模板的 UI 元信息（key、名称、描述、计算模式、分类）。
- `TemplateBuildInput<T>`：构建输入，包含 `graphId` 和用户填写的配置 `T`。
- `TemplateBuildResult`：构建结果，包含 `nodes` 和 `edges`。
- `TemplateContribution<T>`：每个模板必须实现的对象，包含 `metadata` 和 `build(input)`。
- `TwoTableTemplateConfig` / `SingleTableTemplateConfig` / `KAnonymityTemplateConfig` / ...：不同模板所需的配置类型。

## 4. 构建工具（builder.ts）

- `nodeId(graphId, idx)` / `outputAnchor(...)` / `inputAnchor(...)` / `edgeId(...)`：统一生成节点/锚点/边 ID，确保与旧前端 `${graphId}-node-${idx}` 编号一致。
- `createNode(...)`：通用节点构造器，自动设置 `domain` / `name` / `version`。
- `createReadDataNode(...)`：构造 `read_data/datatable` 节点，自动处理 `datatable_selected` 和 `datatable_partition`。
- `createPsiNode(...)`：构造 `data_prep/psi` 节点，属性顺序与旧前端一致。
- `buildSingleTablePrivacyTemplate(...)`：快速构建单表隐私组件模板（read_data → 处理节点）。
- 属性辅助函数：`sAttr` / `ssAttr` / `i64Attr` / `fAttr` / `bAttr` / `jsonAttr` / `naAttr`，统一生成 `{ s: ..., is_na: ... }` 等旧前端属性格式。

## 5. 注册与发现（registry.ts）

`allTemplates` 数组集中注册所有模板。`templatesByCategory()` 按 `basic` / `privacy` / `ml` 分类，供向导渲染分类卡片。

类型守卫函数根据 `key` 判断模板需要双表输入、单表输入、特征列、标签列、预测列名等，用于向导动态表单校验。

## 6. 向导流程（use-template-wizard.ts + template-wizard.tsx）

1. 用户打开向导，默认选择 `psi` 模板。
2. 根据模板类型自动设置默认值：
   - 两表模板：默认填入项目前两个节点。
   - 单表模板：默认填入项目第一个节点。
   - 空白/查询混淆：无需节点选择。
3. 用户切换模板时，表单重置为模板默认值，但保留已填写的图名称。
4. 选择节点后，自动查询该节点数据表列表；选择数据表后，可进一步选择列。
5. 对于 Risk/TEE 模板，默认全选数值型列作为特征列。
6. 点击创建：
   - 调用 `apiClient.createGraph({ projectId, name })` 获取 `graphId`。
   - 调用 `template.build({ graphId, configs })` 生成节点/边。
   - 调用 `apiClient.updateGraph(projectId, graphId, nodes, edges)` 提交完整拓扑。
7. 成功后回调上层，刷新图列表并选中新建图。

## 7. 新增模板步骤

1. 在 `templates/` 下新建 `my-template.ts`，实现 `TemplateContribution<T>`。
2. 在 `templates/index.ts` 导出。
3. 在 `registry.ts` 的 `allTemplates` 中注册。
4. 在 `types.ts` 补充配置类型（如需要）。
5. 在 `i18n/dictionaries.ts` 添加 `templateName.${key}` 和 `templateDesc.${key}` 中英键值。
6. 如果模板需要新的表单字段，在 `use-template-wizard.ts` 和 `template-wizard.tsx` 中扩展。

## 8. 注意事项

- 所有模板节点 ID 必须基于 `graphId`，使用 `builder.ts` 的生成函数，保证后端能正确识别节点。
- 属性顺序和字段名（如 `input/input_ds1/keys`、`receiver_parties`）必须与旧前端保持一致，后端对 nodeDef 的解析顺序敏感。
- 特征列/标签列默认值为简化实现，实际业务中可能需要根据数据类型/语义进一步筛选。
- Risk/TEE 模板节点数较多，创建后 DAG 画布可能需要滚动查看完整流水线。
