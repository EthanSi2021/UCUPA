# UCUPA 基本公理

**状态：已接受的稳定上位公理；约束详细语义；不承担当前状态或计划；不授权实现**

本文把 UCUPA 的立项初心、能力重新判定和开放空间可行性评估压缩为一组已接受的上位约束。它不是新模块、API 清单、实现合同或开发计划；所有详细语义必须服从这些公理。当前权威和实现合同只由[架构权威清单](README.md)导航，当前动作只由[开发计划](development-plan.md)维护。

历史问题证据见《原版 UCUPA 功能与设计图谱》，逐项判断见《UCUPA 能力重新判定账本》，开放体系论证见《UCUPA 作为开放三维空间网络与无限赛博宇宙基础设施的可行性》。《Core Range 与空间根真相决策》保留为聚焦证据草案；Range／root 的已接受目标语义由[整体概念基线](ucupa-concept-mainline-baseline.zh-CN.md) D2、D3、D4 与 D10 承载。

## 1. 定义

> **UCUPA 是由独立权威共同维护的、逻辑无界的稳定空间事实网络：它以 Element 的结构、位姿与领域中立的发现覆盖为最小公共空间语义，以 Facet 承载附着于 Element 的类型化稳定扩展，并通过有证据的接入、发现、读取、观察和变更合同连接空间。**

“逻辑无界”只表示体系可以持续接入新的有限空间和自治权威而无需改变公共语义，不表示无限精度、无限容量、永远可用或一次操作覆盖整个体系。

```text
外部资源与任意上层系统
Applications / robotics / other consumers
                  │
      Enter / Discover / Read / Observe / Propose
                  │
════════════ UCUPA 公共空间薄腰 ════════════
 Core：Element 的存在与空间事实
 Facet：Element 的类型化稳定扩展记录
───────────────────────────────────────────
 Fact authority / federation：唯一事实 owner、delegation、acceptance
 Access control / trust：caller establishment、authorization relation
 Reachability：权威指定联系、连接与独立 freshness
───────────────────────────────────────────
 可替换 Store / index / gateway / transport / deployment
```

## 2. 十条基本公理

### A1. 开放自治

UCUPA 由可独立运营的空间事实 authority 组成。体系的增长不依赖一个中央世界数据库、中央组织或所有参与者共同升级的内部实现。

### A2. Element 同一性

Anchor 与 Entity 是两种封闭的 Element kind。ElementUUID 是稳定、不可复用的身份；rename、Pose update、reparent 和 route change 不改变它。若一次 fact-owner transition 声称同一 Element 连续，UUID 也必须保持不变；[概念与主线基线](ucupa-concept-mainline-baseline.zh-CN.md) D12 负责该连续性的详细语义。UUID 不表示位置、路径、权限或 endpoint，也不存在只凭裸 UUID 查遍整个开放体系的总目录。

### A3. 空间完备性

目标 Core 中，一个普通 Element 的稳定空间事实由 `Structure + parent-relative Pose + domain-neutral Range` 构成。Pose 回答原点与朝向，Structure 回答相对哪个 Anchor，Range 回答该空间存在可能延伸到哪里；三者共同支持通用空间发现。

Range 必须有确定、可组合、保守且不产生 false negative 的相交语义。它的形状、Anchor 聚合方式、父子包含规则、空间根表示和数值编码不由本公理冻结；当前详细目标由主线 D1–D4、D10 唯一承载。

### A4. 空间结构

Anchor 是局部坐标 frame 和结构节点，Entity 是位于 Anchor 中的空间叶 occurrence。结构 parent 单一且无环；业务关系不进入 Core。根、Pose、Range 与空间边界的详细目标由主线 D1、D2、D4、D9 唯一承载。

### A5. 派生值不是真相

WorldPose、结构 ancestry/address、SubtreeEnvelope、children index、空间索引、查询结果、派生事件、按指定联系解析所得的 endpoint、实际可达性和连接缓存都从权威事实与当前运行上下文派生。它们可以持久保存以支持性能或恢复；丢失后可以要求重建索引、重新取快照或重新建立观察，但不能改变权威事实，也不得成为竞争的 Element 真相或第二 writer。

