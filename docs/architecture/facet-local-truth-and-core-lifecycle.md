# UCUPA 本地 Core／Facet 真相闭合实现合同

**状态：P4 已完成本地实现、完整验证，并按用户“验收提交”的授权完成交付验收；本文现为本地 Core／Facet 增量实现合同，精确 current 范围由架构权威清单维护，交付与提交记录见开发计划。**

**P5 增量 notice：** 已验收的[公共接入与事实观察合同](public-access-and-observation.md)直接接替本文的
TypeRef／resolver／类型校验及其镜像、公共 Revision 密钥、原请求绑定对应的持久化条款，并增加公共访问与观察。
**P7.3a 增量 notice：** [公共 profile 第 8 节](public-access-and-observation.md)已把类型语义修订为任意
32-byte 不透明 TypeRef 与有界、逐字节不变的不透明 Payload；Facet 不获取、解析类型制品或执行 schema 校验，
pin／mirror／witness 条款整体移除。第 3.2、8、9.3 节已同步唯一现行类型和持久化形态；
其余 P4 附着、联系、空间/内容归属及独立提交规则继续有效。不保留旧类型读者或两套公共接口。

**P7／Stage 2 successor notice：** [P7 总合同第 8–10 节](owner-transition-contract.md#8-facet-namespace-replacement-与重新接纳)
已接替本文不支持 namespace replacement、同 Facet 重新接纳、撤销关系恢复及退役的限制，
只限已交付的首批 profile；F0 和唯一内容提交域保持。
[RealmAttachment 合同](realm-attachment-contract.md)接替本地授权依赖装配限制，联系由各 owner
各自持久保存，本地与网络均逐次独立授权。精确 current 范围见权威清单，不追改 P4 的历史交付。

本文把主线 D5 的最小 exact Facet truth、D6 无法后移的逐 Element FacetAttachment，以及 D7 对这些事实的
authority-qualified scope 投影，收敛为一个可直接实现和反证的本地切片。它不建立公共
Enter、Discover、远端 trust、mount federation、authority transition 或生产部署。

文中 G1 仅为已验收 Core 本地合同的历史简称，不是当前调度编号；执行安排只见[开发计划](development-plan.md)。

本次直接以“关系指定地址＋稳定身份核对”替换旧 Ref 解析条款，不保留两套联系机制或兼容接口。
地址、反向核验、重试及持久化的对齐不改变当时的 payload 与独立提交边界；
具体新增接口与下述并发边界已随 P4 完成实现、验证和交付验收，不追认为旧合同早已交付的能力。
Core self、Core owner-qualified scope 与共用提交基础已随 Phase 1 交付，本文复用而不重新实现；
用户已在 P3 交付后明确要求开始 P4，随后授权验收并提交；Facet 新附着／类型闭环已按本合同收敛落实。
[架构权威清单](README.md)与受影响合同 notice 在本次 P4 提交中同步切换，范围仅限本文列出的增量。

本文是精确增量，不重新抄写 G1：G1 的 D1–D4 `ElementRecord`、坐标、Range、数值、Structure、公开
create／read／spatial mutation／delete、ElementRevision、`ListDirect`、错误顺序、read-cut 与 geometry oracle
不变量全部继续约束本合同。本文只接替或扩充被新增控制事实必然改变的 clause families：

- Core self owner/ref profile 与 exact-read envelope；
- Element create／delete 的 attachment 原子副作用，以及 ElementRevision 与 attachment-only commit 的关系；
- attachment mutation、permission、convergence 与 Facet 所用的 typed evidence port；
- Core private Store／commit shape、FormatID 与对应持久校验。

未被逐项点名的 G1 行为不得改变。Facet predecessor 的自由字符串类型与 lifecycle 形态由本文整体接替；Realm
只替换第 7 节列出的 scope profile／codec。

## 1. 本合同唯一允许声明的结果

```text
Core-only Element
  = spatial record + FacetAttachment.NONE
  = 完整合法、可持久化且不依赖 Facet 的 UCUPA Element

Optional Facet
  = per-Element proposal -> acceptance -> ACTIVE attachment
  + exact typed records guarded by PRESENT + ACTIVE(self)

Local composition
  = 按关系指定 NextHop 联系受限 typed port，再核对预期 FactAuthorityRef
  != 裸 Ref 解析器、全局目录或 authority-wide 固定 peer
```

本合同必须同时证明：

- Core 与 Facet 是两个独立事实 owner，各自拥有 stable FactAuthorityRef、Store、ReadCut 和 commit domain；
- Core 保存每个 Element 的 FacetAttachment；有效附着的联系信息包括目标 Facet 身份与指定地址，
  实际 typed payload 仍只保存在 Facet，不复制进 attachment；
- Facet 可以完全不部署，Core 的普通行为不因此失败；
- 启用 Facet 时，只有 matching proposal／acceptance 后的 `ACTIVE(self)` owner 可以服务；
- 一个 Core authority 可以把不同 Element attach 到不同 Facet authorities，一个 Facet authority 可以接受来自
  不同 Core authorities 的 Element；
- Realm 只机械承载已经由 D7 冻结的 authority-qualified scope；这不增加 Realm 领域语义或事实 ownership；
- Element 空间记录与 Facet payload 不保存 Endpoint、Cluster、Store path 或 authority-wide peer pair。
  跨服务联系地址只属于精确附着关系的控制记录；没有附着的元素不预填地址。

## 2. 唯一 owner 边界

| 事实或控制事实 | 唯一 owner | 明确不拥有 |
|---|---|---|
| Element existence、Kind、Structure、Pose、Range、ElementRevision | Core | Facet payload、上层类型解释、权限、Route |
| 逐 Element FacetAttachment、pending proposal、AttachmentRevision、正向 NextHop／联系修订 | Core | Facet acceptance、Facet record、部署资源 |
| proposal-bound immutable acceptance | Facet | Core attachment 决定、Core existence |
| 按 AcceptanceKey 保存的反向 Core NextHop／联系修订 | Facet | Core existence、current owner 选择、原 acceptance 的重写 |
| `FacetKey -> 不透明 payload + FacetRevision`、FacetReadCut | Facet | Element 空间真相、业务类型含义、TypeRef 语义解释 |
| caller identity、PermissionBinding 与 Allow／Deny | Realm | Core／Facet 事实、Route |
| 按指定地址装配两个 owner 的本地 port | Composition | 权威联系记录、合成 cut、owner 选择 |

Composition 可以把 Core、Facet、Realm 放在同一进程，但不能因此合并 owner、Revision、Store 或事务。

## 3. 本地值 profile

### 3.1 `FactAuthorityRef`

```text
FactAuthorityRef = canonical lowercase UUIDv4，独立命名类型
```

- 与[Core 挂载合同](core-mount-contract.md#21-一个-core-的稳定身份不增加-authority-对象层)采用同一本地身份 profile，
  不保留原 256-bit FactAuthorityRef 分支；FacetTypeRef 的 32-byte 不透明值不受此项影响。
- 一个逻辑 Core／Facet 各有一个稳定 self Ref，不在服务内新增 Authority 对象层。
  New／CreateDurable 使用安全随机源生成一次并持久化；不接受自选旧 Ref，生成失败则创建失败。
  OpenDurable 只恢复原值；缺失、非法或前后读取不一致不能补生成。
- 本合同只使用 exact equality。Ref 不包含 kind、path、Realm、Cluster、Endpoint、credential、时间或排序语义。
- Core-kind 与 Facet-kind 由 typed owner port 和本地 authority evidence 区分，不制造两套底层 ID。
- Store 的自身身份只有一个 self Ref；对端身份只随精确挂载／附着关系保存，
  不设置 `LocalPeerCoreAuthorityRef`、`LocalPeerFacetAuthorityRef` 或全局固定 authority pair。
- 正常 reopen、换机器或地址不改变 self Ref；私有 token scope 独立，不能导出作 Ref。
  本地 profile 不冻结公共 wire 表示或认证证据；相同 Ref 不能证明没有可写克隆，writer handoff 不在本轮保证内。
- 持有 Ref bytes 不能证明 writer、permission 或 reachability；本合同信任明确注入的本地 typed port。

### 3.2 `FacetTypeRef` 与不透明 Payload

```text
FacetTypeRef = 任意不透明 exactly 32 bytes
FacetKey     = (ElementUUID, FacetTypeRef)

FacetAccessContext = AcceptanceKey {
  FacetAuthorityRef
  CoreAuthorityRef
  ElementUUID
  ProposalRevision
}
```

- Ref 只做 exact equality 与 raw-byte ordering；没有 displayName、slot、版本后缀、URL、registry 编号
  或摘要语义，不解释为地址、名称或版本。
- Go zero value／空 bytes 无效；constructor 与 accessor 都复制 bytes。
- [公共 profile 第 8 节](public-access-and-observation.md#8-公共值不透明内容与编码收口)（P7.3a 修订）直接接替旧的类型编码与验证入口。
  Facet 不获取、解析类型制品或执行 schema 校验；Put 按 (Element, TypeRef) 保存有界、逐字节不变的
  不透明 Payload，Get／ListTypes 只返回当前 key 的内容与类型集合。
- payload 上限保持 1 MiB；同一 Element 的 current FacetTypeRef 数量上限为 256。越界写在任何事实 mutation 前
  返回 `CAPACITY`。
- `FacetAccessContext` 是从 Core exact read 得到的关系 selector，不是持久 FacetKey、全局 locator
  或 caller 提供的 existence evidence。它仍只包含 AcceptanceKey，不把地址变成键或身份。
  调用方由该次 ACTIVE attachment 取得 FacetNextHop；Facet 核对 self 后，按 exact AcceptanceKey
  读取自己保留的 CoreContact，再经其指定地址 fresh-read 预期 Core，核对当前 attachment。
  CoreContact 只联系当初确认的 Core，不替 caller 推导或重选 current owner；不能仅凭 CoreRef 找端口。

## 4. Core 的唯一 exact snapshot

Core 的 exact read 在一个 CoreReadCut 上返回：

```text
CoreElementRead =
  PRESENT {
    CoreAuthorityRef
    CoreReadCut
    ElementRecord {
      UUID / Kind / Structure / Pose / Range / ElementRevision
    }
    FacetAttachment
  }
| NOT_FOUND {
    CoreAuthorityRef
    CoreReadCut
    ElementUUID
  }
```

不存在第二份 `exists` bool、lifecycle Store 或缓存。给 Facet 的最小披露 port 可以从同一 snapshot 投影
`CoreAuthorityRef + ElementUUID + existence + FacetAttachment + CoreReadCut`，但不得另存或独立演进。该 port 使用
独立于完整 Element Read 的明确 PermissionID，不向 Facet service Principal 披露 `ElementRecord`。

### 4.1 FacetAttachment

```text
FacetAttachment =
  NONE {
    AttachmentRevision
  }
| ACTIVE {
    AttachmentRevision
    FacetAuthorityRef
    ProposalRevision
    FacetNextHop
    NextHopRevision
  }
| DETACHED {
    AttachmentRevision
    FormerFacetAuthorityRef
    FormerProposalRevision
  }
```

在 Element 与 Core self Ref 的上下文中，`ACTIVE` 的目标身份与 ProposalRevision 唯一重建以下键；
地址及联系修订不参与键相等：

```text
AcceptanceKey =
  (FacetAuthorityRef, CoreAuthorityRef, ElementUUID, ProposalRevision)
```

Core 不复制 Facet owner 的 `AcceptedAt`；`DETACHED` 保留 former AcceptanceKey 与当前 Core cut，已经足以证明
“曾经 ACTIVE、现在 detached”。successor continuity evidence 属于 D12 转移后继合同，而不是本次 attachment 的新增字段。

- Element create 与初始 `NONE` 在同一个 Core commit 成立，不能观察到 PRESENT 但 attachment 未初始化。
- AttachmentRevision 是 `(CoreAuthorityRef, ElementUUID)` 下独立、不复用的 equality token。
- proposal、cancel、activate、detach 推进 AttachmentRevision 和 CoreReadCut，但不改变 ElementRevision。
- Structure、Pose 或 Range update 推进 ElementRevision 和 CoreReadCut，但不改变 AttachmentRevision、
  target 或 accepted proposal。
- delete 在同一个 Core commit 终止 existence 与 attachment 当前效力，但不跨 Store 删除 Facet bytes。
- ACTIVE attachment 返回所属关系的权威指定 FacetNextHop 与联系修订；
  不包含 payload、Cluster、Facet record existence、运行时连接缓存或额外 RouteRegistration。
- pending 与 ACTIVE 共用该精确关系的一份正向联系记录，Activate 不从旧 Propose 回执复建地址；
  Cancel 清除当前 pending 联系，Detach／Delete 终止正向联系效力。历史地址回执不复活关系。

## 5. 最小 attachment 握手

### 5.1 proposal 与 acceptance

```text
FacetAttachmentProposal {
  CoreAuthorityRef
  ElementUUID
  TargetFacetAuthorityRef
  ProposalRevision       # propose commit 后的 AttachmentRevision
}

FacetAttachmentAcceptance {
  FacetAuthorityRef      # responding self
  CoreAuthorityRef
  ElementUUID
  ProposalRevision
  AcceptedAt             # FacetReadCut
}

Core FacetContact[AcceptanceKey] = FacetNextHop + NextHopRevision  # pending／ACTIVE
Facet CoreContact[AcceptanceKey] = CoreNextHop + NextHopRevision # 与 acceptance 独立维护

AcceptAttachment(exact proposal, InitialCoreNextHop, actor, OperationID)
```

Propose 显式接收目标 FacetRef 与初始 FacetNextHop，并在同一个 Core commit 保存 proposal 与正向联系。
首次 Accept 的有权调用者显式提供 InitialCoreNextHop；它是在 Facet 尚无关系记录时联系已知 Core 的启动地址，
不进入 AcceptanceKey。Facet 验证经该地址联系到预期 Core，并读取 exact current proposal 后，
在首次 acceptance 的同一提交中创建 CoreContact 与联系 token。地址缺失、错误或不可达不能假定 Core 不存在。

Acceptance 由 `AcceptanceKey` 唯一识别、immutable 且 durable retained；本合同不允许删除、覆盖或压缩一份已经提交的
acceptance。`AcceptedAt` 已提供 owner-local fact cut，不增加重复的 AcceptanceRevision。`ProposalRevision` 只是
propose commit 后 `AttachmentRevision` 的语义别名，不分配第二个 token；proposal creation 的 CoreReadCut 已由
Propose mutation result 返回，不在 proposal 中重复保存一个无证明作用的 `ProposedAt`。

重复 Accept 的收敛规则是：

- 第一次接受仍 current 的 exact proposal：`COMMITTED`，推进一次 FacetReadCut，并产生唯一 `AcceptedAt`；
- 已存在 exact AcceptanceKey：不同 OperationID 返回 `UNCHANGED` 与原 acceptance，不推进 cut；原 OperationID
  replay 返回原 `COMMITTED` 结果；
- acceptance 尚不存在，fresh-read 观察到 proposal 已 cancel／替换：`REJECTED`，零提交。
- 已读到合法 PENDING 的在途 Accept 可以在 Core 随后 Cancel 后才提交历史 acceptance；
  独立 Store 不承诺阻止这段交错。Core Activate 必须复核自己的 current proposal，不能复活取消。

Accept 的唯一裁决顺序是：authorize -> retained OperationID lookup -> exact AcceptanceKey lookup；若 acceptance
已经存在就按上条返回，不再要求 proposal current。只有 acceptance 尚不存在时才释放 Facet 锁、经 exact
InitialCoreNextHop fresh-read 预期 Core 的 proposal，回锁后重验本地 key absence、lineage 与所取证据绑定，再首次 commit。
若并发的首次 Accept 已建立记录，只返回相同 acceptance，不覆盖它的 CoreContact。
已存在 acceptance 的任何重复 Accept 均不改地址；InitialCoreNextHop 只初始化一次，后续走第 5.4 节维护。
原 OperationID 绑定包括原启动地址；重放不读新端口、不改原输入或联系记录。历史 acceptance
的 `UNCHANGED` 不表示 proposal 或 attachment 当前有效，Core Activate 仍必须重新验证自己的 pending proposal。

### 5.2 唯一状态机

```text
PRESENT + NONE + no pending proposal
  -- Propose(target, initial FacetNextHop, expected AttachmentRevision) --> NONE + PENDING(exact proposal)

PENDING(proposal)
  -- target Facet Accept(exact proposal) -----------> immutable acceptance
  -- Core Activate(exact acceptance) --------------> ACTIVE(target, proposal revision)
  -- Core Cancel(expected proposal revision) -------> NONE + no pending proposal

ACTIVE(target)
  -- Detach(expected AttachmentRevision) -----------> DETACHED(former target)

any PRESENT state
  -- DeleteElement ---------------------------------> NOT_FOUND
```

规则：

1. pending proposal 是 Core-owned 控制事实，不是第四种普通 attachment state；只由有权 handshake 的调用者读取。
2. Propose 可以指向任意有效 FacetAuthorityRef；它不要求等于某个 Store-global peer。
3. 首次 Facet Accept 必须经显式启动地址核对预期 Core 身份，重新读取 exact current proposal，并确认 target=self；
   已存在 AcceptanceKey 的查询遵守 5.1 的收敛顺序。
4. Core Activate 从当前 pending 的 FacetContact 指定地址读取 exact acceptance并核对预期 Facet 身份；
   外部调用在 Core 锁／Store transaction 外完成，回锁后重新验证 expected proposal revision 再提交，
   保留当前正向联系。地址变化不更换原 proposal 或使合法的已发出请求自动失效。
5. acceptance 不等于 activation。Facet 只有随后观察到 Core `PRESENT + ACTIVE(self, same AcceptanceKey)` 才能服务。
6. Cancel 后 acceptance 可以作为不可见历史保留，但不能复活 proposal。ProposalRevision 永不复用。
7. 本合同的 DETACHED 是终态；reattach、owner replacement、content adoption 与 fencing 属于 D12 转移后继合同。
8. ACTIVE namespace 可以没有任何 Facet record；ListTypes 对它返回确定的 complete empty。
9. 本合同中，同一 ElementUUID 的 acceptance lineage 只绑定一个 CoreAuthorityRef；Facet 对来自另一 CoreAuthorityRef
   的 proposal 返回 `SUBJECT_AUTHORITY_CONFLICT`。同一 Core 的不同 ProposalRevision 可以留下 immutable 历史，
   但只有 Core 当前 ACTIVE 指向的 AcceptanceKey 可服务。跨 Core rebind 只由 D12 转移后继合同解锁。

### 5.3 按关系指定地址联系，不按裸 Ref 解析

与[Core 挂载的联系边界](core-mount-contract.md#22-挂载记录指定下一跳连接只负责送达)一致：
部署层为对外提供服务的 Core／Facet 提供预期调用方可达的稳定入口，关系 owner 明确指定使用哪个入口。
本地 NextHop 使用相同 `local:<name>` profile，宿主以 BindEndpoint／UnbindEndpoint 按地址装配受限端口：

```text
ConnectCore(NextHop, expected CoreAuthorityRef)   -> typed local Core port | UNAVAILABLE
ConnectFacet(NextHop, expected FacetAuthorityRef) -> typed local Facet port | UNAVAILABLE
```

地址是联系输入，Ref 只是目标核对条件；不存在接受裸 Ref 的端口解析器、全局地址目录、
按 ElementUUID 猜 owner、失败后换目标或自动备用寻址。第一次进入 Core 的连接由调用者明确持有。
首次 Accept 的 Core 启动地址由已授权调用者显式提供并验证；形成关系后按该关系当前联系记录调用。

从空间发现到 Facet 的正常链路为：

```text
原入口沿已验证路径读取 Element（Core 挂载处使用其指定地址）
→ 同一 Core cut 的 ACTIVE FacetAttachment：目标 Ref + FacetNextHop
→ 按指定地址连接 Facet，核对预期身份，独立授权
→ Facet 按 AcceptanceKey 读取本地 CoreContact
→ 按指定 CoreNextHop fresh-read 原 Core，核验 existence + ACTIVE + matching acceptance
→ 返回独立 Core／Facet cuts 与 typed payload／合法缺失结果
```

Facet fresh-read Core 不承诺沿用发现时的旧 Core cut，结果必须报告实际取证切面；
不能把旧发现与新 Facet 数据包装成一个全局原子快照。Facet 不解析 TypeRef 或 Payload，
也不保留类型解析端口；这与已取消的事实 Ref → 服务地址解析属于不同边界。

地址由持有记录的一侧授权更新；它是该方向的寻址权威，不是可随意忽略的 hint。
一个 authority 可以被不同关系通过不同入口访问，不要求每个 Ref 随身带地址，也不定义唯一全球地址。
实际服务不可达不能成为 NOT_FOUND；连接对象和可达性缓存可重建，持久联系记录不能随之丢弃。
本轮仍是可信本地 typed port：每次核对目标 kind、身份及精确关系，Ref 字符串相等不代替网络认证。
远端传输／认证另行验证；不把统一 RouteRegistration、referral、lease 列为后继必做前置。

### 5.4 双向联系的本地维护

NextHopRevision 是 owner-local 私有 equality token，绑定发行 scope、关系种类、exact AcceptanceKey 与发行 cut；
与 Core mount 使用同一联系修订规则，但 mount token 不能用于 attachment。它不是新的随机 ID。
不同附着关系没有共享的可变 CoreRef → 地址表，不复制 authority-wide peer topology。

```text
Core.UpdateFacetNextHop(actor, OperationID, AcceptanceKey, expected NextHopRevision, NextHop)
Facet.UpdateCoreNextHop(actor, OperationID, AcceptanceKey, expected NextHopRevision, NextHop)
  -> status + original NextHop + NextHopRevision + owner ReadCut
```

- Core 的既有 pending 读取与 ACTIVE exact read 返回当前正向联系及 token；
  Facet.ReadAcceptance 的 envelope 在原有独立授权下另返回 CoreContact 与 current FacetReadCut。
  该 envelope 不属于 immutable acceptance；Core Activate 仍只把原 acceptance 及 AcceptedAt 作为不可变证据保留。
- 更新仅本地授权、精确关系／联系 CAS 与一次本地提交；不要求对端在线，不读取另一 Store，
  不占挂载 ControlSlot、不触发 Range 协调，也不被元素写门阻塞。
- Core 只维护 exact pending／ACTIVE 的联系；Cancel 清理 pending，Detach／Delete 终止其当前效力。
  Facet 只要保留该 acceptance，就保留并允许有权维护对应 CoreContact，供后续核验原 Core 的失效状态。
  更新历史反向地址不会激活 namespace；每次业务调用仍核验当前 Core。
- 实际地址变化只推进本方 NextHopRevision 与 ReadCut，不推进 AttachmentRevision、ElementRevision、
  FacetRevision，也不改变 ProposalRevision、AcceptanceKey、AcceptedAt、owner 或 payload。
- 原 Propose／Accept 的初始地址只用于第一次初始化。迟到消息、旧成功回执、重复 Accept 或 Activate
  都不得把它覆盖回当前地址；原 OperationID 输入和结果不随地址修复改写。
- 同一地址的合法不变更新为 UNCHANGED，无新 token／cut。原回执精确重放只返回原结果，不改当前地址。
  无原 binding 的新重试才使用新的输入；已经绑定的操作不能改地址后冒充同一意图。
- 新的控制发送从当前关系取地址；已发出的旧地址请求若仍符合身份、关系与操作规则，可以按原证据完成。
  对端来源地址不是身份凭据，响应不能自动修改本方联系记录；更新地址不等于封禁旧连接或 writer fencing。
- 重启恢复从各自 Store 取现存联系记录；宿主只重新绑定对应地址的传输对象。
  正反方向独立提交，不承诺全网自动刷新、多地址故障切换、迁移或失联后的自动寻路。

## 6. Facet 最小真相与行为

```text
FacetKey(ElementUUID, FacetTypeRef)
  -> 不透明 Payload + FacetRevision

Facet-private control:
  （无 per-Type 控制事实；内容、tombstones 与回执同事务持久化）
```

Facet 不保存 per-Type pin、artifact 镜像或 witness 记录；TypeRef 只是 FacetKey 的组成部分，
不是第二类型身份，也没有合同漂移约束。删除某 TypeRef 的最后一条 record 后重新 Put 同一 Ref
按普通 key 规则处理，无 pin 保留。

Facet 只新增以下 owner 行为：

| 行为 | 最小结果 |
|---|---|
| `AcceptAttachment` | exact immutable acceptance + AcceptedAt FacetReadCut |
| `ReadAcceptance` | exact PRESENT／NOT_FOUND at FacetReadCut；PRESENT envelope 独立带当前 CoreContact，不改变 immutable acceptance |
| `UpdateCoreNextHop` | 本地联系 CAS、COMMITTED／UNCHANGED 与原 token／cut；不修改 payload 或接纳 |
| `Get` | FacetAccessContext + exact TypeRef；返回 PRESENT／KEY_NOT_FOUND + FacetRevision、FacetReadCut、Core attachment evidence |
| `ListTypes` | FacetAccessContext 下 ACTIVE namespace 的全部 current FacetTypeRef，按 raw bytes 排序；允许 complete empty |
| `Put` | FacetAccessContext + 有界不透明 payload + absent／revision CAS + OperationID |
| `Delete` | FacetAccessContext + exact key expected FacetRevision + OperationID；不删除 Element |

每个 fresh Get／ListTypes／Put／Delete 都必须按 FacetAccessContext 的 exact AcceptanceKey 取得已保留的
acceptance 与 CoreContact，再经 CoreNextHop 联系其中固定的预期 Core，取得明确 CoreReadCut，并确认：

```text
Element == PRESENT
FacetAttachment == ACTIVE(self FacetAuthorityRef, same AcceptanceKey)
```

同时，Facet 必须在自己的 Store 中确认 exact AcceptanceKey 对应的 immutable acceptance 存在且匹配；缺失、损坏或
不匹配只返回 `UNAVAILABLE`。Get／ListTypes 的结果携带 `AcceptedAt` acceptance evidence；Put／Delete 的 retained
binding 与结果同样按值绑定该 evidence，不能只凭 Core 中的 target Ref 服务。

Fresh Get／ListTypes 不接受调用者提供的旧 Core cut、lifecycle evidence 或任意替换地址，
必须按自己保存的 exact CoreContact 读取 current Core snapshot；Facet 不扫描 acceptance、从 ElementUUID 猜 owner，
也不把联系记录解释成一个可重新选择 current owner 的指针。为避免混合不同事实或地址切面，
本地读取固定采用一次有界验证：

```text
1. 完成独立授权，在 Facet 锁内捕获 F0、exact immutable acceptance、CoreContact／NextHopRevision，然后释放锁
2. 缺少可验证 acceptance 或联系依据则 UNAVAILABLE，不猜测 Core existence；按 F0 指定地址联系预期 Core，取得 @ C
3. 暂存 Core existence／attachment 判定与 evidence，不提前披露状态相关结果
4. 重新进入 Facet 锁；若 current FacetReadCut != F0，返回 FACET_SNAPSHOT_CHANGED
5. 在 F0 中确认 exact acceptance；若 Core 非 matching ACTIVE，返回相应 Core-owned 结果；否则返回 record／KEY_NOT_FOUND／完整 TypeRef 集合
```

步骤 4–5 在同一个 Facet snapshot／锁边界内完成；地址变化也推进 FacetReadCut，不能捕获 F0 后换用 latest 地址。
不自动换端点、刷新切面或无界重试。这样一个在 detach／delete 前完成步骤 2
的在途读取仍可携带旧 Core evidence 返回，但不能看到步骤 2 后才由晚到 Put 建立的 row。

结果必须区分：

- `ELEMENT_NOT_FOUND`：Core 在 exact cut 上确认 Element 不存在；只带 Core evidence，不伪造 Facet cut；
- `NAMESPACE_NONE`：Core 确认 attachment 为 NONE；只带 Core evidence，不伪造 Facet cut；
- `KEY_NOT_FOUND`：ACTIVE(self) namespace 中没有 exact key；必须带 F0 与 matching acceptance；
- `NOT_CURRENT_OWNER`：ACTIVE target 不是 responding Facet authority；
- `NAMESPACE_DETACHED`：Core 在 exact cut 上确认 attachment 为 DETACHED；其公共效果是带 evidence 的 unavailable；
- `UNAVAILABLE`：指定连接、预期身份、Core／Store／当前 attachment evidence 或保留联系无法确认；
- `FACET_SNAPSHOT_CHANGED`：Core check 期间 Facet cut 已变化；不声明 key absence；
- `DENIED`：授权在披露 fact state 前失败。

`ACTIVE(other)` 返回 `NOT_CURRENT_OWNER + Core evidence`；`DETACHED` 返回 `NAMESPACE_DETACHED + Core
evidence`；Core 无法建立 cut 只返回 `UNAVAILABLE`。ACTIVE complete-empty 与 `KEY_NOT_FOUND` 都必须由 F0 与
matching acceptance 证明，不能仅靠 Core evidence。

Put／Delete 在调用 Core 前也捕获 exact acceptance 与当前 CoreContact 的同一来源，
不把其他 key 的地址用于回查。与读取不同，地址后续变化本身不使已经取得的合法 ACTIVE 证据无效：
回锁后复核本地精确关系及原 mutation 前置条件，按其实际取得的 Core cut 提交，不改写联系地址。
Facet mutation 的接受依据绑定它读取到的 exact Core attachment revision，并随结果返回该 evidence。Core 与
Facet 不伪造共同 Revision 或跨 Store 线性化点：Put 在确认 `ACTIVE@C` 后，Core 仍可能先 detach／delete，随后
Put 才在 Facet owner 提交并返回 `COMMITTED + C`；这条已确认写不被倒改，但不再是 current connected-space
payload。任何在 detach／delete 后才建立 current Core read 的 fresh Get 必须隐藏该 row；detach／delete 前的在途
Get 只有通过上述 F0 稳定检查才能返回。物理 orphan cleanup 不属于本合同，隐藏的 bytes 不会因 UUID 复用而重新出现。

## 7. authority-qualified 权限

本合同把 exact resource scope 收敛为：

```text
CoreElementScope(CoreAuthorityRef, ElementUUID)
FacetElementScope(FacetAuthorityRef, ElementUUID)
FacetKeyScope(FacetAuthorityRef, ElementUUID, FacetTypeRef)
RealmAdministrationScope(RealmID)
```

owner qualification 防止真正 authority transition 后旧 Grant 被新 owner 自动接受；它不让 Realm 成为 fact
owner。Attachment handshake、Facet namespace enumeration 和 exact key mutation 使用 owner 自己的明确
PermissionID。

联系维护使用本方独立授权，不继承对端用户或服务身份：

| 操作 | PermissionID | Scope |
|---|---|---|
| Core.UpdateFacetNextHop | `ucupa.core.element.facet-contact-update` | CoreElementScope(self, ElementUUID) |
| Facet.UpdateCoreNextHop | `ucupa.facet.element.core-contact-update` | FacetElementScope(self, ElementUUID) |

读联系只通过该侧已有的 exact attachment／handshake 读取授权，不因持有地址或 Ref 自动允许读取 payload。
首次 Accept 使用显式启动地址仍要独立建立 Core 调用方并获准读取 proposal；地址本身不授予权限。

Core 已随 Phase 1 使用 `CoreElementScope(CoreAuthorityRef, ElementUUID)`，本合同复用该 scope。
剩余 Facet 增量在同一 Greenfield 编译切片中直接替换当前字符串 `FacetScope` 及相应 Realm scope codec、
持久 PermissionBinding／retained outcome encoding 与 Realm Store FormatID。Realm 的 Principal、credential、
binding aggregate CAS、授权顺序和 owner-local revision 语义保持不变；不恢复 Core 的无 owner scope，也不保留无 authority 的 Facet scope constructor、
alternate reader／marker 或迁移路径。这里不引入已知会在 owner transition 时错误继承 Grant 的过渡权限模型。

所有 owner 必须在状态披露、OperationID lookup／replay、Core port 调用和 Store mutation之前授权。Composition
调用另一 owner 时使用显式配置且已获授权的 service PrincipalRef；终端 caller 权限不能被隐式复用为服务权限。

## 8. Revision、OperationID 与 durability

- Core ElementRevision、AttachmentRevision、CoreReadCut 与 Facet FacetRevision、FacetReadCut、双侧 NextHopRevision 都是 owner-scoped
  opaque equality token，不能跨类型、owner 或 cut 比大小。
- 一个 owner-local commit 只推进一次自己的 ReadCut；只推进本次真实改变所属的 lineage token。
- mutation 使用 1..128 bytes caller-proposed OperationID。Retained binding 至少包含 actor、owner ref、operation
  kind、subject、canonical input、precondition 和 payload identity。
- exact retry 先当前授权，再重放原 status、Revision、ReadCut 与 evidence；不重新调用另一 owner、
  不再次 mutation。原 Propose／Accept 启动地址及地址更新的输入均进入各自 retained binding；
  同 OperationID 的不同 binding 返回 `OPERATION_CONFLICT`，不以新地址重写原操作。
- Core Activate 的 retained outcome 按值保存它实际观察到的完整 immutable acceptance（包括 `AcceptedAt`），
  以便 response loss 后 exact replay；这不把 Facet cut 复制进当前 FacetAttachment 真相。
- Core／Facet 各有一个串行化状态提交边界；内存实现与 durable bbolt 实现语义相同。普通 bool Authorizer
  或 peer port 不在 owner lock／Store transaction 内调用。P5 受信本地 view 的最后比较参加 owner 锁下的
  提交/披露排序，不取远程 Realm 数据、不得重入 owner；独立身份及相关 view 规则见公共合同。
- Core Store 持久化 self CoreAuthorityRef、CoreReadCut high-water、Element、attachment／proposal、正向联系及修订、
  tombstone 与 retained outcomes；Facet Store 持久化 self FacetAuthorityRef、FacetReadCut high-water、immutable
  acceptances、按 AcceptanceKey 的反向联系及修订、records、tombstones 与 retained outcomes；
  P5 在两个 owner 原 Store 内增加公开 Revision 的稳定 HMAC 密钥及公共请求精确绑定；Realm Store 持久化 authority-qualified
  PermissionBinding 与其 retained outcomes。三个 Store 都使用各自新的 opaque FormatID。
- Core 在初始发布、每次 commit 与 reopen 时交叉验证：每个 ACTIVE 都能追溯到 matching Activate retained outcome，
  且该 outcome 按值保存同一 immutable acceptance 与 AcceptedAt；每个 DETACHED 都能追溯 matching former ACTIVE
  与 Detach transition。Facet 同样验证每个 acceptance 的 AcceptedAt 属于本 FacetAuthorityRef 的 cut lineage。
  这些当前因果证据在本合同范围内不压缩；任何凭空 ACTIVE／DETACHED、悬空 key 或不可能 cut 都使 Store fail closed。
- 联系的初始化、更新与清理同事实、token、cut、原回执在一次本地事务落盘。反向联系初始化和 acceptance
  同时成立；有 acceptance 却缺少合法联系不能返回可服务或“关系不存在”。ReadAcceptance 也不得掩盖该损坏。
- Core 与挂载合同使用同一套唯一 owner-local 提交历史规则，增加 attachment／联系事件；
  共享的是提交与验证基础，不要求先实现完整挂载功能。
  Facet 同样保存能重放 acceptance、联系变化与内容变更的本地历史。提交和 reopen 使用同一确定性验证规则，
  核对关系键、历史地址结果、token 及清理时点；不能仅校验创建时地址而允许 reopen 任意修改当前地址。
  非推进 cut 的拒绝／不变回执也须验证；不新增第二条竞争的 CoreReadCut 或跨 Store 事件序列。
- 每个 owner Store 只定义并识别一个 exact FormatID；不存在第二 marker reader、转换器、迁移分支或平行 API
  facade。missing、unrecognized 或损坏结构统一 fail closed 为 owner `ErrStore`，不按来源年代分类。
- response loss、reopen、commit fault 与并发重试不能制造重复 commit、回退 token、双 owner 或已确认写丢失。

本合同不承诺 Core create 与 Facet activation／Put 原子。最早可见顺序明确是：

```text
Core PRESENT + NONE
-> Core pending proposal
-> Facet acceptance
-> Core ACTIVE
-> optional Facet Put
```

任何中间失败只停留在已经由相应 owner 提交的状态；OperationID 使重试收敛，不回滚另一 owner。

### 8.1 封闭失败语义与裁决顺序

所有 mutation 先执行共同前缀：不需端口或 fact state 的纯语法、shape、bounds 与 canonical container 验证 -> 对输入中已知 exact scope 授权 -> OperationID
lookup。分别得到 `INVALID`、`DENIED` 或原结果 replay／`OPERATION_CONFLICT` 后立即停止，不读取 fact state。
共同前缀之后只能按下表顺序返回第一个成立的结果：

| 行为 | 共同前缀后的唯一裁决顺序 |
|---|---|
| Core `ProposeAttachment` | Element 不存在 -> `ELEMENT_NOT_FOUND`；expected AttachmentRevision 不等 -> `ATTACHMENT_REVISION_CONFLICT`；已有 pending／ACTIVE／DETACHED -> `STATE_CONFLICT`；否则 commit PENDING |
| Core `CancelAttachment` | Element 不存在 -> `ELEMENT_NOT_FOUND`；expected ProposalRevision 不等 -> `ATTACHMENT_REVISION_CONFLICT`；没有 exact pending -> `PROPOSAL_NOT_CURRENT`；否则 commit NONE |
| Facet `AcceptAttachment` | exact acceptance 已存在且联系完整 -> `UNCHANGED`，损坏则 `UNAVAILABLE`；否则启动地址／预期 Core 不可验证 -> `UNAVAILABLE`；读取时 Element／proposal 不存在、已取消或 exact 字段不匹配 -> `PROPOSAL_NOT_CURRENT`；同 UUID 已接纳另一 CoreAuthorityRef -> `SUBJECT_AUTHORITY_CONFLICT`；回锁后已有 exact acceptance -> `UNCHANGED`；其他本地 lineage 冲突 -> `SUBJECT_AUTHORITY_CONFLICT`；否则原子 commit acceptance + CoreContact |
| Core `ActivateAttachment` | Element 不存在 -> `ELEMENT_NOT_FOUND`；expected ProposalRevision 不等 -> `ATTACHMENT_REVISION_CONFLICT`；没有 exact pending -> `PROPOSAL_NOT_CURRENT`；指定地址／预期 Facet 不可验证 -> `UNAVAILABLE`；exact acceptance absent -> `ACCEPTANCE_NOT_FOUND`；回锁后 proposal 变化 -> `ATTACHMENT_REVISION_CONFLICT`；否则 commit ACTIVE，保留当前正向联系 |
| Core `DetachAttachment` | Element 不存在 -> `ELEMENT_NOT_FOUND`；expected AttachmentRevision 不等 -> `ATTACHMENT_REVISION_CONFLICT`；当前不是 ACTIVE -> `STATE_CONFLICT`；否则 commit DETACHED |
| Core `UpdateFacetNextHop` | Element 不存在 -> `ELEMENT_NOT_FOUND`；无 exact pending／ACTIVE 关系 -> `ATTACHMENT_CHANGED`；联系损坏 -> `UNAVAILABLE`；联系 CAS 不等 -> `CONTACT_REVISION_CONFLICT`；同值 -> `UNCHANGED`；否则本地 commit 联系 |
| Facet `UpdateCoreNextHop` | exact acceptance 不存在 -> `ACCEPTANCE_NOT_FOUND`；联系损坏 -> `UNAVAILABLE`；联系 CAS 不等 -> `CONTACT_REVISION_CONFLICT`；同值 -> `UNCHANGED`；否则本地 commit 联系 |
| Facet `Put` | 本地 exact acceptance／CoreContact 不可验证 -> `UNAVAILABLE`；指定连接／身份／Core 取证失败 -> `UNAVAILABLE`；source Core 结果按下段映射；record CAS 不等 -> `FACET_REVISION_CONFLICT`；容量越界 -> `CAPACITY`；exact equal -> `UNCHANGED`；否则 commit |
| Facet `Delete` | 本地 exact acceptance／CoreContact 不可验证 -> `UNAVAILABLE`；指定连接／身份／Core 取证失败 -> `UNAVAILABLE`；source Core 结果按下段映射；key absent -> `KEY_NOT_FOUND`；expected FacetRevision 不等 -> `FACET_REVISION_CONFLICT`；否则 commit |

FacetAccessContext 结构无效或 `FacetAuthorityRef != self` 属于共同前缀的 `INVALID`。
缺少 exact acceptance／CoreContact，或指定地址／身份／取证无法验证时只能为 `UNAVAILABLE`；
不能在这种情况下虚构 Core-owned absence。Facet 经已核对的 source Core fresh-read 后固定映射：

- `NOT_FOUND -> ELEMENT_NOT_FOUND`；
- `NONE -> NAMESPACE_NONE`；
- `DETACHED -> NAMESPACE_DETACHED`；
- `ACTIVE(other FacetAuthorityRef) -> NOT_CURRENT_OWNER`；
- `ACTIVE(self)` 但 ProposalRevision 与 context 不同 -> `ATTACHMENT_CHANGED`；
- Core port／Store 无法建立 cut -> `UNAVAILABLE`。

Get／ListTypes 暂存这些状态相关判定，按第 6 节先复核 F0，变化则只返回 `FACET_SNAPSHOT_CHANGED`；
不变时才披露 Core-owned 状态或 PRESENT／`KEY_NOT_FOUND`／complete empty。

Retained 规则也是封闭的：

- `COMMITTED`、`UNCHANGED`、`ELEMENT_NOT_FOUND`、`ATTACHMENT_REVISION_CONFLICT`、`STATE_CONFLICT`、
  `PROPOSAL_NOT_CURRENT`、`ACCEPTANCE_NOT_FOUND`、
  `SUBJECT_AUTHORITY_CONFLICT`、`NAMESPACE_NONE`、`NAMESPACE_DETACHED`、`NOT_CURRENT_OWNER`、`ATTACHMENT_CHANGED`、
  `KEY_NOT_FOUND`、`FACET_REVISION_CONFLICT`、`CONTACT_REVISION_CONFLICT` 与
  `CAPACITY` 对 mutation 都保留原 status、DecisionCut／CommitCut 与已有跨 owner evidence；
- `INVALID`、`DENIED`、`OPERATION_CONFLICT`、`UNAVAILABLE` 与只属于 read 的
  `FACET_SNAPSHOT_CHANGED` 不建立新 binding；
- 本地 Store 在 commit 前明确失败返回 `UNAVAILABLE` 且不建立 binding；response loss 或 commit 边界无法确认时
  返回 `INDETERMINATE`，它本身不被保留，但完全相同的 OperationID retry 必须收敛到未执行或已保留结果之一。

任何表外的新失败分类、同一条件的 fallback 映射或根据未授权状态改变优先级都不合规。Go error、wire code 与
DTO 拼写可以后定，但不得合并上述可观察区别。

## 9. 黑盒完成 gate

### 9.1 G1 regression 与 Core-only 闭合

- 不部署 Facet 时，G1 的 root／Anchor／Entity create、read、spatial update、reanchor／reparent、delete、ListDirect、
  geometry oracle、reopen 与全部错误／收敛测试原样通过。
- `ElementRecord` 与 `ListDirect` DTO 保持 G1 形态；ListDirect 不披露 FacetAttachment，后者只由 exact Element read 返回。
- attachment 生命周期 commit 推进 AttachmentRevision／CoreReadCut 而不推进 ElementRevision；仅联系变化推进
  NextHopRevision／CoreReadCut，不推进 AttachmentRevision。空间 update 推进
  ElementRevision／CoreReadCut 而不改变 AttachmentRevision 或 active target。
- Core-only create 在同一个 Core cut 返回 `PRESENT + NONE`；不存在 PRESENT 但 attachment 未初始化的切面。

### 9.2 AuthorityRef 与 attachment

- Core／Facet self Ref 非零、不同、durable reopen 稳定；复制 Ref bytes 不激活第二 owner。
- propose -> accept -> activate；acceptance 后 activation 前 Facet 仍不可服务。
- wrong target、wrong CoreRef、stale AttachmentRevision 与 port unavailable 均不得产生新的有效附着。
  acceptance 尚不存在且 fresh-read 已观察到 proposal cancelled／replaced 时，Accept 零提交并返回精确拒绝；
  合法在途旧读与已存在 acceptance 的重放则遵循下述独立规则，不能混成一律拒绝。
- FacetAccessContext 的 self/CoreRef/ElementUUID/ProposalRevision 任一不匹配都不能回退到扫描或猜 owner；另一
  CoreAuthorityRef 对同一 ElementUUID 的 Accept 返回 `SUBJECT_AUTHORITY_CONFLICT`。
- 一 Core -> 多 Facet、一 Facet <- 多 Core、Core-only 三种装配均通过；Store 中不存在 global peer pair。
- Pose／Range／Structure update 不改变 ACTIVE target；detach／delete 后 fresh Facet read 不返回 payload。
- exact acceptance 只提交一次；不同 OperationID 的重复 Accept 返回同一 acceptance，cancel 后未建立的
  acceptance 若没有读到合法旧 PENDING 则不能补建；已有合法旧读的在途 Accept 可以晚提交历史 acceptance，
  但任何情况下都不能激活已取消 proposal。测试必须覆盖读 PENDING → Cancel → Accept 提交这段交错。
- 完全删除 Ref→port 查找能力后，多 Core／多 Facet 装配、首次 Accept、Activate、业务回查与重启仍可完成；
  没有联系记录时不从同 UUID 或其他 acceptance 猜地址。指定错误服务只返回不可验证，不选择另一 owner。
- Propose 后更新正向地址再 Activate、Accept 后更新反向地址再重复 Accept，均保留新地址及原意图。
  对端离线可独立地址 CAS；并发维护不改 payload、接纳、附着目标或挂载写门；取消后旧地址重试不恢复关系。
- 两侧地址事件的每个持久边界接受响应丢失、重启与损坏注入；旧 token、错关系、错 owner 和未授权维护被拒绝。

### 9.3 Facet exact truth

- 任意陌生 TypeRef、非 JSON payload 字节与容量边界全部按结构/容量边界裁决；payload 不因未识别字节被拒绝，读写和精确重放不新增外部源前置。
- Get／ListTypes／精确重放每次返回原始字节；无解析、规范化或重新编码。
- ACTIVE empty namespace 的 ListTypes 为 complete empty；多类型按 raw bytes 完整排序，无重复或静默截断。
- Put absent/revision CAS、Get、Delete、OperationID replay、response loss 与 durable reopen 保持 owner-local cuts。
- Core check 期间没有 Facet change 时，Get／ListTypes 在 F0 返回；期间发生晚到 Put 时返回
  `FACET_SNAPSHOT_CHANGED`，不能把新 row 与旧 ACTIVE evidence 拼接。
- 捕获 F0 后反向地址变化也返回 `FACET_SNAPSHOT_CHANGED`；该页不换用新地址继续。
  无反向联系／错误身份／失联与已证明不存在分别测试，不得把前者转换成正常空结果。
- Put 读取 `ACTIVE@C` 后并发 detach／delete，可以提交并返回旧 evidence；随后 fresh Get 必须隐藏它，在途 Get
  只能按 F0 稳定规则返回或失败。
- ELEMENT_NOT_FOUND、NAMESPACE_NONE、NAMESPACE_DETACHED、KEY_NOT_FOUND、NOT_CURRENT_OWNER、
  ATTACHMENT_CHANGED、FACET_SNAPSHOT_CHANGED、UNAVAILABLE、DENIED 不互相折叠。
- 并发 mutation 由 Facet owner 串行化；race test 是必需交付门，缺少工具链时列为未完成验证，不能跳过后宣称通过。

### 9.4 Greenfield 减法证明

静态与黑盒测试共同证明本次 Facet 增量中不存在下列形态；不因本节的范围排除而删除已交付的 Core 能力：

- `FacetType string`、第二 FacetKey／scope profile、额外 FormatID reader、转换器或迁移分支；
- `LocalPeer*AuthorityRef`、authority pair、全局 Facet 服务或 ElementUUID 全网 lookup；
- 本次增量新增 Metadata／displayName 字段、API、Store、permission 或 Revision 语义；
- 地址写入 Element 空间记录或 Facet payload，或 Cluster／Store path 混入事实记录；
- 裸 FactAuthorityRef→服务地址／端口解析、authority-wide 可变对端表、失败后的备用 owner 搜索；
- public BatchGet、change log／Observe、Reconcile、mount、federation、transition 或生产机制；
- Core 导入 Facet package、Facet 导入 Core package，或 Composition 成为事实 owner。

Realm gate 还必须覆盖 authority-qualified 三类 scope 的 validity、canonical codec、binding administration、
OperationID replay、durable reopen 与 Store corruption；不存在 unqualified scope 或第二 Realm FormatID reader。

交付执行[开发计划](development-plan.md#7-所有代码交付共同遵守的验证与变更规则)的完整验证要求，
统一入口为：

```text
go run ./cmd/verify
```

## 10. 明确不覆盖

- Metadata、displayName，以及任何通用描述字段或读写面；
- 公共 FactAuthorityRef／FacetTypeRef wire encoding、AuthorityEvidence 和合同分发；
- SpaceReference、SpaceHandle、StructuralAddress、Enter、Discover、Observe 与公共 Propose；
- 网络地址编码、跨机器 transport、TLS／Noise 与安全 handshake、多地址及自动联系发布；
- mount delegation／acceptance、recursive query、partial／complete source proof；
- reattach、Core／Facet owner replacement、writer fencing 与 authority transition；
- Batch、change retention、GAP、physical Reconcile、索引、worker、备份或生产部署。

非联邦公共行为、联系的公共编码／目标验证，以及跨组织传输／认证／mount federation 分别由后继合同证明，
执行顺序由开发计划按实际依赖维护。旧 Ref→RouteRegistration／referral／lease 不作为后续必做项，
不能在本地端口之后偷偷恢复另一套解析系统。

## 11. 接受与生效

原合同、共同联系原则与具体条款已确认定稿，用户随后明确 Start 授权增量实现。
2026-09-07，用户明确要求“你先验收提交”；实现方复核范围并复跑完整 gate 通过后，完成 P4 交付验收。
本次验收与单独提交按下列精确范围生效：

1. 使本合同成为 current 的本地 Core／Facet 增量 implementation contract；G1 Core 合同继续拥有 D1–D4 `ElementRecord`、
   create／spatial update／reanchor／reparent／delete、ElementRevision、ListDirect、空间 CAS／错误／收敛与 geometry
   oracle，不被整体移入 historical；本合同只接替或扩充 Core self authority profile、exact-read envelope、
   create／delete 的 attachment 原子副作用、attachment-only commit、authority-qualified scope、attachment、typed port
   与 Store／commit clause families，并在 G1 合同 notice 中逐项同步这组精确边界；
2. 更新架构权威清单与受影响 notice：Facet predecessor 的 status／notice 一并转为 historical；Realm 既有合同继续
   拥有 identity、credential 与 binding 行为及已交付的 Core-qualified scope；其 notice 只标明由本合同接替剩余 Facet scope profile／codec；
3. 更新 Development plan，仅将本合同覆盖的 Facet 本地闭环能力标为已验收；
   下一动作由唯一计划依据其余能力状态决定，不自动完成 Stage 1 或 UCUPA Baseline；
4. 对每项 current 事实只保留一个明确合同 owner，不建立平行 API、第二 Store reader、迁移路径或兼容 authority。


## 12. P4 已验收实现映射

Core 附着控制位于 `ucupa-core/attachment_*.go`；Facet 的公开值／依赖在
`facet.go`，接纳／联系在 `attachment.go`，内容访问在 `content.go`，
确定性裁决在 `mutation.go`，状态／证据表示在 `state.go`，
持久化生命周期、事务和历史校验分别在 `store*.go`。
全部仍为原 owner 内核包，没有第二 owner 或兼容 facade。

Core 的真实 cut 通过 `CoreReadCut.Evidence()` 投影为共同的本地 opaque 值，
由 Composition 附在 Facet 消费方的窄证据端口上；它不是可独立声称来源真实性的凭据。
P4 交付当时由装配者显式注入 TypeContract resolver／validator；P5 替换为候选 artifact 源和 owner 验证；
P7.3a 起整体移除，Facet 不再注入或调用任何类型源。具体代码交付状态见开发计划，不维护两套类型读者。

实现仅允许既定行为。新增私有 Store marker 只识别当前格式；
已提交业务拒绝／无变化保留原结果而不推进 cut，损坏投影／历史不得自动修复。
交付状态、最终验证证据与下一动作只在开发计划维护；本次生效不授权后续 Phase。
