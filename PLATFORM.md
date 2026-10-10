# UCUPA Platform — 非规范投影

**状态：当前非规范投影；仅投影已接受的基本公理与概念主线；不拥有规范或实现权威；不授权代码实现。**

本文只帮助从产品、真相分类和模块组织三个角度阅读 UCUPA。规范语义仅来自已接受的
[《UCUPA 基本公理》](docs/architecture/ucupa-foundational-axioms.zh-CN.md)与
[《UCUPA 概念与主线基线》](docs/architecture/ucupa-concept-mainline-baseline.zh-CN.md)。
已交付的受限能力从[架构权威清单](docs/architecture/README.md)中的现行合同读取；
实现范围与验证边界见[快照状态](docs/architecture/development-plan.md#1-当前在哪里)。
若本文与二者冲突，本文错误；任何缺失或歧义必须回到规范来源裁决，不能在投影中补造新语义。

## 1. 产品目的

**Spatial infrastructure for the united cyber universe**

UCUPA stands for **The system of United Cyber Universe Position and Anchor**.
It organizes a world as a tree of local coordinate frames, designed for growth in spatial extent
and local detail. Independent operators store and maintain their own parts of that tree, using
common identity and spatial rules rather than one central world database.（A1、A10；D1–D4、D6–D9）

以上是对外定位，不将多个独立空间的互联作为项目的全部定义，也不承诺已验证的无限容量或精度。
“工程原型”仅描述当前实现成熟度；能力和验证范围从现行合同与交付记录读取，不写入定位。

UCUPA 的最小公共空间语义围绕 Element 展开：

- `ElementUUID` 提供稳定且不可复用的同一性；它不是路径、位置、权限、owner 或 endpoint。（A2；D4）
- Core 以 `Structure + parent-relative Pose + Range` 描述 Element 的空间存在与组织。（A3、A4；D1–D4）
- Facet 是可选的不透明内容扩展；Core-only Element 可以完整成立，Facet 不复制 Core 真相，也不拥有 Element existence。TypeRef 不赋予 Facet 内容格式校验职责。（A6；D5、D6）
- authority、access control、reachability 与 deployment 相互正交，不能互相推出。（A7、A10；D6–D8、D12）
- 上层应用可以消费和扩展这些事实，但不能建立竞争的 UCUPA 世界真相。（A10）

“逻辑无界”表示体系能持续接入新的有限空间和自治权威而不改变公共语义；它不承诺无限精度、
无限单点容量、永久可用或一次请求遍历全部空间。（A1、A9、A10；D3、D10）

## 2. 五类公共行为

UCUPA 对外投影为五类产品中立行为；这些名称不冻结 Go API、wire、进程或部署形态。

1. **Enter**：从可验证的 `SpaceReference` 取得边界与明确入口，结合 caller trust evidence 并验证连接目标，建立临时 `SpaceHandle`；不要求先按 Ref 查地址目录。（A8；D7–D9）
2. **Discover**：在明确的结构 closure 中按 `StructuralAddress`、直接结构或 `QueryRegion` 发现未知 Element；有效查询只返回带证据的 `COMPLETE`／`PARTIAL`，无法形成有效查询时则在结果前明确返回 `INVALID`／`DENIED`／`UNAVAILABLE` outcome。（A8、A9；D9、D10）
3. **Read**：从已确认的 Core／Facet authority 读取 exact record、absence 或失败，并携带 Revision、ReadCut 与来源证据。（A6、A9；D4–D6、D11）
4. **Observe**：从 owner read cut 继续观察已提交变化；中断、过期或 source graph 变化必须可恢复或显式报告 GAP。（A9；D11）
5. **Propose**：向相应 fact owner 提交有身份、前置条件和稳定 OperationID 的提案；只有 owner 接受并提交后才产生新事实。（A7；D4–D8、D12）

`Resolve`、`ListDirect` 与 `QueryRegion` 是 Discover 的不同形态；create、update、reanchor、delete
与 Facet mutation 是 owner-specific Propose。公共薄腰不要求一个通用 CRUD 接口。（D4、D5、D10）

## 3. Platform 视图

```text
上层应用与外部资源
Applications / robotics / other consumers
                    │
      Enter / Discover / Read / Observe / Propose
                    │
════════════ UCUPA 公共空间薄腰 ════════════
 Core：Element existence / Structure / Pose / Range
       + per-Element FacetAttachment
 Facet：optional exact opaque content keyed by Element + TypeRef
────────────────────────────────────────────
 fact authority / access-control / reachability / deployment
────────────────────────────────────────────
 可替换 Store / index / transport / gateway / deployment
```

### 3.1 Core 空间视图

Core 产品面独占 Element 空间事实语义；每个有效事实切面由恰一个 Core-kind `FactAuthorityRef` 拥有。
普通 Element record 至少表达稳定 `ElementUUID`、封闭 `Kind`、当前 Structure、相对父 Anchor 的
LocalPose、Range 与 owner-local Revision；root 没有 LocalPose。（A2–A4、A7；D1–D4、D6）

Range 是以该 Element 为根的结构闭包的保守发现覆盖：Entity Range 覆盖该 occurrence，普通 Anchor
Range 覆盖其 zone；bounded parent 必须直接包含每个 child 投影后的完整 Range，独立空间唯一无父
Anchor 使用类型化 `UNBOUNDED_ROOT`。（D2）

坐标真相只有右手 Cartesian `SE(3)`、统一单位且无 scale。polar／geodetic 等只能是边界投影；
数值不确定性必须向不漏报方向收敛，不能制造 false negative。（D1、D3）

### 3.2 Facet 扩展视图

Facet 是可选专用扩展。只有 Core 空间事实的 Element 已经完整；需要独立 owner 保存内容、权限、
Revision 或生命周期时，Facet truth 才由 `(ElementUUID, FacetTypeRef)` 精确定位，
包含 exact payload 与 owner-local Revision。
`FacetTypeRef` 是固定 32-byte 的不透明键，不要求可解析的类型制品或中央 schema service。
Facet 按原始字节保存和返回有界 Payload，不解析内容或执行 schema 校验；上层自行约定和验证
格式与业务含义。权限、容量、接受关系和 Revision 前置条件仍由对应 owner 检查。（A6；D5）

Core existence 是 Facet 可见性和 mutation 的前提。Facet 不能复活已删除 Element；Core 与 Facet
也不伪装成一个跨 owner 原子事务。（A6、A10；D5、D6、D11）

Core 还权威保存并返回该 Element 的 Facet 关系：`NONE`、带 matching proposal/acceptance
的 `ACTIVE(exact FacetAuthorityRef)`、保留未证明删除语义的 `DETACHED`，以及 P7 明确处置后的
`RETIRED`。退役不是从未附着，不能隐去或自动复活；具体终态规则见
[生命周期合同](docs/architecture/owner-transition-contract.md)。它不属于 Element 空间记录；
其 pending／ACTIVE 联系记录保存预期 Facet 身份与指定 `FacetNextHop`。Facet 按 exact AcceptanceKey
保留可维护的 `CoreNextHop`，不覆盖 immutable acceptance，也不建立 current-Core 指针。
Core existence 不是 Facet 可服务的充分条件；Core exact read 从 ACTIVE attachment 形成 `FacetAccessContext`，
Facet 据其选择 exact 关系联系、fresh-read source Core 并验证 matching acceptance。只有 matching `ACTIVE` cut
才能定位完整 namespace；Get／类型枚举还须验证所捕获的 Facet cut 与联系仍成立。（D5、D6、D11）

Core mount 与 Facet attachment 共用“预期 `FactAuthorityRef`＋权威指定 `NextHop`”的联系原则。
两方向由各自本地 owner 维护；纯联系变化只推进联系 token 与该 owner cut，不改变 Element／Facet Revision、
关系身份、生命周期或 writer fence。服务入口由部署环境提供，联系记录决定本关系采用哪个入口；
不是所有 Ref 都携带地址，也不是每个 authority 只有一个全球地址。不再建设强制的 Ref → 地址目录，
目标认证、网络传输和多地址能力仍须分别按现行合同与证据声明；
TypeRef 不参与服务寻址，Facet 不内置类型合同解析器。（A5–A8；D5、D6、D8）

### 3.3 接入、结构地址与发现视图

`SpaceReference` 是可分享的 bootstrap 描述，`SpaceHandle` 是某 caller 当次进入后组合出的临时上下文；
二者都不是新的世界对象、永久 locator 或可转移权限。（A8；D9）

`StructuralAddress` 是已接入空间中从已知 Anchor 上行／下行的结构路径，由选定切面的 Structure 派生；
遵守显式 boundary，reparent 后 current 地址可以变化。
裸 `ElementUUID` 只能确认同一性，不能触发对开放体系的全局查找。（A2、A8；D9）

Discover 用 Start 与向上深度选定 Scope，完整搜索该 Scope 及后代，不设向下深度。
向上深度 -1 表示不限层数，直到已证明的空间根或明确接入边界；不扩大权限，也不把断连当作到根。
UpDepth 是语义范围选择；转发 hop、fan-out、deadline 和 page size 等执行限制不能偷改范围或把未检查空间当空。
每 Core 启动时可选配置 SnapshotRetention，没有默认保留时长；未填关闭，显式有效正时长启用，
无效值使配置失败。关闭仍可查询、分页和按原切面精确读：CurrentVerified 每次核对原 cut 仍 current，
已变就明确过期；Retained 则在有限期限内保留旧视图，普通写入不使旧页失效。
跨 Core 只要本次实际来源（含证明方）有一方关闭，整链使用 CurrentVerified；全部启用才使用 Retained。
ReadMode 在首页确定并公开，资源不足不自动换模式；未启用不等于不可达、过期或内容缺口。
普通写入仍可靠提交、普通读取默认最新；旧链不切 latest，来源及关系仍须验证，不宣称全网一致。（A9；D9、D10）

### 3.4 Realm 授权联系视图

Core／Facet 到 Realm 的访问沿用相同联系原则：`RealmAttachment` 指明已有 `RealmID` 和指定
`NextHop`，接收方按受控联系核对目标并完成身份／权限校验，不另建 Ref 地址目录。
它是授权访问链，不复制 Realm 的 PermissionBinding，不改变事实归属。
已交付的 S2 实现由每个 Core／Facet 在自身 Store 保存唯一联系，不在每个 Element 或 payload
重复记录；支持受控本地装配和独立 Realm 服务的网络联系，以及同 Realm 的指定地址维护。
网络等待在事实锁外，最终读写边界仍核对请求、授权依据和联系代次；不可达不变成离线 Allow。
多个 Realm 各自自治，不要求总 Realm 或多副本。具体行为见
[RealmAttachment 合同](docs/architecture/realm-attachment-contract.md)；
这些能力不等于真实多机 Realm、生产规模或高可用均已验证。（D7、D8）

## 4. Minimum-truth 投影矩阵

| 分类 | 投影内容 | 唯一 owner／来源 | 明确不是 |
|---|---|---|---|
| Core 世界真相 | Element identity、Kind、existence、Structure、LocalPose、Range、ElementRevision，以及删除后 UUID 永不复用的不变量 | 对应 Core-kind `FactAuthorityRef`（A2–A4；D1–D4、D6） | WorldPose、路径、索引、Route、Endpoint、Cluster |
| Facet 世界真相 | 可选 exact `FacetKey`、payload 与 FacetRevision | 对应 Facet-kind `FactAuthorityRef`（A6；D5、D6） | Element existence、FacetAttachment、Core 空间副本、类型服务 endpoint |
| 事实权威控制 | `FactAuthorityRef`、Core delegation／acceptance、逐 Element FacetAttachment proposal／acceptance、writer fence 与 transition 状态 | 每项控制事实的明确 owner（A7、A10；D6、D12） | Realm Allow、实际可达性、部署位置 |
| 身份与权限控制 | caller evidence、owner-local `PrincipalRef`、PermissionBinding、closure-bounded authorization view | access-control／trust owner；当前产品组织由 Realm 承载（A6、A7；D7） | Core／Facet ownership、事实存在、Route |
| 联系控制 | 按 exact 跨服务关系保存的预期目标身份与权威指定 NextHop；事实服务使用 FactAuthorityRef，RealmAttachment 使用 RealmID；具体修订和持久边界由对应合同确定 | 保存该方向联系记录的明确 owner（A5、A7；D6–D8） | Ref → 地址目录、全球单一地址、ownership 切换、permission、writer activation |
| 部署控制 | Cluster、进程、Store、placement 与运行拓扑 | deployment owner；不进入公共空间真相（A7；D8、D12） | Element identity、FactAuthorityRef、空间结构 |
| 派生值 | WorldPose、StructuralAddress、SubtreeEnvelope、索引、query result、派生事件、cursor、cache、地址解析结果与实际连接／可达性状态 | 从权威事实、联系控制及当前运行上下文重建（A5、A9；D8–D11） | 第二真相、全局 Revision、第二 writer、可任意丢弃的联系控制历史 |
| 延后机制 | API／wire 拼写、Store schema、索引算法、partitioning、credential 与 TLS、lease clock、具体部署和产品 Facet 合同 | 由后续明确合同选择（D1、D3、D5、D7–D12） | 当前空间公理或本文可决定的实现 |

每项事实和控制状态都必须有唯一、可验证的 owner 边界。读取多个 owner 时保留 per-owner cuts；
不存在把它们合成全局 Revision、全局原子 snapshot 或跨 owner 事务的隐含权威。（A7、A9、A10；D6、D10、D11）

## 5. Authority、联邦与 transition 规则

- parent Core authority 继续拥有 mount Anchor record；accepted carrier Core authority 只拥有该边界的 contents coverage，不复制 mount Anchor。（D6）
- delegation、acceptance、access permission、指定联系与 deployment 是不同控制关系；其中任何一个都不能自动授予另一个。（A7、A10；D6–D8）
- Realm 建立身份并判断权限，不拥有 Core／Facet 真相；Cluster 只表达承载环境，不是 fact owner。（A7；D6–D8）
- 联系信息只回答本关系怎样抵达预期 authority；按指定入口连接后仍须验证目标与关系。联系失效只导致不可用或部分结果，不能删除事实或转移 authority。（A5、A7；D8、D10）
- 经 D12 连续性证据接受的 owner transition 保持 ElementUUID；新 writer 启用前旧 writer 必须被持久 fencing，允许明确不可用窗口，禁止 shadow、dual write 与双显。（A2、A7、A10；D12）
- reanchor 改变 Structure／Pose 但不改变 ElementUUID；跨 Core transition 不自动迁移或复用 Facet attachment acceptance。（D4、D6、D12）

## 6. 当前产品组织

下表只是当前仓库的产品包装投影，不是公共协议、公理、未来必选模块或永恒模块数量。其他实现可以采用
不同 package、进程与部署拆分，只要保持规范定义的事实 owner、控制边界和公共行为。（A1、A7、A10；D4–D12）

| 产品模块 | 当前包装角色 | 不能据此推出 |
|---|---|---|
| `ucupa-core` | Core 空间事实的当前产品边界 | 一个固定进程、Store 或部署形态 |
| `ucupa-facet` | exact opaque Facet content 的当前产品边界 | Element existence、schema 校验或 Core 空间副本 |
| `ucupa-realm` | access-control／trust 的当前参考实现 | 空间自治或 Core／Facet ownership |
| `ucupa-operator` | 部署意图与运维编排的候选包装 | 主基线已冻结其 API、状态或独立 owner |
| `ucupa-archive` | 离线语义交换的候选包装 | 在线 authority、Store backup 或在线请求路径 |

UCUPA 不再设置 Console 产品模块或内置可视化面板。面向人的浏览、管理和可视化属于外部客户端，
按相关 owner 的公开操作与授权访问；UCUPA 保留自身的可编程操作、观察及诊断职责，
不要求某个 UI 存在才能使用或运行，也不把原 Console 包装转移给 Operator。（A10；主基线第 9 节）

SDK、host、server、transport、Store adapter、index 与 conformance 工具不是新的产品真相 owner；
按地址工作的 DNS／传输适配不构成 UCUPA Ref → 地址目录。
模块同进程或跨进程部署都不能改变上述语义边界。（A1、A5、A7、A10）

## 7. Greenfield 排除

- 不建立全局 World、裸 UUID locator、中央 Element／owner／Cluster directory、全局 Revision、全局事务或中央空间索引。（A1、A7、A10；D6、D9–D11）
- 不把 Anchor tree、authority graph、Realm trust／permission、Route 和 Cluster topology 合成一棵树。（A7、A10；D6–D9）
- 不把 WorldPose、结构地址、索引、query result、event、cursor 或 cache 提升为竞争真相。（A5、A9；D9–D11）
- 不在 Core 中加入 scale、Collider、资产 bounds、LOD、渲染、高频物理或产品业务语义。（A3、A4、A10；D1–D4）
- Metadata 与 displayName 均不属于当前 baseline；只有真实消费者和独立合同证明后才可作为可选能力进入，且不得成为身份、结构、空间查询、寻址、路由或授权的前提。（A2、A4、A8；D4、D9）
- 不把文本名称、alias 或名称解析作为身份、寻址、授权或发现正确性的前提。（A2、A8；D9）
- 不用 exact UUID Grant 代替未知发现授权；UpDepth 明确选择 Scope，不能用执行 hop 或 timeout 偷改该范围。（A8、A9；D7、D10）
- 不用 mirrored truth、shadow Element、dual write、unreachability fencing 或跨 owner 原子承诺掩盖 transition。（A7、A10；D12）
- 不为已删除原型保留版本后缀、旧 Store reader、转换器、迁移支线或兼容 facade。（基本公理第 4 节；主基线第 8 节）
- 不把数据库、消息总线、Kubernetes、当前模块数量或任何当前实现机制写成空间真相。（A1、A5、A10；D8、D11、D12）

## 8. 本投影的权威规则

1. 基本公理约束所有下游设计；主基线 D1–D12 是本文所有具体语义的唯一详细来源。
2. 本文只做有损阅读投影，不接受、修改、补全或否决任何 UCUPA 语义。
3. 本文与规范来源冲突时，以规范来源为准，并修正本文；不得反向解释规范以保留本文措辞。
4. 本文没有引用来源的陈述不能作为 API、Store、wire、部署或实现决策依据。
5. 任何后续实现合同必须声明覆盖的规范条款和公开反例门；“出现在 Platform 视图中”不构成代码授权。
6. 接受或更新本投影仍不自动授权实现。
