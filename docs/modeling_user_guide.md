# 联合建模预定义画布与全流程实测

实测日期：2026-10-15。前端 `http://localhost:8000`，Go 后端、Kuscia Master、Alice/Bob Lite、SecretFlow 均使用真实运行服务。

本页覆盖八类纵向安全联合建模算法及 12 项本地隐私用例。模型数据为固定种子的合成数据，不包含真实个人信息。本文中的 AUC 用于检查链路输出，不是算法精度基准。

## 1. 直接使用预定义画布

侧栏现提供两个独立导航：

- **自定义画布**（`/dag`）：编辑、配置和运行项目中的画布副本。
- **预定义画布**（`/dag-templates`）：搜索算法、单独预览流程，点击“使用此画布”创建独立副本。

![两个独立导航和单模板预览](images/canvas-isolation/predefined-navigation.png)

模板预览为只读，每次仅显示当前选择的模板，不会叠加其他模板节点。各副本分别保存，撤销记录、选中状态和延迟保存响应不跨图复用。所有 26 个注册模板（含空白图）已验证节点间距与对象隔离，八类算法在真实页面完成正向及反向切换检查。

入口一：**项目 → 创建项目 → 管道模式 → 选择算法模板**。完成基本信息、参与方和数据表选择后，系统自动连接各节点。

![创建项目时选择八类算法](images/modeling/predefined-modeling-project-wizard.png)

入口二：**DAG 画布 → 模板向导**，在“机器学习”中选择算法，填写双方数据表、关联键、特征列、标签列和预测列名。无需手动重复画图。

![已有项目内的模板向导](images/modeling/predefined-modeling-canvas-wizard.png)

特征列支持同时选择 Alice、Bob 两方字段；标签和关联键不会自动混入特征。监督学习将预测及保留标签写给标签持有方。KMeans 不要求标签，也不创建二分类评估节点。

![完成双方字段配置](images/modeling/predefined-modeling-configured.png)

点击“创建模板图”后可继续编辑算子参数；核对配置后运行。所有模板均连接：双表读取 → PSI 对齐 → 训练/测试集划分 → 训练 → 测试集预测；监督分类模板再连接二分类评估。

![通过真实前端向导创建的草稿画布](images/modeling/predefined-modeling-created.png)

这里创建的示例是草稿；下面的成功证据来自同一前端构建器导出的实际画布执行。训练结果和评估来自不同划分，避免以训练集拟合结果冒充测试指标。

## 2. 八类算法实测

除 GPC 使用 64 行（48 行训练、16 行测试）外，其余使用 256 行（192 行训练、64 行测试）。KMeans 输出聚类模型和聚类预测表，不适用分类 AUC。

| 算法 | 状态 | 耗时（秒） | 测试样本 | AUC | 真实任务 ID |
| --- | --- | ---: | ---: | ---: | --- |
| 逻辑回归 SS-SGD | SUCCEED | 55.82 | 64 | 0.961576 | `imfs` |
| SSGLM | SUCCEED | 40.48 | 64 | 0.962562 | `tkbt` |
| SecureBoost | SUCCEED | 50.48 | 64 | 0.945320 | `vuit` |
| SS-XGB | SUCCEED | 75.56 | 64 | 0.916256 | `bpbq` |
| 高斯朴素贝叶斯 | SUCCEED | 45.63 | 64 | 0.876847 | `zmkq` |
| KNN | SUCCEED | 176.65 | 64 | 0.797044 | `lirh` |
| KMeans | SUCCEED | 41.01 | — | — | `ypia` |
| 高斯过程分类 | SUCCEED | 96.0 | 16 | 0.875000 | `fdhm` |

耗时包含调度和本机资源排队，不能用来比较算法性能。下列截图均从登录后的真实界面拍摄，运行状态在截图前通过 API 再次确认。

### 逻辑回归 SS-SGD

项目 `jfhqjlgb`，画布 `tysfwkst`。

![逻辑回归 SS-SGD运行成功画布](images/modeling/ss_sgd-canvas.png)

![逻辑回归 SS-SGD产物列表](images/modeling/ss_sgd-results.png)

![逻辑回归 SS-SGD结果详情](images/modeling/ss_sgd-detail.png)

### SSGLM