关系中由有权主体指定的联系方式可以是该关系 owner 的权威控制事实，但不是 Element 空间真相或 Facet payload。必须区分“本关系权威指定了怎样联系目标”与“调用方此刻能否连接”：前者不能当作缓存任意丢弃，后者不能自行创建归属或权限。具体联系机制由主线 D6、D8 定义，本公理不要求 Ref 地址目录、路由注册或某种部署系统。

### A6. Facet 是受控扩展面

Facet 是附着于现存 Element 的稳定类型化记录。Facet truth owner 拥有 key、记录存在、精确 payload、owner-local revision 和最终 mutation；类型定义者拥有 payload 的领域含义与验证。访问控制 owner 将 caller 建立为 PrincipalRef，并持有 PermissionBinding 等授权关系。Core／Facet 定义各自操作所需的 PermissionID 与 Scope，并在最终读取或 mutation 边界强制执行授权结果；当前 Realm 是这一访问控制角色的参考实现，但它不因此拥有或复制 Core／Facet 事实。类型身份必须稳定且不可含糊，但其编码和解析机制另行决定。

Facet 不拥有 Element existence，不复制 Core 空间事实，不复活已删除 Element，也不是“所有非空间数据”的容器。资产字节、账户、工作流、实时物理和系统私有过程仍由各自系统拥有。

### A7. 唯一权威与拓扑正交

每项权威事实在任一有效切面只有一个可验证的语义 owner；复制、缓存、路由与部署不能产生第二 writer。下列五种关系互不推导：

```text
Spatial structure                  = Element 在哪里、相对哪个 frame
Fact-authority / delegation graph  = 谁拥有、提交并证明 Core／Facet 与 delegation 事实
Access-control / trust graph       = 谁建立 caller，并允许其观察事实或提出操作
Reachability graph                 = 当前如何抵达相应事实 owner
Deployment topology               = Store、进程和 Cluster 在哪里运行
```

Anchor 不是 Realm，Realm 不是 Cluster，Route 也不是事实 owner。部署迁移或 route 变化不得改变 Element 身份与空间意义。Core／Facet 及 delegation／acceptance 的各 fact owner、访问控制／trust owner、可达性 owner 和部署 owner 各自拥有明确的控制事实，任何一种关系都不能自动授予另一种关系中的权威，也不能扩大 Core／Facet 的世界语义。

### A8. 接入、寻址与发现分离

消费者分别取得明确的空间入口、caller／trust 上下文和当前 reachability 上下文，再组合它们以解析已知结构地址或发现未知 Element；这些上下文的 owner 可以不同，任何一个都不能推出另外两个。`SpaceReference`、`SpaceHandle` 与 `StructuralAddress` 的详细职责由主线 D9 冻结，具体编码不属于本公理。

结构地址必须从当前 Anchor 结构派生，reparent 后可以变化；UUID 只确认同一性。known-ID point CRUD 不能构成空间基础设施 baseline，调用方也不能被要求预先知道目标 UUID 或猜一个共同祖先。

### A9. 查询与观察必须诚实

空间发现不能因为 Element 中心位于 QueryRegion 外、索引陈旧、相关 fact owner 不可达或执行预算耗尽而静默漏报。完整性必须明确绑定本次查询的 semantic universe、selector、authorization view 与每个 contributing fact owner 的 read cut，并分别保留 frame、fact-owner、freshness、coverage、missing-fact-owner evidence 和 route／endpoint reachability evidence。公理只要求“确认不存在”与“当前无法确认”严格分离；具体结果语义由主线 D10、D11 承载。

语义查询 universe 与执行预算必须分离：调用方可以显式选择一个结构或空间 universe；hop、depth、fan-out、deadline 和 page size 若仅作为预算，耗尽时只能产生 continuation 或不完整结果，不能改变该 universe 内“完整”的含义。

稳定事实变化必须能够从每个 contributing fact owner 的显式 read cut 衔接观察、检测缺口并恢复。多 fact-owner 结果保留各自的 cuts、游标、missing-fact-owner evidence 和独立的 route／endpoint evidence，不合成一个全局原子起点；主线 D11 冻结可观察语义，具体 stream 和 transport 仍是实现机制。事件是权威事实的派生观察，不是第二真相。

### A10. 联邦与上层运行时不合并真相

