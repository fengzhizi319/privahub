# Privahub 使用手册

更新：2026-10-21（功能截图基线 2026-10-15；运维监控截图 2026-10-21）。适用于当前 Go 后端与 `web/apps/secretpad` React 前端。脚本已按生命周期重组到 `scripts/<分类>/`，本文所有命令均使用当前路径；旧版 `scripts/dev-start.sh`、`scripts/smoke-test.sh` 等入口不再存在。产品侧栏显示 Privahub，部分页面仍沿用 SecretPad 名称；本文以实际菜单和按钮名称为准。

## 1. 阅读路线与截图环境

第一次使用按“启动 → 登录 → 节点 → 数据源/数据表 → 项目 → DAG → 运行记录 → 结果”操作。日常使用可直接跳到对应功能；平台管理员另看审计与账户安全，独立机构看 P2P 协作。

本文图片由真实 Chromium 访问本机服务生成，中文、1440×1000，移动端为 390×844；没有拦截接口、伪造返回值或修改浏览器中的身份/权限来制造页面。图中数据为开发演示数据。图片位于 [images/user-guide](images/user-guide/)，逐图路由、平台类型、标题及按钮见 [截图清单](images/user-guide/manifest.json)。第 21 章另有 7 张 1600×1000 运维监控截图，由同一浏览器访问真实 Prometheus 与 Grafana 生成（Grafana 为英文界面），位于 [images/ops-monitoring](images/ops-monitoring/)，单独用 `SHA256SUMS` 校验，不计入下方 60 张产品页面截图。

本次共 60 张截图（CENTER 50 张、AUTONOMY 6 张、EDGE 4 张），覆盖前端路由中全部 36 个页面及关键弹窗/抽屉；同一页面在不同部署上下文中的入口分别留证。清单记录图片 SHA256 和拍摄完成状态。在工作区根目录执行校验：

```bash
python3 scripts/test/check-user-guide.py
```

通过时输出 `{"screenshots": 60, "frontendPages": 36, "links": "passed", "checksums": "passed", "routeCoverage": "passed"}`。脚本还会检查手册引用的图片集合与清单是否完全一致，缺图、多图或哈希不符都会失败。

| 验证范围 | 本次环境与证据 | 阅读时的边界 |
|---|---|---|
| CENTER 主流程 | 本地 Go 后端、Vite、真实 Kuscia master + alice + bob、SecretFlow 计算镜像 | 已执行两方 PSI，读取两个输出，保留项目供页面复查 |
| EDGE 节点视角 | 独立临时 Go 数据库和真实浏览器，节点 ID 为 alice | 展示独立 EDGE 入口；截图实例关闭跨平台同步，不等于中心到边缘同步链路验收 |
| AUTONOMY/P2P | 独立临时 Go 数据库和真实浏览器，初始化演示机构 | 展示工作台、项目、机构、注册表单；不等于部署了两套独立 P2P 机构并完成审批 |
| 外部能力 | 展示当前页面、表单、状态及操作前提 | TEE 硬件、云端 SLS、OSS/ODPS/MySQL、在线特征服务、模型推理仍需各自部署与凭据；页面截图不代表这些服务已验收 |

CENTER、EDGE、AUTONOMY 是**部署/账户上下文**，ADMIN、AUDITOR 是服务端角色，不能混为一谈。侧栏可见和按钮可点都不代替后端资源归属校验。

### 1.1 功能导航

| 入口 | 用途 | 章节 |
|---|---|---|
| 登录、首次改密、用户中心 | 进入系统与维护账户 | §3、§15 |
| 总览、工作台、新手引导 | 查看资源、快捷进入业务 | §4 |
| 隐私场景 | 选择场景，预填项目与模板 | §5 |
| 节点、我的节点、节点路由 | 注册、查看节点和合作关系 | §6 |
| 全部数据源、全部数据表 | 中心平台跨节点资源目录 | §7.1 |
| 数据源及详情 | 注册连接、查看状态与关联节点 | §7.2 |
| 数据表、详情、授权配置 | 导入、查看字段、授权到项目 | §7.3 |
| 特征数据源 | HTTP 在线特征定义与项目授权查询 | §7.4 |
| 项目、图管理、DAG | 组织协作并编排计算 | §8～9 |
| 自定义画布、预定义画布 | 项目编辑器与模板库 | 附录 |
| 运行记录、周期任务及详情 | 追踪执行、配置调度和重跑 | §10～11 |
| 结果、模型 | 查看产物、下载、打包及发布 | §12～13 |
| 消息、审批 | 处理参与方申请与审批结果 | §14 |
| 云端日志、组件版本、审计 | 运行定位、版本核对、安全追溯 | §16 |
| P2P 项目、机构管理、节点注册 | 独立机构协作 | §17 |
| 运维监控 | Prometheus 指标、Grafana 大屏与验收签收 | §21 |
| 语言、主题、移动导航 | 通用交互 | §18 |
| 项目分组、统一列表与详情 | 显示方式、筛选、行高、列设置与抽屉 | 附录 |

## 2. 启动、验证与关闭

### 2.1 环境准备

从工作区根目录执行命令。后端所需 Go 版本以 `privahub/go.mod` 为准，当前模块声明 Go 1.25.0，固定工具链为 go1.25.14；允许 `GOTOOLCHAIN=auto` 自动选择。前端以 `web/package.json` 为准，`packageManager` 为 pnpm 11.7.0；pnpm 11 本身要求 Node.js ≥22.13，建议使用 Node.js 22 或更高版本。还需 Docker、corepack、Python，以及构建 SecretFlow wheel 所需的 conda `sf310` 环境。首次搭建可先运行 `bash scripts/env/diagnose-environment.sh` 检查工具链、磁盘、端口和子仓库。

```bash
cd /home/charles/code/clwork
bash scripts/env/diagnose-environment.sh
bash scripts/dev/start-docker-dev-stack.sh --check
bash scripts/dev/start-docker-dev-stack.sh
bash scripts/dev/show-dev-status.sh
```

`start-docker-dev-stack.sh` 依次检查依赖、构建后端、检查或部署 Kuscia、导入 SecretFlow/Serving 镜像、注册 AppImage、连接上传目录，最后等待后端和前端就绪。已运行的服务会复用，不重置数据库。第一次构建镜像可能较慢，应查看日志而不是重复启动。常用选项：

| 选项 | 作用 |
|---|---|
| `--check` | 只检查依赖与子项目，不启动任何服务 |
| `--status` | 显示各服务状态后退出 |
| `--restart` | 重新编译并重启后端/前端，不影响 Kuscia |
| `--skip-kuscia` | 不部署或检查 Kuscia，只启动前后端 |
| `--skip-build` | 后端二进制已存在时跳过 `go build` |
| `--no-frontend` | 只启动后端 |
| `--reset-kuscia` | 删除脚本创建的 Kuscia 容器与数据后重新部署（确认后执行，不用于日常重启） |

Linux / WSL2 也支持完全非 Docker 模式：Kuscia 使用本地二进制、SecretFlow 使用 conda 环境，启动入口为 `bash scripts/dev/run-no-docker-stack.sh`。该模式需要 sudo 绑定 53/80 等特权端口，详细步骤见 [scripts/dev 运行说明](../../scripts/dev/README.md)。

> 脚本入口已在目录重组中变更：`scripts/dev-start.sh` → `scripts/dev/start-docker-dev-stack.sh`，`scripts/dev-status.sh` → `scripts/dev/show-dev-status.sh`，`scripts/dev-logs.sh` → `scripts/dev/show-dev-logs.sh`，`scripts/dev-stop.sh` → `scripts/dev/stop-dev-stack.sh`，`scripts/smoke-test.sh` → `scripts/test/run-api-smoke-test.sh`，`scripts/user-guide.sh` → `scripts/test/capture-user-guide.sh`，`scripts/check-user-guide.py` → `scripts/test/check-user-guide.py`。完整分类索引见 [scripts/README.md](../../scripts/README.md)。

本机使用工作区 `.env` 的非默认 Kuscia 映射：