项目 `pokpioid`，画布 `xfpgwqzr`。

![SSGLM运行成功画布](images/modeling/ss_glm-canvas.png)

![SSGLM产物列表](images/modeling/ss_glm-results.png)

![SSGLM结果详情](images/modeling/ss_glm-detail.png)

### SecureBoost

项目 `konztqwp`，画布 `cgrusgos`。

![SecureBoost运行成功画布](images/modeling/sgb-canvas.png)

![SecureBoost产物列表](images/modeling/sgb-results.png)

![SecureBoost结果详情](images/modeling/sgb-detail.png)

### SS-XGB

项目 `jjgsmsrn`，画布 `ejpvyloc`。

![SS-XGB运行成功画布](images/modeling/ss_xgb-canvas.png)

![SS-XGB产物列表](images/modeling/ss_xgb-results.png)

![SS-XGB结果详情](images/modeling/ss_xgb-detail.png)

### 高斯朴素贝叶斯

项目 `vvaguwlb`，画布 `nrpymslu`。

![高斯朴素贝叶斯运行成功画布](images/modeling/gnb-canvas.png)

![高斯朴素贝叶斯产物列表](images/modeling/gnb-results.png)

![高斯朴素贝叶斯结果详情](images/modeling/gnb-detail.png)

### KNN

项目 `jxkbtqfi`，画布 `taocnxod`。

![KNN运行成功画布](images/modeling/knn-canvas.png)

![KNN产物列表](images/modeling/knn-results.png)

![KNN结果详情](images/modeling/knn-detail.png)

### KMeans

项目 `biagotxy`，画布 `onsblmfh`。

![KMeans运行成功画布](images/modeling/kmeans-canvas.png)

![KMeans产物列表](images/modeling/kmeans-results.png)

![KMeans结果详情](images/modeling/kmeans-detail.png)

### 高斯过程分类

项目 `miigjawt`，画布 `wgdtfjjd`。

![高斯过程分类运行成功画布](images/modeling/gpc-canvas.png)

![高斯过程分类产物列表](images/modeling/gpc-results.png)

![高斯过程分类结果详情](images/modeling/gpc-detail.png)

## 3. 本地隐私处理实测

| 用例 | 状态 | 任务 ID | 画布 / 产物 / 详情 |
| --- | --- | --- | --- |
| data_classification | SUCCEED | `fweh` | [画布](images/modeling/data_classification-canvas.png) · [产物](images/modeling/data_classification-results.png) · [详情](images/modeling/data_classification-detail.png) |
| differential_privacy | SUCCEED | `qazh` | [画布](images/modeling/differential_privacy-canvas.png) · [产物](images/modeling/differential_privacy-results.png) · [详情](images/modeling/differential_privacy-detail.png) |
| differential_privacy_noisy_count | SUCCEED | `bxgl` | [画布](images/modeling/differential_privacy_noisy_count-canvas.png) · [产物](images/modeling/differential_privacy_noisy_count-results.png) · [详情](images/modeling/differential_privacy_noisy_count-detail.png) |
| differential_privacy_noisy_histogram | SUCCEED | `pmgb` | [画布](images/modeling/differential_privacy_noisy_histogram-canvas.png) · [产物](images/modeling/differential_privacy_noisy_histogram-results.png) · [详情](images/modeling/differential_privacy_noisy_histogram-detail.png) |
| differential_privacy_noisy_mean | SUCCEED | `zilp` | [画布](images/modeling/differential_privacy_noisy_mean-canvas.png) · [产物](images/modeling/differential_privacy_noisy_mean-results.png) · [详情](images/modeling/differential_privacy_noisy_mean-detail.png) |
| differential_privacy_noisy_sum | SUCCEED | `jubo` | [画布](images/modeling/differential_privacy_noisy_sum-canvas.png) · [产物](images/modeling/differential_privacy_noisy_sum-results.png) · [详情](images/modeling/differential_privacy_noisy_sum-detail.png) |
| k_anonymity | SUCCEED | `bvmt` | [画布](images/modeling/k_anonymity-canvas.png) · [产物](images/modeling/k_anonymity-results.png) · [详情](images/modeling/k_anonymity-detail.png) |
| local_differential_privacy | SUCCEED | `sfqt` | [画布](images/modeling/local_differential_privacy-canvas.png) · [产物](images/modeling/local_differential_privacy-results.png) · [详情](images/modeling/local_differential_privacy-detail.png) |
| query_obfuscation | SUCCEED | `txvj` | [画布](images/modeling/query_obfuscation-canvas.png) · [产物](images/modeling/query_obfuscation-results.png) · [详情](images/modeling/query_obfuscation-detail.png) |
| query_obfuscation_batch | SUCCEED | `jguf` | [画布](images/modeling/query_obfuscation_batch-canvas.png) · [产物](images/modeling/query_obfuscation_batch-results.png) · [详情](images/modeling/query_obfuscation_batch-detail.png) |
| sanitization | SUCCEED | `ggpf` | [画布](images/modeling/sanitization-canvas.png) · [产物](images/modeling/sanitization-results.png) · [详情](images/modeling/sanitization-detail.png) |
| l_diversity | SUCCEED | `dfyf` | [画布](images/modeling/l_diversity-canvas.png) · [产物](images/modeling/l_diversity-results.png) · [详情](images/modeling/l_diversity-detail.png) |