Delegation、acceptance、reachability、access-control permission 与 write authority 相互独立；联邦增加可遍历性，但不合并 owner、控制面、时钟或事务。开放体系不要求一个全局根、全局空间索引、`WorldRevision`、跨 owner 原子快照，或与 Anchor tree 同构的事实权威、访问控制、信任、组织或部署层级。

UCUPA 保存跨 Runtime 停止与重启仍成立的稳定空间事实。上层系统可以持久保存自己的资产、创作数据、业务状态、检查点和产品投影，也可以在有限 region／epoch 内拥有实时权威；但它们不能把 UCUPA 的 Element existence、Structure、Pose 或 Range 镜像成另一份独立权威。只有明确映射为 Core／Facet 的稳定变更，经相应 Core／Facet fact owner 独立验证并提交后才成为 UCUPA 事实。

## 3. 公共薄腰的行为结果

十条公理至少要求以下五类产品中立行为；名称不冻结 Go API、wire、模块或进程边界：

1. **Enter**：组合独立取得的空间入口、caller／trust 与当前 reachability 上下文。
2. **Discover**：按 Structure 或空间区域发现未知 Element。
3. **Read**：读取带来源、权威切面与完整性证据的 Core／Facet。
4. **Observe**：观察已提交事实，并在中断后检测缺口和恢复。
5. **Propose**：向 fact-authority evidence 确认的相应 Core／Facet fact owner 提交有身份、有前置条件、可收敛重试的变更；只有该 owner 接受并提交后才产生新事实。

Adapter 可以转换编码、传输和调用者上下文，也可以缓存带证据的投影；它不能补造缺失发现或观察、镜像永久世界、合成全局 Revision、制造跨 owner 原子性，或把 `UNAVAILABLE` 掩盖成空结果。

## 4. 明确排除

- 全局 `World` 对象、中央数据库、裸 UUID locator、全局 Revision 或全局事务；
- 把 Anchor tree、事实权威／delegation graph、访问控制／trust graph、组织结构和 Cluster topology 合并为一棵树；
- 把 displayName、文本路径或名称解析设为身份、路由、授权或发现正确性的前提；
- 把 WorldPose、结构地址、索引、SubtreeEnvelope、事件或查询缓存提升为第二真相；
- 在 UCUPA 中内建 Asset、Mesh、Collider、LOD、渲染、物理或产品业务语义；
- 把外部资产 bounds 自动解释为 Core Range；
- 由中央 bridge 或 Adapter 私建第二套世界索引；
- 为迁移制造 mirrored truth、shadow Element 或跨 fact-owner 原子承诺；
- 把 Store、数据库、Transport、Carrier、Operator、Console 或模块数量写成世界公理；
- 为已删除原型保留版本后缀、旧 Store reader、转换器或兼容分支。

`displayName` 仍可由 Facet 或消费者作为可选展示特性提供，但它不是 UCUPA baseline 的前置条件。

## 5. 可证伪门

任一后继设计若无法满足下列结果，就必须回到公理层重审：

1. 所有上层 Runtime 关闭后，稳定 Element 与 Facet 仍由其各自 fact owner 保存并可重新取得。
2. 从一个明确空间上下文能够发现未知 Element，而不是只能按已知 UUID 读写。
3. 巨大 Element 的中心在查询区域外但覆盖相交时仍可命中；无法证明完整时明确报告不完整。
4. route／endpoint 或所需 fact owner 不可用不会被解释成 Element 不存在。
5. 清空派生地址解析结果、索引、连接缓存与 Client 投影后，可以从保留的权威事实和联系控制记录重新构造它们；这不授权删除持久联系记录或提交历史。
6. 替换 Store、transport、gateway、语言或部署拓扑不改变公共空间语义。
7. 去掉任一具体消费者后，UCUPA 仍是完整、通用的空间体系。

## 6. 权威边界

- 本文只冻结 A1–A10；[概念与主线基线](ucupa-concept-mainline-baseline.zh-CN.md)是服从这些公理的唯一详细目标语义。
- 本文不维护当前实现状态、下一工作、Store、API、模块或阶段；这些内容不能反向证明一项概念应进入公理。
- 投影或实现合同若需要本公理和主线没有的语义，必须先修改相应权威并重新通过反例门，不能在下游自行补造。
- 具体实现仍须独立合同与明确开始；公理或主线的接受本身不授权代码。