| 服务 | 本次地址/端口 | 说明 |
|---|---|---|
| 浏览器前端 | <http://127.0.0.1:8000> | Vite 代理 `/api` 到后端 |
| Go 后端 | `127.0.0.1:8080` | 健康检查 `/api/v1alpha1/healthz` |
| 内部接口 | `9001` | 内部服务使用，不是浏览器登录入口 |
| Kuscia master | gRPC `28083` / HTTP `28082` / 网关 `28080` | 容器 `charles-kuscia-master` |
| alice | gRPC `28183` / HTTP `28182` / 网关 `28180` | 容器 `charles-kuscia-lite-alice` |
| bob | gRPC `28283` / HTTP `28282` / 网关 `28280` | 容器 `charles-kuscia-lite-bob` |

默认映射与本机映射可能不同，启动器的汇总输出才是当前实际地址。通过 `KUSCIA_*_HOST_PORT`、`KUSCIA_*_HOST_PORT_BASE` 配置容器端口，启动器会把对应端口传给 Privahub。详情见 [工作区脚本说明](../../scripts/README.md)。

本机开发栈使用 `notls`，适合本地联调。生产证书、TLS、内部鉴权和密钥配置按 [安全加固指南](security/hardening-guide.md) 配置。

### 2.2 完整验证和重新生成手册图片

```bash
# 验证身份、Cookie/CSRF、项目/画布、审计、真实节点及双向路由
bash scripts/test/run-api-smoke-test.sh --kuscia

# 需要顺带运行一次最小真实 PSI 时（隐含 --kuscia，结束后清理创建的项目/画布）
bash scripts/test/run-api-smoke-test.sh --kuscia --psi

# 没有运行中的开发栈：先构建后端，再自启临时后端和全新数据库做 API 冒烟
bash scripts/build/build-artifacts.sh --backend
bash scripts/test/run-api-smoke-test.sh --spawn --db none

# 启动开发栈，执行真实两方 PSI，生成 CENTER / EDGE / AUTONOMY 中文截图
bash scripts/test/capture-user-guide.sh

# 服务已启动时：复用栈，但重新执行真实 PSI
bash scripts/test/capture-user-guide.sh --no-start

# 已有本次保留的成功 PSI 报告时：只重拍界面
bash scripts/test/capture-user-guide.sh --no-start --skip-psi

# 只重拍 CENTER：不启动临时 EDGE/AUTONOMY 演示服务
bash scripts/test/capture-user-guide.sh --no-start --center-only

# 独立隔离库的系统性验证；临时服务运行完自动退出
cd privahub
MIGRATION_PYTHON=/home/charles/miniconda3/envs/sf310/bin/python \
  python3 scripts/run_system_e2e.py --browser --kuscia
```

`capture-user-guide.sh` 使用已安装的 Playwright Chromium；没有浏览器时在 `privahub/web` 执行 `corepack pnpm exec playwright install chromium`。Python PSI 客户端需要 `requests`，可通过 `USER_GUIDE_PYTHON` 指定解释器。非默认账号通过 `CLWORK_ADMIN_USER` / `CLWORK_ADMIN_PASSWORD` 环境变量传入；已有前端地址或自定义 Chromium 可设置 `USER_GUIDE_WEB_URL` / `PLAYWRIGHT_CHROMIUM_EXECUTABLE`。

截图默认写入 `privahub/docs/images/user-guide/`；`--output DIR` 可指定另一个目录。PSI 资源索引在 `logs/user-guide-psi.json`，截图运行日志在 `logs/user-guide-*/`。报告不记录口令和登录令牌。临时 EDGE/AUTONOMY 服务使用隔离库，拍摄后关闭，主开发栈保持运行。脚本保留真实 PSI 项目和一个**已下线**的未来调度示例；重复完整生成会创建新项目，确认无需保留后可在项目页删除。`--center-only` 只生成 CENTER 记录，会覆盖同一目录的完整清单，因此只适合定点补拍；要校验本文全部 60 张图片，应运行完整截图流程并执行 `python3 scripts/test/check-user-guide.py`。跨仓库测试门禁（脚本、后端、前端、E2E、Playwright）见 [测试文档](../../docs/testing/README.md)。

### 2.3 日志、重启和关闭

```bash
bash scripts/dev/show-dev-logs.sh backend -f
bash scripts/dev/show-dev-logs.sh frontend -f
bash scripts/dev/show-dev-logs.sh alice -f
bash scripts/dev/show-dev-logs.sh all -n 200
bash scripts/dev/show-dev-status.sh --watch 5
bash scripts/dev/start-docker-dev-stack.sh --restart
bash scripts/dev/stop-dev-stack.sh
# 仅在需要同时关闭计算节点时使用：
bash scripts/dev/stop-dev-stack.sh --kuscia
```

关闭浏览器或退出登录不会停止后端或计算任务。`--restart` 只重新编译并重启后端/前端，不影响 Kuscia；`--reset-kuscia` 会删除容器和数据，不用于日常重启。`show-dev-logs.sh` 支持 `backend`、`frontend`、`kuscia-master`、`alice`、`bob`、`all` 日志源，使用 `-l` 可列出当前可用来源。`stop-dev-stack.sh --kuscia` 会停止 Docker 容器或本地 Kuscia Master，但保留数据。