包括分类分级、脱敏、K-匿名、L-多样性、本地差分隐私、DP sum/noisy_count/noisy_sum/noisy_mean/noisy_histogram，以及单条/批量查询混淆。全部重新执行，不复用旧结果作为本轮成功证据。

## 4. 发现的问题和运行边界

- 修复模板向导只加载一方字段的问题，现在可选择双方的特征与标签。监督预测的接收方根据标签归属推导，避免跨方保存标签失败。
- 修复特征默认选择包含标签/关联键的问题，防止标签泄漏；KMeans 支持无标签配置。
- 修复长表单把创建按钮挤出屏幕的问题：弹窗正文滚动、标题和操作栏保持可见；新增画布同步更新 URL，刷新后仍打开刚创建的图。
- 修复 E2E 状态判断：空节点列表不再被 `all([])` 当作成功；停止/取消状态立即报告失败。新增三项回归测试。
- GPC 初次以 256 行、默认 20 次迭代运行，在每 Lite 节点 4 GiB 内存下训练 OOM。已将模板迭代默认值降为 3，并提示小样本验证；64 行重跑成功。降低默认迭代不保证任意规模均能在 4 GiB 下运行，大样本需要资源评估。
- GPC 的 128 行独立基线也已真实跑通，耗时约 462 秒；其结果不替代训练/测试分离的模板验收。
- 这里验证的是纵向安全联合建模（MPC / HEU / SPU）。没有将空白“联邦学习”场景或这些成功任务包装成已实现的横向 FedAvg / 深度学习联邦训练。TEE、SLS、生产模型服务以及独立双机构 P2P 的外部部署不在这 20 项验收结论内。

![保留的 GPC 大样本失败记录](images/modeling/gpc-large-failed-canvas.png)

原失败任务 `wyki`、画布 `qbnkutsd` 保留供排查，Kuscia 容器终止原因是 `OOMKilled`。成功重跑任务见上表。

## 5. 复现和证据校验

```bash
# 安装好前端依赖与 Chromium 后运行；省略 --no-start 会启动全栈
MODELING_PYTHON=/home/charles/miniconda3/envs/sf310/bin/python \
  bash scripts/modeling-test.sh --no-start
```

脚本从真实前端模板导出图结构，经 API 创建项目/数据表/画布，再交给 Kuscia 和 SecretFlow 执行；没有 mock 算法结果。每个算法独立记录成功或失败，失败返回非零。GPC 单独使用 64 行，其余使用 256 行。

项目及数据表为截图复查而保留，重复运行会新增资源；可按报告 ID 在前端删除。脚本不修改原有业务项目，也不自动删除测试以外的数据。

复现脚本及说明见工作区 `scripts/modeling-test.sh --help`、`scripts/README.md`。截图的路径、任务状态和 SHA256 见 [截图清单](images/modeling/manifest.json)，API 产物及报告数据见 [本轮实测记录](testing/modeling-latest.json)。

验证项：前端完整测试（含模板拓扑/配置用例）、typecheck、lint、生产构建；脚本测试、Python 状态轮询回归；20 项真实业务链路和浏览器截图校验。
