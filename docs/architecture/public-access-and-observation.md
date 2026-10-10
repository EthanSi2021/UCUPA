# UCUPA 公共接入与事实观察合同

**状态：P5 设计合同已接受并提交为 `3c00b0c1`；代码与 schema 已实现、完整验证，并按用户“开始，验收并提交”的授权完成交付验收，随本次 S1P5 提交。本文为明确范围内的 current 增量合同，范围由架构权威清单声明，证据与提交记录见开发计划第 8 节。**

本文将[主线 D5、D7、D9–D11](ucupa-concept-mainline-baseline.zh-CN.md)落实为公共行为增量。
它不重做 D1–D4，不建立通用 CRUD owner、独立路由、第二份世界或新授权模型。
五类行为是使用结果，不要求五个同名方法或模块。当前实现仍由[权威清单](README.md)声明；
施工顺序只在[开发计划](development-plan.md)维护。本文作为已验收的 P5 增量，只接替点名的公共值、
调用者/view、公共读写/观察、类型与内容条款；P7.3a 已按[P7 总合同](owner-transition-contract.md)
把第 8 节的类型制品、pin／mirror 与 schema 校验规则修订为不透明 Payload 语义；P1–P4 未被点名的本地行为继续有效。
这不是两套版本并行，也不能用设计接受代证代码完成。