开发栈不包含 Prometheus 与 Grafana；需要“前端 + 运维监控大屏”同时运行时，按 [§21](#21-运维监控前端与-grafana-大屏) 追加启动。监控组件只读取后端 `/metrics`，不写入业务库，也不影响正在执行的计算任务。

## 3. 登录与首次改密

![登录页](images/user-guide/01-login.png)

打开前端地址，填写用户名和当前密码，点击“登录控制台”。开发页面可能预填默认账号；已有数据库中改过密码的账号仍应使用实际密码。默认开发账号仅用于本地环境：`admin / 12345678`。

登录后系统根据后端返回的平台与账户上下文进入新手引导、总览、工作台或节点视角。没有“选择管理员身份”来提升权限的步骤。

### 3.1 首次或管理员重置密码

![首次登录改密](images/user-guide/51-first-password-change.png)

新建/重置账户可能要求先修改密码，此时只允许账户信息、改密和退出相关操作，业务入口受限。页面路由为 `/change-password`；填写当前密码、新密码、确认新密码后提交。页面要求新密码 8～20 位，含大小写字母和数字，且不同于当前密码。成功后重新登录，旧会话会失效。

截图账号由管理员经真实 API 临时创建，拍摄后删除；这不是可复用的内置测试账号。

### 3.2 登录失败时

核对密码、后端健康和浏览器代理地址。密码输错、账号锁定、会话到期等以页面返回原因处理；不要连续尝试默认口令。登录成功但无法操作资源时，先确认平台、角色、所属节点及项目成员关系，见 §15。

## 4. 总览、工作台与新手引导

### 4.1 总览 `/dashboard`

![控制台总览](images/user-guide/03-dashboard.png)

查看项目、节点、任务等统计以及近期项目、节点引导、拓扑和快捷操作。点击项目进入对应协作场景；“新建 DAG 流水线”“新建项目”“导入数据表”“注册节点”分别进入后续流程。统计变化以实际刷新结果为准；节点表中存在一条记录不代表该节点已能执行计算。

### 4.2 工作台 `/workbench`

![工作台](images/user-guide/04-workbench.png)

工作台汇总当前身份的业务入口和项目。展开相关卡片查看概况，使用快捷入口进入 DAG、数据、节点或隐私场景。CENTER 与独立机构模式的工作台内容会随账户上下文变化。

### 4.3 新手引导 `/guide`

![新手引导](images/user-guide/02-guide.png)

按“注册计算节点 → 导入数据表 → 创建协作项目 → 编排 DAG → 运行与查看结果”推进。每步可跳转对应页面、标记完成，也可重新开启交互式引导。“标记为已完成”只记录引导进度，不会替你注册节点、创建项目或执行计算。

## 5. 隐私场景 `/privacy-scenes`

![隐私计算场景](images/user-guide/05-privacy-scenes.png)

场景目录包括 PSI、MPC 风控建模、TEE、分类分级、数据脱敏、K 匿名、L 多样性、本地差分隐私、差分隐私查询、查询混淆和联邦学习。

1. 阅读场景描述和算法标签。
2. 点击“一键创建项目”，进入预填模板、计算模式和参与节点的创建向导。
3. 核对项目名、节点及输入数据授权，完成创建后进入画布。
4. 在画布补齐表、字段、算子参数，再保存和运行。

计算模式不受当前部署支持时按钮不可用。场景卡片是配置入口，不是已完成任务；联邦学习入口使用空白图，需要自行构建训练流程。

## 6. 节点、节点视角与合作关系

### 6.1 节点管理 `/nodes`

![节点集群](images/user-guide/06-nodes.png)

按节点名称搜索，按注册时间排序、翻页，查看节点 ID、状态、地址和类型。“刷新”重新获取状态，“进入节点”切换到单节点资源视角。内置节点或被任务/资源引用的节点会限制删除。

![注册节点表单](images/user-guide/35-register-node.png)

点击“注册新节点”，填写名称并选择当前环境支持的 MPC/TEE 类型。此操作建立管理侧节点记录；实际 Kuscia 连接、证书、网络和路由必须另外就绪。不要把“登记成功”理解为远端计算服务已部署。

### 6.2 单节点资源视角 `/node/{nodeId}`

![节点数据源](images/user-guide/09-node-data-sources.png)

![节点数据管理](images/user-guide/10-node-data-tables.png)

页头显示节点名称、ID 和状态；“数据源 / 数据管理 / 合作节点 / 结果管理”四个页签分别固定当前节点，避免把 alice 的数据操作到 bob。TEE 节点没有普通数据源页签。可以返回节点管理重新选择。

![节点合作关系](images/user-guide/11-node-cooperation.png)

![节点结果](images/user-guide/12-node-results.png)

### 6.3 我的节点 `/p2p/my-node`

![我的节点](images/user-guide/30-my-node.png)

查看节点基本信息、地址及允许展示的认证信息。编辑前确认当前节点归属；需要查看或复制认证材料时只在受控环境使用。中心管理员入口必须指定可访问节点，不能把中心平台 ID 当作计算节点 ID。

### 6.4 节点路由 `/node-routes`

![合作节点及路由](images/user-guide/17-node-routes.png)

选择本方节点，查看合作方连接状态、搜索和排序，必要时刷新。“添加合作节点”用于建立合作关系，按表单提供对方信息并完成当前模式要求的审批。已有计算任务依赖的合作关系不应随意删除。

双向网络、双方证书/认证和心跳都需要正常；本次 alice↔bob 双向路由已通过脚本检查。列表中的 TEE 记录不表示本机有可用 TEE 服务。

## 7. 数据接入与授权

### 7.1 全部数据源和全部数据表

![跨节点数据源目录](images/user-guide/07-all-data-sources.png)

![跨节点数据表目录](images/user-guide/08-all-data-tables.png)

`/all-data-sources` 与 `/all-data-tables` 提供 CENTER 管理员的跨节点目录，支持刷新、检索和分页。点击节点可进入对应上下文。具体注册、导入和授权应在目标节点下进行，防止混淆资源属主。

### 7.2 数据源 `/data-sources` 与详情

![数据源列表](images/user-guide/13-data-sources.png)

选择节点，按名称、状态和类型筛选。列表展示连接到哪些节点以及可用情况。点击名称或“详情”查看连接信息；已绑定数据的数据源及内置 HTTP 数据源会限制删除。

![数据源注册](images/user-guide/36-register-datasource.png)

点击“注册数据源”，按实际类型填写表单。OSS 需要 endpoint/bucket/访问凭据，ODPS 需要 project/endpoint/凭据，MySQL 需要连接地址、数据库和账号。根据表单选择关联节点、测试连接并注册。LOCAL 通常由节点默认存储提供；是否允许在表单创建以实际可选项为准。

![数据源详情](images/user-guide/15-datasource-detail.png)

详情路由为 `/data-sources/detail?ownerId=...&datasourceId=...&type=...`，展示名称、类型、状态、脱敏连接信息与关联节点。截图使用真实的内置 HTTP 数据源，其连接信息为空是预期行为。访问密钥和数据库密码在详情中不应显示明文。

### 7.3 数据表 `/data-tables`

![数据资产](images/user-guide/14-data-tables.png)

先选节点，按名称、状态或数据源类型筛选；表格显示表名、来源、已授权项目、可用状态及操作。页面提示状态可能存在延迟时，点击“刷新”后再判断。

![导入数据表](images/user-guide/37-import-data.png)

点击“添加数据”，按导入方式提供本地文件或已有数据源中的位置，配置表名、字段及类型。数据分级和字段 Schema 是后续选择关联键、标签和隐私处理参数的依据。上传数据必须落到计算节点可读的目录；启动器已把节点上传目录链接到 lite 节点存储。

点击表名打开详情，查看字段与授权项目；“上传”用于对应表的数据上传流程，“批量管理”用于批量操作。删除前先确认授权和任务引用，不能只从列表移除后假设所有引用已清理。

![数据表 Schema 详情](images/user-guide/55-table-schema.png)

![数据表授权管理](images/user-guide/38-data-authorization.png)

授权不是“能看到表就能使用表”。在“授权管理”中选择项目，并按需设置关联键、分组列、标签列和保护开关；字段级 SCQL 配置仅在相应联合分析场景生效。修改和取消授权会影响后续计算，先核对项目与节点。

![新增项目与字段授权](images/user-guide/56-column-authorization.png)

项目侧添加数据表与表侧授权是同一协作边界的入口。P2P 中只能选择已通过且包含当前节点的项目，不能借助授权界面加入未获准项目。

### 7.4 特征数据源 `/feature-datasource`

![特征数据源查询](images/user-guide/16-feature-datasource.png)

![创建特征数据源](images/user-guide/39-create-feature-source.png)

查询时选择项目和节点，点击“查询”查看该上下文中已授权的特征表及字段。创建时填写特征表名、HTTP 服务地址、描述，配置字段名/类型/说明，并选择授权节点。当前在线特征数据源只接受 HTTP 类型。

创建定义不等于在线服务可访问。发布模型前，应另行验证目标节点对特征服务的连通性、认证和字段映射。本机截图不包含外部特征服务验收。

## 8. 项目与图管理

### 8.1 项目 `/projects`

![协作项目](images/user-guide/18-projects.png)

使用搜索和计算模式过滤定位项目；可按计算模式（MPC / TEE）或项目状态分组，并在紧凑列表、分页卡片、全部卡片三种显示方式之间切换（详见附录）。卡片/列表展示项目描述、参与节点、训练流和任务数量，显示方式会记录在当前浏览器。点击名称或卡片打开详情，查看参与节点、授权表和任务；有管理权限时可添加节点、添加/移除数据表以及编辑项目。

![项目参与节点与授权详情](images/user-guide/57-project-detail.png)

![创建项目向导](images/user-guide/34-create-project.png)

1. 点击“创建项目”，先选择计算模式与空白图或场景模板。
2. 下一步填写名称、描述，再选择参与节点并核对授权信息。
3. 完成向导后阅读创建结果提示，确认所有步骤成功。
4. 点击“打开 DAG”进入项目内训练流。

项目删除需要确认，运行中的任务及关联产物应先核对。平台管理员角色也不能绕过服务端成员和资源归属限制。

### 8.2 图管理 `/graphs`

![图管理](images/user-guide/19-graphs.png)

先选择项目，再新建图、打开 DAG 或删除图。图是项目下的计算流程定义，一个项目可以有多个图。修改定义不会自动改写历史任务快照；需要新的运行才能产生新结果。

## 9. DAG 编排、参数和运行

![已完成真实 PSI 的 DAG](images/user-guide/20-dag-psi.png)

截图是本次真实执行成功的三节点 PSI：alice 样本表与 bob 样本表输入隐私求交节点，节点状态均为成功。项目、图和任务 ID 可在截图清单的 `psi` 字段找到。

### 9.1 打开与编辑

1. 从项目的“打开 DAG”进入，或在画布顶部依次选择项目和图。
2. 从左侧“组件”选择算子，或切换“数据表”定位已授权输入。
3. 将算子放到画布，连接输出和下游输入；选择节点配置数据表、列和算子参数。
4. 使用快速配置，按需打开高级配置核对各参与方资源和数据源。
5. 点击“保存”，刷新或重新进入后确认图结构仍在。

画布工具栏提供选择/连线、缩放、定位、撤销/重做、吸附、布局、小地图及导入/导出等操作，以按钮提示为准。“模板向导”生成预设结构；仍要核对输入和参数。

![画布模板向导](images/user-guide/59-template-wizard.png)

### 9.2 图操作和计算

顶部支持新建图、重命名、复制训练流和删除。运行前确认：参与节点 Ready、双方路由正常、输入表已授权、关联键正确、数据源为对应节点可读位置。

点击“运行”后在节点状态、日志和运行记录中观察。未完成时先查具体失败节点和参与方日志，不要连续点击运行。编辑权限不足或查看历史快照时，不能把只读画布当作当前图编辑器。

### 9.3 本次真实 PSI 配置

| 项目 | 配置 |
|---|---|
| 参与方 | alice、bob |
| 输入表 | `alice-table`、`bob-table`（脚本兼容实际表 ID） |
| 关联键 | alice `id1`、bob `id2` |
| 算子 | `data_prep/psi:1.0.0` |
| 协议与连接方式 | `PROTOCOL_RR22`、`inner_join` |
| 输出验证 | 两个 `graph/node/output` 请求均返回有效产物；表及报告可在结果页查看 |

真实计算脚本为 [psi_smoke.py](../../scripts/lib/psi_smoke.py)。演示表只用于开发验证；实际项目应按数据 Schema 选择键和协议。

2026-10-15 截图基线中，alice、bob 各有 9892 条输入，求交结果各为 9892 条；这些数值属于该次演示数据。重新执行 `capture-user-guide.sh` 后以 `logs/user-guide-psi.json` 和结果页为准，不要沿用旧批次的任务 ID 或行数。

## 10. 运行记录 `/job-records`

![运行记录](images/user-guide/21-job-records.png)

选择项目，可进一步选择训练流，按关键词、状态及起止时间筛选并分页。筛选在服务端执行，覆盖全部分页，而不是只过滤当前页；关键词匹配任务名称、Job ID 或训练流名称，输入会做长度限制与通配符转义。列表展示状态、完成节点数、耗时和创建时间；运行中记录会定期刷新。

点击任务或“详情”打开只读回放，查看当次图快照、节点日志和输出。“停止”适用于运行中的任务，可选择多条批量停止。停止请求到最终停止存在过程，`STOPPING` 仍视为活动任务。

![任务回放与执行详情](images/user-guide/54-job-replay.png)

“周期任务记录”页签按调度和子任务筛选，和普通手动执行记录分开。页面没有删除历史记录按钮；不要用删除项目代替日常日志清理。

## 11. 周期任务与详情

![周期任务列表](images/user-guide/24-periodic-tasks.png)

选择项目后查询周期任务，支持按 ID、状态检索和部署时间排序。任务分“生效中”和“已下线”。本次示例从真实成功 PSI 图创建，未来触发，随后立即下线，不会在无人值守时继续执行。

![创建周期任务](images/user-guide/49-create-schedule.png)

先把图运行成功一次，再从画布或周期任务页创建计划。填写任务描述、开始/结束时间、日/周/月周期及执行时间。周计划选择星期，月计划选择日期或月末。确认服务端时区与业务期望后保存。

![周期任务详情](images/user-guide/33-schedule-detail.png)

点击详情查看图、任务信息和历史执行。子任务历史中可停止仍在运行的执行，或对满足条件的失败记录选择“重跑全部 / 重跑失败部分”。正在运行或正在停止时限制计划下线；删除要求计划已下线。操作前核对 scheduleId、子任务 ID 与所属项目，避免重跑错误批次。

尚未触发的未来任务保留在调度子任务列表，不出现在“执行记录”中；创建计划时预生成的图/作业不代表计算已经开始。

## 12. 结果管理 `/results`

![结果列表](images/user-guide/22-results.png)

选择节点，按名称和表/报告/规则/模型类型过滤，按时间排序、分页。节点视角下已经固定属主。点击“详情”查看产物内容或元信息，点击“下载”导出对应结果。

![本次 PSI 结果详情](images/user-guide/50-result-detail.png)

表结果和报告的展示形式不同，报告可以有多个页签、表格和统计项。批量下载/删除按页面支持的产物类型进行；删除会影响后续引用，先核对选择项。

远端 OSS/ODPS/MySQL 产物依赖对应连接和授权；TEE 产物可能需要先申请下载、审批及拉取完成，失败时可按提示重新获取。空列表不等于后端离线，也可能是节点、筛选条件或资源授权不匹配。

## 13. 模型、打包和服务发布 `/models`

![模型与推理服务](images/user-guide/23-models.png)

先选项目，按模型名称/ID、状态查询，按提交时间排序。模型状态决定可用操作：

| 状态/操作 | 使用方式与条件 |
|---|---|
| 打包 | 选择训练图及训练节点，填写模型名称，确认各方数据源与模型路径；必须已有真实模型输出 |
| 发布 | 选择可发布模型，配置各参与方在线特征映射与服务资源，提交后观察发布状态 |
| 发布中 | 等待轮询结果，失败时查看原因后修正配置再发布 |
| 已发布 | 查看服务详情；需要停用时下线 |
| 废弃 | 对可废弃状态执行，确认不会继续使用后再进行删除 |
| 删除 | 仅满足状态和引用约束时开放 |

PSI 产出表和报告，不会自动产生可发布模型。模型打包、Serving 镜像就位、在线特征服务可达和真实推理是不同步骤；本次没有通过虚构模型填充列表，也没有声称已完成端到端推理。

![模型打包配置](images/user-guide/52-pack-model.png)

![模型服务发布配置](images/user-guide/53-publish-model.png)

以上为真实表单截图，未提交没有训练输出的打包或发布请求。正式操作应先完成训练，提供各参与方模型与特征服务信息。

## 14. 消息与审批 `/messages`

![消息和审批中心](images/user-guide/25-messages.png)

通过发起/待处理/已处理等页面筛选条件定位消息，打开详情核对发起方、参与方、申请内容与审批进度。按权限对待处理项同意或拒绝，并确认各参与方结果更新。

P2P 项目创建、项目归档、合作节点或 TEE 下载等业务可触发审批。审批对象与资源必须匹配；收到消息不会自动授予资源访问权限。拒绝之前核对业务影响，同意之后仍需检查部署/拉取/执行状态。

本机截图列表为空时表示当前筛选没有消息，不表示已演示所有审批分支。需要完整多机构审批时部署双方独立实例，见 §17。

## 15. 用户中心与权限边界 `/account`

![用户中心](images/user-guide/29-account.png)

这里查看用户名、平台类型、平台节点 ID 和所属类型；填写当前密码、新密码和确认密码进行改密。改密成功后后端撤销会话，需要使用新密码重新登录。右上角账户菜单提供账户入口与退出登录。

当前前端没有独立的用户/角色管理页面，也没有可配置的入站 **Scope Key** 入口。后台用户管理 API、ADMIN/AUDITOR 角色、节点归属和项目成员校验由服务端执行，不能在手册中把它们描写成一个不存在的前端管理台。

权限变更后旧访问/刷新会话会失效；重新登录也只能获得变更后的权限。已归档项目仍校验成员关系，AUTONOMY 也不能绕过资源校验。浏览器使用 HttpOnly 会话 Cookie，状态变更附带 CSRF 校验；调试 API 的 Bearer 令牌不应粘贴到截图或文档中。详细设计见 [认证授权复核](security/authorization-review.md)。

## 16. 日志、版本与安全审计

### 16.1 云端日志 `/cloud-logs`

![云端日志查询](images/user-guide/26-cloud-logs.png)

先选项目，按需填写 Job ID、Task ID、图节点 ID、计算节点，选择是否同时查询各参与方，点击“拉取日志”。在日志查看器中检索和定位失败信息。

此页依赖阿里云 SLS 集成；尚未配置时不能据空结果认定任务没有日志。开发环境先用 DAG 节点日志、运行记录和 `bash scripts/dev/show-dev-logs.sh backend -f`（或 `alice` / `bob` / `all`）定位。本次展示查询表单，未配置外部 SLS 验收。

### 16.2 组件版本 `/component-versions`

![组件版本](images/user-guide/27-component-versions.png)

查看 Kuscia、SecretFlow、Serving、控制台、DataProxy、SCQL 和 TEE 等已配置镜像版本，点击“刷新”获取当前配置。未配置的组件不一定显示；镜像标签是部署核对依据，不是容器健康证明。需要确认运行状态时再查启动器、容器和任务日志。

### 16.3 审计 `/audit`

![安全审计](images/user-guide/28-audit.png)

具有 ADMIN/AUDITOR 角色时，可按页面提供的条件查询审计事件、查看结果和翻页。使用“校验哈希链”检查审计链完整性；读到事件不等于完成验真，应查看校验返回状态。

没有角色权限时应显示权限提示或后端拒绝。审计用于追踪访问、拒绝和管理操作，不代替算法执行日志。本次 API 冒烟验证也调用了审计验真接口。

## 17. P2P/AUTONOMY 与 EDGE 模式

以下截图由**真实临时平台实例**产生，不是用 CENTER 会话伪造菜单。临时实例共用本地计算基础设施、隔离业务数据库，关闭跨平台同步，用于说明不同平台的操作入口。完整多机构流程仍需部署双方独立 Kuscia、网关与 Privahub，并配置互信。

### 17.1 AUTONOMY 工作台与 P2P 项目 `/workbench`、`/p2p/projects`

![独立机构工作台](images/user-guide/40-p2p-workbench.png)

![P2P 项目](images/user-guide/41-p2p-projects.png)

工作台进入本机构协作入口。P2P 项目按全部/我发起/受邀、状态、计算模式和名称过滤。创建项目后向参与机构发起审批；受邀方核对内容后同意/拒绝。只有满足参与方审批条件才可进入计算流程。

项目发起方可在允许状态下修改名称和描述。已批准项目归档需要对应审批，未批准项目按业务状态允许直接归档。不要绕过待审批状态直接打开图并运行。

![P2P 项目创建表单](images/user-guide/60-create-p2p-project.png)

### 17.2 我的节点与机构管理 `/p2p/my-node`、`/institutions`

![独立机构我的节点](images/user-guide/42-p2p-my-node.png)

![机构管理](images/user-guide/43-institutions.png)

查看本机构名称、ID、主节点和附属节点；按权限新增、切换或管理节点，查看短期注册 Token。刷新 Token 会影响仍在使用旧 Token 的注册流程。主节点及已授权节点可能禁止删除，以具体禁用原因提示为准。

CENTER 使用平台属主，没有同样的机构管理入口；访问受限路由时应回到允许的首页，而不是显示一份空机构当成正常数据。

### 17.3 机构节点注册 `/inst-register`

![机构节点注册](images/user-guide/44-inst-register.png)

可在表单和 JSON 两种输入方式之间切换。填写 domainId、名称、模式、API 端口、传输端口、协议和网络地址，按要求上传客户端证书、私钥及 Token 文件。核对生成的 `json_data` 再提交。私钥是节点身份材料，不应放进项目附件或工单截图。

注册既要求字段合法，也要求真实远端服务、网络和证书对应；截图只展示空表单，没有把无效证书提交成功伪装成完整注册。

### 17.4 EDGE 节点视角

![EDGE 数据源](images/user-guide/45-edge-data-sources.png)

![EDGE 数据表](images/user-guide/46-edge-data-tables.png)

![EDGE 合作节点](images/user-guide/47-edge-cooperation.png)

![EDGE 结果](images/user-guide/48-edge-results.png)

EDGE 账户默认进入自身节点，侧栏围绕本节点数据源、数据表、合作节点、结果、消息与账户展开。临时演示库未接入中心数据同步，所以部分列表为空；实际部署应检查同步状态和节点授权后再使用这些入口。

## 18. 通用操作：语言、主题与移动端

![移动端导航](images/user-guide/31-mobile-navigation.png)

窄屏点击左上角导航按钮展开菜单，选择目标页面后收起；按 Escape 可以关闭。宽表格在其容器内横向滚动，避免为了查看右侧操作误认为字段消失。

![深色主题](images/user-guide/32-dark-theme.png)

右上角语言选择支持中英文切换；主题选择支持明色、暗色、跟随系统三种模式，移动端在账户菜单内以三个按钮切换。选择记录在当前浏览器（`secretpad-locale`、`secretpad-theme`），页面标题和 `html lang` 随语言更新；跟随系统时，操作系统主题变化会立即生效。主题和引导进度属于浏览器偏好，不改变权限和数据。右上角消息图标进入消息中心，账户菜单用于账户信息及退出。

| 检查项 | 操作 | 通过标准 |
|---|---|---|
| 明色/暗色 | 页头主题选择切换 Light / Dark | 页面立即切换；表头、卡片、输入框和文字对比正常，无白色残留区域 |
| 跟随系统 | 选择 System 后切换操作系统深浅色 | 页面即时跟随；刷新后仍显示 System |
| 中文/英文 | 页头语言选择切换中文 / English | 侧栏、页头、按钮、提示和页面标题同步切换；刷新后保持 |
| 移动端 | 390 px 宽度打开账户菜单 | Light / Dark / System 三个按钮可选，选择后菜单关闭并立即生效 |

以上检查已由 Chromium 自动化覆盖：明暗切换写入 `data-theme` 与 `.dark`，跟随系统监听 `prefers-color-scheme`，中英文切换更新 `document.title` 与 `header` 文案。

## 19. 按顺序完成一次真实计算

| 步骤 | 操作 | 应看到什么 | 不符合预期时 |
|---|---|---|---|
| 1 | `bash scripts/dev/start-docker-dev-stack.sh` | 前后端健康、Kuscia 三节点和双向路由就绪 | 查看 `--check` 结果、`show-dev-status.sh`、端口、容器和启动日志 |
| 2 | 浏览器登录 | CENTER 总览/引导 | 检查当前密码、账户上下文和会话 |
| 3 | 节点页进入 alice/bob | 两方 Ready，数据页有输入表 | 检查路由、数据源、DomainData |
| 4 | 创建 MPC 项目，加入 alice/bob，授权表 | 项目详情中参与方和表对应 | 检查节点归属与表授权 |
| 5 | 新建图，两个样本表连接 PSI | 三个节点、两条边 | 核对组件输入输出、关联键与参数 |
| 6 | 保存、运行 | 两个读表与 PSI 均成功 | 从首个失败节点检查日志与数据路径 |
| 7 | 运行记录打开本次任务 | `SUCCEED`，3/3 节点完成 | 等待同步后刷新，核对 Job ID |
| 8 | 结果页选择 alice，查看本次输出 | 表/报告可打开；允许时可下载 | 核对节点、筛选、归属及存储类型 |
| 9 | 按需创建周期任务 | 成功图可创建；本示例为已下线 | 检查图是否成功过、时间范围和任务状态 |
| 10 | 审计查询/验真 | 事件可查询，链校验成功 | 检查角色、条件及服务日志 |

本次截图生成的真实资源 ID 和输出类型以 [manifest.json](images/user-guide/manifest.json) 的 `psi` 为准，避免复制截图中的旧 ID 后误查其他批次。

## 20. 常见问题与验收范围

| 现象 | 排查/处理 |
|---|---|
| 端口被其他服务占用 | 使用启动器输出的进程/端口信息定位；可配置 BACKEND_PORT、FRONTEND_PORT、INNER_PORT，不能直接停止不属于本项目的服务 |
| 本机 Kuscia 不是 18083 | 按 `.env` 配置端口变量，重新启动后端使连接配置生效 |
| 登录后回到首页 | 会话过期、角色/平台限制或资源不属于当前身份；检查账户信息和后端拒绝原因 |
| 数据表上传成功但计算找不到文件 | 检查 PRIVAHUB_DATA_DIR 与 lite 节点挂载目录链接，确认文件位于正确节点 |
| 有项目但画布没数据表 | 检查项目参与节点、表授权，以及是否选错项目/图 |
| 任务处于 STOPPING | 等待实际计算停止及状态同步，停止中仍限制下线等操作 |
| 创建周期任务按钮不可提交 | 先成功运行图，再检查图、节点、时间与周期表单 |
| 模型页没有可发布模型 | PSI 不是模型训练；先运行训练并打包真实模型，确认 Serving 和特征服务 |
| 机构页面入口消失 | CENTER 没有 AUTONOMY 机构管理入口，检查部署模式 |
| SLS 无日志/TEE 不可用 | 分别配置云日志服务或 TEE 环境，普通中心启动不会自动具备这些能力 |
| 截图脚本失败 | 检查 logs/user-guide-*/、Chromium 安装、实际密码、保留 PSI 项目和前端地址；失败会返回非零，不以旧图片当作本轮成功 |

页面访问、真实 API 冒烟、真实两方 PSI 和完整外部设施验收是不同层次。本文明确标注已运行与未部署部分；迁移及系统测试的详细边界见 [迁移验收说明](migration-acceptance.md) 和 [测试说明](testing.md)。

### 20.1 验证记录

**截图基线（2026-10-15）**

| 检查 | 结果 |
|---|---|
| 后端 | `go test ./...`、`go vet ./...`、service 包 `go test -race` 通过 |
| 前端 | 类型检查、Lint、生产构建通过；当次 62 个测试文件、413 项测试通过 |
| 系统性测试 | 独立数据库 HTTP 业务流程、权限撤销/刷新会话验证、10 项 Chromium E2E、真实两方 PSI 通过 |
| 启动与脚本 | 完整栈启动和重启、真实 API/Kuscia 冒烟、当次 36 项脚本测试通过 |
| 手册素材 | 60 张 PNG、SHA256、文档链接及 36 个前端路由页面截图覆盖检查通过 |

**当前复核与测试基线（2026-10-21）**

| 检查 | 结果 | 说明 |
|---|---|---|
| 手册素材 | ✅ 60 张截图 / 36 个前端页面 | `python3 scripts/test/check-user-guide.py`，链接、哈希与路由覆盖全部通过 |
| 脚本入口 | ✅ 与 `scripts/<分类>/` 当前目录一致 | 本文命令逐项对照各脚本 `--help` |
| 后端单元 | ✅ 全部包通过 | `CGO_ENABLED=1 go test -count=1 ./...`，未启用 `-race` |
| 前端单元 | ✅ 69 个文件 / 483 条用例 | `corepack pnpm test`，未启用覆盖率 |
| 脚本自测 | ✅ 82 个用例，0 失败，1 SKIP | `test_smoke_spawn` 因未构建后端而跳过 |
| 运维监控 | ✅ Prometheus 双目标 UP、Grafana 健康、数据源与大屏已加载 | 2026-10-21 本地实测，详见 §21.7 |

当前测试基线的详细环境、跳过项和证据见 [测试文档](../../docs/testing/README.md)；它只代表本机当次执行，不代表 CI 或发布结论。截图基线拍摄过程中修复了未来调度预渲染作业误入执行记录、结果详情对 Go 空切片及 sf.report 数组格式不兼容、弹窗/抽屉键盘焦点与 Escape 关闭缺失，以及启动器工具链和内部端口检查不准确的问题，并补充相关回归验证。

## 21. 运维监控：前端与 Grafana 大屏

本章在已经运行的开发栈上追加 Prometheus 与 Grafana，验证“业务前端可用 + 指标被抓取 + 运维大屏可读”。所有截图与命令均在 2026-10-21 本机真实环境执行，截图位于 [images/ops-monitoring](images/ops-monitoring/)，校验文件为 `SHA256SUMS`。生产环境请使用 Compose 生产档或 Helm，并替换默认口令、启用 TLS。

### 21.1 组件、端口与数据流

| 组件 | 默认地址 | 作用 | 是否由开发栈自动启动 |
|---|---|---|---|
| Privahub 前端 | <http://127.0.0.1:8000> | 业务控制台 | 是 |
| Privahub 后端 | `127.0.0.1:8080` | REST API 与 `/metrics` | 是 |
| Prometheus | <http://127.0.0.1:9090> | 每 15 秒抓取 `/metrics`，加载告警规则 | 否 |
| Grafana | <http://127.0.0.1:3000> | 查询 Prometheus，展示平台总览大屏 | 否 |

```text
浏览器 ── 8000 ──▶ Vite 前端 ── /api ──▶ Go 后端 8080 ── /metrics ──▶ Prometheus 9090
                                                   │
浏览器 ── 3000 ──▶ Grafana ── PromQL ──────────────┘
```

实际端口以 `bash scripts/dev/show-dev-status.sh` 和 `.env` 为准。监控只读取指标，不修改业务数据；关闭浏览器不会停止前端、后端或监控容器。

### 21.2 启动：开发栈 + Prometheus + Grafana

先确认前端、后端与 Kuscia 已就绪：

```bash
cd /home/charles/code/clwork
bash scripts/dev/start-docker-dev-stack.sh --status
curl -fsS http://127.0.0.1:8080/api/v1alpha1/healthz
curl -fsS -o /dev/null -w 'frontend=%{http_code}\n' http://127.0.0.1:8000/
curl -fsS http://127.0.0.1:8080/metrics | grep '^secretpad_http_requests_total' | head
```

健康检查应返回 `{"data":{"service":"secretpad-go","status":"healthy"},...}`，前端返回 200，指标端点应出现 `secretpad_http_requests_total`。随后启动监控（Linux / WSL2，使用 host 网络让容器访问宿主机 `127.0.0.1`）：

```bash
cd /home/charles/code/clwork
docker rm -f sfwork-prometheus-local sfwork-grafana-local >/dev/null 2>&1 || true

docker run -d --name sfwork-prometheus-local --network host \
  -v "$PWD/deploy/prometheus/prometheus.local.yml:/etc/prometheus/prometheus.yml:ro" \
  -v "$PWD/deploy/prometheus/alerts.yml:/etc/prometheus/alerts.yml:ro" \
  prom/prometheus:v2.55.1 \
  --config.file=/etc/prometheus/prometheus.yml \
  --storage.tsdb.path=/prometheus \
  --web.listen-address=127.0.0.1:9090 \
  --web.enable-lifecycle

docker run -d --name sfwork-grafana-local --network host \
  -e GF_SECURITY_ADMIN_USER=admin \
  -e GF_SECURITY_ADMIN_PASSWORD=admin \
  -e GF_USERS_ALLOW_SIGN_UP=false \
  -e GF_SERVER_HTTP_ADDR=127.0.0.1 \
  -v "$PWD/deploy/grafana-local/datasource.yml:/etc/grafana/provisioning/datasources/datasource.yml:ro" \
  -v "$PWD/deploy/grafana/dashboards:/etc/grafana/provisioning/dashboards:ro" \
  grafana/grafana:11.2.0
```

等待并核验三个入口：

```bash
curl -fsS http://127.0.0.1:9090/-/ready
curl -fsS http://127.0.0.1:3000/api/health
GRAFANA_USER=admin GRAFANA_PASSWORD=admin  # 与上方 docker run 的本地开发默认值一致
curl -fsS -u "$GRAFANA_USER:$GRAFANA_PASSWORD" http://127.0.0.1:3000/api/datasources
```

- `prometheus.local.yml` 的抓取目标是宿主机 `127.0.0.1:8080`，因此 Prometheus 必须使用 `--network host`；普通桥接网络会把 `127.0.0.1` 解析为容器自身。
- `grafana-local/datasource.yml` 指向 `http://127.0.0.1:9090`，与上述 host 网络配套；容器编排场景改用 `deploy/grafana/datasources/datasources.yml`（地址为 `prometheus:9090`）。
- 两个服务都只监听回环地址。Grafana 镜像版本与截图一致为 11.2.0；Compose 默认使用 11.3.1，可按需替换。
- macOS / Windows 的 Docker Desktop 不支持 `--network host`：可直接在宿主机运行 Prometheus/Grafana 二进制并加载 `prometheus.local.yml` 与 `grafana-local/datasource.yml`，或使用下一节的一体化 Compose 方式。

一体化 Compose（会启动容器化 Privahub，不能与当前开发栈同时占用 8080）：

```bash
cd deploy/docker-compose
cp .env.example .env   # 首次
docker compose --profile monitoring up -d
```

### 21.3 Grafana 登录与数据源

打开 <http://127.0.0.1:3000>，开发环境默认账号 `admin / admin`。首次登录如提示修改密码，可先跳过；正式环境必须通过 `GRAFANA_ADMIN_PASSWORD` 设置强口令并修改默认账户。

![Grafana 登录页](images/ops-monitoring/02-grafana-login.png)

Grafana 启动时自动加载两份 provisioning 配置：

| 配置 | 内容 | 核验方法 |
|---|---|---|
| 数据源 | 名称 `Prometheus`，uid `prometheus`，默认数据源，地址 `http://127.0.0.1:9090` | Grafana → Connections → Data sources，或调用 `/api/datasources` |
| 大屏 | 文件夹内 JSON 自动加载，uid `sfwork-overview`，标题“SFWork 平台总览” | Grafana → Dashboards，或调用 `/api/search?query=` |

`uid` 不能随意修改：大屏 JSON 通过 `uid: prometheus` 引用数据源，改名会出现 “Datasource not found”。

### 21.4 平台总览大屏

地址：<http://127.0.0.1:3000/d/sfwork-overview?from=now-30m&to=now&refresh=30s>。墙体大屏可追加 `&kiosk` 隐藏导航栏。

![Grafana 平台总览大屏](images/ops-monitoring/03-grafana-dashboard.png)

![Grafana kiosk 大屏模式](images/ops-monitoring/04-grafana-dashboard-kiosk.png)

| 面板 | 指标/查询 | 判读方式 |
|---|---|---|
| 服务可用性 | `up{job="privahub"}`、`up{job="prometheus"}` | 两条均为 `UP`（值 1）；出现 `DOWN` 先看 Targets |
| 请求速率 | `rate(secretpad_http_requests_total[5m])` 按 endpoint | 空闲时为 0 或没有曲线；产生访问后出现分接口曲线 |
| P95 延迟 | `histogram_quantile(0.95, ...duration_seconds_bucket...)` | 只统计已产生请求的时间窗；无请求时无数据 |
| 5xx 错误率 | 5xx 请求速率 / 总请求速率 | 正常应接近 0%；持续升高时结合后端日志定位 |
| 审计队列深度 | `privahub_audit_queue_depth` | 持续增长说明审计写入跟不上 |
| 审计 Spool 待重放 | `privahub_audit_spool_bytes` | 大于 0 表示存在待重放数据，检查审计下游 |
| 审计事件写入结果 | `privahub_audit_events_total` 按 outcome | 观察 persisted/dropped 等结果比例 |
| Kuscia CRD Reconcile | `secretpad_kuscia_reconcile_total` 按 resource/result | 观察错误结果是否增长，配合 Kuscia 日志；当前 Go 后端只注册了指标、未在业务路径累加，因此显示 `No data`，属于已知限制 |

大屏默认时间窗为最近 30 分钟、30 秒刷新。截图中请求速率在 12:55 出现阶跃，是执行验收时产生的真实流量；没有流量时面板显示 0 或空图并不等于监控故障。`Kuscia CRD Reconcile` 面板在当前版本没有样本，不能据此判断 Kuscia 正常或异常，应以 Targets、任务状态和 Kuscia 日志为准。

### 21.5 Prometheus Targets、指标与告警规则

![Prometheus Targets](images/ops-monitoring/01-prometheus-targets.png)

```bash
# 就绪与目标状态
curl -fsS http://127.0.0.1:9090/-/ready
curl -s 'http://127.0.0.1:9090/api/v1/targets?state=active' \
  | python3 -c "import json,sys; [print(t['labels']['job'], t['health'], t['scrapeUrl']) for t in json.load(sys.stdin)['data']['activeTargets']]"

# up 指标应为 1
curl -s 'http://127.0.0.1:9090/api/v1/query?query=up' \
  | python3 -c "import json,sys; [print(r['metric']['job'], r['value'][1]) for r in json.load(sys.stdin)['data']['result']]"
```

预期输出 `privahub up` 与 `prometheus up`；截图中的两个目标均为 `UP`，抓取耗时在毫秒级。告警规则随 `alerts.yml` 加载，共 4 组 9 条：

| 规则组 | 规则 |
|---|---|
| `sfwork.availability` | `SfworkTargetDown` |
| `sfwork.http` | `PrivahubHighErrorRate`、`PrivahubHighLatencyP95` |
| `sfwork.runtime` | `PrivahubGoroutineLeak`、`KusciaReconcileErrors` |
| `sfwork.audit` | `AuditEventsDropped`、`AuditQueueBacklog`、`AuditSpoolBacklog`、`AuditAnchorFailing` |

```bash
curl -s http://127.0.0.1:9090/api/v1/rules \
  | python3 -c "import json,sys; [print(g['name'], len(g['rules'])) for g in json.load(sys.stdin)['data']['groups']]"
```

![Prometheus 告警规则](images/ops-monitoring/07-prometheus-rules.png)

页面应显示 4 个规则组、9 条规则，`State` 为 `OK`，`Error` 为空；有规则处于 `PENDING`/`FIRING` 时按规则名和标签定位真实资源。

### 21.6 前端与 Grafana 同时运行

两个服务互不依赖，可同时打开并长期运行；关闭任一浏览器页面都不会影响另一个服务或后端任务。

![前端控制台与 Grafana 大屏同时运行](images/ops-monitoring/06-frontend-and-grafana.png)

| 窗口 | 地址 | 用途 |
|---|---|---|
| 业务控制台 | <http://127.0.0.1:8000> | 建项目、跑 DAG、看结果 |
| 运维大屏 | <http://127.0.0.1:3000/d/sfwork-overview?kiosk> | 观察可用性、流量、延迟、审计与 Kuscia 状态 |

![前端控制台（中文、明色）](images/ops-monitoring/05-frontend-dashboard.png)

同时运行检查：

```bash
curl -fsS -o /dev/null -w 'frontend=%{http_code} ' http://127.0.0.1:8000/
curl -fsS -o /dev/null -w 'grafana=%{http_code}\n' http://127.0.0.1:3000/api/health
```

两个值都应为 200。生产展示建议为 Grafana 创建只读 Viewer 账户，不要在大屏上使用管理员账户；`kiosk` 只是隐藏导航，不改变权限。

### 21.7 验收确认（签收清单）

前置条件：开发栈与监控容器均已启动，执行人可访问 8000、8080、9090、3000。逐项记录“本次实测”作为交付证据；未执行或不符合的项目不能签收。

| 序号 | 验收项 | 检查方法 | 通过标准 | 本次实测（2026-10-21） |
|---|---|---|---|---|
| 1 | 业务前端 | 浏览器打开 8000，使用实际账号登录 | 登录成功并进入总览/引导，无白屏或持续报错 | ✅ 登录后进入控制台总览 |
| 2 | 后端健康 | `curl -fsS http://127.0.0.1:8080/api/v1alpha1/healthz` | 返回 `status: healthy` | ✅ `secretpad-go / healthy` |
| 3 | 指标端点 | `curl -fsS http://127.0.0.1:8080/metrics \| grep secretpad_http_requests_total` | HTTP 200，包含业务与 Go 运行时指标 | ✅ 200，指标可读 |
| 4 | Prometheus 就绪与抓取 | `/-/ready`、`/api/v1/targets?state=active`、查询 `up` | `Ready`；`privahub`、`prometheus` 均 `UP` | ✅ 2/2 UP |
| 5 | 告警规则 | `curl -s http://127.0.0.1:9090/api/v1/rules` 或 `/rules` 页面 | 4 组 9 条规则已加载，状态 OK、无解析错误 | ✅ `sfwork.availability/http/runtime/audit`，9 条全部 OK |
| 6 | Grafana 健康 | `curl -fsS http://127.0.0.1:3000/api/health` | `database: ok` | ✅ `database ok`，11.2.0 |
| 7 | 数据源 | Grafana 数据源页或 `/api/datasources` | `Prometheus`、uid `prometheus`、默认数据源 | ✅ 已自动配置 |
| 8 | 平台总览大屏 | 打开 `/d/sfwork-overview`，产生少量业务访问后观察 | 服务可用性 UP；请求速率/延迟/审计出现曲线；Kuscia 面板无样本时显示 `No data`（已知限制） | ✅ 8 个面板；可用性、流量、延迟、审计有数据 |
| 9 | 同时运行 | 同时打开 8000 与 3000，执行 §21.6 命令 | 两个入口均为 200，互不影响 | ✅ 见并排截图 |
| 10 | 权限与暴露面 | `ss -ltn \| grep -E ':(3000\|9090)'`，检查 Grafana 账户 | 仅监听回环；非默认口令用于非本机环境 | ✅ 均绑定 127.0.0.1 |
| 11 | 重启恢复 | 按 §21.8 执行 `docker restart` 后复查健康与 Targets | 两个服务恢复，抓取目标重新 UP | ✅ 1 秒内恢复，2/2 UP |

签收记录：

| 项目 | 内容 |
|---|---|
| 验收结论 | □ 通过　□ 有条件通过　□ 不通过 |
| 验收人 / 日期 |  |
| 遗留问题与整改期限 |  |
| 证据位置 | `privahub/docs/images/ops-monitoring/`、`logs/`、Prometheus/Grafana 容器日志 |

### 21.8 停止、重启与数据保留

```bash
# 暂停与恢复（保留容器内数据）
docker stop sfwork-grafana-local sfwork-prometheus-local
docker restart sfwork-prometheus-local sfwork-grafana-local

# 删除本地监控容器（容器内指标历史与 Grafana 状态一并丢失）
docker rm -f sfwork-grafana-local sfwork-prometheus-local

# 只停止业务开发栈，保留监控
bash scripts/dev/stop-dev-stack.sh
```

上面的启动命令没有挂载数据卷，Prometheus 指标和 Grafana 运行状态保存在容器内，删除容器即清空；业务数据、审计和 Kuscia 数据不受影响。需要跨重启保留历史时，启动时追加 `-v sfwork-prometheus-data:/prometheus` 和 `-v sfwork-grafana-data:/var/lib/grafana`，并在清理时显式删除对应卷。

### 21.9 监控故障排查

| 现象 | 排查/处理 |
|---|---|
| Grafana 面板全部 No data | 先在 Prometheus 查询 `up`；检查数据源地址与时间窗；请求类面板需要先产生业务访问 |
| Datasource not found | 数据源 `uid` 必须为 `prometheus`，与大屏 JSON 引用一致；确认 provisioning 挂载路径 |
| Prometheus 目标 DOWN | 后端未启动、端口不一致或被防火墙拦截；在 Prometheus 容器内确认能访问 `127.0.0.1:8080/metrics` |
| 桥接网络下目标 127.0.0.1 失败 | 容器内的 127.0.0.1 是容器自身；Linux/WSL2 使用 `--network host`，或把目标改为 `host.docker.internal:8080` |
| `/metrics` 返回 404 | 检查 `observability.enable_metrics` 与 `metrics_access`，见 [配置说明](configuration.md) |
| Grafana 打不开或健康检查失败 | `docker logs sfwork-grafana-local`；检查 3000 端口占用与 `GF_SERVER_HTTP_ADDR` |
| 大屏列表为空 | 确认 `dashboards.yml` 与 `sfwork-overview.json` 已挂载到 `/etc/grafana/provisioning/dashboards`，稍等一个扫描周期 |
| Kuscia Reconcile 面板 No data | 当前 Go 后端未在该指标上累加，属于已知限制；改用 Targets、任务状态和 Kuscia 日志判断 |
| 告警规则未加载 | 检查 `alerts.yml` 是否挂载为 `/etc/prometheus/alerts.yml`，Prometheus 启动日志应有 rule files 加载记录 |
| 修改配置不生效 | 开发容器未挂载可写卷；修改仓库文件后按 §21.8 删除并重新创建容器 |

## 联合建模补充指南

八类算法预定义画布、12 项本地隐私实测及截图见 [联合建模预定义画布与全流程实测](modeling_user_guide.md)。

## 自定义画布与预定义画布

侧栏“自定义画布”（`/dag`）进入项目编辑器；“预定义画布”（`/dag-templates`）进入独立模板库。模板按机器学习、基础模板、隐私组件和引导模板分类，每组显示数量并支持展开、折叠；默认展开机器学习。搜索会自动展示各分类中的匹配模板，清空搜索后恢复原来的折叠状态。选择算法可单独预览，点击“使用此画布”创建专属副本，再配置数据、参数并运行。每个画布独立保存，模板之间不叠加。

![独立的自定义和预定义画布导航](images/user-guide/60-predefined-canvases.png)

## 项目分类与场景查找

项目页支持按计算模式（MPC / TEE）或项目状态分组，每组显示数量并可折叠。显示方式支持紧凑列表、分页卡片和全部卡片：默认紧凑列表，每个项目一行且不分页；全部卡片保留原来的全量展示；分页卡片才按每组 6 个项目分页。显示方式会在当前浏览器记住。搜索会匹配项目名称、ID、描述和参与节点。分类依据项目实际字段，不会根据项目名称猜测业务类型。

![项目分组与分页](images/project-browser/projects-grouped.png)

自定义画布的项目选择器按计算模式分组。隐私场景页支持按分类筛选，并可搜索场景名称、描述和技术标签；筛选不影响创建权限与计算模式限制。

![隐私场景分类筛选](images/project-browser/scenes-filtered.png)

## 主页菜单布局与项目视图

侧栏按使用流程排列为工作入口、项目与画布、数据资源、任务与成果、节点与协作、平台管理。不同部署和账号仍只显示可访问入口，空分组不展示。自定义画布与预定义画布保持两个独立入口。

项目页“显示方式”可随时选择：

- 紧凑列表：每个项目一行，显示名称、模式、状态、训练流数量、任务数量、参与节点和操作，全部匹配项目一页展示；窄屏可在列表内部横向滚动。
- 分页卡片：每组每页 6 个项目。
- 全部卡片：不分页，恢复全部项目卡片展示。

切换显示方式不会修改项目配置。列表中点击名称打开详情，描述与 ID 可通过名称悬停查看，或在详情中读取。

![紧凑项目列表与菜单分组](images/project-browser/projects-list.png)

## 统一列表、筛选和详情

数据表、模型、结果、运行记录列表右上方提供“行高”和“列设置”，可调整密度、可见列和宽度，浏览器会记住选择。表头通用排序作用于当前页；运行记录的关键词、状态、起止时间筛选在服务端执行，覆盖全部分页。清空筛选保留当前项目或节点。

行操作保留常用入口，其他操作位于“更多”。项目、模型和结果使用统一详情抽屉，通过标签切换信息；Escape 关闭抽屉。DAG 配置区增加校验摘要，可展开待完善项并定位到字段。

![统一列表和任务筛选](images/visual-system/06-job-records.png)

![项目详情抽屉](images/visual-system/08-project-details.png)

总览与工作台优先显示近期任务、最近项目和待处理事项，资源统计可展开查看。完整改动、截图与验证方法见 [页面信息展示优化](display-optimization.md)。
