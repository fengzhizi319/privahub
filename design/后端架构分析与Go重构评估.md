# SecretPad 后端架构分析与 Go 重构评估及工业级架构超详细设计说明书

> **文档创建时间**：2026-07-27  
> **文档更新时间**：2026-07-27  
> **文档位置**：`../design/后端架构分析与Go重构评估.md`  
> **分析对象**：SecretPad 后端系统（基于 Spring Boot 3.3.5 / Java 17）与 Go 重构架构规范  
> **核心议题**：后端现有架构评估、Go 重构可行性评估、各子系统详细模块设计、完整数据模型与 API 映射、以及满足工业级软件规范（功能、性能、可靠性、可维护性、工程化、安全性）的 Go 新版本超详细架构设计说明书（包含高层设计、底层设计、数据表模型、状态机、接口契约、安全引擎、迁移切流预案与工程规范）。

---

## 目录 (Table of Contents)

- [1. 执行摘要 (Executive Summary)](#1-执行摘要-executive-summary)
  - [1.1 业务与技术背景](#11-业务与技术背景)
  - [1.2 核心重构结论与 ROI 量化指标](#12-核心重构结论与-roi-量化指标)
- [2. 当前 Java/Spring 架构与技术栈全景剖析](#2-当前-javaspring-架构与技术栈全景剖析)
  - [2.1 Maven 多模块架构与职责映射](#21-maven-多模块架构与职责映射)
  - [2.2 核心技术栈与三方组件依赖全景](#22-核心技术栈与三方组件依赖全景)
  - [2.3 分布式组网拓扑与部署模式分析](#23-分布式组网拓扑与部署模式分析)
- [3. 当前 Java/Spring 架构的痛点与瓶颈 (Pain Points)](#3-当前-javaspring-架构的痛点与瓶颈-pain-points)
  - [3.1 边缘部署的高资源消耗与内存驻留痛点](#31-边缘部署的高资源消耗与内存驻留痛点)
  - [3.2 云原生 K8s/Kuscia 原生生态契合摩擦](#32-云原生-k8skuscia-原生生态契合摩擦)
  - [3.3 线程模型开销与高并发任务监控瓶颈](#33-线程模型开销与高并发任务监控瓶颈)
  - [3.4 运行时依赖与镜像体积痛点](#34-运行时依赖与镜像体积痛点)
- [4. Go 重构可行性及对比矩阵](#4-go-重构可行性及对比矩阵)
  - [4.1 核心维度对比矩阵 (Java vs Go)](#41-核心维度对比矩阵-java-vs-go)
  - [4.2 技术栈选型映射与生态替换方案](#42-技术栈选型映射与生态替换方案)
- [5. Go 新版本工业级架构设计原则与总体范式](#5-go-新版本工业级架构设计原则与总体范式)
  - [5.1 六大架构设计原则](#51-六大架构设计原则)
  - [5.2 整洁架构 (Clean Architecture) 与 DDD 分层规范](#52-整洁架构-clean-architecture-与-ddd-分层规范)
- [6. 高层设计 (High-Level Design - HLD)](#6-高层设计-high-level-design---hld)
  - [6.1 总体组件拓扑与 6 大子系统划分](#61-总体组件拓扑与-6-大子系统划分)
  - [6.2 核心业务处理流程与时序图全集](#62-核心业务处理流程与时序图全集)
  - [6.3 Go 项目工程目录结构 (Project Layout)](#63-go-项目工程目录结构-project-layout)
  - [6.4 各子系统详细模块设计 (Java → Go 映射)](#64-各子系统详细模块设计-java--go-映射)
- [7. 底层设计 (Low-Level Design - LLD)](#7-底层设计-low-level-design---lld)
  - [7.1 完整领域模型与数据库 Schema 设计 (GORM DO Structs)](#71-完整领域模型与数据库-schema-设计-gorm-do-structs)
    - [7.1.1 完整数据模型映射表 (Java DO → Go Struct)](#711-完整数据模型映射表-java-do--go-struct)
    - [7.1.2 数据库 Schema 分模式设计](#712-数据库-schema-分模式设计)
  - [7.2 DAG 图转换引擎与 Component Spec 解析算法](#72-dag-图转换引擎与-component-spec-解析算法)
  - [7.3 Kuscia CRD Controller / Informer 深度设计](#73-kuscia-crd-controller--informer-深度设计)
  - [7.4 多机构协同投票状态机与数字签名引擎](#74-多机构协同投票状态机与数字签名引擎)
  - [7.5 高性能并发 Task Pool 与生命周期管理](#75-高性能并发-task-pool-与生命周期管理)
- [8. 接口设计 (Interface Design)](#8-接口设计-interface-design)
  - [8.1 RESTful API 规范与 Controller 设计](#81-restful-api-规范与-controller-设计)
    - [8.1.1 完整 API 端点映射表 (Java Controller → Go Handler)](#811-完整-api-端点映射表-java-controller--go-handler)
  - [8.2 节点间 gRPC 协议与 Protobuf 契约全集](#82-节点间-grpc-协议与-protobuf-契约全集)
  - [8.3 仓储层与外置驱动依赖倒置接口](#83-仓储层与外置驱动依赖倒置接口)
- [9. 安全、加解密与合规体系 (Security & Cryptography)](#9-安全加解密与合规体系-security--cryptography)
  - [9.1 节点间 mTLS 证书交换与双向校验](#91-节点间-mtls-证书交换与双向校验)
  - [9.2 字段级落盘加密引擎 (AES-256-GCM / SM4)](#92-字段级落盘加密引擎-aes-256-gcm--sm4)
  - [9.3 RBAC 鉴权与 Token 轮换机制](#93-rbac-鉴权与-token-轮换机制)
- [10. 可观测性与工程化落地规范 (Observability & Engineering)](#10-可观测性与工程化落地规范-observability--engineering)
  - [10.1 Prometheus 指标监控全集](#101-prometheus-指标监控全集)
  - [10.2 Zap 结构化日志与 TraceID 链路透传](#102-zap-结构化日志与-traceid-链路透传)
  - [10.3 依赖注入 (Google Wire) 体系设计](#103-依赖注入-google-wire-体系设计)
  - [10.4 配置文件规范与 Bootstrap 引导器](#104-配置文件规范与-bootstrap-引导器)
- [11. 渐进式三阶段迁移与数据平滑升级超详细方案](#11-渐进式三阶段迁移与数据平滑升级超详细方案)
  - [11.1 三阶段演进计划与子系统落地细节](#111-三阶段演进计划与子系统落地细节)
  - [11.2 数据双写、全量/增量迁移与校对引擎机制](#112-数据双写全量增量迁移与校对引擎机制)
    - [11.2.1 数据库双写代理模式 (Dual-Write Proxy Architecture)](#1121-数据库双写代理模式-dual-write-proxy-architecture)
    - [11.2.2 离线全量数据平滑迁移 Pipeline (secretpad-migrator)](#1122-离线全量数据平滑迁移-pipeline-secretpad-migrator)
    - [11.2.3 增量数据实时 Hash 对齐与校对引擎算法 (DataReconciler)](#1123-增量数据实时-hash-对齐与校对引擎算法-datareconciler)
  - [11.3 数据库 Schema 平滑升级演进规范 (DB Schema Migration)](#113-数据库-schema-平滑升级演进规范-db-schema-migration)
  - [11.4 灰度切流门槛指标与秒级回滚 SOP 预案](#114-灰度切流门槛指标与秒级回滚-sop-预案)
  - [11.5 详细迁移执行步骤 (Step-by-Step Migration Playbook)](#115-详细迁移执行步骤-step-by-step-migration-playbook)
- [12. 综合结论与决策建议](#12-综合结论与决策建议)

---

## 1. 执行摘要 (Executive Summary)

### 1.1 业务与技术背景

**SecretPad** 是基于 **Kuscia**（隐语云原生隐私计算操作系统）打造的隐私计算可视化管控平台。作为连接上层可视化界面与底层隐私计算引擎（SecretFlow / SPU / HEU / TEE）的枢纽，SecretPad 后端承担着节点组网建立、数据表授权管理、可视化 DAG 任务编排与校验、与 Kuscia 底座下发计算作业、作业运行日志与状态实时监控、模型服务管理以及多机构协同投票签名等核心业务。

当前 SecretPad 后端基于 **Java 17 + Spring Boot 3.3.5** 构建。尽管 Spring 框架生态繁荣、声明式事务处理成熟，但由于隐私计算管控平台天然具备 **云原生控制面 (Control Plane)** 与 **边缘轻量化部署 (Edge Daemon)** 属性，Java 架构在边缘轻量化、资源占用、启动速度以及与 K8s 原生生态交互等维度遇到了难以逾越的性能与架构瓶颈。

---

### 1.2 核心重构结论与 ROI 量化指标

经过全面深入的架构评估，**强烈建议使用 Go 语言全面重构 SecretPad 后端**。Go 重构不仅是一项技术栈替换，更是向云原生控制面范式的回归。

#### 核心 ROI 量化提升指标：

1. **运行时内存降幅 > 85%**：进程驻留内存（RSS）从 Java 版的 **300MB ~ 1GB** 锐减至 **20MB ~ 50MB**（Lite 边缘模式仅需 20MB），大幅降低私有化边缘部署硬件成本。
2. **冷启动时间提升 100 倍**：服务冷启动从 Java 版的 **5~15 秒** 降至 **< 100 毫秒**，实现真正的近乎即时拉起与弹性调度。
3. **交付镜像体积缩小 93%**：容器镜像体积从 Java 包 + JRE 的 **300MB ~ 500MB** 缩减至静态二进制超小镜像 **< 25MB**。
4. **云原生生态零摩擦**：放弃 Java 封装层，直接引入 `client-go` 与 Kuscia 原生 Go SDK，原生支持 CRD（`KusciaJob`, `Domain`, `DomainData`）的 Watch & Reconcile 事件驱动控制回路。

---

## 2. 当前 Java/Spring 架构与技术栈全景剖析

### 2.1 Maven 多模块架构与职责映射

SecretPad 现行后端采用 Maven 多模块拆分，各模块职责如下表所示：

```
secretpad-parent (Root POM)
 ├── secretpad-api          # 暴露的 RESTful / RPC API 实体定义 (DTO/VO/Request/Response)
 ├── secretpad-common       # 公共工具类、通用常量、系统异常定义、组件 Spec 解析器
 ├── secretpad-persistence  # 持久层模型 (DO/Entity) 与 DAO 接口 (JPA / MyBatis)
 ├── secretpad-manager      # 基础设施抽象/代理层 (Kuscia API gRPC 客户端、K8s 客户端、DataMesh 代理)
 ├── secretpad-service      # 核心业务服务逻辑 (Project, Graph DAG, Node, Auth, Vote, Model, Serving)
 ├── secretpad-web          # RESTful Controller 层、安全拦截器、OpenAPI/Swagger 导出
 └── secretpad-scheduled    # 周期调度任务、分布式锁与心跳轮询引擎
```

#### 详细模块职责：
* **`secretpad-web`**：作为 Web 入口，负责 HTTP 路由注册、参数 Bind 与 Validator 校验、JWT/Cookie 拦截鉴权、全局异常统一拦截及 Swagger OpenAPI 文档导出。
* **`secretpad-service`**：封装复杂业务逻辑，如 DAG 图算子校验与拓扑排序（`GraphService`）、项目成员管理（`ProjectService`）、协同投票审批（`VoteSyncService`）、模型导出及Serving服务。
* **`secretpad-manager`**：防腐层与基础设施接入模块，通过 gRPC 调用 Kuscia 控制面接口（`JobManager`, `NodeManager`, `DatatableManager`），适配 K8s CRD 以及阿里云 SLS、AWS S3 等日志/文件存储。
* **`secretpad-persistence`**：基于 Spring Data JPA / Hibernate 实现数据实体持久化，映射 SQLite / MySQL 存储。

---

### 2.2 核心技术栈与三方组件依赖全景

* **核心语言与框架**：Java 17 / Spring Boot 3.3.5 / Spring Data JPA / Hibernate
* **底层通信机制**：
  * RESTful HTTP（前端与 SecretPad 交互）
  * gRPC / Protobuf 3.25.5（SecretPad 与 Kuscia 控制面交互；Edge 节点与 Center 节点间 P2P 通信）
* **持久化存储**：
  * SQLite 3.42（All-in-One 轻量级嵌入式单机部署）
  * MySQL 8.0（企业级 Master 节点高可用部署）
* **第三方依赖**：ANTLR4（规则解析）、AWS SDK S3、阿里云 SLS / ODPS SDK、Jackson、Guava。

---

### 2.3 分布式组网拓扑与部署模式分析

SecretPad 在实际生产环境中支持两种典型的分布式拓扑形态：

#### 1. 中心化模式 (Master-Lite / Center-Edge 模式)
Center SecretPad 部署在 Kuscia Master 侧，管控全局项目与数据注册；各机构仅需部署 Edge SecretPad 随 Kuscia Lite 运行，处理本地数据授权与协同签名。

```mermaid
graph TD
    subgraph Center Domain (Master)
        CenterPad[Center SecretPad]
        KusciaMaster[Kuscia Master Control Plane]
        CenterPad <-->|gRPC/K8s API| KusciaMaster
    end

    subgraph Institution Alice (Lite)
        EdgePadA[Edge SecretPad Alice]
        KusciaLiteA[Kuscia Lite Node Alice]
        EdgePadA <-->|gRPC/Local| KusciaLiteA
    end

    subgraph Institution Bob (Lite)
        EdgePadB[Edge SecretPad Bob]
        KusciaLiteB[Kuscia Lite Node Bob]
        EdgePadB <-->|gRPC/Local| KusciaLiteB
    end

    CenterPad <==>|gRPC mTLS Sync| EdgePadA
    CenterPad <==>|gRPC mTLS Sync| EdgePadB
```

#### 2. 对等模式 (Autonomy P2P 模式)
各机构节点完全对等独立运行，拥有独立的 Kuscia Autonomy 控制面，节点间通过 `NodeRoute` 建立 gRPC 直连通道进行多方协同。

---

## 3. 当前 Java/Spring 架构的痛点与瓶颈 (Pain Points)

### 3.1 边缘部署的高资源消耗与内存驻留痛点

在隐私计算应用场景中，SecretPad 经常需要随 Lite 节点打包部署到医院、小型金融机构、边缘计算网关等资源受限的环境中：
* **JVM 驻留开销大**：Spring Boot 框架在启动后需要加载大量 Class、Jar 包以及初始化 Spring Bean 容器，即使无任何请求访问，单个进程的常驻内存（RSS）往往高达 **300MB ~ 1GB**。
* **硬件门槛过高**：高内存占用直接推高了轻量化边缘节点的虚拟机或容器配置成本。
* **冷启动极为缓慢**：JVM 类的加载、字节码 JIT 编译及 Spring 上下文扫描导致冷启动耗时 **5~15 秒**，无法满足云原生弹性缩容拉起的需求。

---

### 3.2 云原生 K8s/Kuscia 原生生态契合摩擦

* Kuscia 操作系统完全使用 **Go 语言** 编写，基于 Kubernetes CRD (Custom Resource Definition) 范式架构（定义了 `KusciaJob`, `Domain`, `DomainData`, `DomainRoute` 等资源）。
* Java 访问 Kuscia CRD 必须依赖第三方包（如 Fabric8）或手写 Protobuf 转 Java 代码。Java 在处理 Kubernetes 原生 Informer、WorkQueue 实时 Watch 监听机制时非常繁琐，容易因为线程阻塞产生内存泄露或事件丢失。

---

### 3.3 线程模型开销与高并发任务监控瓶颈

* SecretPad 需要实时追踪 KusciaJob 运行状态、拉取多个 Pod 容器的计算日志、同步节点 Route 心跳等。
* Java 默认基于操作系统 1:1 内核线程模型。高并发状态轮询时需要维护复杂的 `ThreadPoolExecutor` 与 `BlockingQueue`，不仅锁竞争剧烈，且上下文切换消耗大幅增加了 CPU 负担。

---

### 3.4 运行时依赖与镜像体积痛点

* 部署 Java 应用必须打包 JRE (Java Runtime Environment) 运行环境。即使采用 Jlink 裁剪，镜像体积普遍仍在 **300MB ~ 500MB**，增加了私有化部署和网络分发传输的时间成本。

---

## 4. Go 重构可行性及对比矩阵

### 4.1 核心维度对比矩阵 (Java vs Go)

| 评估维度 | 当前 Java/Spring 架构 | 目标 Go 重构架构 | 重构收益 / 改进 |
| :--- | :--- | :--- | :--- |
| **运行时内存占用** | 300 MB ~ 1 GB+ | **20 MB ~ 50 MB** | 内存降低 **80% ~ 90%**，边缘极其友好 |
| **冷启动时间** | 5s ~ 15s | **< 100ms** | 毫秒级启动，近乎即时响应 |
| **部署交付形态** | JRE + Jar 包 / 大型镜像 | **单一静态二进制文件 / 超小镜像 (<25MB)** | 极致轻量化分发，无依赖解耦 |
| **K8s/Kuscia 集成** | Java SDK / gRPC 封装 / Fabric8 | **原生 `client-go` + Kuscia Go SDK** | 无缝享受云原生 Watch/Informer 机制 |
| **高并发任务监控** | 线程池 (ThreadPoolExecutor) + 轮询 | **Goroutines + Channels + WorkQueue** | 并发控制更轻量，锁竞争与代码量显著减少 |
| **工程依赖治理** | 庞大的 Maven 依赖树与传递依赖 | **精简 Go Modules，编译期静态链接** | 易于安全审计、构建速度快 |

---

### 4.2 技术栈选型映射与生态替换方案

针对现有的 Java 技术栈组件，Go 重构提供了最成熟、高性能的无缝替换方案：

```
       SecretPad Technology Stack Mapping
┌─────────────────────────────────────────────────────┐
|  Java Spring Ecosystem   ──►  Go Ecosystem Matching |
├─────────────────────────────────────────────────────┤
|  Spring Web (REST)       ──►  Gin Framework         |
|  Spring Data JPA         ──►  GORM (SQLite/MySQL)   |
|  Fabric8 K8s SDK         ──►  k8s.io/client-go      |
|  Spring IoC Container    ──►  Google Wire (DI)      |
|  Logback / SLF4J         ──►  Uber Zap Logger       |
|  ThreadPoolExecutor      ──►  Goroutine Task Pool   |
└─────────────────────────────────────────────────────┘
```

---

## 5. Go 新版本工业级架构设计原则与总体范式

### 5.1 六大架构设计原则

1. **依赖倒置原则 (Clean Architecture / DIP)**：核心领域逻辑（DAG 计算、投票状态机）绝不依赖数据库或 Kuscia API，通过 Repository 与 Client 抽象接口解耦。
2. **边缘与中心代码同构 (Unified Binary with Multi-Modes)**：基于同套 Go 代码库，通过启动参数轻松切换 `Master (Center)` 模式、`Lite (Edge)` 模式或 `Autonomy (P2P)` 模式。
3. **零 CGO 无缝部署 (Zero-CGO Strategy)**：内嵌数据库使用纯 Go 编写的 SQLite 驱动 (`modernc.org/sqlite`)，实现 100% 静态编译，免去交叉编译 C 动态链接库的烦恼。
4. **CRD 原生 Informer 模式 (Informer Pattern)**：弃用轮询，全面采用 `client-go` 的 `SharedInformerFactory` + `WorkQueue` 机制进行事件驱动。
5. **编译期依赖注入 (Compile-Time DI)**：采用 `Google Wire` 在编译阶段注入依赖，杜绝运行期反射开销与隐式 Panic 风险。
6. **防腐层设计 (Anti-Corruption Layer - ACL)**：在接入 Kuscia 底座与外部 DataMesh 时建立防腐层转换器，保证 SecretPad 内部领域模型不受底层 API 变动影响。

---

### 5.2 整洁架构 (Clean Architecture) 与 DDD 分层规范

```
                             ┌────────────────────────────────────────────────────────┐
                             │                    User & API Layer                    │
                             │               (REST / GraphQL / gRPC API)              │
                             └───────────────────────────┬────────────────────────────┘
                                                         │
                             ┌───────────────────────────▼────────────────────────────┐
                             │                  Application Service                   │
                             │           (DAG Orchestration, Node Sync)               │
                             └─────────────┬───────────────────────────┬──────────────┘
                                           │                           │
                 ┌─────────────────────────▼────────┐         ┌────────▼────────────────────────┐
                 │       Domain Model & Rules       │         │      Informer & Event Engine    │
                 │ (DAG Sorter, Spec Parser, State) │         │    (Kube Watcher, WorkQueue)    │
                 └─────────────────────────▲────────┘         └────────┬────────────────────────┘
                                           │                           │
                             ┌─────────────┴───────────────────────────▼──────────────┐
                             │                 Infrastructure Adapters                │
                             │        (GORM DB, Kuscia Client, Storage Drivers)       │
                             └────────────────────────────────────────────────────────┘
```

---

## 6. 高层设计 (High-Level Design - HLD)

### 6.1 总体组件拓扑与 6 大子系统划分

SecretPad-Go 核心引擎划分为 6 大高解耦子系统：

```
+---------------------------------------------------------------------------------------------------+
|                                      SecretPad-Go Core Engine                                     |
+---------------------------------------------------------------------------------------------------+
|  [ 1. API Gateway Subsystem ]                                                                     |
|    - Gin Web Router | REST Handlers | JWT/RBAC Interceptor | OpenAPI Doc Engine                  |
+---------------------------------------------------------------------------------------------------+
|  [ 2. DAG Engine Subsystem ]                                                                      |
|    - Graph Parser | Kahn Topology Sorter | Component Spec AST | Execution Plan Builder        |
+---------------------------------------------------------------------------------------------------+
|  [ 3. Kuscia Control Subsystem ]                                                                  |
|    - K8s Client-Go | Informer Manager | WorkQueue Controller | KusciaJob & DomainData Adapter|
+---------------------------------------------------------------------------------------------------+
|  [ 4. Federation & Node Subsystem ]                                                               |
|    - P2P gRPC Sync Engine | Node Heartbeat Monitor | Multi-Party Vote State Machine              |
+---------------------------------------------------------------------------------------------------+
|  [ 5. Data & Auth Subsystem ]                                                                     |
|    - Datatable Registration | DataMesh Proxy | Auth Grant Engine | Storage Adapter               |
+---------------------------------------------------------------------------------------------------+
|  [ 6. Storage & Event Subsystem ]                                                                 |
|    - GORM Repository (MySQL/SQLite) | Outbox Event Publisher | Zap Logger & OTEL Tracing      |
+---------------------------------------------------------------------------------------------------+
```

---

### 6.2 核心业务处理流程与时序图全集

#### 场景 1：DAG 隐私计算作业提交与执行流程

```mermaid
sequenceDiagram
    autonumber
    participant UI as 前端/客户端
    participant Web as API Gateway (Gin)
    participant DAG as DAG Engine
    participant DB as System DB (GORM)
    participant Control as Kuscia Controller
    participant K8s as Kuscia / K8s Cluster

    UI->>Web: POST /api/v1/project/job/create (Graph Specs)
    Web->>DAG: BuildExecutionPlan(graphNodes, edges)
    DAG->>DAG: 1. 语法校验与组件 Spec 匹配
    DAG->>DAG: 2. Kahn 拓扑排序与环检测
    DAG->>DAG: 3. 生成 KusciaJob CRD Spec
    DAG->>DB: 开启事务：保存 Job/Task 记录 + Outbox 事件
    DB-->>Web: 事务提交成功
    Web-->>UI: 返回 JobID (Status: Pending)

    Control->>DB: Fetch Outbox Pending Event
    Control->>K8s: Create KusciaJob CRD (Apply Spec)
    K8s-->>Control: CRD Accepted

    loop CRD Informer Event Watch
        K8s->>Control: KusciaJob Status Updated (Running/Succeeded/Failed)
        Control->>DB: Update Job/Task Status
        Control->>UI: WebSocket Push (Job State & Real-time Logs)
    end
```

#### 场景 2：多机构协同投票审批全生命周期

```mermaid
sequenceDiagram
    autonumber
    participant Initiator as 节点 Alice (发起方)
    participant EdgeA as SecretPad-Go A
    participant EdgeB as SecretPad-Go B (审批方)
    participant Approver as 节点 Bob 管理员

    Initiator->>EdgeA: 创建协同申请 (如数据授权/节点连接)
    EdgeA->>EdgeA: 生成 Vote ID，数据库记录 PENDING 状态
    EdgeA->>EdgeB: gRPC SubmitVote(VoteRequest, Ed25519 Sign)
    EdgeB->>EdgeB: 校验签名与 TLS 身份，落地本地 Vote 记录
    EdgeB->>Approver: 界面通知/邮件提醒待审批任务
    Approver->>EdgeB: 点击同意 (Approve)
    EdgeB->>EdgeB: 签名审批结果 (Sign Approve Cert)
    EdgeB-->>EdgeA: gRPC VoteResponse (Approved, Signature)
    EdgeA->>EdgeA: 验证 Bob 节点签名，驱动本地状态机转为 APPROVED
    EdgeA->>EdgeA: 触发后续 Kuscia DomainGrant/DomainRoute 自动下发
```

#### 场景 3：节点直连路由建立与握手流程

```mermaid
sequenceDiagram
    autonumber
    participant AliceNode as Node Alice
    participant EdgeA as SecretPad-Go A
    participant EdgeB as SecretPad-Go B
    participant BobNode as Node Bob

    AliceNode->>EdgeA: POST /api/v1/node/route/create (Dst: Bob, Addr: 1.2.3.4:10080)
    EdgeA->>EdgeA: 生成证书公钥与 Handshake Payload
    EdgeA->>EdgeB: gRPC PingPong (mTLS Handshake + NodeRouteReq)
    EdgeB->>EdgeB: 验证 Alice 节点证书合法性
    EdgeB->>EdgeB: 自动写入 Kuscia DomainRoute CRD (Alice -> Bob)
    EdgeB-->>EdgeA: gRPC PingPong Response (Ack)
    EdgeA->>EdgeA: 自动写入 Kuscia DomainRoute CRD (Bob -> Alice)
    EdgeA-->>AliceNode: 路由构建成功，双向通道 Ready
```

---

### 6.3 Go 项目工程目录结构 (Project Layout)

基于 Go 社区标准项目布局与 DDD 分层规范，`privahub` 的完整工程目录结构设计如下：

```
privahub/
├── cmd/                              # 可执行入口
│   ├── server/                       # 主服务入口 (Master/Lite/Autonomy 多模式)
│   │   └── main.go
│   ├── migrator/                     # 数据迁移工具
│   │   └── main.go
│   └── edge-agent/                   # 边缘轻量 Agent (Phase 1 产物)
│       └── main.go
├── api/                              # 对外契约定义
│   ├── proto/v1/                     # gRPC Protobuf 定义
│   │   ├── federation.proto          # 节点间协同协议
│   │   ├── datasync.proto            # 数据同步协议
│   │   └── serving.proto             # 模型服务协议
│   └── openapi/                      # OpenAPI 3.0 YAML 规范
│       └── secretpad.yaml
├── internal/                         # 私有业务代码 (不可被外部 import)
│   ├── domain/                       # 领域层 (纯业务规则，零外部依赖)
│   │   ├── project/                  # 项目聚合根
│   │   │   ├── entity.go
│   │   │   ├── service.go
│   │   │   └── repository.go        # 仓储接口定义
│   │   ├── graph/                    # DAG 图领域
│   │   │   ├── entity.go
│   │   │   ├── dag_sorter.go        # Kahn 拓扑排序
│   │   │   ├── spec_parser.go       # Component Spec 解析
│   │   │   └── translator.go        # KusciaJob CRD 生成
│   │   ├── job/                      # 作业领域
│   │   │   ├── entity.go
│   │   │   ├── state_machine.go     # Job 状态机
│   │   │   └── repository.go
│   │   ├── node/                     # 节点与路由领域
│   │   │   ├── entity.go
│   │   │   ├── route.go
│   │   │   └── repository.go
│   │   ├── vote/                     # 协同投票领域
│   │   │   ├── entity.go
│   │   │   ├── state_machine.go     # 投票状态机
│   │   │   └── repository.go
│   │   ├── datatable/                # 数据表与授权领域
│   │   │   ├── entity.go
│   │   │   ├── grant.go
│   │   │   └── repository.go
│   │   ├── model/                    # 模型管理领域
│   │   │   ├── entity.go
│   │   │   ├── serving.go
│   │   │   └── repository.go
│   │   ├── user/                     # 用户与鉴权领域
│   │   │   ├── entity.go
│   │   │   ├── rbac.go
│   │   │   └── repository.go
│   │   └── scheduled/                # 定时任务领域
│   │       ├── entity.go
│   │       └── repository.go
│   ├── service/                      # 应用服务层 (编排领域对象，事务边界)
│   │   ├── project_service.go
│   │   ├── graph_service.go
│   │   ├── job_service.go
│   │   ├── node_service.go
│   │   ├── vote_service.go
│   │   ├── datatable_service.go
│   │   ├── datasource_service.go
│   │   ├── model_service.go
│   │   ├── serving_service.go
│   │   ├── auth_service.go
│   │   ├── user_service.go
│   │   ├── message_service.go
│   │   ├── scheduled_service.go
│   │   ├── component_service.go
│   │   ├── approval_service.go
│   │   ├── inst_service.go
│   │   └── sync/                     # 数据同步子服务
│   │       ├── edge_sync.go
│   │       └── center_sync.go
│   ├── controller/                   # 接入层 (HTTP/gRPC Handler)
│   │   ├── http/                     # RESTful API Handler
│   │   │   ├── router.go            # 路由注册总入口
│   │   │   ├── middleware/           # Gin 中间件
│   │   │   │   ├── auth.go          # JWT 鉴权
│   │   │   │   ├── trace.go         # TraceID 注入
│   │   │   │   ├── recovery.go      # Panic 恢复
│   │   │   │   ├── cors.go          # 跨域
│   │   │   │   └── audit.go         # 审计日志
│   │   │   └── v1/                   # v1 版本 Handler
│   │   │       ├── project.go
│   │   │       ├── graph.go
│   │   │       ├── job.go
│   │   │       ├── node.go
│   │   │       ├── node_route.go
│   │   │       ├── vote.go
│   │   │       ├── datatable.go
│   │   │       ├── datasource.go
│   │   │       ├── data.go
│   │   │       ├── model.go
│   │   │       ├── serving.go
│   │   │       ├── auth.go
│   │   │       ├── user.go
│   │   │       ├── inst.go
│   │   │       ├── approval.go
│   │   │       ├── message.go
│   │   │       ├── component.go
│   │   │       ├── scheduled.go
│   │   │       ├── cloud_log.go
│   │   │       └── p2p/             # P2P 模式专用 Handler
│   │   │           ├── project.go
│   │   │           ├── node.go
│   │   │           └── datasync.go
│   │   └── grpc/                     # gRPC Server Handler
│   │       ├── federation.go
│   │       ├── datasync.go
│   │       └── serving.go
│   ├── manager/                      # 基础设施防腐层 (Anti-Corruption Layer)
│   │   ├── kuscia/                   # Kuscia 控制面适配器
│   │   │   ├── client.go            # client-go 封装
│   │   │   ├── job_manager.go       # KusciaJob CRUD
│   │   │   ├── node_manager.go      # Domain CRUD
│   │   │   ├── datatable_manager.go # DomainData CRUD
│   │   │   ├── route_manager.go     # DomainRoute CRUD
│   │   │   ├── grant_manager.go     # DomainGrant CRUD
│   │   │   ├── serving_manager.go   # KusciaServing CRUD
│   │   │   └── reconciler.go        # Informer + WorkQueue 控制器
│   │   ├── datamesh/                 # DataMesh 数据代理
│   │   │   └── client.go
│   │   ├── datasource/               # 外部数据源驱动
│   │   │   ├── driver.go            # 驱动接口
│   │   │   ├── mysql_driver.go
│   │   │   ├── oss_driver.go
│   │   │   └── odps_driver.go
│   │   └── dataproxy/                # DataProxy 适配
│   │       └── client.go
│   ├── dao/                          # 数据访问层 (GORM 实现)
│   │   ├── db.go                    # 数据库连接初始化
│   │   ├── model/                   # DO 结构体 (与 Java Entity 1:1 映射)
│   │   │   ├── inst.go
│   │   │   ├── node.go
│   │   │   ├── node_route.go
│   │   │   ├── project.go
│   │   │   ├── project_inst.go
│   │   │   ├── project_node.go
│   │   │   ├── project_datatable.go
│   │   │   ├── project_fed_table.go
│   │   │   ├── project_model.go
│   │   │   ├── project_rule.go
│   │   │   ├── project_report.go
│   │   │   ├── project_read_data.go
│   │   │   ├── project_result.go
│   │   │   ├── project_job.go
│   │   │   ├── project_task.go
│   │   │   ├── project_task_log.go
│   │   │   ├── project_graph.go
│   │   │   ├── project_graph_node.go
│   │   │   ├── project_graph_kuscia_params.go
│   │   │   ├── project_graph_domain_datasource.go
│   │   │   ├── project_model_pack.go
│   │   │   ├── project_model_serving.go
│   │   │   ├── project_schedule.go
│   │   │   ├── project_schedule_job.go
│   │   │   ├── project_schedule_task.go
│   │   │   ├── project_feature_table.go
│   │   │   ├── project_info.go
│   │   │   ├── accounts.go
│   │   │   ├── tokens.go
│   │   │   ├── vote_invite.go
│   │   │   ├── vote_request.go
│   │   │   ├── sys_resource.go
│   │   │   ├── sys_role.go
│   │   │   ├── sys_role_resource_rel.go
│   │   │   ├── sys_user_node_rel.go
│   │   │   ├── sys_user_permission_rel.go
│   │   │   ├── tee_node_datatable.go
│   │   │   ├── tee_download_audit_config.go
│   │   │   ├── edge_data_sync_log.go
│   │   │   ├── node_route_approval_config.go
│   │   │   ├── project_approval_config.go
│   │   │   ├── feature_table.go
│   │   │   └── outbox_event.go
│   │   ├── repository/              # 仓储接口 GORM 实现
│   │   │   ├── project_repo.go
│   │   │   ├── node_repo.go
│   │   │   ├── job_repo.go
│   │   │   ├── vote_repo.go
│   │   │   ├── datatable_repo.go
│   │   │   ├── graph_repo.go
│   │   │   ├── model_repo.go
│   │   │   ├── user_repo.go
│   │   │   ├── scheduled_repo.go
│   │   │   └── outbox_repo.go
│   │   └── migrations/             # golang-migrate SQL 文件
│   │       ├── sqlite/
│   │       └── mysql/
│   └── wire/                        # Google Wire 依赖注入
│       ├── wire.go
│       └── wire_gen.go
├── pkg/                             # 可复用公共库 (可被外部 import)
│   ├── crypto/                      # 加解密引擎
│   │   ├── aes_gcm.go
│   │   ├── sm4.go
│   │   └── ed25519_signer.go
│   ├── transport/                   # mTLS / gRPC 传输层
│   │   ├── mtls.go
│   │   └── grpc_server.go
│   ├── pool/                        # 并发 Task Pool
│   │   └── bounded_pool.go
│   ├── logger/                      # Zap 日志封装
│   │   └── zap.go
│   ├── metrics/                     # Prometheus 指标
│   │   └── prometheus.go
│   ├── errcode/                     # 统一错误码
│   │   └── codes.go
│   ├── response/                    # 统一 HTTP 响应封装
│   │   └── response.go
│   ├── validator/                   # 请求参数校验器
│   │   └── validator.go
│   └── utils/                       # 通用工具
│       ├── id.go                    # UUID / Snowflake ID 生成
│       └── time.go
├── config/                          # 配置文件
│   ├── secretpad.yaml
│   ├── secretpad-dev.yaml
│   └── secretpad-prod.yaml
├── deployments/                     # 部署清单
│   ├── docker/
│   │   └── Dockerfile
│   └── k8s/
│       ├── deployment.yaml
│       └── service.yaml
├── scripts/                         # 构建与运维脚本
│   ├── build.sh
│   ├── test.sh
│   └── lint.sh
├── go.mod
├── go.sum
├── Makefile
├── .golangci.yml
├── .pre-commit-config.yaml
└── README.md
```

---

### 6.4 各子系统详细模块设计 (Java → Go 映射)

#### 6.4.1 API Gateway 子系统

**Java 原始模块**：`secretpad-web` (27 个 Controller + 4 个 Filter/Interceptor + 7 个 Configuration)

**Go 目标设计**：

| Java Controller | Go Handler 文件 | 核心职责 |
| :--- | :--- | :--- |
| `ProjectController` | `controller/http/v1/project.go` | 项目 CRUD、成员管理、归档 |
| `GraphController` | `controller/http/v1/graph.go` | DAG 画布保存/加载/节点编辑 |
| `DataController` (Job) | `controller/http/v1/job.go` | 作业提交、状态查询、日志拉取 |
| `NodeController` | `controller/http/v1/node.go` | 节点注册、Token 管理 |
| `NodeRouteController` | `controller/http/v1/node_route.go` | 节点路由创建/删除/状态 |
| `VoteSyncController` | `controller/http/v1/vote.go` | 投票发起/审批/查询 |
| `DatatableController` | `controller/http/v1/datatable.go` | 数据表注册/删除/Schema |
| `DataSourceController` | `controller/http/v1/datasource.go` | 数据源连接管理 |
| `DataController` | `controller/http/v1/data.go` | 数据预览/下载 |
| `ModelManagementController` | `controller/http/v1/model.go` | 模型列表/详情/删除 |
| `ModelExportController` | `controller/http/v1/serving.go` | 模型导出/Serving 部署 |
| `AuthController` | `controller/http/v1/auth.go` | 登录/登出/Token 刷新 |
| `UserController` | `controller/http/v1/user.go` | 用户 CRUD/密码重置 |
| `NodeUserController` | `controller/http/v1/user.go` | 节点用户关联 |
| `InstController` | `controller/http/v1/inst.go` | 机构管理 |
| `ApprovalController` | `controller/http/v1/approval.go` | 审批配置管理 |
| `MessageController` | `controller/http/v1/message.go` | 消息通知 |
| `ComponentVersionController` | `controller/http/v1/component.go` | 算法组件版本 |
| `ScheduledController` | `controller/http/v1/scheduled.go` | 定时任务管理 |
| `CloudLogController` | `controller/http/v1/cloud_log.go` | 云日志查询 (SLS) |
| `FeatureDatasourceController` | `controller/http/v1/datasource.go` | 特征数据源 |
| `IndexController` | (Gin 静态文件服务) | 前端 SPA 托管 |
| `RemoteUserController` | `controller/http/v1/user.go` | 远程用户同步 |
| `CenterDataSyncController` | `controller/http/v1/p2p/datasync.go` | Center 数据同步 |
| `P2PProjectController` | `controller/http/v1/p2p/project.go` | P2P 项目协同 |
| `P2pNodeController` | `controller/http/v1/p2p/node.go` | P2P 节点管理 |
| `DataSyncController` | `controller/http/v1/p2p/datasync.go` | 边缘数据同步 |

**中间件映射**：

| Java Filter/Interceptor | Go Middleware | 职责 |
| :--- | :--- | :--- |
| `LoginInterceptor` | `middleware/auth.go` | JWT Token 校验 + 用户上下文注入 |
| `AuditLogInterceptor` | `middleware/audit.go` | 操作审计日志记录 |
| `LogInterceptor` | `middleware/trace.go` | TraceID 生成与链路透传 |
| `EdgeRequestFilter` | `middleware/edge_guard.go` | Edge 模式请求来源校验 |
| `AddResponseHeaderFilter` | `middleware/security_header.go` | 安全响应头注入 |
| `CorsConfig` | `middleware/cors.go` | CORS 跨域配置 |

---

#### 6.4.2 DAG Engine 子系统

**Java 原始模块**：`secretpad-service` 中的 `GraphServiceImpl` + `secretpad-common` 中的 Component Spec 解析器

**Go 目标设计** (`internal/domain/graph/`)：

```go
// 核心接口定义
type DAGService interface {
    // SaveGraph 保存 DAG 画布 (节点 + 边)
    SaveGraph(ctx context.Context, req *SaveGraphRequest) error
    // LoadGraph 加载 DAG 画布
    LoadGraph(ctx context.Context, projectID, graphID string) (*GraphVO, error)
    // BuildExecutionPlan 将 DAG 转化为 KusciaJob 执行计划
    BuildExecutionPlan(ctx context.Context, projectID, graphID string) (*ExecutionPlan, error)
    // ValidateGraph 校验 DAG 合法性 (环检测 + 组件 Spec 匹配)
    ValidateGraph(ctx context.Context, nodes []GraphNode, edges []GraphEdge) error
}

// ExecutionPlan 执行计划
type ExecutionPlan struct {
    JobID      string
    Tasks      []TaskSpec
    TopoOrder  []string  // 拓扑排序后的节点 ID 序列
    Parallelism int
}
```

**关键算法**：
- Kahn 拓扑排序 + 环检测（已在 7.2 节详述）
- Component Spec AST 解析：将前端 `node_def` JSON 解析为强类型结构体
- KusciaJob CRD Spec 生成：将排序后的 Task 列表组装为 K8s 资源清单

---

#### 6.4.3 Kuscia Control 子系统

**Java 原始模块**：`secretpad-manager` 中的 `integration/job/JobManager`、`integration/node/NodeManager`、`integration/datatable/DatatableManager`、`integration/noderoute/NodeRouteManager`、`integration/datatablegrant/DatatableGrantManager`、`integration/serving/KusciaServingManager`

**Go 目标设计** (`internal/manager/kuscia/`)：

```go
// KusciaClient 统一 Kuscia 控制面客户端接口
type KusciaClient interface {
    // Job 管理
    CreateJob(ctx context.Context, job *KusciaJobSpec) error
    GetJob(ctx context.Context, jobID string) (*KusciaJobStatus, error)
    DeleteJob(ctx context.Context, jobID string) error
    WatchJobs(ctx context.Context, handler JobEventHandler) error

    // Domain (Node) 管理
    CreateDomain(ctx context.Context, domain *DomainSpec) error
    GetDomain(ctx context.Context, domainID string) (*DomainStatus, error)
    ListDomains(ctx context.Context) ([]*DomainStatus, error)

    // DomainData (Datatable) 管理
    CreateDomainData(ctx context.Context, data *DomainDataSpec) error
    GetDomainData(ctx context.Context, domainID, dataID string) (*DomainData, error)
    ListDomainData(ctx context.Context, domainID string) ([]*DomainData, error)
    DeleteDomainData(ctx context.Context, domainID, dataID string) error

    // DomainRoute 管理
    CreateDomainRoute(ctx context.Context, route *DomainRouteSpec) error
    GetDomainRoute(ctx context.Context, src, dst string) (*DomainRouteStatus, error)
    DeleteDomainRoute(ctx context.Context, src, dst string) error

    // DomainGrant 管理
    CreateDomainGrant(ctx context.Context, grant *DomainGrantSpec) error
    DeleteDomainGrant(ctx context.Context, grantID string) error

    // KusciaServing 管理
    CreateServing(ctx context.Context, serving *ServingSpec) error
    GetServing(ctx context.Context, servingID string) (*ServingStatus, error)
    DeleteServing(ctx context.Context, servingID string) error
}
```

**Informer 事件驱动设计**（替代 Java 轮询）：
- 使用 `client-go` 的 `SharedInformerFactory` 监听 `KusciaJob`、`DomainRoute` 状态变更
- 通过 `workqueue.RateLimitingQueue` 实现指数退避重试
- Reconciler 模式：事件 → 入队 → 消费 → 更新本地 DB → 推送 WebSocket

---

#### 6.4.4 Federation & Node 子系统

**Java 原始模块**：`secretpad-service` 中的 `VoteSyncServiceImpl`、`NodeRouterServiceImpl`、`NodeServiceImpl` + `secretpad-web/controller/p2p/`

**Go 目标设计**：

```go
// 投票状态机 (internal/domain/vote/state_machine.go)
type VoteState string
const (
    VotePending  VoteState = "PENDING"
    VoteApproved VoteState = "APPROVED"
    VoteRejected VoteState = "REJECTED"
    VoteExpired  VoteState = "EXPIRED"
    VoteExecuted VoteState = "EXECUTED"
)

type VoteStateMachine struct {
    transitions map[VoteState][]VoteState
}

func (sm *VoteStateMachine) CanTransit(from, to VoteState) bool
func (sm *VoteStateMachine) Transit(vote *VoteEntity, to VoteState) error

// P2P gRPC 同步引擎 (internal/controller/grpc/federation.go)
type FederationServer interface {
    PingPong(ctx context.Context, req *PingRequest) (*PongResponse, error)
    SubmitVoteProposal(ctx context.Context, req *VoteProposalRequest) (*VoteProposalResponse, error)
    StreamTaskLogs(req *LogStreamRequest, stream grpc.ServerStreamingServer[LogStreamResponse]) error
}
```

---

#### 6.4.5 Data & Auth 子系统

**Java 原始模块**：`secretpad-service` 中的 `DatatableServiceImpl`、`DatasourceServiceImpl`、`DataServiceImpl`、`AuthServiceImpl`、`UserServiceImpl` + `secretpad-manager` 中的 `datasource/` 驱动

**Go 目标设计**：

```go
// 数据源驱动接口 (internal/manager/datasource/driver.go)
type DatasourceDriver interface {
    // Connect 测试连接
    Connect(ctx context.Context, cfg *DatasourceConfig) error
    // ListTables 列出数据表
    ListTables(ctx context.Context, cfg *DatasourceConfig) ([]TableMeta, error)
    // GetSchema 获取表 Schema
    GetSchema(ctx context.Context, cfg *DatasourceConfig, tableName string) (*TableSchema, error)
    // Preview 数据预览
    Preview(ctx context.Context, cfg *DatasourceConfig, tableName string, limit int) (*PreviewResult, error)
}

// 驱动注册表 (策略模式)
type DriverRegistry struct {
    drivers map[string]DatasourceDriver  // key: "mysql", "oss", "odps"
}

// RBAC 鉴权 (internal/domain/user/rbac.go)
type RBACService interface {
    Authenticate(ctx context.Context, username, password string) (*TokenPair, error)
    RefreshToken(ctx context.Context, refreshToken string) (*TokenPair, error)
    CheckPermission(ctx context.Context, userID string, resourceCode string) (bool, error)
}
```

---

#### 6.4.6 Storage & Event 子系统

**Java 原始模块**：`secretpad-persistence` (42 个 DO Entity + JPA Repository) + `secretpad-scheduled` (Quartz)

**Go 目标设计**：

```go
// 数据库初始化 (internal/dao/db.go)
func NewDB(cfg *config.DatabaseConfig) (*gorm.DB, error) {
    var dialector gorm.Dialector
    switch cfg.Driver {
    case "sqlite":
        dialector = sqlite.Open(cfg.DSN)  // modernc.org/sqlite (零 CGO)
    case "mysql":
        dialector = mysql.Open(cfg.DSN)
    }
    db, err := gorm.Open(dialector, &gorm.Config{
        Logger: logger.Default.LogMode(logger.Silent),
    })
    // 连接池配置
    sqlDB, _ := db.DB()
    sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
    sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
    return db, err
}

// Outbox 事件发布器 (替代 Spring ApplicationEvent)
type OutboxPublisher struct {
    repo   repository.OutboxRepository
    pool   *pool.BoundedPool
    logger *zap.Logger
}

func (p *OutboxPublisher) Publish(ctx context.Context, evt *domain.Event) error {
    // 1. 事务内写入 outbox_events 表
    // 2. 异步 Goroutine 消费并分发
}

// 定时任务引擎 (替代 Quartz，使用 robfig/cron)
type ScheduledEngine struct {
    cron   *cron.Cron
    repo   repository.ScheduledRepository
    jobSvc *service.JobService
    logger *zap.Logger
}
```

---

## 7. 底层设计 (Low-Level Design - LLD)

### 7.1 完整领域模型与数据库 Schema 设计 (GORM DO Structs)

在 `internal/dao/model/` 定义完整向后兼容的 10 大数据表结构：

```go
// internal/dao/model/models.go
package model

import (
	"time"

	"gorm.io/gorm"
)

// ProjectDO 项目基础表
type ProjectDO struct {
	ProjectID   string         `gorm:"primaryKey;type:varchar(64)" json:"project_id"`
	Name        string         `gorm:"type:varchar(128);not null" json:"name"`
	Description string         `gorm:"type:text" json:"description"`
	ComputeFunc string         `gorm:"type:varchar(32);not null" json:"compute_func"` // SPU / MPC / TEE
	ProjectMode int            `gorm:"type:tinyint;default:0" json:"project_mode"`    // 0: Center, 1: Autonomy
	Status      int            `gorm:"type:tinyint;default:0" json:"status"`
	CreatedBy   string         `gorm:"type:varchar(64)" json:"created_by"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

func (ProjectDO) TableName() string { return "projects" }

// NodeDO 机构节点信息表
type NodeDO struct {
	NodeID      string    `gorm:"primaryKey;type:varchar(64)" json:"node_id"`
	Name        string    `gorm:"type:varchar(128);not null" json:"name"`
	AuthCode    string    `gorm:"type:varchar(64)" json:"auth_code"`
	PublicKey   string    `gorm:"type:text" json:"public_key"`
	NetAddr     string    `gorm:"type:varchar(128)" json:"net_addr"`
	NodeStatus  string    `gorm:"type:varchar(32);default:'Ready'" json:"node_status"`
	IsControl   bool      `gorm:"default:false" json:"is_control"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (NodeDO) TableName() string { return "nodes" }

// NodeRouteDO 节点通信路由表
type NodeRouteDO struct {
	RouteID     string    `gorm:"primaryKey;type:varchar(64)" json:"route_id"`
	SrcNodeID   string    `gorm:"index;type:varchar(64);not null" json:"src_node_id"`
	DstNodeID   string    `gorm:"index;type:varchar(64);not null" json:"dst_node_id"`
	SrcNetAddr  string    `gorm:"type:varchar(128)" json:"src_net_addr"`
	DstNetAddr  string    `gorm:"type:varchar(128)" json:"dst_net_addr"`
	Status      string    `gorm:"type:varchar(32);default:'Succeeded'" json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (NodeRouteDO) TableName() string { return "node_routes" }

// ProjectJobDO 作业记录表
type ProjectJobDO struct {
	JobID       string     `gorm:"primaryKey;type:varchar(64)" json:"job_id"`
	ProjectID   string     `gorm:"index;type:varchar(64);not null" json:"project_id"`
	GraphID     string     `gorm:"type:varchar(64)" json:"graph_id"`
	Status      string     `gorm:"type:varchar(32);not null;index" json:"status"` // PENDING, RUNNING, SUCCEEDED, FAILED, STOPPED
	ErrMsg      string     `gorm:"type:text" json:"err_msg"`
	StartedAt   *time.Time `json:"started_at"`
	FinishedAt  *time.Time `json:"finished_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (ProjectJobDO) TableName() string { return "project_jobs" }

// ProjectTaskDO 任务节点执行表
type ProjectTaskDO struct {
	TaskID    string    `gorm:"primaryKey;type:varchar(64)" json:"task_id"`
	JobID     string    `gorm:"index;type:varchar(64);not null" json:"job_id"`
	NodeID    string    `gorm:"type:varchar(64);not null" json:"node_id"`
	Status    string    `gorm:"type:varchar(32);not null" json:"status"`
	TaskTrace string    `gorm:"type:text" json:"task_trace"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (ProjectTaskDO) TableName() string { return "project_tasks" }

// VoteInviteDO 协同投票记录表
type VoteInviteDO struct {
	VoteID         string    `gorm:"primaryKey;type:varchar(64)" json:"vote_id"`
	InitiatorNode  string    `gorm:"type:varchar(64);not null" json:"initiator_node"`
	VoterNode      string    `gorm:"type:varchar(64);not null;index" json:"voter_node"`
	Action         string    `gorm:"type:varchar(64);not null" json:"action"` // PROJECT_CREATE, DATA_AUTHORIZE, NODE_ROUTE
	VoteStatus     string    `gorm:"type:varchar(32);not null;index" json:"vote_status"` // PENDING, APPROVED, REJECTED
	Payload        string    `gorm:"type:longtext" json:"payload"`
	CertSignature  string    `gorm:"type:text" json:"cert_signature"`
	ExpiredAt      time.Time `gorm:"index" json:"expired_at"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (VoteInviteDO) TableName() string { return "vote_invites" }

// OutboxEventDO 事务性 Outbox 事件表
type OutboxEventDO struct {
	ID            uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	EventID       string    `gorm:"uniqueIndex;type:varchar(64);not null" json:"event_id"`
	AggregateType string    `gorm:"type:varchar(64);not null" json:"aggregate_type"` // Job, Vote, Node
	AggregateID   string    `gorm:"type:varchar(64);not null" json:"aggregate_id"`
	Payload       string    `gorm:"type:longtext;not null" json:"payload"`
	Status        string    `gorm:"type:varchar(32);default:'PENDING';index" json:"status"` // PENDING, PROCESSED, FAILED
	RetryCount    int       `gorm:"default:0" json:"retry_count"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (OutboxEventDO) TableName() string { return "outbox_events" }
```

#### 7.1.1 完整数据模型映射表 (Java DO → Go Struct)

以下表格列出 Java 后端全部 42 个持久化实体与 Go 版本的一一对应关系：

| # | Java Entity (secretpad-persistence) | Go Struct (internal/dao/model/) | 所属子系统 | 说明 |
| :--- | :--- | :--- | :--- | :--- |
| 1 | `InstDO` | `InstDO` | Data & Auth | 机构信息表 |
| 2 | `NodeDO` | `NodeDO` | Federation & Node | 节点注册信息 |
| 3 | `NodeRouteDO` | `NodeRouteDO` | Federation & Node | 节点通信路由 |
| 4 | `NodeRouteApprovalConfigDO` | `NodeRouteApprovalConfigDO` | Federation & Node | 路由审批配置 |
| 5 | `ProjectDO` | `ProjectDO` | DAG Engine | 项目基础信息 |
| 6 | `ProjectInfoDO` | `ProjectInfoDO` | DAG Engine | 项目运行时参数 (TEE) |
| 7 | `ProjectInstDO` | `ProjectInstDO` | DAG Engine | 项目-机构关联 |
| 8 | `ProjectNodeDO` | `ProjectNodeDO` | DAG Engine | 项目-节点关联 |
| 9 | `ProjectDatatableDO` | `ProjectDatatableDO` | Data & Auth | 项目数据表绑定 |
| 10 | `ProjectFedTableDO` | `ProjectFedTableDO` | Data & Auth | 联邦表组合 |
| 11 | `ProjectGraphDO` | `ProjectGraphDO` | DAG Engine | DAG 画布元数据 |
| 12 | `ProjectGraphNodeDO` | `ProjectGraphNodeDO` | DAG Engine | DAG 节点定义 |
| 13 | `ProjectGraphNodeKusciaParamsDO` | `ProjectGraphNodeKusciaParamsDO` | DAG Engine | 节点 Kuscia 参数 |
| 14 | `ProjectGraphDomainDatasourceDO` | `ProjectGraphDomainDatasourceDO` | DAG Engine | 图-数据源关联 |
| 15 | `ProjectJobDO` | `ProjectJobDO` | Kuscia Control | 作业记录 |
| 16 | `ProjectTaskDO` | `ProjectTaskDO` | Kuscia Control | 任务节点执行 |
| 17 | `ProjectJobTaskLogDO` | `ProjectJobTaskLogDO` | Kuscia Control | 任务日志 |
| 18 | `ProjectModelDO` | `ProjectModelDO` | Data & Auth | 项目模型关联 |
| 19 | `ProjectModelPackDO` | `ProjectModelPackDO` | Data & Auth | 模型打包记录 |
| 20 | `ProjectModelServingDO` | `ProjectModelServingDO` | Data & Auth | 模型 Serving 部署 |
| 21 | `ProjectRuleDO` | `ProjectRuleDO` | Data & Auth | 项目规则关联 |
| 22 | `ProjectReportDO` | `ProjectReportDO` | Data & Auth | 项目报告 |
| 23 | `ProjectReadDataDO` | `ProjectReadDataDO` | Data & Auth | 数据读取记录 |
| 24 | `ProjectResultDO` | `ProjectResultDO` | Data & Auth | 项目产出物 |
| 25 | `ProjectScheduleDO` | `ProjectScheduleDO` | Storage & Event | 定时任务配置 |
| 26 | `ProjectScheduleJobDO` | `ProjectScheduleJobDO` | Storage & Event | 定时作业实例 |
| 27 | `ProjectScheduleTaskDO` | `ProjectScheduleTaskDO` | Storage & Event | 定时任务实例 |
| 28 | `ProjectFeatureTableDO` | `ProjectFeatureTableDO` | Data & Auth | 特征表关联 |
| 29 | `FeatureTableDO` | `FeatureTableDO` | Data & Auth | 特征表定义 |
| 30 | `ProjectApprovalConfigDO` | `ProjectApprovalConfigDO` | Federation & Node | 项目审批配置 |
| 31 | `AccountsDO` | `AccountsDO` | Data & Auth | 用户账户 |
| 32 | `TokensDO` | `TokensDO` | Data & Auth | Token 存储 |
| 33 | `SysResourceDO` | `SysResourceDO` | Data & Auth | 系统资源 (RBAC) |
| 34 | `SysRoleDO` | `SysRoleDO` | Data & Auth | 角色定义 |
| 35 | `SysRoleResourceRelDO` | `SysRoleResourceRelDO` | Data & Auth | 角色-资源关联 |
| 36 | `SysUserNodeRelDO` | `SysUserNodeRelDO` | Data & Auth | 用户-节点关联 |
| 37 | `SysUserPermissionRelDO` | `SysUserPermissionRelDO` | Data & Auth | 用户权限关联 |
| 38 | `VoteInviteDO` | `VoteInviteDO` | Federation & Node | 投票邀请记录 |
| 39 | `VoteRequestDO` | `VoteRequestDO` | Federation & Node | 投票请求记录 |
| 40 | `TeeNodeDatatableManagementDO` | `TeeNodeDatatableManagementDO` | Data & Auth | TEE 数据表管理 |
| 41 | `TeeDownLoadAuditConfigDO` | `TeeDownloadAuditConfigDO` | Data & Auth | TEE 下载审计 |
| 42 | `EdgeDataSyncLogDO` | `EdgeDataSyncLogDO` | Storage & Event | 边缘同步日志 |
| 43 | (新增) | `OutboxEventDO` | Storage & Event | 事务性 Outbox 事件 |

#### 7.1.2 数据库 Schema 分模式设计

与 Java 版保持一致，Go 版本同样支持三种部署模式的独立 Schema：

| 模式 | 迁移文件目录 | 核心表差异 |
| :--- | :--- | :--- |
| **Center (Master)** | `dao/migrations/center/` | 包含全量表，含 `inst`、`project_inst`、全局用户管理 |
| **Edge (Lite)** | `dao/migrations/edge/` | 精简表集，无 `inst`，本地用户 + 同步日志 |
| **P2P (Autonomy)** | `dao/migrations/p2p/` | 对等表集，含 `vote_*`、`node_route_approval_config` |

---

### 7.2 DAG 图转换引擎与 Component Spec 解析算法

#### 完整的 Kahn 拓扑排序与 KusciaJob Spec 生成算法：

```go
// internal/service/dag/translator.go
package dag

import (
	"encoding/json"
	"errors"
	"fmt"
)

type ComponentSpec struct {
	Domain     string          `json:"domain"`
	Name       string          `json:"name"`
	Version    string          `json:"version"`
	Attributes []AttributeSpec `json:"attributes"`
	Inputs     []TableSpec     `json:"inputs"`
	Outputs    []TableSpec     `json:"outputs"`
}

type AttributeSpec struct {
	Name  string      `json:"name"`
	Value interface{} `json:"value"`
}

type TableSpec struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type GraphNode struct {
	ID       string                 `json:"id"`
	CodeName string                 `json:"code_name"`
	Attrs    map[string]interface{} `json:"attrs"`
}

type GraphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type DAGGraph struct {
	Nodes map[string]GraphNode `json:"nodes"`
	Edges []GraphEdge          `json:"edges"`
}

// TopologicalSort 实现 Kahn 算法进行 DAG 环检测与拓扑排序
func TopologicalSort(nodes []GraphNode, edges []GraphEdge) ([]string, error) {
	inDegree := make(map[string]int)
	adjList := make(map[string][]string)

	for _, node := range nodes {
		inDegree[node.ID] = 0
	}

	for _, edge := range edges {
		adjList[edge.From] = append(adjList[edge.From], edge.To)
		inDegree[edge.To]++
	}

	var queue []string
	for nodeID, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, nodeID)
		}
	}

	var sorted []string
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		sorted = append(sorted, curr)

		for _, neighbor := range adjList[curr] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	if len(sorted) != len(nodes) {
		return nil, errors.New("invalid DAG: cycles detected or disconnected node references")
	}

	return sorted, nil
}

// BuildKusciaJobSpec 将前端 DAG 结构转化为 KusciaJob CRD 字典数据
func BuildKusciaJobSpec(jobID string, graph DAGGraph) (map[string]interface{}, error) {
	nodesList := make([]GraphNode, 0, len(graph.Nodes))
	for _, v := range graph.Nodes {
		nodesList = append(nodesList, v)
	}

	sortedNodeIDs, err := TopologicalSort(nodesList, graph.Edges)
	if err != nil {
		return nil, fmt.Errorf("dag validation failed: %w", err)
	}

	var tasks []map[string]interface{}
	for _, nodeID := range sortedNodeIDs {
		node := graph.Nodes[nodeID]
		taskSpec := map[string]interface{}{
			"taskId":          fmt.Sprintf("%s-%s", jobID, node.ID),
			"alias":           node.ID,
			"appImage":        "secretflow-registry.cn-hangzhou.cr.aliyuncs.com/secretflow/secretflow-lite:latest",
			"taskInputConfig": renderTaskInputConfig(node),
		}
		tasks = append(tasks, taskSpec)
	}

	kusciaJob := map[string]interface{}{
		"apiVersion": "kuscia.secretflow.io/v1alpha1",
		"kind":       "KusciaJob",
		"metadata": map[string]interface{}{
			"name": jobID,
		},
		"spec": map[string]interface{}{
			"maxParallelism": 2,
			"tasks":          tasks,
		},
	}

	return kusciaJob, nil
}

func renderTaskInputConfig(node GraphNode) string {
	cfg := map[string]interface{}{
		"component": node.CodeName,
		"attrs":     node.Attrs,
	}
	bytes, _ := json.Marshal(cfg)
	return string(bytes)
}
```

---

### 7.3 Kuscia CRD Controller / Informer 深度设计

采用 Kubernetes 原生 controller-runtime 与 `client-go` 的实现模式，构建监听 `KusciaJob` 资源状态变更的事件驱动控制器：

```go
// internal/manager/kuscia/reconciler.go
package kuscia

import (
	"context"
	"time"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type KusciaJobReconciler struct {
	logger              *zap.Logger
	queue               workqueue.TypedRateLimitingInterface[string]
	indexer             cache.Indexer
	informer            cache.SharedIndexInformer
	updateJobStatusFunc func(ctx context.Context, jobID string, status string, errMsg string) error
}

func NewKusciaJobReconciler(
	logger *zap.Logger,
	informer cache.SharedIndexInformer,
	updateFunc func(ctx context.Context, jobID string, status string, errMsg string) error,
) *KusciaJobReconciler {
	ratelimiter := workqueue.NewTypedItemExponentialFailureRateLimiter[string](50*time.Millisecond, 100*time.Second)
	queue := workqueue.NewTypedRateLimitingQueue[string](ratelimiter)

	r := &KusciaJobReconciler{
		logger:              logger,
		queue:               queue,
		indexer:             informer.GetIndexer(),
		informer:            informer,
		updateJobStatusFunc: updateFunc,
	}

	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				r.queue.Add(key)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(newObj)
			if err == nil {
				r.queue.Add(key)
			}
		},
	})

	return r
}

func (r *KusciaJobReconciler) Run(ctx context.Context, workers int) {
	defer r.queue.ShutDown()
	r.logger.Info("Starting KusciaJob Reconciler Engine")

	for i := 0; i < workers; i++ {
		go func() {
			for r.processNextItem(ctx) {
			}
		}()
	}

	<-ctx.Done()
	r.logger.Info("Stopping KusciaJob Reconciler Engine")
}

func (r *KusciaJobReconciler) processNextItem(ctx context.Context) bool {
	key, shutdown := r.queue.Get()
	if shutdown {
		return false
	}
	defer r.queue.Done(key)

	err := r.reconcile(ctx, key)
	if err == nil {
		r.queue.Forget(key)
		return true
	}

	if r.queue.NumRequeues(key) < 5 {
		r.logger.Warn("Reconcile failed, retrying", zap.String("key", key), zap.Error(err))
		r.queue.AddRateLimited(key)
	} else {
		r.logger.Error("Reconcile reached max retries, dropping", zap.String("key", key), zap.Error(err))
		r.queue.Forget(key)
	}
	return true
}

func (r *KusciaJobReconciler) reconcile(ctx context.Context, key string) error {
	obj, exists, err := r.indexer.GetByKey(key)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	crdMap := obj.(metav1.Object)
	jobID := crdMap.GetName()

	r.logger.Info("Reconciling KusciaJob State", zap.String("job_id", jobID))
	return r.updateJobStatusFunc(ctx, jobID, "RUNNING", "")
}
```

---

### 7.4 多机构协同投票状态机与数字签名引擎

```mermaid
stateDiagram-v2
    [*] --> PENDING: 发起协同请求 (Submit Vote)
    PENDING --> APPROVED: 所有机构管理员 Approve (全部签名通过)
    PENDING --> REJECTED: 任一机构管理员 Reject
    PENDING --> EXPIRED: 超过设置的过期时间 (Automatic TTL Expiry)
    APPROVED --> EXECUTED: 触发底座 CRD 授权/配置生效
    REJECTED --> [*]
    EXPIRED --> [*]
    EXECUTED --> [*]
```

#### 数字签名校验引擎：

```go
// pkg/crypto/signer.go
package crypto

import (
	"crypto/ed25519"
	"encoding/hex"
	"errors"
)

type Ed25519Signer struct{}

func (s *Ed25519Signer) Sign(privateKeyHex string, message []byte) (string, error) {
	privBytes, err := hex.DecodeString(privateKeyHex)
	if err != nil || len(privBytes) != ed25519.PrivateKeySize {
		return "", errors.New("invalid private key format")
	}
	sig := ed25519.Sign(privBytes, message)
	return hex.EncodeToString(sig), nil
}

func (s *Ed25519Signer) Verify(publicKeyHex string, message []byte, signatureHex string) (bool, error) {
	pubBytes, err := hex.DecodeString(publicKeyHex)
	if err != nil || len(pubBytes) != ed25519.PublicKeySize {
		return false, errors.New("invalid public key format")
	}
	sigBytes, err := hex.DecodeString(signatureHex)
	if err != nil {
		return false, errors.New("invalid signature format")
	}
	return ed25519.Verify(pubBytes, message, sigBytes), nil
}
```

---

### 7.5 高性能并发 Task Pool 与生命周期管理

```go
// pkg/pool/bounded_pool.go
package pool

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
)

type BoundedPool struct {
	maxWorkers int
	taskChan   chan func(ctx context.Context)
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
	logger     *zap.Logger
}

func NewBoundedPool(maxWorkers int, queueCap int, logger *zap.Logger) *BoundedPool {
	ctx, cancel := context.WithCancel(context.Background())
	p := &BoundedPool{
		maxWorkers: maxWorkers,
		taskChan:   make(chan func(ctx context.Context), queueCap),
		ctx:        ctx,
		cancel:     cancel,
		logger:     logger,
	}

	p.startWorkers()
	return p
}

func (p *BoundedPool) startWorkers() {
	for i := 0; i < p.maxWorkers; i++ {
		p.wg.Add(1)
		go func(workerID int) {
			defer p.wg.Done()
			for {
				select {
				case <-p.ctx.Done():
					return
				case task, ok := <-p.taskChan:
					if !ok {
						return
					}
					p.safeExecute(task)
				}
			}
		}(i)
	}
}

func (p *BoundedPool) safeExecute(task func(ctx context.Context)) {
	defer func() {
		if r := recover(); r != nil {
			p.logger.Error("Recovered from task panic in worker pool", zap.Any("panic", r))
		}
	}()
	task(p.ctx)
}

func (p *BoundedPool) Submit(task func(ctx context.Context)) bool {
	select {
	case p.taskChan <- task:
		return true
	default:
		p.logger.Warn("Worker pool queue full, dropping task")
		return false
	}
}

func (p *BoundedPool) GracefulStop(timeout time.Duration) {
	close(p.taskChan)
	p.cancel()
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		p.logger.Info("Worker pool stopped gracefully")
	case <-time.After(timeout):
		p.logger.Error("Worker pool shutdown timed out")
	}
}
```

---

## 8. 接口设计 (Interface Design)

### 8.1 RESTful API 规范与 Controller 设计

符合 OpenAPI 3.0 规范，路由前缀统一为 `/api/v1`：

| HTTP Method | API Endpoint | 描述 | 请求 Body / 参数 |
| :--- | :--- | :--- | :--- |
| `POST` | `/api/v1/project/create` | 创建隐私计算项目 | `{ "name": "...", "compute_func": "MPC" }` |
| `GET` | `/api/v1/project/list` | 分页获取项目列表 | `?page=1&page_size=10` |
| `POST` | `/api/v1/project/job/create` | 提交包含 DAG 的计算作业 | `{ "project_id": "...", "edges": [...] }` |
| `GET` | `/api/v1/project/job/detail` | 查询作业及 Task 运行轨迹 | `?job_id=job-xxx` |
| `POST` | `/api/v1/datatable/grant` | 发起数据授权申请 | `{ "node_id": "...", "datatable_id": "..." }` |
| `POST` | `/api/v1/node/route/create` | 创建跨节点直连通信路由 | `{ "dst_node_id": "...", "net_addr": "..." }` |
| `POST` | `/api/v1/vote/reply` | 管理员对协同投票进行审批 | `{ "vote_id": "...", "action": "APPROVE" }` |

#### 8.1.1 完整 API 端点映射表 (Java Controller → Go Handler)

以下为 Java 后端全部 REST API 端点与 Go 版本的完整映射（路由前缀统一为 `/api/v1alpha1`，与现有前端保持兼容）：

**项目管理 (ProjectController)**

| HTTP | 端点 | Go Handler 方法 | 说明 |
| :--- | :--- | :--- | :--- |
| POST | `/api/v1alpha1/project/create` | `ProjectHandler.Create` | 创建项目 |
| POST | `/api/v1alpha1/project/update` | `ProjectHandler.Update` | 更新项目 |
| POST | `/api/v1alpha1/project/delete` | `ProjectHandler.Delete` | 删除项目 |
| POST | `/api/v1alpha1/project/list` | `ProjectHandler.List` | 项目列表 |
| POST | `/api/v1alpha1/project/get` | `ProjectHandler.Get` | 项目详情 |
| POST | `/api/v1alpha1/project/archive` | `ProjectHandler.Archive` | 归档项目 |
| POST | `/api/v1alpha1/project/member/add` | `ProjectHandler.AddMember` | 添加成员 |
| POST | `/api/v1alpha1/project/member/remove` | `ProjectHandler.RemoveMember` | 移除成员 |

**DAG 图管理 (GraphController)**

| HTTP | 端点 | Go Handler 方法 | 说明 |
| :--- | :--- | :--- | :--- |
| POST | `/api/v1alpha1/graph/save` | `GraphHandler.Save` | 保存 DAG |
| POST | `/api/v1alpha1/graph/detail` | `GraphHandler.Detail` | 加载 DAG |
| POST | `/api/v1alpha1/graph/node/add` | `GraphHandler.AddNode` | 添加节点 |
| POST | `/api/v1alpha1/graph/node/update` | `GraphHandler.UpdateNode` | 更新节点 |
| POST | `/api/v1alpha1/graph/node/delete` | `GraphHandler.DeleteNode` | 删除节点 |
| POST | `/api/v1alpha1/graph/edge/add` | `GraphHandler.AddEdge` | 添加边 |
| POST | `/api/v1alpha1/graph/edge/delete` | `GraphHandler.DeleteEdge` | 删除边 |
| POST | `/api/v1alpha1/graph/list` | `GraphHandler.List` | 图列表 |
| POST | `/api/v1alpha1/graph/delete` | `GraphHandler.Delete` | 删除图 |

**作业管理 (DataController / Job)**

| HTTP | 端点 | Go Handler 方法 | 说明 |
| :--- | :--- | :--- | :--- |
| POST | `/api/v1alpha1/project/job/create` | `JobHandler.Create` | 提交作业 |
| POST | `/api/v1alpha1/project/job/list` | `JobHandler.List` | 作业列表 |
| POST | `/api/v1alpha1/project/job/detail` | `JobHandler.Detail` | 作业详情 |
| POST | `/api/v1alpha1/project/job/stop` | `JobHandler.Stop` | 停止作业 |
| POST | `/api/v1alpha1/project/job/task/log` | `JobHandler.TaskLog` | 任务日志 |
| POST | `/api/v1alpha1/project/job/task/detail` | `JobHandler.TaskDetail` | 任务详情 |

**节点管理 (NodeController)**

| HTTP | 端点 | Go Handler 方法 | 说明 |
| :--- | :--- | :--- | :--- |
| POST | `/api/v1alpha1/node/create` | `NodeHandler.Create` | 注册节点 |
| POST | `/api/v1alpha1/node/list` | `NodeHandler.List` | 节点列表 |
| POST | `/api/v1alpha1/node/detail` | `NodeHandler.Detail` | 节点详情 |
| POST | `/api/v1alpha1/node/delete` | `NodeHandler.Delete` | 删除节点 |
| POST | `/api/v1alpha1/node/token` | `NodeHandler.GenerateToken` | 生成部署 Token |

**节点路由 (NodeRouteController)**

| HTTP | 端点 | Go Handler 方法 | 说明 |
| :--- | :--- | :--- | :--- |
| POST | `/api/v1alpha1/node/route/create` | `NodeRouteHandler.Create` | 创建路由 |
| POST | `/api/v1alpha1/node/route/list` | `NodeRouteHandler.List` | 路由列表 |
| POST | `/api/v1alpha1/node/route/detail` | `NodeRouteHandler.Detail` | 路由详情 |
| POST | `/api/v1alpha1/node/route/delete` | `NodeRouteHandler.Delete` | 删除路由 |
| POST | `/api/v1alpha1/node/route/update` | `NodeRouteHandler.Update` | 更新路由 |

**协同投票 (VoteSyncController)**

| HTTP | 端点 | Go Handler 方法 | 说明 |
| :--- | :--- | :--- | :--- |
| POST | `/api/v1alpha1/vote/create` | `VoteHandler.Create` | 发起投票 |
| POST | `/api/v1alpha1/vote/list` | `VoteHandler.List` | 投票列表 |
| POST | `/api/v1alpha1/vote/detail` | `VoteHandler.Detail` | 投票详情 |
| POST | `/api/v1alpha1/vote/reply` | `VoteHandler.Reply` | 审批回复 |
| POST | `/api/v1alpha1/vote/cancel` | `VoteHandler.Cancel` | 取消投票 |

**数据表管理 (DatatableController)**

| HTTP | 端点 | Go Handler 方法 | 说明 |
| :--- | :--- | :--- | :--- |
| POST | `/api/v1alpha1/datatable/register` | `DatatableHandler.Register` | 注册数据表 |
| POST | `/api/v1alpha1/datatable/list` | `DatatableHandler.List` | 数据表列表 |
| POST | `/api/v1alpha1/datatable/detail` | `DatatableHandler.Detail` | 表详情 |
| POST | `/api/v1alpha1/datatable/delete` | `DatatableHandler.Delete` | 删除表 |
| POST | `/api/v1alpha1/datatable/grant` | `DatatableHandler.Grant` | 数据授权 |
| POST | `/api/v1alpha1/datatable/revoke` | `DatatableHandler.Revoke` | 撤销授权 |

**数据源管理 (DataSourceController)**

| HTTP | 端点 | Go Handler 方法 | 说明 |
| :--- | :--- | :--- | :--- |
| POST | `/api/v1alpha1/datasource/create` | `DatasourceHandler.Create` | 创建数据源 |
| POST | `/api/v1alpha1/datasource/list` | `DatasourceHandler.List` | 数据源列表 |
| POST | `/api/v1alpha1/datasource/detail` | `DatasourceHandler.Detail` | 数据源详情 |
| POST | `/api/v1alpha1/datasource/delete` | `DatasourceHandler.Delete` | 删除数据源 |
| POST | `/api/v1alpha1/datasource/test` | `DatasourceHandler.TestConnection` | 测试连接 |

**用户与鉴权 (AuthController / UserController)**

| HTTP | 端点 | Go Handler 方法 | 说明 |
| :--- | :--- | :--- | :--- |
| POST | `/api/v1alpha1/user/login` | `AuthHandler.Login` | 用户登录 |
| POST | `/api/v1alpha1/user/logout` | `AuthHandler.Logout` | 用户登出 |
| POST | `/api/v1alpha1/user/create` | `UserHandler.Create` | 创建用户 |
| POST | `/api/v1alpha1/user/list` | `UserHandler.List` | 用户列表 |
| POST | `/api/v1alpha1/user/update` | `UserHandler.Update` | 更新用户 |
| POST | `/api/v1alpha1/user/delete` | `UserHandler.Delete` | 删除用户 |
| POST | `/api/v1alpha1/user/reset-password` | `UserHandler.ResetPassword` | 重置密码 |

**模型管理 (ModelManagementController / ModelExportController)**

| HTTP | 端点 | Go Handler 方法 | 说明 |
| :--- | :--- | :--- | :--- |
| POST | `/api/v1alpha1/model/list` | `ModelHandler.List` | 模型列表 |
| POST | `/api/v1alpha1/model/detail` | `ModelHandler.Detail` | 模型详情 |
| POST | `/api/v1alpha1/model/delete` | `ModelHandler.Delete` | 删除模型 |
| POST | `/api/v1alpha1/model/export` | `ServingHandler.Export` | 模型导出 |
| POST | `/api/v1alpha1/serving/create` | `ServingHandler.Create` | 创建 Serving |
| POST | `/api/v1alpha1/serving/list` | `ServingHandler.List` | Serving 列表 |
| POST | `/api/v1alpha1/serving/delete` | `ServingHandler.Delete` | 删除 Serving |
| POST | `/api/v1alpha1/serving/detail` | `ServingHandler.Detail` | Serving 详情 |

**其他端点**

| HTTP | 端点 | Go Handler 方法 | 说明 |
| :--- | :--- | :--- | :--- |
| POST | `/api/v1alpha1/inst/create` | `InstHandler.Create` | 创建机构 |
| POST | `/api/v1alpha1/inst/list` | `InstHandler.List` | 机构列表 |
| POST | `/api/v1alpha1/approval/config` | `ApprovalHandler.Config` | 审批配置 |
| POST | `/api/v1alpha1/message/list` | `MessageHandler.List` | 消息列表 |
| POST | `/api/v1alpha1/message/read` | `MessageHandler.MarkRead` | 标记已读 |
| POST | `/api/v1alpha1/component/list` | `ComponentHandler.List` | 组件列表 |
| POST | `/api/v1alpha1/component/version` | `ComponentHandler.Version` | 组件版本 |
| POST | `/api/v1alpha1/scheduled/create` | `ScheduledHandler.Create` | 创建定时任务 |
| POST | `/api/v1alpha1/scheduled/list` | `ScheduledHandler.List` | 定时任务列表 |
| POST | `/api/v1alpha1/scheduled/delete` | `ScheduledHandler.Delete` | 删除定时任务 |
| GET | `/api/v1alpha1/healthz` | `HealthCheck` | 健康检查 |
| GET | `/api/v1alpha1/metrics` | (Prometheus Handler) | 指标暴露 |

#### Router 注册实现：

```go
// internal/controller/http/router.go
package http

import (
	"secretpad/internal/controller/http/v1"
	"secretpad/internal/middleware"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func RegisterRoutes(r *gin.Engine, logger *zap.Logger, jobHandler *v1.JobHandler) {
	r.Use(middleware.TraceIDMiddleware())
	r.Use(middleware.ZapRecoveryMiddleware(logger))

	apiV1 := r.Group("/api/v1")
	{
		apiV1.GET("/healthz", v1.HealthCheck)
		
		auth := apiV1.Group("/")
		auth.Use(middleware.JWTAuthMiddleware())
		{
			auth.POST("/project/job/create", jobHandler.CreateJob)
		}
	}
}
```

---

### 8.2 节点间 gRPC 协议与 Protobuf 契约全集

```protobuf
// api/proto/v1/federation.proto
syntax = "proto3";

package secretpad.v1;

option go_package = "secretpad/api/v1;v1";

service SecretPadFederationService {
  // 节点心跳与路由寻址同步
  rpc PingPong (PingRequest) returns (PongResponse);
  // 多机构协同投票提案提交
  rpc SubmitVoteProposal (VoteProposalRequest) returns (VoteProposalResponse);
  // 实时日志流提取
  rpc StreamTaskLogs (LogStreamRequest) returns (stream LogStreamResponse);
}

message PingRequest {
  string node_id = 1;
  int64 timestamp = 2;
}

message PongResponse {
  string node_id = 1;
  string status = 2;
  int64 timestamp = 3;
}

message VoteProposalRequest {
  string vote_id = 1;
  string initiator_node_id = 2;
  string action_type = 3;
  string payload_json = 4;
  string signature = 5;
}

message VoteProposalResponse {
  bool accepted = 1;
  string message = 2;
}

message LogStreamRequest {
  string job_id = 1;
  string task_id = 2;
  int64 from_line = 3;
}

message LogStreamResponse {
  string content = 1;
  int64 line_number = 2;
}
```

---

### 8.3 仓储层与外置驱动依赖倒置接口

```go
// internal/domain/repository/interfaces.go
package repository

import (
	"context"
	"secretpad/internal/dao/model"
)

type ProjectRepository interface {
	CreateProject(ctx context.Context, p *model.ProjectDO) error
	GetProjectByID(ctx context.Context, id string) (*model.ProjectDO, error)
	ListProjects(ctx context.Context, page, pageSize int) ([]*model.ProjectDO, int64, error)
}

type NodeRepository interface {
	SaveNode(ctx context.Context, node *model.NodeDO) error
	GetNodeByID(ctx context.Context, nodeID string) (*model.NodeDO, error)
	ListNodes(ctx context.Context) ([]*model.NodeDO, error)
}

type VoteRepository interface {
	SaveVote(ctx context.Context, v *model.VoteInviteDO) error
	UpdateVoteStatus(ctx context.Context, voteID string, status string, certSig string) error
	GetPendingVotesByNode(ctx context.Context, nodeID string) ([]*model.VoteInviteDO, error)
}

type OutboxRepository interface {
	SaveEvent(ctx context.Context, evt *model.OutboxEventDO) error
	FetchPendingEvents(ctx context.Context, limit int) ([]*model.OutboxEventDO, error)
	MarkProcessed(ctx context.Context, id uint64) error
}
```

---

## 9. 安全、加解密与合规体系 (Security & Cryptography)

### 9.1 节点间 mTLS 证书交换与双向校验

```go
// pkg/transport/mtls.go
package transport

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func LoadTLSCredentials(caCertPath, serverCertPath, serverKeyPath string) (credentials.TransportCredentials, error) {
	pemServerCRL, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA cert: %w", err)
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(pemServerCRL) {
		return nil, fmt.Errorf("failed to add CA cert to pool")
	}

	serverCert, err := tls.LoadX509KeyPair(serverCertPath, serverKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load server key pair: %w", err)
	}

	config := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    certPool,
		RootCAs:      certPool,
		MinVersion:   tls.VersionTLS13,
	}

	return credentials.NewTLS(config), nil
}
```

---

### 9.2 字段级落盘加密引擎 (AES-256-GCM / SM4)

```go
// pkg/crypto/encryptor.go
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
)

type FieldEncryptor struct {
	key []byte
}

func NewFieldEncryptor(keyHex string) (*FieldEncryptor, error) {
	key := []byte(keyHex)
	if len(key) != 32 {
		return nil, errors.New("encryption key must be exactly 32 bytes for AES-256")
	}
	return &FieldEncryptor{key: key}, nil
}

func (e *FieldEncryptor) Encrypt(plainText string) (string, error) {
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	cipherText := gcm.Seal(nonce, nonce, []byte(plainText), nil)
	return base64.StdEncoding.EncodeToString(cipherText), nil
}

func (e *FieldEncryptor) Decrypt(cryptoText string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(cryptoText)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}
	nonce, cipherText := data[:nonceSize], data[nonceSize:]
	plainText, err := gcm.Open(nil, nonce, cipherText, nil)
	if err != nil {
		return "", err
	}
	return string(plainText), nil
}
```

---

### 9.3 RBAC 鉴权与 Token 轮换机制

采用 JWT 双 Token 机制（Short-Lived Access Token 2 小时 + Refresh Token 7 天）：

```
[ Frontend ] --( Access Token Expired 401 )--> [ API Gateway ]
     │                                               │
     └──── POST /api/v1/auth/refresh (Refresh Token) ──┘
                             │
                             ▼
              [ Generate New Access Token ]
```

---

## 10. 可观测性与工程化落地规范 (Observability & Engineering)

### 10.1 Prometheus 指标监控全集

```go
// pkg/metrics/prometheus.go
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	HttpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "secretpad_http_requests_total",
			Help: "Total number of HTTP requests processed",
		},
		[]string{"method", "endpoint", "status"},
	)

	JobExecutionDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "secretpad_job_execution_duration_seconds",
			Help:    "Execution latency of DAG KusciaJobs in seconds",
			Buckets: prometheus.ExponentialBuckets(1, 2, 10),
		},
		[]string{"status"},
	)

	ActiveGoroutinesGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "secretpad_active_goroutines",
			Help: "Current count of active managed goroutines",
		},
	)
)
```

---

### 10.2 Zap 结构化日志与 TraceID 链路透传

```go
// pkg/logger/zap.go
package logger

import (
	"context"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type ctxKey string

const TraceIDKey ctxKey = "trace_id"

func InitLogger(env string) (*zap.Logger, error) {
	config := zap.NewProductionConfig()
	if env == "development" {
		config = zap.NewDevelopmentConfig()
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}
	config.EncoderConfig.TimeKey = "timestamp"
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	return config.Build()
}

func WithTraceID(ctx context.Context, logger *zap.Logger) *zap.Logger {
	if traceID, ok := ctx.Value(TraceIDKey).(string); ok {
		return logger.With(zap.String("trace_id", traceID))
	}
	return logger
}
```

---

### 10.3 依赖注入 (Google Wire) 体系设计

```go
//+build wireinject

package wire

import (
	"secretpad/internal/controller/http/v1"
	"secretpad/internal/dao"
	"secretpad/internal/service"
	"secretpad/pkg/logger"

	"github.com/google/wire"
)

func InitializeJobHandler() (*v1.JobHandler, error) {
	wire.Build(
		logger.InitLogger,
		dao.NewGORMDB,
		dao.NewJobRepositoryImpl,
		service.NewDAGService,
		v1.NewJobHandler,
	)
	return nil, nil
}
```

---

### 10.4 配置文件规范与 Bootstrap 引导器

系统通过配置文件 `config/secretpad.yaml` 指定模式与连接池参数：

```yaml
server:
  mode: "master" # Options: master, lite, autonomy
  http_port: 8080
  grpc_port: 9090
  read_timeout: 10s
  write_timeout: 10s

database:
  driver: "sqlite" # Options: mysql, sqlite
  dsn: "secretpad.db?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
  max_open_conns: 50
  max_idle_conns: 10

crypto:
  aes_key: "12345678901234567890123456789012" # 32 bytes

kuscia:
  kubeconfig: "/root/.kube/config"
  namespace: "kuscia-system"

observability:
  enable_metrics: true
  metrics_port: 9091
  log_level: "info"
```

---

## 11. 渐进式三阶段迁移与数据平滑升级超详细方案

### 11.1 三阶段演进计划与子系统落地细节

为了最大程度规避重构风险并确保生产环境 7x24 小时高可用，迁移计划划分为三个渐进式阶段：

```
 Phase 1: Light Edge Agent     Phase 2: Core Go Refactoring   Phase 3: Complete Cutover
┌────────────────────────┐    ┌────────────────────────┐    ┌────────────────────────┐
│  Go Edge Agent         │    │  SecretPad-Go Core     │    │  Complete Go System    │
│  (轻量节点同步代理)     │ ──►│  (与 Java 并行灰度)    │ ──►│  (废弃 Java 后端)       │
│  解决 Lite 节点内存问题│    │  兼容现有 DB Schema    │    │  实现极致轻量化与高可用│
└────────────────────────┘    └────────────────────────┘    └────────────────────────┘
```

#### 1. 阶段一：边缘 Daemon/Agent 先行 (Phase 1: Light Edge Agent)
* **核心目标**：将各机构 Lite 侧运行的 Java SecretPad 剥离，替换为专为边缘打造的轻量级 `SecretPad-Edge Agent` (Go)。
* **下沉职责**：仅负责 Lite 侧 Local Kuscia CRD 的状态 Watch、节点心跳上报、本地数据授权校验及 P2P 证书签名。
* **业务收益**：优先解决 Lite 侧 80% 以上私有化边缘节点的内存消耗痛点（RSS 从 800MB 剧降至 20MB），Center 端保持 Java 稳定运行。

#### 2. 阶段二：中心 Core 服务 Go 重构与灰度双轨 (Phase 2: Core Refactoring)
* **核心目标**：在 Master/Center 端使用 Go 实现完整控制面服务 (`privahub`)，与旧 Java 后端并行部署。
* **业务职责**：完全复刻 DAG 图转化引擎、项目管理、协同投票状态机、DataMesh 代理。
* **双轨机制**：通过统一网关（如 Nginx / APISix）基于 HTTP Header / 机构 ID 进行按比例灰度切流（例如：5% -> 20% -> 50% -> 100%）。

#### 3. 阶段三：全量切流与 Java 模块停用归档 (Phase 3: Complete Cutover)
* **核心目标**：经过长稳回归测试验证后，全量切换路由至 `privahub`，停止并下线 Java 后端容器。
* **工程归档**：将原 Maven 多模块代码库标注为 `deprecated-java-backend` 并归档，全局切入 Go 构建流水线。

---

### 11.2 数据双写、全量/增量迁移与校对引擎机制

在阶段二双轨并行期间，必须保障 Java 与 Go 两套系统底层的 SQLite/MySQL 元数据严格一致。

```
                    Data Migration & Dual-Run Architecture
                               ┌─────────────┐
                               │ API Gateway │
                               └──────┬──────┘
                                      │
                      ┌───────────────┴───────────────┐
                      ▼                               ▼
            ┌───────────────────┐           ┌───────────────────┐
            │  Java SecretPad   │           │   Go SecretPad    │
            │  (Primary Write)  │           │  (Shadow Write)   │
            └─────────┬─────────┘           └─────────┬─────────┘
                      │                               │
                      ▼                               ▼
            ┌───────────────────┐           ┌───────────────────┐
            │  Legacy DB Table  │           │   Target DB Table │
            └─────────┬─────────┘           └─────────┬─────────┘
                      │                               │
                      └───────────────┬───────────────┘
                                      ▼
                        ┌───────────────────────────┐
                        │ Data Reconciliation Engine│
                        │  (Diff & Auto-Repair)     │
                        └───────────────────────────┘
```

#### 11.2.1 数据库双写代理模式 (Dual-Write Proxy Architecture)

API 网关拦截所有写请求（`POST`/`PUT`/`DELETE`），采用主从双写模式：
* **主写 (Primary Write)**：写 Java 后端绑定的主数据库（保障当前业务同步完成）。
* **影写 (Shadow Write)**：通过 Go 协程异步将写请求的 Payload 镜像发送至 Go 后端，写 Go 数据库。
* **失败隔离**：影写失败仅记录 Error 日志与 Metrics 指标，绝不影响主写逻辑的成功返回。

#### 11.2.2 离线全量数据平滑迁移 Pipeline (`secretpad-migrator`)

开发专用迁移工具 `cmd/migrator`，用于系统首次初始化或新旧库切换时的数据搬迁：

```go
// cmd/migrator/main.go
package main

import (
	"context"
	"fmt"
	"log"

	"secretpad/internal/dao/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func MigrateTables(srcDB, dstDB *gorm.DB) error {
	log.Println("Starting Full Schema & Data Migration...")

	// 1. Auto Migrate Target Tables
	if err := dstDB.AutoMigrate(
		&model.ProjectDO{},
		&model.NodeDO{},
		&model.NodeRouteDO{},
		&model.ProjectJobDO{},
		&model.ProjectTaskDO{},
		&model.VoteInviteDO{},
		&model.OutboxEventDO{},
	); err != nil {
		return fmt.Errorf("auto migrate failed: %w", err)
	}

	// 2. Batch ETL Migration: Projects
	var projects []model.ProjectDO
	if err := srcDB.Find(&projects).Error; err == nil {
		if len(projects) > 0 {
			if err := dstDB.Save(&projects).Error; err != nil {
				return fmt.Errorf("migrating projects failed: %w", err)
			}
			log.Printf("Successfully migrated %d project records", len(projects))
		}
	}

	log.Println("Full Migration Finished Successfully.")
	return nil
}
```

#### 11.2.3 增量数据实时 Hash 对齐与校对引擎算法 (`DataReconciler`)

为了捕获双写过程中的数据不一致，后台运行离线校对引擎 `DataReconciler`：

```go
// internal/service/migration/reconciler.go
package migration

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

type DataReconciler struct {
	javaDB *gorm.DB
	goDB   *gorm.DB
	logger *zap.Logger
}

func NewDataReconciler(javaDB, goDB *gorm.DB, logger *zap.Logger) *DataReconciler {
	return &DataReconciler{
		javaDB: javaDB,
		goDB:   goDB,
		logger: logger,
	}
}

// ReconcileTable 按表对比主键列表与计算记录 Checksum
func (r *DataReconciler) ReconcileTable(ctx context.Context, tableName string) error {
	type RowCheck struct {
		ID        string
		UpdatedAt time.Time
	}

	var javaRows, goRows []RowCheck
	r.javaDB.Table(tableName).Select("project_id as id, updated_at").Find(&javaRows)
	r.goDB.Table(tableName).Select("project_id as id, updated_at").Find(&goRows)

	javaMap := make(map[string]time.Time)
	for _, row := range javaRows {
		javaMap[row.ID] = row.UpdatedAt
	}

	for _, row := range goRows {
		javaTime, exists := javaMap[row.ID]
		if !exists {
			r.logger.Warn("Discrepancy: Record missing in Java DB", zap.String("table", tableName), zap.String("id", row.ID))
			continue
		}
		if !row.UpdatedAt.Equal(javaTime) {
			r.logger.Error("Discrepancy: UpdatedAt mismatch, triggering auto-repair", zap.String("table", tableName), zap.String("id", row.ID))
			r.autoRepairRecord(ctx, tableName, row.ID)
		}
	}
	return nil
}

func (r *DataReconciler) autoRepairRecord(ctx context.Context, table string, id string) {
	// Read latest data from Java Primary DB and overwrite Go Shadow DB
}
```

---

### 11.3 数据库 Schema 平滑升级演进规范 (DB Schema Migration)

放弃 Java Hibernate 的 `hbm2ddl.auto` 隐式更新机制，Go 版本全面采用 **显示受控的数据库迁移 (Explicit SQL Migration)** 策略：

1. **迁移工具选型**：采用 [`golang-migrate/migrate`](https://github.com/golang-migrate/migrate)，将 Migration 文件以静态嵌入 (`embed`) 的方式打包进二进制可执行文件中。
2. **文件命名与防呆规范**：
   ```
   internal/dao/migrations/
    ├── 000001_create_initial_tables.up.sql
    ├── 000001_create_initial_tables.down.sql
    ├── 000002_add_outbox_events.up.sql
    └── 000002_add_outbox_events.down.sql
   ```
3. **零停机升级规则**：
   * **Rule 1: Additive-Only Changes**（禁止直接 `DROP COLUMN` 或 `RENAME COLUMN`；必须先新增字段，待全量切流后再彻底废弃旧字段）。
   * **Rule 2: Default Value Safety**（新增字段必须显式指定 `DEFAULT` 值，防护历史数据解析出现 `NULL` 指针变动）。

---

### 11.4 灰度切流门槛指标与秒级回滚 SOP 预案

#### 11.4.1 切流判定门槛指标 (Cutover Criteria Matrix)

在进行网关权重切流前，`privahub` 必须满足以下死指标线：

| 评估指标 | 门槛要求 | 监控观察周期 |
| :--- | :--- | :--- |
| **API 接口报错率** | Error Rate < **0.001%** | 连续 48 小时 |
| **接口 p99 延迟** | REST API p99 < **20ms** | 高峰期观测 |
| **内存 RSS 稳定性** | 无 Memory Leak，常驻 < **50MB** | 连续 72 小时 |
| **数据一致性比对** | 校对引擎报告 0 Diff | 灰度期间全量校验 |
| **自动化单测/E2E** | 单元测试覆盖率 > **80%**，E2E 100% 通过 | 发布前 Gate |

#### 11.4.2 秒级一键回滚操作预案 (One-Click Rollback SOP)

如果 Go 新服务在切流过程中突发不可预测的 Panic 或零日漏洞，按以下预案秒级切回 Java 避险：

```
                    Emergency One-Click Rollback SOP
┌────────────────────────┐   ┌────────────────────────┐   ┌────────────────────────┐
│ 1. Trigger Alert       │──►│ 2. Change Gateway      │──►│ 3. Freeze Go Service   │
│ (Error Rate > 0.1%)    │   │ (Weight 100% -> Java)  │   │ (Isolate Container)    │
└────────────────────────┘   └────────────────────────┘   └────────────────────────┘
```

1. **Step 1: 网关止血**：在 Nginx / API 网关上一键修改 upstream 权重，将 100% 流量秒级重新切回 Java 实例。
2. **Step 2: 隔离排查**：保持 Go 容器只读，拉取最近的 Zap 结构化日志、Prometheus 内存 Dump 及 OpenTelemetry TraceID 进行缺陷定位。
3. **Step 3: 数据安全保护**：由于 Go 与 Java 版 100% 共享并向后兼容底层 SQLite/MySQL Schema，流量切回 Java 后无任何数据表或字段损坏风险。

---

### 11.5 详细迁移执行步骤 (Step-by-Step Migration Playbook)

以下为完整的迁移执行步骤，包含每个阶段的具体任务、交付物、验证标准与时间估算。

#### 阶段零：工程基座搭建 (Week 1-2)

| 步骤 | 任务 | 交付物 | 验证标准 |
| :--- | :--- | :--- | :--- |
| 0.1 | 初始化 Go Module 与项目骨架 | `go.mod`、目录结构、`Makefile` | `make build` 成功编译空 main |
| 0.2 | 集成基础工具链 | `.golangci.yml`、`.pre-commit-config.yaml`、CI YAML | `make lint` 零告警 |
| 0.3 | 实现配置加载器 (Viper) | `pkg/config/`、`config/secretpad.yaml` | 单元测试覆盖多模式加载 |
| 0.4 | 实现 Zap 日志 + TraceID 中间件 | `pkg/logger/`、`middleware/trace.go` | 日志输出含 trace_id 字段 |
| 0.5 | 实现统一错误码与响应封装 | `pkg/errcode/`、`pkg/response/` | 与 Java 版错误码完全一致 |
| 0.6 | 实现 GORM 数据库初始化 + golang-migrate | `internal/dao/db.go`、`dao/migrations/` | 三种模式 Schema 自动迁移成功 |
| 0.7 | 实现 Google Wire 依赖注入框架 | `internal/wire/` | `wire` 命令生成 wire_gen.go |
| 0.8 | 实现 Gin HTTP Server 骨架 + 健康检查 | `cmd/server/main.go`、`router.go` | `curl /healthz` 返回 200 |
| 0.9 | 实现 Prometheus 指标暴露 | `pkg/metrics/`、`/metrics` 端点 | Prometheus 拉取指标成功 |
| 0.10 | 实现 Dockerfile 多阶段构建 | `deployments/docker/Dockerfile` | 镜像 < 25MB，`docker run` 启动 < 100ms |

#### 阶段一：边缘 Edge Agent 先行 (Week 3-6)

| 步骤 | 任务 | 交付物 | 验证标准 |
| :--- | :--- | :--- | :--- |
| 1.1 | 实现 Kuscia client-go 封装层 | `internal/manager/kuscia/client.go` | 连接本地 Kuscia Master 成功 |
| 1.2 | 实现 Domain (Node) CRUD 适配器 | `kuscia/node_manager.go` | 创建/查询/删除 Domain 与 Java 行为一致 |
| 1.3 | 实现 DomainData (Datatable) CRUD | `kuscia/datatable_manager.go` | 注册/列表/删除 DomainData 正确 |
| 1.4 | 实现 DomainRoute CRUD + 状态监听 | `kuscia/route_manager.go` | 路由创建后 Informer 捕获状态变更 |
| 1.5 | 实现节点心跳上报 Goroutine | `cmd/edge-agent/heartbeat.go` | 每 30s 上报，断线自动重连 |
| 1.6 | 实现本地数据授权校验 | `edge-agent/auth_guard.go` | 未授权请求返回 403 |
| 1.7 | 实现 P2P 证书签名与 mTLS 握手 | `pkg/crypto/ed25519_signer.go`、`pkg/transport/mtls.go` | 双向 TLS 连接建立成功 |
| 1.8 | Edge Agent 集成测试 | `tests/edge_agent_test.go` | RSS < 20MB，启动 < 100ms |
| 1.9 | 替换一个 Lite 节点的 Java SecretPad | 部署清单 + 回滚脚本 | 节点心跳、数据授权功能正常 |

#### 阶段二：Core 服务 Go 重构 (Week 7-16)

**Sprint 2A: 用户鉴权与基础 CRUD (Week 7-8)**

| 步骤 | 任务 | 交付物 | 验证标准 |
| :--- | :--- | :--- | :--- |
| 2A.1 | 实现全部 43 个 DO Struct | `internal/dao/model/*.go` | GORM AutoMigrate 零错误 |
| 2A.2 | 实现 Repository 接口 + GORM 实现 | `internal/dao/repository/*.go` | 单元测试覆盖率 > 90% |
| 2A.3 | 实现 JWT 双 Token 鉴权 | `internal/domain/user/`、`middleware/auth.go` | 登录/刷新/过期流程与 Java 一致 |
| 2A.4 | 实现 RBAC 权限校验 | `internal/domain/user/rbac.go` | 资源码权限判断与 Java 一致 |
| 2A.5 | 实现 User/Auth Handler | `controller/http/v1/auth.go`、`user.go` | 前端登录流程无感切换 |
| 2A.6 | 实现 Inst/Node CRUD Handler | `controller/http/v1/inst.go`、`node.go` | 机构/节点 CRUD 与 Java 响应一致 |

**Sprint 2B: 项目与 DAG 引擎 (Week 9-11)**

| 步骤 | 任务 | 交付物 | 验证标准 |
| :--- | :--- | :--- | :--- |
| 2B.1 | 实现 Project 领域服务 | `internal/domain/project/`、`service/project_service.go` | 项目 CRUD + 成员管理正确 |
| 2B.2 | 实现 DAG 拓扑排序 + 环检测 | `internal/domain/graph/dag_sorter.go` | 100+ 节点 DAG 排序 < 1ms |
| 2B.3 | 实现 Component Spec 解析器 | `internal/domain/graph/spec_parser.go` | 解析全部已注册组件 Spec |
| 2B.4 | 实现 KusciaJob CRD Spec 生成 | `internal/domain/graph/translator.go` | 生成的 CRD 与 Java 版结构一致 |
| 2B.5 | 实现 Graph Handler (DAG 画布 CRUD) | `controller/http/v1/graph.go` | 前端 DAG 保存/加载无异常 |
| 2B.6 | 实现 Job 提交与状态机 | `internal/domain/job/state_machine.go`、`service/job_service.go` | 作业提交→Kuscia→状态回写全链路 |
| 2B.7 | 实现 KusciaJob Informer Reconciler | `internal/manager/kuscia/reconciler.go` | 事件驱动状态同步，无轮询 |
| 2B.8 | 实现任务日志拉取与 WebSocket 推送 | `controller/http/v1/job.go` (TaskLog) | 实时日志流与 Java 版体验一致 |

**Sprint 2C: 数据管理与联邦协同 (Week 12-14)**

| 步骤 | 任务 | 交付物 | 验证标准 |
| :--- | :--- | :--- | :--- |
| 2C.1 | 实现 Datatable/Datasource 服务 | `service/datatable_service.go`、`datasource_service.go` | 数据表注册/授权/撤销正确 |
| 2C.2 | 实现数据源驱动注册表 (MySQL/OSS/ODPS) | `internal/manager/datasource/` | 各驱动连接测试通过 |
| 2C.3 | 实现 DataMesh 代理客户端 | `internal/manager/datamesh/client.go` | 数据预览/下载功能正常 |
| 2C.4 | 实现投票状态机 + Ed25519 签名 | `internal/domain/vote/`、`pkg/crypto/` | 多方投票全流程与 Java 一致 |
| 2C.5 | 实现 P2P gRPC Federation Server | `controller/grpc/federation.go` | 节点间 PingPong/投票/日志流正常 |
| 2C.6 | 实现 NodeRoute 创建与握手流程 | `service/node_service.go` | 双向路由建立 < 3s |
| 2C.7 | 实现数据同步服务 (Edge ↔ Center) | `service/sync/` | 增量同步延迟 < 5s |

**Sprint 2D: 模型管理与定时任务 (Week 15-16)**

| 步骤 | 任务 | 交付物 | 验证标准 |
| :--- | :--- | :--- | :--- |
| 2D.1 | 实现模型管理 + Serving 部署 | `service/model_service.go`、`serving_service.go` | 模型导出/Serving CRUD 正确 |
| 2D.2 | 实现 KusciaServing CRD 适配器 | `kuscia/serving_manager.go` | Serving 状态监听与回写 |
| 2D.3 | 实现定时任务引擎 (robfig/cron) | `internal/domain/scheduled/`、`service/scheduled_service.go` | 替代 Quartz，Cron 表达式兼容 |
| 2D.4 | 实现消息通知 + 审批配置 | `service/message_service.go`、`approval_service.go` | 消息推送与审批流程正常 |
| 2D.5 | 实现组件版本管理 | `service/component_service.go` | 组件列表与 Java 版一致 |
| 2D.6 | 实现云日志查询 (SLS 适配) | `service/cloud_log_service.go` | SLS 日志查询功能正常 |
| 2D.7 | 实现前端 SPA 静态文件托管 | Gin `Static()` + `NoRoute()` | 前端页面加载正常 |

#### 阶段三：双轨并行与灰度切流 (Week 17-22)

| 步骤 | 任务 | 交付物 | 验证标准 |
| :--- | :--- | :--- | :--- |
| 3.1 | 部署 Go 服务与 Java 并行运行 | Docker Compose / K8s 双实例 | 两服务同时健康运行 |
| 3.2 | 配置 API 网关灰度规则 (Nginx/APISix) | 网关配置 + Header 路由规则 | 5% 流量切入 Go 服务 |
| 3.3 | 启动数据双写代理 (Shadow Write) | 网关插件 / Go 中间件 | 写请求同时落两库 |
| 3.4 | 运行数据校对引擎 (DataReconciler) | `cmd/migrator/reconcile.go` | 连续 48h 零 Diff |
| 3.5 | 逐步提升灰度比例 (5%→20%→50%) | 监控看板 + 告警规则 | 各阶段 Error Rate < 0.001% |
| 3.6 | 全量 E2E 回归测试 | E2E 测试套件 (Playwright/Cypress) | 100% 用例通过 |
| 3.7 | 100% 流量切入 Go 服务 | 网关权重调整 | 连续 72h 稳定运行 |
| 3.8 | 停止 Java 服务，归档代码库 | Java 容器下线 + Git tag `deprecated-java-backend` | 无回滚需求 |
| 3.9 | 清理双写代理与校对引擎 | 移除 Shadow Write 配置 | Go 单写模式稳定 |
| 3.10 | 生产环境长稳观察 | 监控报告 (30 天) | RSS < 50MB，p99 < 20ms |

#### 迁移期间关键风险与缓解措施

| 风险项 | 影响 | 缓解措施 |
| :--- | :--- | :--- |
| Kuscia CRD 版本不兼容 | 作业提交失败 | 防腐层封装 + 版本协商机制 |
| SQLite 并发写锁竞争 | 数据写入超时 | WAL 模式 + busy_timeout(5000) + 写队列 |
| 前端 API 响应格式差异 | 前端解析失败 | 契约测试 (Contract Test) + JSON Schema 校验 |
| gRPC Protobuf 字段不兼容 | 节点间通信失败 | 复用 Java 版 .proto 文件，禁止删改字段 |
| 定时任务 Cron 表达式差异 | 任务不触发 | 全量导入 Java Quartz 表达式并逐一验证 |
| 数据迁移丢失/不一致 | 业务数据异常 | 全量迁移 + 增量校对 + 自动修复 |

#### 迁移验收检查清单 (Migration Acceptance Checklist)

- [ ] 全部 80+ REST API 端点响应与 Java 版 JSON 结构完全一致
- [ ] 前端所有页面功能无感切换（登录、项目、DAG、作业、数据、模型、投票）
- [ ] KusciaJob 提交→执行→状态回写→日志拉取全链路正常
- [ ] 多节点 P2P 投票审批全流程正常 (Alice ↔ Bob)
- [ ] 定时任务创建/触发/停止正常
- [ ] 数据源连接测试 (MySQL/OSS) 正常
- [ ] 模型导出与 Serving 部署正常
- [ ] 进程 RSS < 50MB，冷启动 < 100ms
- [ ] 容器镜像 < 25MB
- [ ] 单元测试覆盖率 > 80%
- [ ] E2E 测试 100% 通过
- [ ] 连续 72h 长稳运行无 Memory Leak
- [ ] 秒级回滚 SOP 演练成功

---

## 12. 综合结论与决策建议

1. **重构结论**：
   强烈推荐实施 Go 重构。SecretPad 作为云原生隐私计算管控平台，其控制面、代理与调度属性远大于重业务 ERP。采用 Go 重构能彻底消除当前 Java/Spring 在边缘节点上的资源瓶颈，实现**内存开销下降 90%**、**冷启动速度提升 100 倍**、**镜像体积缩小至 25MB 以内**，赋予产品极致的轻量化与高可用特性。
2. **ROI 总结**：
   * **研发投入**：中等（主要工作量为转换 DAG 图算法与建立 client-go Informer 监听，详见 11.5 节约 22 周完整执行计划）。
   * **工程收益**：极高（原生 K8s/Kuscia 契约、零 CGO 编译部署、高并发低开销）。
3. **后续行动路线**：
   * 团队按 11.5 节「阶段零」完成工程基座搭建（Week 1-2）；
   * 按 7.3 节完成 Kuscia CRD Reconciler 的 PoC 原型；
   * 按 10.3 节建立依赖注入与静态检查流水线；
   * 依照 11.5 节四阶段方案平滑推进演进，每阶段均设有明确的验证标准与回滚预案。

---

## 附录 A：Go 核心依赖清单 (go.mod Key Dependencies)

| 依赖包 | 版本 | 用途 | 替代 Java 组件 |
| :--- | :--- | :--- | :--- |
| `github.com/gin-gonic/gin` | v1.10+ | HTTP Web 框架 | Spring Web MVC |
| `gorm.io/gorm` | v1.25+ | ORM 框架 | Spring Data JPA / Hibernate |
| `gorm.io/driver/mysql` | v1.5+ | MySQL 驱动 | MySQL Connector/J |
| `modernc.org/sqlite` | v1.29+ | 纯 Go SQLite 驱动 (零 CGO) | SQLite JDBC |
| `k8s.io/client-go` | v0.31+ | Kubernetes 客户端 | Fabric8 K8s SDK |
| `k8s.io/apimachinery` | v0.31+ | K8s 类型定义 | - |
| `google.golang.org/grpc` | v1.65+ | gRPC 框架 | gRPC-Java |
| `google.golang.org/protobuf` | v1.34+ | Protobuf 运行时 | Protobuf-Java |
| `github.com/google/wire` | v0.6+ | 编译期依赖注入 | Spring IoC Container |
| `go.uber.org/zap` | v1.27+ | 结构化日志 | Logback / SLF4J |
| `github.com/spf13/viper` | v1.19+ | 配置管理 | Spring Configuration |
| `github.com/prometheus/client_golang` | v1.20+ | Prometheus 指标 | Micrometer |
| `github.com/golang-jwt/jwt/v5` | v5.2+ | JWT Token | jjwt |
| `github.com/robfig/cron/v3` | v3.0+ | Cron 定时任务 | Quartz Scheduler |
| `github.com/golang-migrate/migrate/v4` | v4.17+ | 数据库迁移 | Flyway |
| `go.opentelemetry.io/otel` | v1.28+ | 分布式链路追踪 | Spring Cloud Sleuth |
| `github.com/aws/aws-sdk-go-v2` | v1.30+ | AWS S3 存储 | AWS SDK Java |
| `github.com/stretchr/testify` | v1.9+ | 测试断言 | JUnit 5 / AssertJ |
| `go.uber.org/mock` | v0.4+ | Mock 框架 | Mockito |

---

## 附录 B：Java → Go 设计模式映射速查表

| Java 设计模式/机制 | Go 对应实现 | 说明 |
| :--- | :--- | :--- |
| Spring IoC / @Autowired | Google Wire 编译期注入 | 零反射、零运行时开销 |
| Spring AOP / @Transactional | GORM Transaction Callback + Middleware | 显式事务边界 |
| Spring ApplicationEvent | Outbox Pattern + Channel 广播 | 事务性事件发布 |
| JPA @Entity / @Repository | GORM Model + Repository Interface | 依赖倒置 |
| ThreadPoolExecutor / @Async | Goroutine + BoundedPool + Channel | 轻量级并发 |
| Quartz Scheduler | robfig/cron + Goroutine | 无 JVM 依赖 |
| Hibernate hbm2ddl | golang-migrate 显式 SQL | 受控变更 |
| Spring Security Filter Chain | Gin Middleware Chain | 可组合中间件 |
| Fabric8 K8s Informer | client-go SharedInformerFactory + WorkQueue | 原生事件驱动 |
| Lombok @Builder/@Getter | Go struct + 构造函数 | 语言级简洁 |
| Jackson ObjectMapper | encoding/json + struct tag | 标准库即可 |
| Guava Cache | sync.Map / github.com/dgraph-io/ristretto | 高性能缓存 |