**后续增量 notice：** [P6 网络合同](network-access-contract.md)承接网络公共接入及联邦；
[P7 总合同第 10 节](owner-transition-contract.md#10-读取发现旧快照与观察)承接首批转移的来源失效、
终末观察与重新进入；[RealmAttachment 合同](realm-attachment-contract.md)承接持久授权联系及
网络 Realm 核验。本文“本合同不包含”的表述限定 P5 切片，不否认上述独立交付；未被点名的
公共范围、原操作、分页、观察与权限不变量保持，未启用 profile 不因文档收口变成已支持。

## 1. 使用结果与本轮范围

调用方应当能完成：

```text
明确空间入口 + 独立建立的调用者上下文
  → Enter → 临时 SpaceHandle
  → Discover → 未知 Element、路径、实际范围与读取依据
  → Read → Core 事实、同 cut 附着；按需访问独立 Facet
  → Observe → 从已读依据开始的变化通知、续接或明确缺口
  → Propose → owner 校验、持久提交、可收敛回执
```

P5 首个完整公共 profile 是**单 Core 内的明确 Anchor 闭包，以及该闭包的可选 Facet**，
使用明确、可信的本地连接与调用者建立。多个 Facet 各自拥有 namespace 和流，不合成全局 cut。
它不是 Core／Facet 共用 Store，也不要求一个总 Realm。

P2／P3 的双 Core 本地能力继续保留、回归；本合同不靠给这些入口改名就宣称公共联邦完成。
公共 profile 遇必须跨 mount 的操作明确报告不支持的 coverage：入口 context 不成立则 UNAVAILABLE；
已定义查询的相关后代未处理则 PARTIAL。安全剪枝能够证明不相关的 mount 无需联系远端。
不能返回 COMPLETE 后才在脚注说明忽略了挂载。跨 Core 公共 source graph 的扩展仍须相应联邦证明。

本合同不包含网络服务、远端认证、嵌套联邦、writer／owner 迁移、生产部署或 Stage 2 RealmAttachment。
公共格式不等于公网传输；受信任的本地绑定也不能被标成远端身份认证。

## 2. 实现前基线与本轮增量

下表记录 P5 开工前的差距，不是另保留一套实现。现在的公共入口与验证产物见
[Core API](../../ucupa-core/api/README.md)、[Facet API](../../ucupa-facet/api/README.md)及开发计划的 P5 交付记录。

| 已有依据 | P5 需要增加什么，不得怎样代证 |
|---|---|
| Core `ReadElement`、`ListDirect`、`QueryRegion`、路径读及可选快照 | 明确入口／边界、公共地址和值表示；不能要求消费者认识私有 token 或先知道目标 UUID |
| Core owner-qualified exact scope；query 对解析出的 Scope 授权 | D7 的显式 AnchorClosureScope、操作 cut 上的成员判断与授权 view；原 exact scope 本身不是通用闭包授权 |
| Core `commits` 包含空间、mount、attachment 的唯一顺序 | 经授权的 committed projection 与 cursor；不直接公开 journal、OperationID、actor 或原始命令 |
| Facet `events` 保存命令与各种结果 | 只从实际推进 Facet cut 的提交生成变化；不能把拒绝、不变和重放当新事实 |
| P4 的 TypeRef／witness／注入 validator | 任意 32-byte 不透明 TypeRef 与有界 exact 不透明 Payload；Facet 不获取、解析类型制品或执行 schema（P7.3a 修订） |
| Realm 原 `Authorize` 返回 bool，binding 有 aggregate revision | 受控的授权 view 校验结果；不能由 adapter 用时间戳、缓存 bool 或整库公开版本冒充 view |

对应实现入口：[Core](../../ucupa-core/instance/instance.go)、[查询](../../ucupa-core/internal/query/operation.go)、
[Core 提交](../../ucupa-core/internal/commit/owner.go)、[Facet 裁决](../../ucupa-facet/internal/namespace/mutation.go)、
[Realm 授权](../../ucupa-realm/internal/policy/binding.go)、[共同 scope](../../shared/values/types.go)。

## 3. Enter：入口不是权限，也不是新世界对象

### 3.1 SpaceReference

可分享描述至少包含 BoundaryAnchorRef（CoreRef、AnchorUUID）、指定的 Core NextHop，
以及调用者可识别的 trust-bootstrap hint。hint 可以陈旧，但不含 secret，不授予权限，
不允许覆盖接收方的 Realm 或认证配置。发布者不是因为发布 reference 就成为事实 owner。

本地 profile 的 trust context 由显式装配提供，分别确认目标 Core 与 caller；不把 JSON 中自报的
PrincipalRef 当作已认证身份。缺地址不查 Ref 目录；错目标和失联不解释成空间不存在。

本地公共字段固定为 `boundary:{core,anchor}`、`nextHop`、可选 `trustHint`。
`core` 是 FactAuthorityRef，`anchor` 是 Anchor UUID；编码依第 8 节。`trustHint` 只是调用方可忽略的
不可信提示，不能创建认证会话。调用者的可信上下文作为独立调用参数传入，不放在 reference 中。

### 3.2 建立与使用 SpaceHandle

Enter 先验证输入与独立身份上下文，再向预期 Core 核对入口 Anchor、当前空间形态和必要授权。
成功返回 owner 发行、caller-bound、运行内的临时 handle，以及允许披露的边界／来源依据。
handle 不预枚举子树，不含全局 Revision，不是可转让的授权票据。

- root 或非 root Anchor 都可作为发布边界；进入 Entity 不是进入空间。
- 后续操作重新检查当前权限与所依赖关系；Enter 成功不冻结永久 Allow。
- 边界在同一 Core 中移动，不改变 BoundaryAnchorRef；旧路径、旧 cut 按各自规则失效。
- 边界删除或所属 authority 不匹配，旧 handle 不自动接到别处；不得猜新 owner 或永久重定向。
- Close／过期／实例重启后的 handle 不复用；从原 reference 重新 Enter。正常重新接入不改变 UUID。
- 已知目标但未授权，返回 DENIED 且不确认它存在；只有授权和权威读依据成立后才能返回 NOT_FOUND。

SpaceHandle 负责边界和调用上下文；QueryContinuation、读取快照和 ObservationCursor 各负责自己的寿命，
不是把三者合并成一个“万能 token”。丢弃 handle 不删除任何事实或持久联系。

### 3.3 最小公共调用面

下表固定请求责任与必需值，不要求添加一个通用 facade。字段的规范编码归第 8 节，
已有事实、final tuple、CAS 与 receipt 字段直接引用 owner 合同，不再手写第二份事实模型。

| 行为 | 最小输入 | 结果与约束 |
|---|---|---|
| `Enter` | 独立 caller context、SpaceReference、`intent` | 返回 handle、boundary、该具体操作的最小授权依据；读、发现、create/update/delete 都可作为 intent，不设泛 Propose intent |
| `CloseSpace` | caller、handle | 幂等释放本地派生上下文；不是删除空间 |
| `ReadElement` | caller、handle、结构地址、可选 read basis、`prepareObserve` | 同一 Core cut 的 Element/Attachment 与 read receipt；开启准备时还返回 preparation |
| `ListDirect` | caller、handle、Anchor 地址、limit、可选 continuation、`prepareObserve` | 按现行列表顺序分页，首批确定读取依据和可选准备；续页不重新建立准备 |
| `QueryRegion` | caller、handle、Start/Basis 地址、UpDepth、sphere、selector、limit、可选 continuation、`prepareObserve` | 实际 Scope、ActualUpDepth、matches、状态/缺口、receipt 与续页依据 |
| owner-specific Core mutation | caller、有效 handle、操作名、原 final tuple、CAS、OperationID | 复用原命令及精确重放；reference-only 只用于第 7 节的回执恢复，不能代替新提交所需 handle |
| Facet 内容访问 | caller、Core 披露的指定 Facet 联系、AcceptanceKey、原 type/payload/前置条件 | 独立核验 Facet 身份与权限、Core 当前关系；不把 Core handle 当 Facet 授权 |

结构地址的字段是 `origin`、`up`、`down`：Origin 是本 handle 内的已知 Anchor，可缺省为 boundary；
up 是非负真实父边数，down 是 UUID 链。Core 在同一 cut 验证 Origin 到 boundary 的联系与每条路径边，
不把调用方自报的 ancestry 当证明。上行不能越过 boundary；未知、过时路径不能以另一份最新 cut 偷补。
发现结果返回从已验证 Origin 到 Scope/结果的可复读路径，不另发可绕过边界的裸 UUID 入口。
UpDepth 是查询选择范围的参数，和地址里的 up 不是同一项。

Enter 只验证所选 intent 所需的既有权限，不预授予其他行为；例如 query-region intent 不额外要求
element.read，因此不会仅因进入而泄露 Facet 联系。只有写权限的 caller 也能按相应 create/update/delete
intent 进入，只取得 boundary 有效性与 handle，不返回 ElementRead，不强迫它先取得读取权。
intent 的具体集合及组合权限以第 4 节表为准；update intent 同时检查 update 与 place。
每个后续操作仍按第 4 节独立裁决，进入成功不能充当最终 Parent/成员校验。

## 4. 公共权限：沿用精确关系，补齐闭包 scope

`AnchorClosureScope(BoundaryAnchorRef)` 是新的精确 scope 变体；不改变
`PrincipalRef + PermissionID + OperationScope`，也不引入角色树、通配路径或策略语言。
Realm 只判断这个 scope 是否被明确授予，Core 在自己的事实 cut 上验证成员关系。

1. 使用者取得闭包 Discover 权限后，可以发现该闭包中未来新建的未知元素，不需要为每个 UUID 预授予权限。
2. Enter 检查其实际要建立的发现／读取权限，不新增只有名字而没有独立职责的“进入权限”。
   Discover、空间 Read、Observe 与 Propose 分别授权；Discover 不自动许可修改，
   也不自动授予 Core exact-read 中的 Facet 入口披露、Facet payload 或附着／联系控制权限。
3. 公共 owner 方法必须核对 handle boundary、当前授权 boundary 和本次事实 cut 的成员关系。
   更新要求源 Element 与最终 placement 都满足所用授权范围；移出范围不能只凭旧路径通过。
   既有 exact scope 的独立管理调用继续按本来权限执行，不能绕过公共 handle 的边界。
4. closure 内的本地结构可由 Core 私有遍历，但只有允许披露的记录和解释它所需的路径能返回。
   首个 profile 使用**完整授权闭包**，不提供闭包内部逐节点隐藏／混合 ACL；exact grant 不被猜成闭包 grant。
   此限制是声明的能力范围，不意味着未来可以把隐藏分支当不存在。
5. Facet 按自己的 owner-qualified namespace／key 权限执行；Core closure grant 不跨到 Facet。
   Facet 反向回查 Core 继续使用固定服务身份，不能借用客户端身份或跳过 P4 的有效关系核验。

### 逐操作授权映射

Core 公共路径显式选用 `AnchorClosureScope(handle.boundary)`，不先失败再回退 exact grant。
下表的成员对象均在 owner 的同一 read/commit cut 上验证；授权 scope 相同不等于只验证一个对象。

| Core 操作 | PermissionID（保留既有名称） | 必须在 closure 内的对象 |
|---|---|---|
| ReadElement | `ucupa.core.element.read` | 被读 Element；该权限仍披露同 cut 的完整 ElementRead/Attachment envelope |
| ListDirect | `ucupa.core.anchor.list-direct` | 所列 Anchor 与所披露的直接内容 |
| QueryRegion | `ucupa.core.anchor.query-region` | Start、实际 Scope、Basis、返回记录及解释路径 |
| CreateAnchor / CreateEntity | `ucupa.core.anchor.create` / `ucupa.core.entity.create` | 最终 Parent；初始元素由同一提交纳入该 closure |
| UpdateElement | `ucupa.core.element.update` **且** `ucupa.core.element.place` | 源 Element **及**最终 Parent；不能仅因两者同在闭包而省掉 place 权限 |
| DeleteAnchor / DeleteEntity | `ucupa.core.anchor.delete` / `ucupa.core.entity.delete` | 原被删对象；非空锚拒绝等原约束不变 |

建立根不是已进入空间内的操作，仍为原独立引导接口。挂载、附着和联系控制继续使用各自具体
PermissionID/目标组合，不因上述 closure 权限获得；其 exact 管理入口不被改造成泛 closure 控制权限。
Facet 的 Get/Put/Delete 使用原 FacetKeyScope，ListTypes 使用原 FacetElementScope；没有泛 Discover/Propose grant。
公共内容观察新增 `ucupa.core.element.observe`（Core closure 或 exact CoreElementScope），
`ucupa.facet.element.observe`（FacetElementScope）和 `ucupa.facet.observe`（FacetKeyScope）。
观察还必须持有所依附读取的具体权限；Core 侧服务控制观察见第 6.6 节，不隐式放宽客户端权限。

### 授权 view 与撤销

公共读取、分页和观察需要一个由授权边界发行、绑定 caller establishment 及本次精确要求的 view。
取得 Allow 与 view 必须基于同一次授权状态；披露前验证该 view 仍成立。
比较期间变更则拒绝／重试，不能通过“前后各读一次 bool 都是 true”漏掉撤销后重新授权的 ABA。

从 Realm 已有身份、凭据与 binding 变更依据派生**相关要求的授权代次**；相关 binding 的删除／重授，
即使最终 Allow 相同，也使旧 view 失效。无关 binding 或不可披露事实的变化不作为公共 view 事件。
这个派生 view 不成为另一份 PermissionBinding，也不暴露 Realm 全库 revision 或隐藏主体数量。
授权服务无法确认时是 UNAVAILABLE，不冒充明确 Deny。

最小消费方端口为 `AuthorizeView(caller, requirements) -> decision, opaqueView` 与
`ValidateView(caller, requirements, opaqueView) -> VALID / CHANGED / DENIED / UNAVAILABLE`。
requirements 是去重、规范排序的精确 PermissionID/Scope 集合；一次授权在同一 Realm 状态下取得结果。
requirements 始终由事实 owner 按本次操作派生，不能照抄客户端自报权限。
view 是 Realm 以本次运行随机密钥对 Realm 身份、caller establishment、规范要求集合及相关代次计算的
32-byte HMAC-SHA-256；私有输入采用无歧义字段编码，比较使用 constant-time equal。
验证重算当前要求及代次，不需要额外的 view 注册表、TTL 或持久缓存；密钥不出 Realm，重启更换。
该 token 只供本地受信端口核验，不是远端签名、跨 Realm 信任或新的联系凭据，不公开 binding 序号。
MAC 生成和比较使用标准库能力，参照 [Go HMAC 文档](https://pkg.go.dev/crypto/hmac)，不自造散列认证算法。
已确认当前不允许是 DENIED；当前重新允许但旧代次不同是 CHANGED；无法证实则 UNAVAILABLE。

派生代次只随相关的有效身份/凭据或 binding 状态变更推进，拒绝、unchanged、精确重放不推进。
删除后必须保留该 binding 的派生代次墓碑，不能因 map 项移除后又插入而回到原代次。
恢复时由现有已验证历史重建；Realm 重启使旧运行内 view 失效，不把派生 map 当第二份持久 Grant。
事实 owner 先取得 view，再在自己的 cut 读取/裁决，并在披露或提交的授权检查点验证 view。
该检查点之后发生的撤权不追回已经线性化的操作；禁止声称跨 Realm/事实 Store 有全局原子事务。
可衔接观察的读取额外绑定 Observe 要求，顺序见第 6.2 节；普通 Read 不自动申请 Observe 权限。
本轮直接扩展消费方所需本地端口，不用两次 bool 适配成 view，也不以 Stage 2 为由省略校验。

## 5. Discover／Read：复用已有空间语义

StructuralAddress 只有明确 Origin、上行父边数和下行 UUID 链。每条边在所选 Core cut 上验证，
不能把 old path 解析失败后偷偷改成裸 UUID 查找。返回路径须能用于按原依据复读。

公共查询必须把 Start、Basis、实际 Scope 限定在 handle boundary 内。
首个 profile **同时保留 P3 的 Basis 必须为实际 Scope 或其后代 Anchor 的限制**。
例如 boundary R 下的兄弟 A/B，Start=A、UpDepth=0、Basis=B 仍是 INVALID，不因为 B 在 R 内就放行。
这不是新增空间规则；支持 Scope 外但 boundary 内的坐标参考不在本轮暗中加入。
UpDepth 规则不变：缺省／0 留在 Start，正数最多上溯相应真实父边，-1 到已证明 boundary／root，
小于 -1 非法；上行遇失联不能冒充边界。到达显式 boundary 后停止，不再越界寻找更高祖先。
在选定 Scope 自身及全部后代中匹配闭球相交，selector 不截断 Anchor 遍历，不增加向下深度。

P3 的 ReadMode、分页顺序、限额和失败语义继续适用：

- 不填 SnapshotRetention 仍是关闭旧视图保留；不因 Enter 或 Observe 自动开启或补默认时长。
- CurrentVerified 只能续用仍 current 的原 cut；Retained 按原视图及其有限寿命读取。
- 分页改变深度、范围、basis、selector 或 caller，必须重新发起查询，不能续用旧游标。
- 多页的查询状态只描述选定来源及范围；全部页面取完且 status 为 COMPLETE 才可称该轮完整。
- 公开记录带实际 owner、Revision、cut、范围／路径与授权依据，不复制私有状态或伪造全局切面。

Core exact read 保留 P4 同 cut 的空间记录与附着 envelope。ListDirect／QueryRegion 的结果不额外
泄露 Facet 联系。读到 ACTIVE 后，直接使用关系指定的 Facet 地址和 AcceptanceKey；Facet 返回的
当前内容仍经 P4 F0／Core fresh-check，不承诺读到“与旧 Core 快照同一时刻”的 Facet payload。
需要当前 namespace 时须重新核对当前附着；旧快照上的 ACTIVE 不是长期可访问凭据。

## 6. Observe：有限、可续接的已提交变化通知

### 6.1 固定的最小形态

先提供显式拉取的 `BeginObserve / PollChanges / ReleaseObserve` 行为，不先建消息总线、后台 SDK watcher
或服务端推送。BeginObserve 指定读取时已取得的 preparation；PollChanges 带上一批的 cursor 请求下一批。
只有 owner 已可靠提交的空间、内容或联系／附着等控制变化才可产生通知。

每条通知包含不透明的稳定 ChangeID、发行 owner、允许披露的变化类别和提交依据。
首个 profile 的通知表示**相关事实已变，需重读／重查**，不携带原始写命令、actor、OperationID、
历史 payload 或自动生成的 Enter／Exit／Move 业务事件。一次 Anchor 移动可以使整个查询投影失效，
不能只通知“Anchor 记录变了”却宣称其后代的派生结果仍新鲜。

这是有顺序、可去重的失效通知，不是每帧状态流或完整历史查询 API。同一 commit 的多个相关副作用
作为一个 owner-local change group 交付，不拆开 create 与初始 NONE，或 delete 与附着失效。
公开批次不得把半个 group 作为“已消费”；容量不足明确失败，不截断后确认。

### 6.2 ReadCut 到 cursor 的衔接

需要后续观察的初始读取显式传 `prepareObserve=true`；普通未准备的 Read 不申请 Observe 权限，
也不能只拿一个旧 cut 在 Begin 时临时补造观察资格。准备失败时该次请求明确失败，不能降成普通读取后
仍宣称衔接有效。已有旧快照可以继续普通读取；要开始观察则重新建立当前读取依据，而非追溯任意历史时刻。

固定顺序如下：

1. Realm 在同一状态取得本次 Read/Discover 要求与对应 Observe 要求的联合 view。
2. owner 在选择当前 cut C 的同一提交排序临界区，登记 preparation；其私有内容为 caller/view、
   所选 boundary 或 exact namespace/key、查询条件、起始 C、有限通知队列及绝对期限。
   登记失败先返回 RESOURCE_EXHAUSTED，不留下部分可用 token，也不阻止后来普通写入。
3. 用 C 完成本次读取；必要时保留的跨页快照仍独立按 P3 管理。发布读取结果及 preparation 前验证 view；
   失败释放未交付的准备。C 后的相关提交已经进入该准备，不能等 Begin 才登记。
4. BeginObserve 验证同一 caller、条件和联合 view，将该 preparation **原地幂等激活**并返回初始 cursor。
   重试同一 preparation 不重新起流、不续期；不能以新的 Observe view 替换旧的。

分页的 preparation 属于第一批确定的 C；后续页复用，不重新从中间页面创建。调用方可以边翻页边 Poll，
但原查询未 Complete 的缺口仍存在；初始查询失败或改条件时释放准备并重查。
经授权证明的空列表、空空间结果或不存在的 Facet key 仍是合法读取依据，可以返回 preparation，
以观察后续新建/首次 Put；不能把“当前无结果”误当登记失败。无权或无法证实则不能建立这种空基线。
Read 权限保持、Observe 单独撤销又重授，即使发生在首次 Begin **之前**，也会因联合 view 改变而失效。
当前无权则 DENIED，已重新允许但代次不同则 GAP，均不补看失权期间变化；没有 preparation 只能新读。

在准备和授权仍有效且保留足够的前提下，第一次观察覆盖所有相关 `commit > C`，不从 Begin 的 latest 开始。
Core 的依据仍是既有唯一 `commits` 顺序；Facet 只有实际推进 cut 的 Committed 项进入投影。
拒绝、UNCHANGED、精确重放和查询本身不产生新事实通知。

cursor 绑定 owner／发行运行、caller／authorization view、boundary／selector、起始依据和扫描位置。
客户端不能编辑位置跳段；相同 cursor 可重复请求，可能重复收到稳定 ChangeID，但不能漏段。
新的 cursor 只推进到 owner 已验证扫描完的前缀，不能推进到“打算扫描”的位置。

可靠提交不依赖事件接收方在线。提交后通知投影或缓冲失败，允许该读侧失效并要求重新取依据，
但不能把已提交事实回退、伪报写入未发生，或让后续观察静默越过缺失段。
持久事实／回执与派生通知不是两套写事务：不新增可以自行决定事实的 outbox owner。

最小实现采用每个 preparation/观察自身的有限失效队列，pending 与 active 共用一份，不建立持久消息库。
当前 owner 的提交顺序同时序列化登记、相关性判定、投影和失败标记；同一个已提交 group 要么完整入队，
要么该观察被标记 GAP。禁止“发布最新 cut，随后尚未标记投影缺口就允许 Poll 跳过去”的窗口。
失效标记不要求追加第二次事实提交；异常退出直接使整次运行的 token 失效。重启不重放公共队列，旧流 GAP。
无关提交既不入队也不消耗该观察的保留额度，不能用全库活动造成空通知或可推算的序号变化。
通知回收不删除原 durable journal；有限观察也不要求把整个 journal 重放成公共流。

旧范围在转移下结束时，观察失效遵循[P7 总合同](owner-transition-contract.md)的终末证据条款：
该 owner 自己的转移决定先 fence 后仍未 settle 的丢失不是最终 GAP 证据，队列保持待定，既不流入
普通变化也不立即 GAP；决定 settle 后的下一次提交披露恰好一个独占 `terminal` 组，绑定该决定的
operation、source/target、final cut、settle 状态与指定联系，同一 cursor 幂等可重读，耗尽保留窗口后
明确 GAP。决定最终未完成（授权失效、TTL 到期、上下文回收或实例重启）以及任何不可证明的丢失
（父侧 gate 关闭、外部 carrier、他 owner 决定或混合丢失）仍立即或经窗口丢失后 GAP，不披露部分证据。
终末组不绑定任何 manifest 内容，也不授予对后继 Core 的访问。

### 6.3 覆盖新出现的结果

空间观察至少跟踪所选范围的成员／结构变化和全部坐标依据，不能只订阅首次命中的 UUID 或安全剪枝后的当前候选。
首个实现可以保守观察整个已授权 handle boundary：其中变化触发重新 QueryRegion，
允许多触发，不能漏掉新建、移入、父 Anchor 移动或 Range 改变导致的后来命中。
相关性在每次提交的 before/after 结构上判断，不能等 Poll 才用最新成员表判断历史变化；
移出 closure 的那次提交可以使原结果失效，之后仅在外部发生的修改不能继续进入原队列。
Query/List 准备只投影允许披露的空间变化，不因保守观察整个 boundary 而暴露 Facet 控制活动。
ReadElement 准备可包含该读取原本允许披露的 attachment envelope 变化，Facet payload 仍不进入 Core 通知。

BeginObserve 的 selector 与该语义范围绑定。深度等查询条件改变后先重新查询，再建立新观察，
不向旧查询的排序链补入元素。Observe 的 cursor 不是 QueryContinuation。
PARTIAL 的初始查询可以建立观察，但缺口不能因为之后收到几个变化而被标成完整；必须重新查询证明闭合。

### 6.4 权限、寿命与失败

每批披露前重新验证 caller、相关授权 view 与必要关系，不能用旧 Allow 授权历史内容。
相关 view 改变后旧观察终止；恢复授权时从新的读取依据开始，不能沿旧 cursor 补看失权期间的事实。
原本不在披露范围内的记录、owner、数量及变化频率不进入公共通知。
token 使用固定尺寸不透明表示；不能原样公开内部提交序号或以 token 长度／空心跳数量表达隐藏活动。
空轮询也不能凭 cursor 字节是否改变推导未授权 commit 数量；这是逻辑披露约束，不承诺常数时间网络。

观察具有独立的有限 cursor／投影保留策略，不与 SnapshotRetention 的开关或寿命绑在一起。
**关闭查询快照，不等于关闭 Observe；观察通知不是跨请求保存整份旧空间视图。**
保留不足、上下文过期、投影丢段或发行实例重启导致不能衔接，明确 GAP；GAP 要求新读取依据，
不伪装成空批次，也不偷偷改读最新。普通暂时失联且尚不能确认丢段时是 UNAVAILABLE，保留原 cursor，
恢复后再由 owner 判定能否续接。执行容量／计算限额不足是 RESOURCE_EXHAUSTED，不假称历史已丢。

ReleaseObserve 只释放运行内观察资源；到期可安全回收，不删除 Core／Facet 当前事实、回执或 Store 历史。
慢消费者不能阻塞提交，也不能要求无限保留。观察容量和期限必须明确、有界、可测试；不沿用十秒假定。

### 6.5 调用字段、资源与错误

| 调用 | 必需字段 | 返回／重试 |
|---|---|---|
| BeginObserve | caller、preparation | cursor；同 preparation 幂等激活，不产生另一份队列 |
| PollChanges | caller、cursor、limit | 完整 groups、nextCursor；limit 是 group 数，默认 64、最大 256；无变化返回空批与同一 cursor |
| ReleaseObserve | caller、preparation 或 cursor | 同 caller 可重复释放；释放整个观察及其派生 token，不撤销写入 |

每个 group 的公共字段是 `changeId`、`owner`、`category`、`basis`；category 是本次允许披露的
`spatial`、`content`、`attachment` 或 `contact` 类别集合。basis 是该提交的 opaque 依据，不是
可用于重新建流的 preparation，也不承诺该提交的历史 payload 可读。一个 commit 的类别合并为一组。
ChangeID 在同 owner 运行及保留范围内稳定；cursor 按 role/caller/view 隔离，不把提交序号编码在公开 ID 中。

启用公共 profile 的 Core/Facet 启动策略 `PublicAccessPolicy` 固定以下值；省略该策略时原本地 owner 操作
仍可用，公共上下文发行返回 UNAVAILABLE（未配置），不是偷偷启用默认 Observe 或 SnapshotRetention。
完整 P5 的验收 profile 必须显式配置两侧；策略复制后本次运行不热修改。

| 配置 | 规则 |
|---|---|
| ContextTTL | 必填正 duration；无默认秒数；handle/普通 read receipt 从发行时计时 |
| ObservationTTL | 必填正 duration；无默认秒数；从 preparation 登记开始，Begin/Poll/重试不续期 |
| MaxContexts | 默认 128；限制 owner 的 handle、普通 read receipt、分页引用与 preparation/观察根上下文总数 |
| MaxContextsPerCaller | 默认 32，不能大于 MaxContexts；服务身份同样计数，不豁免 |
| MaxGroupsPerObservation | 默认 1,024；未完成及已激活观察共用该队列额度 |
| MaxPublicBytes | 默认 64 MiB；保守核算派生上下文、队列、token 索引及返回批次保留占用；不只算 payload |
| MaxConcurrentPublicReads | 默认 64；同时执行的公共读/轮询/准备有界，排队也须可取消 |

计数/字节字段省略才选择表中有限工程默认，显式零或负数非法；时长不从这些默认值推导。
P3 SnapshotRetention/快照容量仍是另一份配置；普通 read receipt 不保存历史空间，不能替代 snapshot。
活跃 handle 的后续 receipt/观察不超过其剩余寿命；Facet 服务侧依赖到期会使组合流 GAP，不能自动补长。
到期限不继续交付，新提交正常持久化；尚未消费的队列满使该观察 GAP、释放队列并在原 token 有效期内保留终止标记。
新上下文分配或执行容量不足是 RESOURCE_EXHAUSTED；内存压力若必须丢弃已建观察，则明确使其 GAP。
同 cursor 的重复 Poll 可重交保留 group；不因首次交付就丢弃其唯一副本。调用方使用 nextCursor 后，
owner 可以回收已确认消费的前缀；再用指向已回收前缀的旧 cursor 明确 GAP，不影响仍连续的新 cursor。
超过计数/字节/时间上限时也明确 GAP，不承诺无限重试历史。慢消费者只影响自己的队列，
不等待消费者确认后才允许事实提交；并行重复请求的回收判断须在同一观察锁下完成。

| 情形 | 公共结果 |
|---|---|
| 字段、token 编码/角色、limit 或条件非法 | INVALID；不改变游标或事实 |
| 当前 caller/权限明确不成立 | DENIED；不附资源存在性或旧通知 |
| 授权代次改变后已重新允许、原 preparation/观察丢失或期限到达 | GAP，要求新读取；不重新解释旧 cursor |
| 普通 handle/receipt 丢失 | INVALID_CONTEXT，重新 Enter/Read；不把普通 context 错误说成空间不存在 |
| 已确认且获授权的事实不存在 | NOT_FOUND；只有 owner 的当前证据可给出 |
| 对端暂时断连、当前授权/关系无法证实 | UNAVAILABLE；不推进 cursor，恢复后重新判能否续接 |
| 当前 execution/新 context 资源不足 | RESOURCE_EXHAUSTED；不是读不到历史的借口，也不伪报 COMPLETE |

这是公共错误分类，不重命名各 owner 的内部错误或合并 CAS CONFLICT/写入 INDETERMINATE。
错误映射在实际读取/提交边界完成；错误格式不得附私有 token、Store 键或隐藏来源数量。

### 6.6 Core 与 Facet 的组合

组合协调由 **Facet owner** 负责，Composition 仅适配端口。P4 ContentRead 中的 Core evidence 是固定
Facet 服务身份取得的控制证据，**不是客户端的 Core read receipt**；不能转成客户端 Core cursor，
也不要求仅有 Facet 内容权限的客户端额外获得整个 Core 的观察权。

Core 向该固定服务身份提供最小控制端口 `ReadAttachmentWithPreparation(nextHop, AcceptanceKey)`，
原 `ucupa.core.element.facet-attachment-read` 与新增 `ucupa.core.element.facet-attachment-observe`
均需在 exact CoreElementScope 明确授予。该端口按第 6.2 节原子取得当前控制证据及私有 preparation，
只跟踪该 Element 的存在性、exact attachment/key、联系修订，不披露空间 payload 或其他元素。
`BeginAttachmentObserve / PollAttachmentChanges / ReleaseAttachmentObserve` 的句柄只供该服务身份使用；
服务授权失效与客户端授权失效分别判断，服务身份不是全空间特权协调者。

Facet 的 prepared Get/ListTypes 固定以下顺序，不构造跨 Store 事务：

1. 取得客户端的 Facet read + 对应 Observe 联合 view；在 Facet 锁下捕获 F0、AcceptanceKey、反向联系
   修订，并登记 pending 本地准备，防止等待 Core 时漏掉 Facet 提交。
2. 释放 Facet 锁，按该份指定反向地址，以固定服务身份取得 Core 当前控制证据 C0 和控制 preparation。
   不持有 Facet 提交锁等待另一个 owner；Core 使用它自己的 view 和锁，不共享客户端授权。
3. 重新锁定 Facet，验证 F0 与原 acceptance/contact 未变，Core 证据仍适用于 exact key，
   两侧准备未丢段/过期，当前客户端 view 成立；失败清理两侧未发行准备并明确重读或不可用。
4. 成功才发行 Facet preparation，私下绑定 F0、本地队列及 Core 控制依赖。客户端只收到自己的
   Facet token，不收到服务 view/cursor。Begin 激活两侧依赖可幂等重试；一侧失败不能发行半个成功组合。

这两个依据是可核验的因果依赖，不声称 C0 与 F0 同时取得。Core 的每次后续 delete/detach/contact
变化均在内部队列中，不必等待 Facet 再写入才能发现。Poll 先核对该依赖的连续性和当前 exact 关系，
再披露 Facet 内容通知；已确定关系失效则停止组合流，服务依赖 GAP 则组合 GAP，暂时不可达则 UNAVAILABLE。
没有后台轮询时，“发现变化”发生在下一次 Poll，不承诺推送时限；它不能返回空批并谎称依赖仍被证明有效。

每次读取 Facet 通知前继续核验当前 exact attachment，不经验证交付旧 namespace 内容或其详情。
新附着、启用新类型或新增可披露 Facet 来源时，先取得该来源的合法 read basis，再建立自己的 cursor；
不把它当作从旧 Core cut 就已观察。无法证明衔接则 GAP／PARTIAL，必要根或 exact attachment 不成立则 UNAVAILABLE。
地址变化不改变事实身份，但旧联系不能被默默替换后继续；按新联系重验，无法接续就重新建立依据。

服务控制变化只驱动重验或组合失效，不伪造一个 Facet 内容 commit。客户端若另有 Core 读取/观察权，
可建立自己的 Core 流，与 Facet 流分别消费；这种客户端可见组合只包含其被允许知道的 owner/cursor vector。
没有 Core 观察权时，不公开服务控制流的序号、事件、view 或 cursor，也不因 Facet token 有效而借权。
释放、超时或客户端撤权时，Facet 释放自身与内部 Core 派生依赖；对端失联时由对端绝对期限兜底回收。
没有跨 Core／Facet 的统一顺序或原子起点。
非联邦多来源范围也必须验证这些规则；单独跑通 Core Observe 不等于完整 P5 已完成。

## 7. Propose：显式调用 owner，不新增事务层

沿已验证的入口或附着联系把请求交给实际 owner；保留 owner-specific 的 final tuple／payload、
CAS、精确 OperationID 和原始回执。入口授权、事实约束和最终提交仍由对应 owner 执行。

- 状态改变经校验并可靠持久化后才确认；不延迟保存来制造查询一致性。
- 丢响应不是可证明拒绝；同 actor、同 OperationID、同规范请求重试，不随机换 ID 再写一次。
- INVALID、DENIED、CONFLICT、UNAVAILABLE、INDETERMINATE 与已确认 Committed／Unchanged 分开。
- 联系刷新和身份建立不自动重试非幂等业务，不修改原始前置条件，不把冲突回执“修成”新请求。
- Core、Facet 的多个提交各自报告，不能提供一个假装跨 Store 全有或全无的 Propose。
- Observe 只反映实际 commit；收到通知不代替原始 mutation 回执，未收到通知也不证明写入失败。

### closure 写入与删除后的回执

新的公共 Core 命令在既有 operation binding 中记录 `AccessBinding={mode,boundary?}`：mode 为 exact
或 closure；closure 的 boundary 为原 BoundaryAnchorRef。它与原 actor、operation kind、规范输入、
前置条件及 outcome 一起原子持久化，不另建操作库。具体权限与目标由 operation kind/输入确定性派生，
不保存第二张授权表、祖先树、旧授权 view 或凭据。首次受理新命令必须有有效 handle、当前授权与同 cut 成员证明。

首次通过授权与成员校验时，在同一 operation 中记录 AdmissionCut，表示开始裁决前的 owner cut。
Store 校验对照该 cut 的历史成员关系：Committed 的前态须匹配 AdmissionCut，Unchanged/已记录 Rejected
的决定态须匹配 AdmissionCut；值必须与唯一提交历史交叉核对，不能让一个自报标记代替证明。
纯授权/成员范围拒绝不形成可绕过授权的持久凭据；事实裁决产生的可保留结果仍按原规则保存。

同 OperationID 的精确重试先识别原绑定，再按当前相同 scope 的具体权限核对回执访问。
不能先要求“删除后的对象仍在当前 closure”才查回执，否则已成功删除且丢响应的请求将永远不能收敛。
当前 scope 授权只准披露该 actor 自身的原操作结果，不授权重放历史数据的一般查询。

为 handle 失效/重启甚至原 boundary 已删除的情形，提供纯读取 `RecoverOutcome(reference, caller,
operationId, exactOriginalRequest)`：只按 reference 指定地址核对原 Core 身份，检查当前原 scope 权限，
返回同 actor、同 AccessBinding、同规范请求的既有回执。它**永不执行找不到的新命令**，不建立新 handle，
不恢复 retired Anchor。新凭据建立的同 Principal 可以恢复；旧 credential/view 不成为永久依赖。
没有该 actor 的可披露回执返回 OUTCOME_UNKNOWN，而不是元素 NOT_FOUND；同 actor 请求或 boundary 不一致
返回 CONFLICT，不附原命令详情；其他 actor 的结果不得披露。原权限撤销时 DENIED，不能凭 OperationID 借权。

这项原记录扩展直接采用唯一现行编码并补历史校验/重启测试，不新增旧记录解释器、旧 scope 兼容入口。
Facet 现有 exact key/namespace 重放顺序保持 P4 规则，不因公共类型或观察新增解析/当前存在性前置。

## 8. 公共值、不透明内容与编码收口

以下固定 P5 本地公共 profile 的字段与验证规则，不再把格式选择留作 TBD。机器 schema 与
conformance 文件在实际实现时归所属 `api/`；实际需要的生成绑定另按工程规范归位。
这些产物只落实本文，不另造版本路径、旧 reader 或备用格式，也不提前建立空目录。

### 8.1 值与 evidence

公共请求／响应 envelope 是无 BOM 的 UTF-8 [JCS / RFC 8785](https://www.rfc-editor.org/rfc/rfc8785)
字节。接收方先严格解析，再验证与重新 JCS 编码的字节完全一致；拒绝重复或未知 envelope 字段、
非法 Unicode、非有限数值及非规范输入。JCS 不做 Unicode normalization；普通 `json.Marshal` 不代证合规。
空间值继续使用已验收的 UUID、有限 binary64/projective-quaternion 与显式 root Range；消息解析不替 owner
归一化或裁决。字段缺省不同于 null；root 不填 Parent/LocalPose，不以零 UUID 或负 radius 代替语义变体。

| 公共值 | 固定表示 |
|---|---|
| UUID、FactAuthorityRef、RealmID | 沿既有已确认的精确身份文本，不由地址、名称或 envelope 自报信息推导信任 |
| 二进制字段、`payload` | [RFC 4648 §5](https://www.rfc-editor.org/rfc/rfc4648#section-5) base64url 字符串，仅 `A-Z a-z 0-9 - _`；无 padding/空白，末尾未用位为零，解码后重编码必须逐字相同 |
| OperationID | 将既有 1–128 bytes 精确编码为上述 base64url，不把任意 Go string 字节强行改成 Unicode 文本 |
| UpDepth、地址 up、ActualUpDepth | 规范十进制字符串；无正号/前导零/负零，UpDepth 允许 -1 或 int64 非负值，其余为 uint64 非负值；不经过 JSON number 舍入 |
| limit、资源计数与其他已声明小整数 | JSON 非负安全整数且满足各字段上限；不得将身份/Revision/cut 改编码为数字 |
| `typeRef` | 任意不透明 32-byte 值，解码恰好 32 bytes；公开为上述无 padding 的 43 字符串，不加地址或可变名称；Facet 不解释其内容或来源 |
| 公共 token | 恰好 `{owner,kind,value}`；owner 是发行者的已确认身份，kind 是该字段的精确用途，value 为上述编码的 32-byte 不透明值 |

token 的 kind 分别固定为 `space-handle`、`record-revision`、`attachment-revision`、`contact-revision`、
`read-receipt`、`query-continuation`、`observation-preparation`、`observation-cursor`、`change-id`、
`change-basis`；授权端口的 `authorization-view` 只供受信端口，不直接附在客户端响应里。
使用点必须核对 owner、kind、发行上下文；私有 Store scope、cut serial、binding 代次及操作历史均不发布，
也不原样发布现有 CoreCutEvidence.Bytes。TypeRef 是内容键的一部分而非运行 token，不随 caller 或重启改变。

普通 read receipt 是随机 32-byte、caller/view-bound 的运行内引用；owner 私下绑定真实 cut 与读取条件。
同一读链复用，独立读取即使取得同一 cut 也重新发行，不能由引用变化推算闭包外提交。其他运行 token
同样按角色隔离；第 6.5 节统一限制这些上下文的数量、字节与寿命，不新增 TTL 注册表或默认秒数。
receipt 不保留另一份空间事实；过期/丢失按第 6.5 节 INVALID_CONTEXT 或 GAP，重新读取，不能解码序号续用。

公开 Revision 的 value 固定为 HMAC-SHA-256：owner 使用持久随机 32-byte 私钥，对 owner、kind 与原私有
Revision 的无歧义长度分隔编码作域分离认证。该密钥是 owner Store 的内部编码基础，不是新身份或授权；
随 Store 同生命周期保存，重启不能重生，相同事实 Revision 的公开等值/CAS 语义保持不变。
该持久密钥缺失或损坏按 Store 完整性错误处理，不能通过自动生成另一把密钥伪装成功恢复。
CAS 只计算当前候选 Revision 的 tag 再比较，不反解 MAC、不扫描全历史。原 expected tag 原样进入既有
OperationID binding 的规范请求；先识别第 7 节 exact replay，再做 current CAS，不改写成当前 tag。
运行内 read receipt 不承担可持久重放的 expected Revision 职责；RecoverOutcome 仍是第 7 节纯回执读取。
本地地址只携带指定 NextHop；不增加网络认证、TLS 或已验证远端身份的暗示。

### 8.2 不透明 Payload 与容量

Facet 对 `(Element, TypeRef)` 保存和返回有界、逐字节不变的不透明 Payload；它不获取、
解析类型制品，不执行 schema 校验，也不把 payload 语义（JSON、文本或其他）作为接受条件。
payload 的字节内容仅在保存与返回之间保持逐字节一致：调用者放什么字节，Get 就返回什么字节，
不做 Unicode 替换、规范化、重排版或补默认。公共 envelope 仍是 8.1 的严格 JCS 解码，
payload 字段本身按 base64url 精确解码；同一操作重试与原结果恢复不重解释字节。

容量为固定硬上限，越界按公共错误表处理，不产生内容提交：

| 有限资源 | 硬上限 |
|---|---|
| 单个 payload 解码后字节数 | 1,048,576 bytes（1 MiB） |
| 每个 Element 的当前 TypeRef 数 | 256 |

达到 256 上限后的新 TypeRef Put 与超限 payload Put 均整体失败；已存在 key 的覆盖写
受 payload 上限约束但不新增 TypeRef 计数。ListTypes 按原始字节完整排序返回当前集合。

### 8.3 持久化与恢复

内容事实、tombstones、cut、公开 Revision 密钥与操作回执同事务持久化；不存在 per-Type
pin、artifact 镜像或 witness 记录。删除某个 key 即按原 owner 规则处理；没有 orphan
pin 概念，也不因保留历史而披露已删除内容。重启从唯一 Store 恢复，重验历史后即可服务
Get/Delete/精确重放，无需外部类型源在线；不存在镜像完整性故障这一类别。
Store 格式标识由 Facet 唯一定义；旧格式 Store 严格拒绝打开，不做兼容读取或转换。

### 8.4 错误与独立验证

| 情形 | 必须结果 |
|---|---|
| 请求编码、TypeRef 长度或公共结构硬限额不成立 | INVALID；零内容提交，拒绝回执继续按原 owner 规则 |
| 无当前 ListTypes/Get 权限，或旧 view 已失效 | 按第 4/6.5 节 DENIED 或上下文失效；不先确认 key 存在 |
| 当前获权 namespace/key 不含该 TypeRef | NOT_FOUND；不因别处 Element 保留同 TypeRef 而披露 |
| payload 超过 1 MiB，或该 Element 当前类型已达 256 上限 | RESOURCE_EXHAUSTED；零内容提交，不影响既有 key |
| 执行预算/公共容量耗尽 | RESOURCE_EXHAUSTED；不产生内容提交 |
| Store 完整性故障 | 公共面 UNAVAILABLE；不附私有路径/bytes、不自动修复或继续带损坏状态服务 |

独立 corpus 必须覆盖：JCS 数字/符号零/Unicode 排序与重复键；base64url padding/未用位/截断；token 跨
caller/owner/kind 与重启；Revision 重启稳定及旧 expected tag 精确重试；未预装业务代码的客户端
ListTypes → exact Get 不透明往返；namespace/key 越权、撤权、detach 与 F0 并发；
非 JSON payload 字节、1 MiB/256 临界值与超限、所有资源临界值；首次 Put 提交前/后故障及
重启后仍从完整 Store 恢复 Get/Delete/精确重放；Store 损坏必须阻止错误加载；
公共 envelope 与 base64url 编码的规范性不证明 Payload 符合任何业务格式；Facet 不做这种格式判断。
实际实现直接更新受影响的记录/历史校验、测试和例子，不增加旧 TypeRef/witness/public 格式
兼容读者、映射目录或转换服务；以上为已接受合同的实现约束，不把现有 P4 opaque 基础宣称成已完成公共 profile。

## 9. 失败导向验收

以下是实现必须满足的验收预期；实际测试路径、执行结果和交付状态见开发计划第 8 节的 P5 实现交付记录，不能仅以本表存在代证通过。

| 反例 | 必须观察到的结果 |
|---|---|
| reference 自报 Principal、Realm 或另一个 Core | 不能替换可信装配；无授权不披露存在性 |
| 无地址／指定地址接错对象 | 失败，不按 Ref 扫描，不自动换 owner |
| Enter 后新建从未预授权 UUID 的 child | 合法闭包 Discover 可发现；不是预注入 ID 的演示 |
| UpDepth=-1、Basis／路径试图越过发布边界 | 到已证明边界停止或 INVALID，不泄露外部父链 |
| boundary R 的兄弟 A/B；Start=A、UpDepth=0、Basis=B | INVALID；不能把“在 boundary 内”误解成放宽原 Basis∈Scope |
| 边界或目标 reparent，复用旧路径 | 逐边复核，不通过裸 UUID 偷补寻址 |
| 只有 Discover，没有空间 Read／Facet 权限 | 只能取得该行为可披露内容；无写权限或隐式 Facet 许可 |
| 只有 create closure 权限，或只有 update 没有 place | 前者可按 create intent 进入并创建但不能读；后者即使两端在闭包内也 DENIED |
| grant 删除后重授，前后 bool 都为 true | 旧 view／cursor 失效；不补看失权期间历史 |
| Read 始终有权，但 Observe 在首次 Begin 前撤销/重授 | preparation 的联合 view 失效，不能用 Begin 当时的新 Allow 补发历史 |
| 普通 Read 没准备，拿旧 cut 开始观察 | 明确要求新 prepared Read；不现场补造旧时刻的 Observe 资格 |
| 初始读 C 与 BeginObserve 之间已有 C+1 commit | 第一次 Poll 不漏 C+1，或明确 GAP，不从 latest 开始 |
| 捕获 C、登记 preparation、提交 C+1 的所有交错 | 不出现已捕获 C 却漏登记 C+1 的窗口；失败不留下半准备 |
| 原查询无命中，之后新建／移入／父锚移动／Range 变大 | 有相关失效通知，重查可以发现新结果 |
| ancestor 移动但 EntityRevision 不变 | 空间观察仍失效；不把 unchanged EntityRevision 当世界投影不变 |
| 同一 commit 的空间与 attachment 副作用 | 一个 group；消费位置不能落在中间 |
| Facet 历史多一个 rejected／unchanged／重放操作 | 没有新事实通知或假推进的事实顺序 |
| 收到批次后响应丢失，再用旧 cursor 请求 | 可重复稳定 ChangeID，不能漏下一段 |
| durable commit 后派生通知丢失 | 原写入事实保留；观察 GAP，不静默继续 |
| 通知入队或标记失败，期间并发 Poll | 只交付已验证连续前缀或 GAP，不能越过缺口 |
| 快照关闭／开启，分别执行 read→observe | 两者都能按观察保留规则衔接，不偷开快照 |
| 慢消费者、期限到期、释放、重启 | 旧上下文明确 GAP／失效，当前事实和回执不丢 |
| 使用 nextCursor 后回收已消费前缀，再用原 cursor | 旧 cursor GAP，仍连续的新 cursor 可用；无变化不产生另一串 cursor |
| 未配置 PublicAccessPolicy、TTL 缺失/非法、或资源耗尽 | 公共面明确不可用/INVALID/资源失败，原普通提交不受影响，快照不暗中启用 |
| 仅暂时断连，是否丢段未知 | UNAVAILABLE，恢复后再判续接或 GAP，不 advance cursor |
| 扫描工作不足但历史仍在 | RESOURCE_EXHAUSTED，不伪称数据缺口或 COMPLETE |
| Core detach／delete，Facet 没有新写入 | 组合观察仍停止／重验，旧 namespace 不继续披露 |
| caller 只有 Facet 权限，Core 仅授权固定 Facet 服务身份 | 合法 Facet 观察可用，但服务 Core receipt/view/cursor 不被转交或借权 |
| Facet F0 后等待 Core 时内容/联系改变；两侧准备仅一侧成功 | prepared Read 失败并清理/到期回收，不交付半个组合或伪造共同 cut |
| 新增 Facet 来源或授权新增来源 | 新 read basis + 独立 cursor，不沿旧隐藏位置泄露历史 |
| 相关分支受限／不支持 mount | 保留 PARTIAL／UNAVAILABLE，不能报完整空集 |
| 隐藏／边界外事实持续变化 | 没有公开详情、可推算序号或按隐藏变化产生的通知频率 |
| 极值数值、畸形 token、跨角色 token | 确定拒绝或数值缺口，不溢出、不静默归一成另一个请求 |
| 陌生 TypeRef 与任意非 JSON payload 字节 | 按授权保存并逐字节返回；不要求预装映射、不解析内容、不因未识别字节拒绝 |
| payload 超 1 MiB 或该 Element 第 257 个类型 | RESOURCE_EXHAUSTED，零内容提交，既有 key 不受影响 |
| 同一 payload 重放、重启后读取 | 每次返回原始字节；无解析、规范化或重新编码 |
| Store 或公共 Revision 密钥损坏 | Store 完整性失败，不自动修复、重新生成或把损坏当暂时类型不可用 |
| 删除成功但响应丢失，随后重启或删除原 boundary | RecoverOutcome 只恢复自身原回执；新命令不执行、事实不二次改变 |
| 换 boundary、换 actor 或改规范请求恢复同一 OperationID | 不重解释原 AccessBinding，不泄露其他 actor 回执 |
| 清空派生 handle／连接／观察缓存 | 能重新接入、读取和建立观察，不靠客户端世界副本恢复真相 |

## 10. 实现交付门与收口边界

复审缺口及普通工程取舍已收为具体规则，用户已接受本合同；本次实现、完整验证及授权的交付验收已完成。
下面是本轮已落实、后续维护不得降低的产物与证明，不是另一次开放式规划，不要求使用者选择库、字段名或目录：

1. 按第 3–8 节的字段/编码实际产生所属模块的公共 schema 和独立消费样例，
   验证极值、身份/role 隔离、无历史兼容格式；不能只补 Go 别名或 Store JSON 转发。
2. 实现 exact/closure AccessBinding、具体 PermissionID 组合、Realm view 取得/验证及决定历史核对；
   验证 write-only 入口、删除后回执、撤权/重授和重启，不复制 Grant、不实现 Stage 2 联系记录。
3. 实现 prepared Read、原地 Begin、有限队列及 GAP 失效规则，验证第 9 节交错和资源边界；
   Core/F0 各保留独立 cut，Facet 拥有内部控制观察组合，Composition 不接管其状态。
4. 按第 8 节实现不透明 Payload 的保存、返回、容量限制与恢复，
   验证正反例/逐字节不变性/独立解码；某个库的默认行为不是合同或互操作证据。

源码归位遵守[工程规范](../../ENGINEERING.md)：Core 的 entry／closure／observe、Facet 的 observe／内容
按职责分文件，各 owner 保留唯一状态和裁决；Facet 同时拥有私有 Core 控制观察依赖及组合生命周期；
Realm 保留身份与授权所有权；Composition 只接线，不保存第二份组合事实或替 owner 做权限判断。
只有真实公共值进入 shared；schema／conformance 在所属 `api/`，不提前新建 SDK、transport 或 bindings 空目录。
跨 owner 反例在 acceptance，私有提交故障在所属模块，普通消费者不得接触 Store 或私有历史。

P5 局部实现必须标明覆盖范围；只有五类公共行为、开放类型本地验证及声明的 Core／Facet 多来源观察
全部通过对应 gate，才可交付完整 P5。用户确认设计、授权实现、技术验证、验收和提交分别记录。
合同收口及本次设计接受不代替实现交付与验证；代码开工和提交授权单独记录。
本文不授权远端服务、下游业务、后续 Phase 或自动提交。
