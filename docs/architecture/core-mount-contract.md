# 双 Core 最小挂载实现合同

**状态：P2 挂载控制与 P3 路径／组合查询均已实现、完整验证并经用户接受；其受限本地已交付范围为 current，由架构权威清单声明。代码交付与提交记录见开发计划，不扩大为网络、嵌套或多 writer 合同。**

**后续增量 notice：** [P6 网络合同](network-access-contract.md)承接公共网络联邦；
[P7 总合同](owner-transition-contract.md)承接同原始 Store writer handoff、首批 Core 转移和简单
carrier replacement 的相应限制；未接续 outgoing 外部边仍须在源撤回前安全拒绝。
[RealmAttachment 合同](realm-attachment-contract.md)承接当前 owner 的持久授权联系与网络核验。
本文原空间、挂载、查询和权限边界未被点名的部分保持，不把上述增量追记为 P2／P3 当时已实现。

用户已接受的方向与取舍：沿挂载结构访问，挂载记录同时指定稳定 Core 身份与下一跳地址，
不再以独立的 Ref → 地址解析作为 Core 挂载前置；修改挂载锚 Range 时，承载侧该空间的全部新元素事实写入可以暂停，
断连时暂停可能持续至恢复，不承诺固定时限。用户先授权据此修订合同，随后单独授权 Phase 1 共用最小基础实现，
并在验证完成后确认其达到提交条件，已单独提交。随后用户明确授权继续 P2，实施第 1–4 节的挂载控制
及第 5 节对既有本地方法的必要防漏限制。P2 完成代码和完整验证后，用户检阅交付并明确授权单独提交；
该次确认仅覆盖上述 P2 范围。此后 P3 路径／组合查询按独立授权完成实现与完整验证，
用户确认按既定设计接受该交付并授权单独提交；第 5 节的受限双 Core 读取范围由此生效。
当前执行状态只由[开发计划](development-plan.md)维护。

本合同继承[主线 D6–D10](ucupa-concept-mainline-baseline.zh-CN.md)的归属与授权原则，
共同联系方式已同步到主线 D8 与[Facet 附着合同](facet-local-truth-and-core-lifecycle.md)；
第 6 节说明其对现行实现的增量，不把新方向伪装成旧 Ref 路由的不变实现。
继承[本地空间真相](core-local-spatial-truth.md)及[本地查询](core-local-query.md)的几何语义。
它不修改基本公理，也不把同机实验等同于完整开放联邦。

## 1. 两层信息、范围与唯一归属

本设计区分元素的空间事实与挂载处的联系信息，不把 Element 扩展成部署对象：

- **空间事实**回答“这个元素是谁，在空间中如何存在与组织”：身份、Structure、Pose、Range，
  根锚仍按既有特殊规则表示，不补造 Parent 或 LocalPose。
- **挂载信息**回答“这条跨 Core 边界下的内容由谁承载、从哪个入口访问”：
  在挂载控制记录中关联目标 Core 的稳定标识与指定下一跳地址。普通 Element 不重复附加地址；
  “挂载锚”只是普通 Anchor 承担的角色，不新增 Element kind，也不复制一份空间事实。

空间结构决定访问哪一支，挂载记录提供跨 Core 入口，宿主／部署层负责使入口可达。
机器、Pod、副本及 Cluster 拓扑不进入 Element 空间模型，不要求空间树映射部署层级。
同一逻辑 Core 更换机器但保持服务入口不变时，不因迁机而需要修改空间事实或挂载地址；
同一既有挂载目标的入口地址修改不改变空间事实，也不等于换承载方；空间修改不自动改地址。
这不取消业务约束：修改挂载锚 Range
仍须遵循后文的双侧包覆校验与写入协调，不能把“两层分工”误解成无需协作。

首个验收场景：两个独立 Core、各自独立 Store、一条挂载边、无嵌套挂载，使用可信进程内显式连接。

```text
Core A：拥有 R、L、M
R（唯一无界根）
├─ L ─ 本地 Element
└─ M（普通 Anchor，其 Element 记录只在 A）
   ├─ 子 Anchor ─ …   Core B 拥有
   └─ Entity         Core B 拥有
```

- M 自身的 Element 记录属于 A；挂载指向 B 承载其内容，不表示 M 自身部署在 B。
  B 的直接内容以外部 M 为 Parent 与局部坐标参考。
- B 不拥有、复制或修改 M，不创建替代 M 的本地 root；两个已有独立根不能默认以 identity 变换接合。
- A 保留 M 的空间事实及委派控制；B 独立保存已接受内容、接纳控制、写门及本地操作结果。
- 只允许 A 的空普通 Anchor 与没有 Element claim 的 B 建立首次挂载。
- 两方不共享 Store、事务、操作结果命名空间或跨 Core 锁；各自按本地提交边界裁决。
- “两实例、一条边”限定本切片，不成为 UCUPA 永久容量限制。拒绝本轮不支持的嵌套、替换和已有内容迁移，
  不能接受请求后只处理其中一部分。
- 不做 Facet 新功能、Observe、网络传输、自动发现、Route 模块、全局目录、复制及 writer handoff。

本轮不提供挂载内容退役或重新挂接。撤销后内容保留，M 不能普通删除；这不是宣称整个挂载生命周期已经完成。

## 2. 稳定身份、连接与权限

### 2.1 一个 Core 的稳定身份，不增加 Authority 对象层

本切片一个 Core 对应一个逻辑事实负责方，负责自己保存的一组 Element；
`FactAuthorityRef` 就是这个 Core 的稳定标识，不是在 Core 内再放一组 Authority 对象。
沿用该类型名，不平行增加 CoreID／CarrierID；本地 Go/Store profile 采用独立命名类型的 canonical lowercase UUIDv4。

- `New`／`CreateDurable` 使用安全随机源生成一次；不接受调用者指定旧 Ref，失败则创建失败。
- `OpenDurable` 只恢复已存 Ref；缺失、格式错误或前后读取不一致均拒绝，不能补生成。
- `Core.AuthorityRef()` 供可信本地装配取得身份；A 与 B 必须不同。
- 同一 Store 正常 reopen、换连接对象或地址不改变 Ref。身份不是权限主体、地址、root UUID 或 ElementID。
- 私有 token scope 继续独立限定 Revision／CoreReadCut 的发行空间，不导出为 Ref。
- P2 不增加持久 writer epoch；P3 只增加运行内快照寿命绑定，不承担写者 fencing。相同 Ref 或 scope 不能证明不存在克隆 writer；回滚备份、可写克隆、
  永久丢库和跨实例 writer 转移不在保证内。
- 该编码只冻结本地 profile，不宣称冻结公共网络编码或密码学 AuthorityEvidence。

### 2.2 挂载记录指定下一跳，连接只负责送达

```text
A 的 M 挂载记录：CarrierAuthority = B 的稳定 Ref，NextHop = 指定 B 地址
B 的 incoming 记录：ParentAuthority = A 的稳定 Ref，NextHop = 指定 A 地址
```

部署前提是：每个对外提供服务、参与挂载的 Core，由宿主／部署层提供自身稳定的可连接入口，
在预期调用方的网络范围可用。UCUPA 不负责分配该地址，也不另建从 Core Ref 解析地址的服务。
下一跳可以是部署层的稳定服务入口，不必是实例的临时 IP；当前切片用下述本地地址实现同一角色。
“稳定入口”不等于永久在线、全网可达或不可重配；地址背后的目标仍须核对预期 Core 身份。

这两条是各自关系方向的寻址权威，不是可以任意忽略的 hint；A 管正向记录，B 管反向记录。
权威来自经本方授权写入的挂载控制记录，不是 IP／字符串自身证明可信。
只在跨 Core 的关系处保存，不给每个普通 Element 加地址；M 的 Element 空间记录与归属仍只在 A，
其挂载记录指定的是承载下方内容的 B。没有额外全局 Ref → 地址表，也不允许失败后沿裸 Ref 搜索替代目标。

本地 `NextHop` 是可持久化的地址值，profile 为 `local:<name>`：name 为 1–63 个小写 ASCII 字母、
数字或连字符，首字符为字母或数字；不做隐式别名、大小写转换或重定向。名字不从 Ref 派生。
这是同机地址格式，不冻结未来网络地址格式或实现网络传输。
可信宿主用 `BindEndpoint(address NextHop, peer PeerPort)`／`UnbindEndpoint(address)`
装配“已指定地址 → 本地受限端口”；这个可重建的传输适配只按地址调用，不接受裸 Ref 寻址。
PeerPort 不向普通调用者导出原始 Store、未授权快照或任意写入入口。

1. 初始接入由调用者持有已知入口连接；跨挂载时从该关系的持久记录取得指定地址和预期 Ref。
2. 每次新的控制 exchange 读取当前指定地址；查询使用其固定切面中的地址，变化按第 5 节处理。
   缺少地址绑定、误连、关闭或中断明确失败，不自动尝试其他地址、全局解析或返回空内容。
3. 每次调用核对已建立的对端身份、expected target、source、精确关系及操作／读取绑定；
   接收方在自己的临界区验证，发送方校验响应来源与绑定。Ref 相等本身不是远端认证。
4. peer 调用、调用者建立和 Authorizer 一律在 Core 锁与 Store transaction 外。
5. 首次信任由可信本地装配给出；跨机器的认证与传输另行设计。测试端口可注入失败、重复和延迟。
6. 运行时连接对象不进入 Store，重建时只绑定明确地址，不自动建立挂载、授予权限或恢复已撤销关系。
   地址记录本身则是持久控制事实，不能因传输适配或缓存丢失而删除。

控制端口至少包含 Prepare（新意图独立授权）、Finish（核对终局并履行精确义务）和
ReadDecision（仅查询已知 peer attempt 的终局）。ReadDecision 不能代替公开 mount-read，
不能枚举其他操作或凭新请求建立恢复义务；读取端口只执行第 5 节授权后的捕获与切面复核。

地址回答“连接哪里”，Ref 回答“是否为预期 Core”；两者都不替代 Realm 授权或空间合法性。
单一指定地址失效，就失去该入口的可达性，直到显式修正或恢复；没有自动绕行承诺。

### 2.3 地址维护与空间协调分离

首次建立在 A 保留时初始化正向地址，在 B 首次 Prepare 时初始化反向地址，均绑定精确 MountKey。
之后两侧分别通过本地 `UpdateMountNextHop` 修正自己的记录；不要求对端在线、不占 ControlSlot，
不受 B 元素写门限制。不存在的关系不能借更新地址创建。

- 只改本方向的 NextHop，不能改目标 Ref、MountKey、AdmissionKey、Element、写门或挂载生效状态。
  不推进 MountRevision／ElementRevision；实际地址变化推进独立 NextHopRevision 与本方 CoreReadCut。
- Establishing、Active、Revoking、Detached 及 B 已 Prepared 的精确关系均可修正地址；
  即使有未收尾操作，也不能以“先完成范围协调”阻止修复连接。更新不会自行打开已撤销入口。
- 原始 Establish 输入中的地址只初始化一次；控制意图、OperationID 绑定及原终局永久不改。
  重试／恢复使用当前指定地址发送原意图；迟到 Prepare、Finish、响应及精确重放不得覆盖新地址。
- 地址修复不要求重新 Prepare 已合法准备的范围。决定依据是精确意图、关系、空间状态与门，
  不是要求对方的整个 CoreReadCut 永远等于准备时刻；P3 查询的保留与联系失效分别按第 5 节裁决。
- 已发出的旧地址消息可以按原身份、关系、操作及阶段验证后完成；不靠来源地址决定身份或拒绝合法收尾。
  更新地址不是封禁旧连接、writer fencing、复制、迁移或承载方替换。
- 两方向独立提交，不要求全局同步；部署地址变化后由有权调用者明确修正引用该地址的记录。
  本轮不做自动发布、多地址、故障转移或全网刷新。失联双方都无法提供新地址时，不承诺自行恢复可达性。

### 2.4 owner-qualified 权限与本地调用者

Phase 1 已将 Core 原有的裸 `ElementScope(UUID)` 替换为下列 scope；本合同继续复用：

```text
CoreElementScope(TargetCore FactAuthorityRef, ElementUUID)
```

Core 从自己的持久 Ref 构造 requirement，不信任 caller 填入的目标 owner。
Realm 只机械编解码并比较完整 scope；没有无 owner grant fallback，没有权限继承。

`TargetCore` 是处理该操作的 authority，不一定是作用域 UUID 的 Element owner：
A 读 M 的 scope 为 (A,M)；B 接纳、列举或查询 M 下内容的 scope 为 (B,M)，不因此声明 B 拥有 M。

本地调用上下文显式按 owner 提供各自已建立的 PrincipalRef；可以是不同 Principal。
该上下文由可信进程内调用边界提供，不是可随意自报的网络凭据，也不允许 A 的 Allow 替代 B 的 Authorizer。
缺少必要的对端 caller context 视为未获准，不默认为 A 的 actor。

身份／凭据建立及权限绑定、Allow／Deny 仍由 Realm 角色负责；Core 只定义操作所需 requirement、
执行检查时序并消费裁决。下文“各方独立建立 caller”指各自的信任边界，不要求 Core 实现身份服务。
Core 持久保存 PrincipalRef 是原始意图的绑定与恢复证据，不复制账户、凭据或权限表。
Range 越界、CAS 冲突、写门关闭和连接不可用则属于 Core／连接合同，不由 Realm 判定。

保留普通 Core 操作的 PermissionID；增加：

| 权限 | 目标 scope | 含义 |
|---|---|---|
| `ucupa.core.anchor.mount-control` | A 的 M | 提议建立、修改挂载控制／正向地址或撤销 |
| `ucupa.core.anchor.mount-accept` | B 的外部 M | 接受首次建立／新的范围协调意图，或修改本方反向地址 |
| `ucupa.core.anchor.mount-read` | 被读取 Core 的 M | 读取该侧可披露的关系与控制操作状态 |

范围更新在 A 还要求原有 subject-update 与 final-parent-place。
状态读取不会因为曾获写权限而自动放行。Facet 调用 Core 的已有 grants／适配测试只机械补 owner，
不增加 FacetAuthorityRef、attachment 或 Facet 本地闭环的新行为。

## 3. 持久事实、修订标记与 Store 校验

### 3.1 精确键与状态

```text
MountKey = (ParentAuthority, MountAnchorUUID, ProposalRevision, CarrierAuthority)
AttemptKey = (ParentAuthority, OperationID)
AdmissionKey = (MountKey, 产生当前有效范围的 OperationID)

A:
  State = None | Establishing | Active | Revoking | Detached
  MountRevision
  MountKey? + CurrentAdmissionKey?
  OutgoingContact? = NextHop + NextHopRevision       # 目标 Ref 来自 MountKey
  ControlSlot = empty | OperationID
  MountOperations[OperationID] = exact request + decision + closure evidence

B:
  Incoming = empty | exact MountKey + immutable acceptance
  ParentContact? = NextHop + NextHopRevision         # 目标 Ref 来自 MountKey
  Admission = exact AdmissionKey + Range + parent decision evidence
  Gate = Open | Preparing(AttemptKey) | Closed
  PeerAttempts[AttemptKey] = exact request + Prepared/Rejected/Closed + retained evidence
```

- `MountRevision` 是新控制 token，私有字段为发行 scope、M UUID、发行时本地 cut serial；零值无效。
  同一 M 的控制事实变化发行新 token；它不是 Element Revision。
- 建立保留时发行的 MountRevision 即 ProposalRevision，永久作为 MountKey 的关系世代；取消后不能复用。
- `NextHopRevision` 绑定本方私有 scope、关系种类、精确 MountKey 与地址发行时 cut；不增加随机身份。
  Facet 附着采用相同规则但绑定 AcceptanceKey；两类 token 不可互用，不另造联系身份或第二个 CoreReadCut。
  同一地址的合法不变更新不发行新 token；独立地址 CAS 避免普通元素写入或范围协调阻塞维护。
  当前联系记录须属于当前登记关系；取消清理关系时一并清理，旧地址及回执只作为不可复活的历史保留。
  A 仅保存 Abort 时不能清掉 OutgoingContact，须收到 B 的 exact Closed 并完成收尾后才清理；
  B 已保存 Closed 后可清理被取消的 incoming／ParentContact，迟到消息只重放该终局。
- AdmissionKey 复用原操作身份作为接纳代次，不另造随机 AdmissionID。仅提交建立／Range 改动才更换它；
  Pose-only、地址更新、B 普通内容更新与取消不更换。
- immutable acceptance 绑定 MountKey、首次准备 tuple／Range 依据和 B 的 AcceptedAt cut；
  后续写门、准入及当前效力独立记录，不回写旧 acceptance。
- A 的控制 OperationID 与现有普通 mutation 共用一个 owner-local 意图命名空间。
  同 ID 的 kind、actor、对端 actor、目标、CAS 或 canonical input 不同，均为 ErrOperationConflict。
- B 的 peer attempt 使用 (parent owner, operation ID)，与 B 自己的应用 OperationID 分开。
- 两侧地址更新各自使用本方普通 mutation 的 OperationID 命名空间，不另设地址操作 ID 系统。
- 关闭／拒绝记录永久保留，含先收到取消再收到准备的情形；没有 pending 不等于可以重开旧尝试。
- A 的 None 也可保留历史 MountRevision／proposal lineage；不存在的关系不是伪造一份活跃 MountKey。

### 3.2 唯一 CoreReadCut，扩展提交历史

所有会改变可观察事实、接纳、写门、关系或控制进度的本地提交，均推进现有唯一 CoreReadCut。
只有实际改变某个 Element 记录的提交才推进该 Element 的 Revision。
原始成功／拒绝／UNCHANGED 结果的精确重放不推进 cut；单纯保留不改变事实的终局拒绝亦沿用现行规则。

Phase 1 已建立独立提交序列、记录显式提交位置，并移除原 `cutSerial == revisionSerial + 1` 的硬约束。
当前历史仍只实现 Element mutation；本合同须在同一基础上增加控制转换与相应重放验证，
不能只在 Store 中添几张表而保留仅识别 Element mutation 的验证器。

继续采用唯一的 owner-local 持久提交序列；本合同的目标覆盖：

```text
ElementCommitted
MountReserved
PeerPrepared / PeerRejected
MountDecided
PeerClosed
MountSettled
NextHopUpdated
```

- 每个真实状态转换有唯一连续 cut，事件包含 canonical 输入、适用的旧状态／授权裁决及新状态依据。
- 同一个确定性状态转换器用于提交和 reopen 验证；授权裁决作为当时的外部输入保留，
  reopen 不重新调用现时 Realm 改判历史，也不声称 Store 能验证历史权限服务的真实性。
- 每个推进 cut 的事件，以及保留在该 cut 上的不变／拒绝 outcomes，都要重放验证；不能只检查成功事件。
- 新 ElementRevision 独立计数；控制事件可同时更新 M 的最终 tuple，但不能为未变 Element 制造 Revision。
- 每步在一个本地事务中保存事实、控制状态、cut、事件及原始回执，然后才发布内存状态。
- 继续执行现行 persist 失败后 reload 规则：只接受完整 previous 或 next，其他情况使实例不可继续服务。
  如果无法确定落盘结果，返回不确定的 Store 错误，不能回报确定取消。
- 增量 Store marker 在实现时一次替换；只保留一个当前格式，无旧 reader、兼容、自动迁移或回退。
- reopen 与每次 persist 同时核对 owner Ref 和私有 scope；事件重放必须重建全部当前状态与 retained outcomes。

### 3.3 本地结构形态

| 形态 | 必须满足的条件 |
|---|---|
| 未进入空间 | 没有 root、有效 incoming 或 Element claim；可保留已关闭控制尝试 |
| 有根 A | 唯一不可变无界 root，无 incoming |
| 外部父 B | 无本地 root，唯一 incoming；每条本地 parent 链终止于该关系的外部 M |

- B 准备接纳期间不能 CreateRoot；所有本地 Create 必须拒绝 UUID=M。
  M 不进入 B 的 Anchor、Entity 或 tombstone 表；边界保留不等于复制记录。
- `Parent=M` 只通过 exact incoming context 解释；其他 Parent 必须是本地 Anchor。
  除这个声明的外部终点外，孤儿、环或第二独立空间均拒绝。
- B 的直接 child 使用当前 Admission 的有限 Range 做精确包含校验；其余父子使用既有本地算法。
- A 已保留／激活 M 后不能再接纳其本地直接 child。
- Detached 的 B 保留最后合法上下文供 Store 校验和授权后的 owner-local 读取，但关闭写门，
  不把这些记录冒充当前 connected-space 内容。
- 本地投影只能沿已证明的 frame 链；不能要求不存在于 B 的 M record，也不能伪造 root identity Pose。

### 3.4 UUID 保证边界

ElementUUID 仍是稳定元素身份，不能偷偷改成“owner + UUID 才是身份”。
生产者必须不重复发行、不挪用其他元素 UUID；每个 owner 永久拒绝自己已经 claim 的 UUID。

独立 Store 不做全网查重，不能对任意恶意 caller 输入承诺跨 owner 的预提交全域唯一性。
在已授权组合范围中发现两个 owner 同时声明相同 ElementUUID，返回 ErrIdentityConflict：
不合并、不任选一方、不重新解释为两个合法元素。结果中的 source + UUID 仅定位声明来源。

B 对外部 M 的撞号必定本地拒绝；其他尚未读取空间里的冲突不宣称已检查。
要求任意跨 owner 撞号在写前都必被拒绝，需要额外 claim 协调或另定 ID 分配模型，不由本轮暗中实现。

## 4. 控制调用与收敛

### 4.1 应用只提交意图，不驱动 Prepare／Commit

本轮采用一个 tagged 意图调用，避免给应用暴露协议步骤：

```text
ApplyMount(ctx, LocalCallers, OperationID, MountCommand) -> MountControlResult, error

MountCommand =
  Establish { M, ExpectedElementRevision, CarrierAuthority, CarrierNextHop, ParentNextHop }
  Update    { M, ExpectedElementRevision, ExpectedMountRevision, Final SpatialTuple }
  Revoke    { M, ExpectedMountRevision }

ReadMount(actor, M) -> MountRead, error
ReadMountOperation(actor, M, OperationID) -> MountControlResult, error
RecoverMounts(ctx) -> recovery report

UpdateMountNextHop(actor, OperationID, M, MountKey, ExpectedNextHopRevision, NextHop)
  -> NextHopUpdateResult { OperationID, MountKey, NextHop, NextHopRevision, Changed, Cut }, error
```

- `ApplyMount` 在 A 调用。LocalCallers 绑定各 owner 独立建立的 Principal；首次建立与范围 Prepare 需要 B 的 caller。
- 建立／实际改 Range 若缺少已建立的 B caller，必须在 A 持久保留之前返回零结果 ErrDenied，
  不绑定 OperationID、不占槽。否则事后补 actor 会改变原请求，导致无法恢复的 Pending。
  有已绑定的 B actor 而 B 当前策略拒绝，则是可以由 A 保存 Abort 的明确拒绝，不混同于缺少身份。
- Update 接受完整最终 Parent／Pose／Range，不要求应用提交一个可能非法的中间状态。
- Establish 的两个地址由发起者显式提供，分别须能在 A、B 的传输上下文使用；B 独立授权接受首次
  ParentNextHop，不从网络来源地址或对方报文自动学习。A 正向地址在保留时落盘，B 反向地址在 Prepare 时落盘，
  不等到挂载 Active 后才保存，确保中途重启仍有恢复依据。
- 精确重试同一 ApplyMount 推进原操作；每次调用最多执行一次前向阶段遍历，遇不可用或取消等待即返回，
  不在调用中无限循环。网络 deadline／wire profile 不属于此处。
- 两个 Read 只读，不隐式执行协议。MountRead 含 State、MountRevision、允许披露的 MountKey／AdmissionKey、
  当前 Range 依据、该侧 NextHop／NextHopRevision、ControlSlot 摘要和该侧 Cut；不因此披露完整 Pose／祖先链。
  未知操作只在授权后返回 ErrNotFound。
- RecoverMounts 是可信宿主的受限恢复入口，每次遍历已有未收尾操作一次，不接受新意图、不绕过新 Commit 授权。
  它不新增后台 daemon；接入应用可在重连或重启后显式调用。对方已知操作也可按精确消息恢复。
  A 从 OutgoingContact 取地址发送原 Prepare／Finish；B 从 ParentContact 取地址读取 A 原决定。
  没有对应的运行时地址绑定就报告待恢复，不从裸 Ref 找连接、不丢掉持久地址或解锁写门。
- 不提供另一个客户端 Cancel。首次建立／更新在持久终局前由明确校验拒绝收敛到 Abort；
  单纯断连、ctx 结束或停止等待保留 Pending，不自动取消。未来若消费者需要主动撤回，再增加单独合同。

地址更新在持有对应联系记录的 A 或 B 本地调用，使用第 2.4 节本方权限；不能由 A 的授权代改 B。
先做 canonical 验证、当前授权与原 OperationID 检查，再在本地提交边界核对精确关系和 NextHopRevision。
无权限、形状非法或意图冲突不占新 ID；已授权的关系缺失／错误及版本冲突保留明确拒绝，成功或不变保存原始回执。
拒绝返回零结果及相应 error；精确重试先重新授权，再重放原结果或原错误，不以当前地址重写旧回执。
实际更新通过 NextHopUpdated 事件原子保存地址、token、cut 和结果；不变结果无新 cut。
本方 Store 结果不确定时返回 ErrStore，须沿用原 ID 查询／重试，不能声称更新未发生。
ReadMountOperation 仍只读取 ApplyMount 操作；地址更新的结果通过精确重试取得，ReadMount 用于读取当前地址。

### 4.2 结果不是普通 MutationResult

```text
MountControlResult {
  OperationID, MountAnchorUUID, MountKey?             # exact binding
  Decision: Pending | Committed | Aborted
  Settled: bool
  DecisionCut?                                      # A 的原始决定 cut
  Record?: ElementRecord                            # 更新的原始结果，不是 latest
  Changed: bool
  AbortCause?: Denied | NotFound | Conflict | RevisionConflict
  BlockedBy?: Unavailable | OwnerMismatch | PeerStore
}
```

- Pending 必须未 Settled、没有 DecisionCut，不声称没有在途提交。
- Committed／Aborted 的终局不可变；Settled=false 表示 B 关闭与 A 收尾尚未全部确认。
- Committed 不意味着当前关系仍活跃；原成功回执重放不复活后来已撤销的关系。
- Aborted 必定无新 Element 记录；取消收尾未完成时，仍可能保持写门或控制占用。
- Range／tuple 实际不变的合法 Update 为 Committed、Changed=false、Settled=true，
  返回原记录与 cut；不发行 ElementRevision，不为调用了新接口而暂停 B。
- Range 不变但 Pose／Parent 实际改变时，先检查 Element／Mount CAS 与空控制槽，再本地原子提交；
  Committed、Changed=true、Settled=true，推进 ElementRevision，但不更换 AdmissionKey。
- 形状非法、当前调用无授权、OperationID 绑定冲突：零结果加相应 error，不披露资源状态。
- 已授权且在保留前明确的 NotFound、kind／结构／几何 Conflict 或 RevisionConflict，持久保留
  Aborted、Settled=true 及当时 cut，不向 B 发消息；ErrMountBusy 不接纳、不占 OperationID。
- A 能确认原持久状态时，返回非零结果；对端暂时不可用用 BlockedBy 表达，不能假造 Abort。
  对端明确 Prepare 拒绝会由 A 决定 Abort；没有匹配的 Prepared 不能提交建立／范围变更。
- A Store 结果无法确认时为零结果 ErrStore；调用者只能以同 ID 查询／重试恢复，不能改 ID 当作“没提交”。

### 4.3 建立

| 步骤 | 本地原子变化 | 必须重验的条件 |
|---|---|---|
| A 保留 | None → Establishing；发行 ProposalRevision；保存意图及控制占用 | M 非 root、CAS、空直接内容、无未处置关系；冻结 M tuple 与本地 child 接纳 |
| B 准备 | 保存 immutable acceptance、Prepared 与关闭写门 | 独立授权、精确身份／关系／请求；无 root、无 Element claim、无冲突 incoming |
| A 决定 | 匹配准备则 Active + Commit，否则 Abort | 原 actor 再授权；M Revision、空锚、保留阶段与 B 所确认 tuple／Range 仍匹配 |
| B 关闭 | Commit 安装 Admission 并开门；Abort 清除未生效 incoming | 只履行 exact A 终局；保存 Closed，拒绝迟到 Prepare |
| A 收尾 | 保存关闭证明并释放控制占用；Abort 后回 None | 对端确已持久关闭同一次尝试，不复用旧完成回执 |

A Active 而 B 尚未安装 Admission，不是完整空空间。
A Abort 而 B 尚未确认关闭，也不能提前恢复 A 对 M 的本地 child 接纳。

### 4.4 修改挂载锚最终 tuple

| 步骤 | 本地原子变化 | 必须重验的条件 |
|---|---|---|
| A 保留 | Active 中登记意图、独占控制槽，冻结 M tuple 与新撤销 | 初始 Element／Mount CAS、关系及候选最终 tuple 的本地合法性 |
| B 准备 | 验证直接内容包覆，保存 Prepared，并关闭该空间全部新事实写门 | 当前 MountKey、旧 AdmissionKey、精确请求；校验与关门在同一提交临界区 |
| A 决定 | 新 M + 新 AdmissionKey + Commit，或只保存 Abort | 再授权；M CAS、精确保留状态、父包覆、无环和 B 的匹配证明 |
| B 关闭 | Commit 安装新 Admission；Abort 保留旧 Admission；记录 Closed 并开门 | 终局与 exact attempt 匹配；取消后不可因迟到证明重开 |
| A 收尾 | 保存 B 完成证明，释放槽 | 下一次范围操作或撤销须在此之后 |

保留操作自己推进 MountRevision，所以决定阶段验证的是“本操作的精确保留状态”；
不能拿初始 ExpectedMountRevision 与自己更新后的 token 比较而自造冲突。

如果 Range 未变且没有待决操作，Update 可按 A 的本地原子路径处理 Pose／Parent，不需要 B Prepare。
A 的其他分支不会因这个控制槽停写；它们仍独立满足本地结构与包覆规则。
普通 UpdateAnchor 也只在这个条件下处理已挂载 M；Range 改动必须明确返回 ErrMountCoordinationRequired，
不能用 A 的本地 children 为空绕过协调。该错误与 ErrMountBusy 均是未接纳错误，不占用 OperationID；
调用方可用该 ID 提交正确的 ApplyMount 意图。有待决控制槽时，M 的普通新 Update 返回 ErrMountBusy，
精确历史回执仍按权限重放。任何实际空间变更都仍由应用提交，没有自动扩大或回写。

### 4.5 撤销

撤销是单向决定，不套用范围的 Prepare：

1. A 检查权限、Active、关系 CAS、无控制占用，在一个提交中记录不可取消的 Commit、
   Revoking、控制占用并关闭该 connected-space 入口。
2. B 根据已接受关系及精确终局持久关闭写门，记录 Closed／Detached；不是重新决定是否让 A 撤销。
3. A 收到封锁证明后变为 Detached、Settled，释放占用。

B 失联时 A 能关闭自己的入口，但不能声称 B 已停止本地写入。Range 尝试未结清则撤销返回 ErrMountBusy，
须先恢复该尝试。撤销中及已撤销 coverage 是不可用分支，不是可以忽略的空集合。

### 4.6 权限、重放与恢复顺序

- 新公开意图：无状态 canonical 验证 → A 当前授权 → 查 OperationID → 本地状态裁决；
  外部调用后再次在本方临界区检查精确状态。
- B 首次 Prepare 独立授权。已经 Prepared 是对该精确操作履行终局的持久义务，
  后续 close 不要求原用户仍有新写权限，否则已 Commit 的范围会因撤权永久无法收尾。
- A 产生新 Commit 前再次检查原 actor 权限；失败由受限恢复逻辑保存 Abort。
  当前用户无权的调用本身仍返回 ErrDenied，不泄露或替代这个持久终局。
- 恢复只能推进已存在的精确意图；不能凭旧授权、旧连接或旧 acceptance 创建新业务操作。
- B 普通 mutation：授权 → 查精确原回执 → 写门 → 原 CAS／几何裁决。
  写门关闭时新请求返回 ErrMountBusy，不占用其 OperationID；旧回执授权后可重放。
- 因而“普通元素更新不会触发范围协调”不等于“暂停期间普通元素仍能写”。
- 不持有本方锁调用另一方或 Realm；准备、终局、关门、开门、操作结果均在本方提交边界共同发布。
- 没有 B 超时解锁、Abort 改判 Commit、丢响应回滚或旧 Closed 证明关闭新操作。
- B close 只释放该尝试拥有的门：先查 exact Closed／Rejected 历史，已关闭只重放而不改变当前状态；
  从未 Prepared 的 Abort 只增加关闭记录，不能清除别的 incoming／写门；不匹配当前 Prepared 的 Commit 拒绝。
  Revoke 的 Finish 则匹配当前已接受关系后封锁，不要求虚构一次范围 Prepared。
- A 的恢复使用保存的原 A actor，对 Pending 的新 Commit 重新授权；拒绝则 Abort。
  B 只为自己已知的尝试读取 A 的终局并履行义务，不通过恢复重选 caller。
- 当前 bool Authorizer 只证明各自检查时点，不证明授权与 Core 提交跨 Store 原子，也不检测撤权后恢复的 ABA。

## 5. 读取、路径与组合切面

**P3 已实现、验证并验收：本节为向上深度、范围内完整发现及可选快照保留的路径、组合与失败规则，具体实现证据见开发计划。**
P2 已交付的 owner-local 防漏限制继续有效；修订读取计划不重做第 1–4 节的持久控制状态机。
本地查询／快照生命周期只由[本地查询合同](core-local-query.md)定义，本节补充路径和双 Core 组合。

### 5.1 最少公开读取增量

```text
ResolvePath(ctx, LocalCallers, Path, at *ConnectedCut) -> PathRead, error
ListDirectPath(ctx, LocalCallers, Path, at *ConnectedCut, limit, continuation)
  -> ConnectedDirectPage, error
QueryRegionPath(ctx, LocalCallers, PathRegionQuery, at *ConnectedCut, limit, continuation)
  -> ConnectedQueryPage, error
ReleaseConnectedCut(ctx, LocalCallers, ConnectedCut) -> error

Path = { OriginAnchorID, Up: uint64, Down: []ElementID }
PathRegionQuery = { StartPath, UpDepth: int64, BasisPath, Sphere, Selector }
```

Path 从接收 Core 中已知的 Origin Anchor 开始，先沿真实父边上行 Up，再沿 Down 的 UUID 链逐边下行。
Up 是地址中**实际**经过的父边数；越过根是非法路径，不做饱和。
Query 的 UpDepth 则是语义选择：正数从 Start 最多上溯相应层数，-1 不限制层数直到已证明的根；
小于 -1 非法，0／省略取 Start。Path.Up 与 ActualUpDepth 仍是非负的真实边数，不接受 -1；二者不混淆。
路径中间元素均为 Anchor，终端可为 Entity；空 Down 且 Up=0 指 Origin 自身。
不引入裸 UUID／Ref 全网定位或独立路由；相同查询的 StartPath 与 BasisPath 必须具有同一个 Origin。

Origin 通常由接收 Core 拥有；唯一外部入口是 B 已接纳的 M，此时先按 ParentContact 联系 A，
核对身份和当前 exact 挂载效力。空 B、仅承载直属 Entity 的 B 因此也能进入，不造替代根。
B 子 Anchor 的父边可以到 A 的 M，再到 A 的祖先；跨服务不额外算一层，M 始终只属于 A。
接收 Core 不是空间边界，不能因为入口在 B 就拒绝必要的向上路径或把 B 误作独立根。
本 profile 没有跨独立根或嵌套挂载，也不把未来 SpaceHandle 的显式边界静默省略。

QueryRegionPath 在同一选定切面的结构中解析 Start，上溯选择 Scope，然后完整搜索 Scope 及全部后代；
Basis 只解释查询球，必须在该 Scope 内，不必在 Start 子树内。
返回 Scope 及其实际路径，兄弟分支、祖先直属 Entity、Scope 自身均按同一规则参与。
正数超过可证明根时停止，-1 同样须证明真实根，不能以接收 Core 的边界代替它；
未来若使用明确 SpaceHandle boundary，-1 只到该已声明边界，不跨独立根。
断连、无权、未接纳不是根，不能自动降低深度；必要上溯失败不返回一个截短的成功 Scope。
Complete 只说明所选 Scope 的闭包完整，不声称 Scope 外没有命中。

```text
Source = { Authority: FactAuthorityRef, Cut: CoreReadCut }
Match = { ElementID, Kind, Revision, Path, Source }
ConnectedQueryPage = {
  Matches, Cut: ConnectedCut, ReadMode: QueryReadMode, ResolvedScopePath, ActualUpDepth,
  Status, Gap, Continuation?
}
ConnectedDirectPage = { RecordsWithSource, Cut: ConnectedCut, ReadMode: QueryReadMode, Status, Gap, Continuation? }
PathRead = Present { Record, Source, Path, Cut, ReadMode }
        | NotFound { VerifiedPrefix, AbsenceSource, Cut, ReadMode }
```

Match.Path 相对**解析后的 Scope**，不包含 Scope 自身。ResolvedScopePath 是从原 Origin 到 Scope 的地址：
从 StartPath 上溯时先消去 Down 尾部，剩余上行再合入 Up，不能直接在原路径 Up 上累加。
输入路径须逐边验证；合法的 Up 后 Down 可能绕回原分支，例如从 B 的 S 上溯 R 再下行回 S 的 Entity。
此类发现后拼出的地址允许输入，由 Resolve 在同一选定切面的树中归一为最短树路径，不另读 latest，
不把两次经过同一挂载当成两个来源。规范化结果用于绑定与比较；数值累加检查溢出，不靠字符串猜邻接。

发现后仍到原入口，沿 **ResolvedScopePath 追加 Match.Path 的下行链**调用 ResolvePath，
at 使用原 ConnectedCut。返回记录必须与 Match 的 ID／Kind／Revision／Source 相符，
exact read 权限另查。不能把原 StartPath 直接加 Match.Path，更不能拿 Source.Authority 去另找地址。
M 只以 A 的记录出现一次。Source.Cut 是归属证据，不是能直接读取保留历史的句柄；
本节不提供“仅把 Source.Cut 传给普通 ReadElement”这一历史读取捷径。

ListDirectPath 只列直接子完整记录，M 的直接内容来自 B，不重复列 M。
NotFound 只由当时负责邻接的一方在指定视图中证明；连接失败不等于不存在。
Query 只返回摘要，不要求未知命中事先获得 full-record read grant。

B 入口即使查询完全在 B 自有子树中，也须核对 A 的当前 exact 挂载仍有效，独立获得 (A,M) mount-read。
这不要求 A 全空间 query，只披露必要关系依据。
A 已撤销而 B 尚未收到通知时，不能返回 connected-space 成功；
不需要这种保证的读取须明确用 owner-local 方法，不在失败时静默降级。

**继续保持 P2 防漏门：** owner-local ListDirect 遇由远端承载的 M，QueryRegion 的实际 Scope 闭包遇未处置 coverage，
或深度解析需跨外部父，返回零结果 ErrMountContextRequired。Active／Establishing／Revoking／Detached 都不是空邻接。
B 上以外部 M 为目标的本地 ListDirect、QueryRegion、ProjectToAncestor 同样需要挂载上下文；
ReadElement(M) 只能证明 B 不拥有该记录，不能证明空间中 M 不存在。
在单个 owner 内完全闭合的普通读取不代证整条连接当前活跃。

### 5.2 首次捕获组合，后续固定模式，不共享锁

ConnectedCut 是接收 Core 运行内的私有读取上下文，绑定原入口、读取范围、各方 caller、固定 ReadMode、
每个来源的 CoreReadCut 与运行绑定、精确关系／Admission／联系修订及 coverage。
只有 Retained 另绑定各来源的运行内 snapshot 句柄；CurrentVerified 不借元数据保留旧 Element、
邻接表、几何状态或完整命中数组。它不是 WorldRevision、权限凭证、公开 wire token 或持久查询 Store。

#### 5.2.1 先选模式，再准入资源

每个 owner 的可选 SnapshotRetention 遵守本地查询合同：未配置关闭保留，显式有限正时长启用，
没有默认值，配置在本次运行固定。首次披露前确定实际来源及模式：

- 所有实际来源均启用，整条上下文才为 Retained；任何实际来源未启用，整条上下文为 CurrentVerified。
  用于上溯、Basis、挂载效力或其他必要证明但不输出元素的来源也计入，不能只看输出来源。
- Excluded 来源不是实际贡献者；未授权或未验证来源不提供能力证明，也不伪造“未配置”。
  若是必要来源，无法取得证明则整个请求失败；普通后代可以成为明确的 coverage 缺口。
- 能力和容量是两件事：必要来源开启但容量满、拒绝或失联不能使模式静默降级。
  首次捕获的来源或配置证明不再匹配，则清理失败，不在同次调用隐式更换来源集合或模式。
  模式确定为 CurrentVerified 时，各方仅提供临时当前视图，即使其中一个 owner 自己开启了保留也不留下空间 pins。
- 模式确定前，不能闭合的普通后代可记为 Excluded；一旦选定来源与模式，首页捕获中任一已选来源失败，
  则按原因整次失败并清理，不再删去它重选模式或循环捕获。任一已选来源的保留准入／借用额度不足
  均为 ErrSnapshotCapacity，包括非必要后代；后续页同样不以容量为由移除来源。
- 首页及所有后续返回公开同一 ReadMode，并绑定进 ConnectedCut／continuation。
  Retained 到期后不得切换 CurrentVerified；CurrentVerified 也不升级、重 pin 或添加恢复后的 Excluded 来源。
  当前配置不合原运行绑定，必须重新开始，不能用重启后新配置重解释旧 Cut。

没有外部效力依赖时采用单来源同样规则；涉及唯一挂载边时按以下协议捕获。
A／B 始终表示父方／承载方，不表示请求入口顺序。模式探测只取得已授权的配置／来源候选，
不是空间一致性证明；最终捕获仍须匹配这些候选。

#### 5.2.2 捕获与逐次验证

1. A 锁外授权，短读锁内捕获 cA 不可变临时视图；Retained 同时准入并保留该视图。
2. 按 cA 指定地址联系 B，核对身份；B 独立建立 caller、授权，短读锁内捕获 cB；
   仅 Retained 准入保留。没有跨 Core 持锁或同一数据库事务。
3. exact MountKey、有效 AdmissionKey 与 Range 必须匹配。
   M 的 Pose／Parent 和祖先取 cA，B 内容与局部结构取 cB，不拼 latest。
   Admission 的旧 ElementRevision 不要求 Pose-only 更新停止；包覆依据与投影依据分别核对。
4. Retained 初次再检查 A 可读且 current=cA，成功才封存来源。
   B 捕获瞬间被 A 前后不变括住，因此不要求 B 在此时仍 current=cB。
   已证实 A cut 改变，清理并返回 ErrReadCutExpired；A 无法联系为 ErrUnavailable，
   本方 Store 不可读为 ErrStore，不能把无法检查伪装成 cut 已过期。
5. 在选定视图解析 Start／UpDepth／Scope／Basis、计算和排序；若事先为确定权限暂时解析过，
   必须证明最终解析与已获准 exact Scope 相同，否则清理本次捕获、返回零结果 ErrReadCutExpired，
   由调用方重新开始；不在内部换 Scope／cut 重试，不静默扩权或缩小。
6. 披露前，各 owner 锁外再授权，随后检查本方可读、运行／上下文有效及第 5.3 节的关系和联系门。
   Retained 另检查每个所需 pin 仍在期限内；CurrentVerified 则按 B→A 的顺序核对 current=cB、current=cA。
   没有 B 时只核对本来源。任一确定改变为零结果 ErrReadCutExpired；失联和拒绝仍按第 5.5 节分类。

CurrentVerified 首页及每次续页、同 Cut Resolve／List 都使用步骤 6 的 current 复核。
后续调用先对原范围重新授权，再从各来源 current 捕获临时视图，要求恰好等于固定 cA/cB；
计算后仍按 B→A 复核，不能选新 cuts 或拿已开启来源的旧 pin 补救。
临时视图必须一直存活至本次计算与披露检查结束，随后释放。最后 B→A 复核是独立检查时点，
不承诺返回瞬间全部来源仍 current；在单次运行 cut 不回绕的前提下，可证明所用组合曾共同成立。
旧来源重启导致运行绑定不符，不能仅凭相同持久 cut 恢复上下文。

B 作为入口时先独立授权取得 incoming／ParentContact 候选，再按上述 A、B 捕获与复核。
最终 B 视图中的联系、关系须与候选匹配；变化不自动重定向。
为解释上行路径而取到的 A 记录只供获准的必要前缀，不能因协调器已接触就直接披露。

这不是同一全球时间戳、跨 Store 原子事务或“返回时仍最新”。
Retained 初次捕获期间 A 写入可能导致重开，成功封存后普通写入不使旧页过期；
CurrentVerified 的来源变化则要求重查。A 新 Range 已 Commit 而 B 尚未安装 matching Admission 时
不能建立新组合；已封存 Retained 仍可用旧 Range 与旧 B 内容，但必须通过当前关系效力门。

首页之前固定来源和 coverage：Closed 表示来源已验证并纳入固定切面，
只有 Retained 跨请求保留其视图；Excluded 表示未闭合的不透明缺口。
首页后不得加入来源、替换切面／pin 或将 Excluded 升级；后续操作只使用原 Cut 范围内的 Closed 来源。
必要 Scope／Basis／上溯链不闭合时不发布半成品 ConnectedCut；扩大范围必须新建上下文。

首次创建 ConnectedCut 的操作决定其读取范围；持有 owner 临时或保留 map 都不等于公开全 owner 读取：

| 创建操作 | 可复用的事实读取范围 |
|---|---|
| QueryRegionPath | 实际 Scope 及完整后代；只有已验证来源可读，Excluded 仍是缺口 |
| ListDirectPath | 目标 Anchor 及其直接子记录／邻接，不含任意后代 |
| ResolvePath | 已验证路径前缀与终端；NotFound 还固定作出缺失判定的父邻接依据 |

额外祖先／挂载前缀只供解释与效力核对，不扩张内容读取范围。
at 超出范围返回 ErrInvalid；范围内必要来源 Excluded 或未纳入则 ErrUnavailable，
不得报 NotFound 或现抓另一来源。Query 后精确读命中合法；
只 Resolve M 后，不可因已有 A pin 就用同 Cut 新增 B 并列举其内容。


### 5.3 固定事实，不冻结权限或当前连接效力

每页、路径精确读、直接列举都重新检查本次 LocalCallers 和各 owner 权限。
快照只固定空间事实及其组合依据，不冻结 Allow。
它也不能让已经关闭的挂载入口继续向外披露历史内容。

| 首页之后的变化 | 已封存查询的处理 |
|---|---|
| 普通 create/update/reparent/delete；M Pose／Parent；合法 Range／Admission 更新 | Retained 使用旧视图／Scope；CurrentVerified 确认任一来源 cut 改变则 ErrReadCutExpired。新查询读取新提交 |
| A 不再 Active 或 exact MountKey 不同 | 当前连接门关闭；必要上下文失败，普通后代按缺口规则处理，不借 B 的旧 Active 继续成功 |
| 已证实依赖的指定 NextHopRevision 改变 | 该 ConnectedCut 明确失效，ErrReadCutExpired；底层事实快照不因此改写 |
| 固定地址不可达，无法取得其所有者的当前联系证据 | 不可用／后代缺口，不能猜测地址已变或 snapshot 已过期 |
| Retained pin 到期／释放；任一模式的运行结束、组合元数据释放或丢失 | 整页 ErrReadCutExpired，不用相同 Ref/cut 或 latest 复活上下文 |
| 权限撤销 | 拒绝披露，必要 Scope／Basis 为 ErrDenied，普通未知后代可终止为 Partial |
| 指定地址接错 Core | 不接受其数据，不能把同名地址当作原来源或更新事实归属 |

**已确定取舍：**仅 Retained 跨普通写入和范围更新继续旧分页；撤销仍关闭 connected-space 的当前入口，
地址修订仍作废依赖该方向旧联系的连接查询。故障或维护可能要求重新查询，但不恢复全局 Ref resolver。
每页必须从 A 取得当前挂载效力，不能仅信 B 的旧接纳；
必要双向联系的修订也须由保存该方向的 owner 核对，不能由传输缓存猜测。
每次披露的各边界检查时点独立，不声称 Realm、A、B 跨 Store 原子撤权／撤销；
最终检查之后仍可发生撤销／到期，不承诺调用者收到时仍获准或未到期，已返回旧页不被追溯改写。

授权矩阵沿用整段可披露 Scope profile：

| 行为 | 本地／A 要求 | B 独立要求 |
|---|---|---|
| QueryRegionPath | 实际 Scope 的 query-region；在该 Scope 外解释入口／上行路径的必要前缀 exact-read | Scope 包含 M 内容时 (B,M) query；Scope 是 B 内 S 时 (B,S) query |
| ResolvePath | 解释路径所需前缀及终端 exact-read；挂载入口另有 mount-read | B 路径／终端 exact-read；外部 M 邻接可由 (B,M) list-direct 授权 |
| ListDirectPath | 必要前缀 exact-read；目标 Anchor list-direct；挂载入口另有 mount-read | 目标 M 用 (B,M) list-direct；B 内目标用 (B,目标) list-direct |

- 上溯不扩权；最终 Scope 无 query grant 时整体拒绝，不降低深度后返回另一问题的结果。
  先私下解析仅用于授权目标，不提前泄露祖先、cut 或存在性。
  无法解析时沿路径处理每条已知前缀的披露权限；未获准前缀不返回 NotFound／Conflict 或挂载细节。
  本地方向无法形成 Scope 的诊断按本地查询合同的 exact Start 失败门处理；跨 owner 必要前缀
  须先有该 owner 的 exact-read／mount-read 依据，无授权统一 ErrDenied，不能以错误分类泄露拓扑。
- Scope=M 时，M 记录属于 A：query 要求 (A,M) 与 (B,M) query，列举要求双方相应 list；
  ResolvePath(M) 还须 A exact-read。A mount-read 不能替代内容授权。
- Scope 在 B 的较小 S 中时，不要求整个 A/B 的 query；A 必要效力／前缀独立核验即可。
- Scope query 允许其 owner 内闭包摘要和内部几何输入，不传递为另一 Core 的 grant；
  Basis 内部解析不调用额外要求 full-record 的公开 ResolvePath。
- 同 Cut 的后续行为按本次操作授权，不额外要求创建该 Cut 时的 query／list 许可仍存在。
  例如 query 已撤销但必要路径 exact-read 仍有效，允许在原范围中读该记录；反之不得由原 query 代替 exact-read。
  对原 Cut 已固定、但本次不贡献内容的来源，只验证绑定 caller／句柄的运行、cut／pin 与当前联系效力，
  不披露记录或邻接。这不是内容许可，也不能加入来源、扩大范围或恢复 Excluded／terminal。
- 未知后代 B 无权可成为不透明缺口；若 Start／上溯／Scope／Basis 必须依赖 B，则是必要上下文，
  不能包装成普通后代缺口。M 自身在 A 已获准披露，不因 B 无权自动隐藏。
- bool 授权端口不证明撤权后恢复的 ABA；没有全局特权协调器、隐藏分支授权视图或远端认证实现。
  旧快照不能为 Facet 当前存在性／附着授权检查背书，Facet 生命周期规则不在本轮改写。

### 5.4 生命周期、几何与资源

Retained 的各 owner 按各自显式时长保留不可变视图，固定 TTL、不续租、有限准入和并发容量。
每个来源用本地单调时钟判断期限；任一所需 pin 过期整条上下文失效，组合不能延长来源寿命，
无需比较全局时间。末页不自动释放，供同 Cut 精确读；显式释放尽力清理来源 pins，失联由各 owner TTL 兜底。

CurrentVerified 只跨请求保留有界组合元数据，供模式／范围绑定与 terminal Partial 的终止标记使用；
没有旧空间 pins，也不从未配置的 SnapshotRetention 衍生默认元数据 TTL。
元数据数量与字节占用均有上限；容量不足可淘汰无借用的 CurrentVerified 元数据，旧 Cut 随即明确过期，
若仍无额度则零结果 ErrSnapshotCapacity，不无限增长或挤掉承诺期限内的 Retained 上下文。
淘汰后的旧 token／at 不得重建登记以绕过终止标记。因此 current 未变是续页的必要条件，不是无限可用承诺。
当前模式末页也可继续同 Cut 精确读，直到运行结束、显式释放、元数据丢失／淘汰或原 cut 已变。

两模式的组合元数据及并发临时借用均受限，不因来源有额度就无限建组合。
ReleaseConnectedCut 对原 caller 的合法绑定幂等，包括已到期或已释放上下文；零值／错误绑定仍拒绝。
释放先使上下文不再可披露，借用安全退出后回收；CurrentVerified 只清理组合元数据和临时借用。
取消／失败清理未交付捕获，已到期但借用未退出仍计占用；不长时间持锁等待，不影响其他查询或事实写入。
重启不恢复任一模式的运行上下文；配置关闭不等于这些资源校验或单次读取一致性关闭。

- 完整扫描、精确过滤、canonical UUID 排序，不把 mount Range 剪枝作为首轮正确性前提。
- 闭球相交／相切；selector 只筛输出。M 与 B 直接 child 的父边只算一次。
- 合法 UpDepth 不限制向下闭包；整条几何证明链的 1024 edges／65536 rational bits 工作界不在跨 Core 时重置。
- 必要 Scope/Basis 坐标上下文超界为 ErrUnrepresentable；个别候选超界为全查询 ExactWorkLimit。
- 不把舍入中心加原半径当安全排除球；中心无法输出有限 Pose 也可能精确命中。
- limit=1..256 只限输出，不保证扫描、排序、CPU 或时延有同等界。
  资源限额保证明确拒绝与有界保留，不冒充完整执行预算系统或空间深度。
- 快照视图不引用完整提交／回执历史，不增加快照 Store，不长期占用 bbolt 事务或跨 Core 锁；
  P2 持久日志及其恢复校验仍保留，未在此删除。

### 5.5 完整性、失败与续页

Gap 是同一 query/ConnectedCut 的全范围摘要位：ExactWorkLimit、UnresolvedCoverage。
数值界未排除的分支及所选 Scope 内失联／无权的后代均不能当空；Scope 外不属于缺口。
不附带未授权 source 身份、数量或拓扑。

| 情形 | 结果 |
|---|---|
| 入口／Start 上溯／Scope／Basis 必要来源或当前关系无法验证 | 零结果 ErrUnavailable；无授权为 ErrDenied；本方 Store 不可读为 ErrStore |
| 已授权路径明确不存在／查询终端不是 Anchor | ErrNotFound／ErrConflict；无权不披露存在性，断连不冒充缺失 |
| Basis 非 Anchor、不存在或不属于实际 Scope | 授权后统一 ErrInvalid，不细分探测其他空间 |
| 模式确定前普通后代 B 未就绪、拒绝、坏证据或无法联系 | 记 Excluded，其他可验证项 + Partial / UnresolvedCoverage，不使用未获准 B 内容 |
| 模式确定后、首页捕获中已选来源失联、拒绝或证据不匹配 | 整次按原因 ErrUnavailable／ErrDenied／已证实 cut 改变时 ErrReadCutExpired；不移除来源重选模式 |
| 初次 cut 复核确认变化；CurrentVerified 原 cut 已变；运行上下文／Retained pin 过期；依赖的联系修订改变 | 零结果 ErrReadCutExpired，不能把 latest 混入原链 |
| 本方／任一已选来源保留准入、组合元数据或借用额度不足 | 零结果 ErrSnapshotCapacity，不因容量移除来源或切换模式；已承诺的保留句柄／TTL 不变 |
| 某个实际来源未配置保留，但原切面可验证 | 正常 CurrentVerified 查询／分页，不产生错误或 Gap |
| 已贡献 B 续页时无法完成权限／当前效力检查，且非必要上下文 | 丢弃本页全部依赖 B 的项和证据，A 检查后返回 terminal Partial，无 continuation |
| 已贡献 B 的 Retained pin 明确过期／丢失，或 CurrentVerified 证实 B cut 已变 | 整页 ErrReadCutExpired，不把可恢复的 latest 伪装为原来源 |
| 正确来源在合法视图证明邻接缺失 | PathRead.NotFound；不向另一分支搜索同 UUID |
| 已授权组合中 UUID 冲突 | ErrIdentityConflict，不合并、不按地址择一 |
| token 形状／caller／query／范围绑定不符 | ErrInvalid；资源相关检查在授权后，非 nil error 不带暂存数据 |

ConnectedCut 的读取范围不可扩大。首页 at 可选，须仍有原范围及所需来源；
续页 at 必须 nil，固定上下文来自 continuation。
continuation 绑定 Origin／规范路径、Start、UpDepth、Basis、解析后 Scope／ActualUpDepth、
球／selector、各 caller、ReadMode、来源 cuts／运行及可选句柄、当前连接门的精确关系／联系、coverage 与上一页末 UUID。
limit 可调整；改深度必须新开链，不能沿旧排序位置继续。
-1 与当前恰好到根的正数仍分别绑定，不因解析出相同 Scope 就允许互换 continuation。

- 首页 Excluded 后代恢复，也不能补入旧链；要补全就新查。否则新增的小 UUID 命中会被漏掉。
- 两模式都不因更新改写固定 Scope／路径／排序／Gap：Retained 仍用旧视图，CurrentVerified 拒绝已变切面。
  新创建、删除、重挂父的 current 状态不能混入旧链。
- 无缺口且无剩余页才 Complete；有后续页是 Partial，Gap 可为空；数值／coverage 缺口末页仍保留。
  Complete 指累计并集；仅首页 Complete 且空证明所选 Scope 无匹配。
- terminal Partial 只能终止原链，不声称完整；已返回旧页仍是当时合法观察。
  已封存后此类终止保留原来源登记和模式，不是从已选来源集合移除 B 后重选模式。
  组合上下文须在其运行寿命内保留终止／未闭合标记，后续重放、旧页 Cut 或 at 都不能绕过。
  B 原为 Closed 后失去披露依据，即使 pin 尚在或原 current cut 未变，也不能由 at 重新纳入；须新开查询。
  此标记只在内存中保存，不是 Store 事实；并发借用须在披露前核对该标记，
  不能一边已终止来源、一边再按旧 Closed 宣称 Complete；根上下文已失效时不用 Partial 包装。
- 错误页零结果。重开必须丢弃原累计链用于“完整集合”的解释，不能拼接新旧快照。
  无自动重试切 latest、自动改地址、TTL 续租或查询句柄持久恢复。
- 相同 token 重放在有效期与当前许可内稳定；调用者不能把重放页重复计数。
  单一语义查询分成两 Core 后，结果集合、Scope、结构深度和同视图排序必须与单 Core 对照一致。

## 6. 当前合同的明确增量与不启动项

| Phase 1 前的基础假设 | 已交付基础与目标合同增量 |
|---|---|
| 非空 Core 必有一个本地 root | 有根或经明确 incoming 接纳的外部父形态；仍只有一个连接空间根 |
| Core 没有公共 owner 身份 | Phase 1 已交付一个 Core 的稳定 FactAuthorityRef；不新增对象层，不复用 token scope |
| Core scope 只有 ElementUUID | Phase 1 已交付 CoreElementScope(TargetCore, ElementUUID) 及 Realm 编解码／比较 |
| cut 与 Element 提交一一对应 | Phase 1 已交付独立提交序列；P2 已增加控制事件及其重放，继续使用同一个 CoreReadCut |
| children 都来自本地 Store | local 或 exact accepted remote coverage；不可达不是空 |
| QueryPage 只含 current Core cut | P3 直接修订 QueryRegion 为向上深度与可选有期限快照；路径方法组合明确来源，不新增兼容查询入口 |
| Update 只检查本地 children | 活跃挂载 Range 要协调；普通 Pose-only 继续本地处理 |
| 没有挂载联系记录 | 双方向各自指定 NextHop；独立地址 CAS 与持久事件，不干扰空间协调 |

保留主线 D6 的稳定 owner、双侧确认、外部父、admission／projection 分离和唯一 carrier；
但 Core 挂载寻址明确改用本合同的“结构 → 指定地址 → 核对 Core”，不再沿用裸 Ref 解析前置。

共同联系原则已获用户确认，并在本次文档对齐中同步以下主线条款；
该文档对齐本身不作为挂载／组合查询代码或目标 API 的实现证据：

| 受影响条款 | 已对齐的共同联系规则及 Core 限定 |
|---|---|
| D4 不保存 Route／Endpoint | Element 空间记录仍无地址；所属挂载控制记录保存权威指定 NextHop，不是可丢弃缓存 |
| D6 事实身份与委派 | 一 Core 一稳定 Ref，不增加 Authority 对象层；MountKey 固定双方，双向地址独立于归属及空间 tuple |
| 原 D8 按 Ref 解析 RouteRegistration，Core／Facet 强制同一 Route 合同 | 两者都从已知关系直接取得本方向指定下一跳，核对身份及关系；不设裸 Ref resolver、隐含全局目录或备用解析路径 |
| D9 入口与路径 | 初始入口明确连接信息；跨挂载正向访问和反向验证沿已知关系取地址，不凭 UUID／Ref 全网定位 |
| D10 来源与发现后读取 | Source 标记归属和 cut，不承担寻址；用原入口、完整路径及 ConnectedCut 读取发现的元素 |
| D11 可达性不混入事实 cut | 运行时是否可达仍独立；关系指定了哪个地址属于持久控制事实，变化推进该 Core 的 cut |
| D12 Route 指向新 writer | Core 后继设计改为显式指定地址的维护；更地址不等于 fencing、owner replacement 或迁移完成 |
| 原执行计划的统一 Ref Route 前置 | 不再作为 Core／Facet 后继必做项；跨机器传输、身份认证、嵌套及规模证明仍需后续合同 |

这是定向替换，不是把旧 Core／Facet RouteRegistration／referral／lease／多 Endpoint 解析统统延期保留。
Facet 的正向联系保存在 Core 的逐 Element 附着记录，反向联系由 Facet 按 exact AcceptanceKey 保存；
两者复用稳定 Ref、指定 NextHop、独立联系修订与单 owner 提交原则，不共享可变 Ref→地址表。
其字段、首次接纳、维护权限与 F0 读取规则由 Facet 合同定义，不复制到本合同，也不要求先完成 Facet 才能验证挂载。
尚未实现的网络认证不能以 Ref 字符串相等或进程内对象核对冒充。

D7／D10 的整段本地授权 profile 不能代证隐藏分支的 view vector／opaque closure 协议。
本轮不改其上位目标；也不启动 Facet 本地闭环、完整开放联邦、迁移或网络服务化。
既有本地合同与已交付 Phase 1 增量继续约束当前实现；主线和计划已统一共同联系原则，
P2 与 P3 均按各自明确 Start 完成受限实现及完整验证，并分别取得用户验收与单独提交授权。
不维护两套并行联系规范，也不建立旧形态兼容层；P7.3a 已移除 Facet 类型解析端口，且不恢复事实 Ref → 地址解析。

## 7. 实现分解与验收

Phase 1 共用最小基础已单独交付；P2 控制与 P3 组合查询均已完成验证并经用户接受，各自单独提交。
执行分段与默认顺序只由[开发计划](development-plan.md)维护；
本合同的必要证明范围如下，不形成另一份调度计划：

1. 稳定 Core 身份／scope、双向指定地址与统一事件历史：Store 校验和原本地回归成立，作为后续挂载控制的基础。
2. 建立、普通 B 内容、范围协调与撤销：原始结果、写门、恢复和故障注入闭合。
3. 路径解析、直接列举、跨 Core QueryRegion：依赖已验证的控制关系，并同时证明 cuts、权限、分页与缺口。
4. 双 durable Core 黑盒示例与全仓验证，结果交用户验收；不以内部辅助调用代替公开路径。

最低必测集：

| 类别 | 必须能证伪的行为 |
|---|---|
| 首次挂载 | 空锚／空 B、非空拒绝、无伪 root、A 不再接纳本地孩子、B 未激活不写 |
| 激活竞态 | B 按旧范围准备后，A 不能普通 Update 缩小再激活；CAS 和精确保留状态不能绕过 |
| 范围竞争 | 缩小与 B 新增／移动竞争不能共同越界；相切接受；失败不自动扩大或改写 |
| 非范围更新 | M Pose／Parent-only 不暂停 B、不改 B 局部记录；新查询用新投影，已保留查询用原投影 |
| 写门 | 所有新 create/update/reparent/delete 被 gate 拦住；旧回执可重放；被阻止请求不占用 OperationID |
| 终局 | A Abort 不改判；B 不超时解锁；取消先于 Prepare、迟到 Prepared／Closed 不恢复旧状态 |
| 重启／落盘 | 每个持久步骤前后单边崩溃、响应丢失；latest Store 恢复同一状态；previous/next 之外 poison |
| 撤销 | A 关闭入口与 B 停写分别证明；失联不假报完成；Detached 保留内容且不能普通删／重挂 |
| 权限 | A/B 独立 caller 和 owner scope；错 owner grant 无效；撤权后不披露回执，但既有收尾义务可恢复 |
| 空间发现 | A 兄弟到 B、B Basis 到 A 兄弟；巨大实体中心在外但相切；M 只出现一次；结构深度及精确工作界跨边界不重置 |
| 空承载侧入口 | 空 B／仅有直属 Entity 的 B 可从外部 M 进入；M 摘要及完整记录来自 A，各自授权；不造 root、不漏 Scope |
| 查询切面 | 首次组合与普通并发写；Retained 旧页稳定，CurrentVerified 逐次复核；可选配置、期限、容量、释放与重启；不伪全局 Revision |
| 缺口／分页 | 根失败、B 后代拒绝／断连、真实空、数值缺口各自正确；恢复源不加入旧链；末页缺口不消失 |
| 身份 | 换错 peer、新空 Store、外部 M 撞号、已观察跨 owner UUID 冲突明确拒绝，不按 endpoint 选事实 |
| 无隐式路由 | 没有 Ref→Peer／地址查找接口时，正向查询、发现后沿原路径读、B 反向验效和双侧恢复全部可达 |
| 地址修复 | 双向地址独立授权／CAS；Pending／写门关闭时仍能修复；对端离线可提交；不改空间、归属、admission 或原意图 |
| 地址重放 | Prepare 后改地址再重试／重启；原操作不冲突；迟到消息不覆盖新地址、不复活旧关系，结果重放不改状态 |
| 地址与查询 | 改地址使依赖该方向旧联系的 ConnectedCut 过期；不可联系时不猜过期或 absence；旧链不自动换地址／绕行 |
| 回归 | 原本地 test/race/vet、Realm scope 存储、Facet→Core 机械适配、依赖检查与公开示例 |

### 7.1 已做的设计检查，不等于实现验证

前一轮范围协调有限模型检查了 208 个可达状态、624 条迁移；其范围仅为一次缩小和后续撤销，
证明力度不超过该抽象模型。去掉超时禁止、不可变终局或关闭记录，均可构造反例。

本轮分别完成控制状态／Store、身份／权限和组合查询的独立只读审查。
在有期限快照修订之前，曾对“双侧捕获、B 复核、A 复核、一次 A 控制变化、一次 B 变化”枚举 30 种合法次序：
10 种通过 cut 复核的次序均满足该模型的来源组合见证；
若 A 控制变化不推进 cut，则出现旧关系通过复核的反例。
这份旧捕获模型仅保留为设计过程证据，不验证本次运行内保留、首次封存与后续披露规则。

以上不覆盖真实 Store 故障、任意长消息序列、网络认证或活性，不是生产测试或完整协议证明。
上述是实现前的设计证据；P2 当前实施状态以开发计划为准，不以这些模型代替代码验证或自动验收已有 QueryRegion。
本次指定下一跳修订另经地址生命周期、全访问链和主线差异的只读审查；
上述有限模型没有验证新增地址状态机，新增验收行必须在实现中通过故障与并发测试。

### 7.2 指定下一跳的反例审查

| 反例 | 本合同要求的结果 |
|---|---|
| 指定地址误接另一 Core | 预期 Ref 不匹配，拒绝其数据；不能按地址修改归属 |
| 同一 Core 正常重启或更换地址 | 保留 Ref；重建连接或独立修正地址，不改 Element 与挂载关系 |
| 范围协调暂停时原地址失效 | 本地仍能维护地址，再恢复原意图；不能要求先解除写门才能修地址 |
| 地址更新后收到旧 Prepare／Finish／旧更新重试 | 只处理精确意图或重放回执，不覆盖当前地址，不重开旧关系 |
| 查询分页中改地址 | 不能换端点拼接旧页；确认所依赖联系修订改变则过期，证据不可得则不可用／缺口 |
| B 尚无自有 Anchor | 以已接纳外部 M 进入，A 提供锚事实，B 提供其内容，不强造本地根 |
| 两个可写副本持有同一 Ref | 单靠地址与 Ref 无法判别合法 writer；本轮不支持，须另行 fencing／交接合同 |

这些反例支持职责划分的必要性，不构成对任意部署、攻击、规模或消息序列的正确性证明。
本轮的简洁性来自只沿已知关系连接、复用网络传输；不是承诺无需地址维护、远端认证或后续部署约束。

### 7.3 P3 深度与快照修订的反例门

| 反例 | 目标合同必须满足 |
|---|---|
| B 内 Start 上溯到 A，再命中 A 的另一兄弟 | 计真实父边，返回 ResolvedScopePath；原入口同 Cut 读回，不凭 Ref 定位 |
| -1 到真实根／只是联系不上父方 | 前者完整选择根并报告实际层数；后者必要上下文失败，不偷缩范围；Path.Up 不接受 -1 |
| B 内以 -1 上溯时撤权、数值上下文超界或快照到期 | 按相应必要上下文失败；不把最后见到的祖先当成功 Scope，不用 latest 补链 |
| 加深查询后出现排序更早的元素 | 新建查询；拒绝旧 cursor，不漏项 |
| A 先 pin M 的旧 Pose，随后 A 移动 M、B 新建 E，再 pin B | A 最后 cut 复核失败，不发布从未共同存在的“旧 M + 新 E” |
| Retained 的 B pin 后普通写入，A 未变 | 首次组合可成立；后续页使用旧 B，不额外要求 B current 不变 |
| 组合封存后 Range 合法变化、祖先换父或元素删除 | Retained 从 pins 读旧事实；CurrentVerified 拒绝已变 cut，新查询读取新事实 |
| A 已撤销，B 仍保存旧 Active | 当前 A 效力门拒绝，历史 pin 不延长挂载访问 |
| 地址更新／未知地址失联 | 已证实联系变更才使 ConnectedCut 过期；不知变化则不可用／缺口，不绕行 |
| 页末仍要读完整记录，或快照已到期／owner 重启 | 末页不自动释放；到期／重启明确失败，不用相同 cut 重建旧句柄 |
| 模式确定后快照容量满／一侧捕获后另一侧失败 | 整次明确失败，不移除已选来源；清理未交付资源，写入与原句柄不被挤掉 |
| 准入后撤权、借用中到期、B 披露前失联 | 每页重新授权和寿命检查；零结果或 terminal Partial，不披露未验证缓存 |

| A/B 均关，或仅一侧开启保留 | CurrentVerified，双方不留跨请求空间 pins；原 cut 不变时多页正常完成 |
| B 仅提供必要证明但关闭保留 | 仍选择 CurrentVerified，不因其没有输出记录而省略该来源 |
| CurrentVerified 首页后 B 写入 | 续页及同 Cut Resolve 拒绝；不能用 A 旧 pin 加 B latest |
| 两侧开启而必要一侧容量满 | 明确容量失败，不伪装未配置或静默降级 |
| A 开、B 关，选 CurrentVerified 后 B 临时计算额度不足 | 整次容量失败，不能删掉 B 再改为仅 A 的 Retained |
| Retained pin 到期但 current 未变 | 仍过期，不重建句柄或切换模式 |
| CurrentVerified 元数据被释放／淘汰后 B 恢复 | 旧 Cut 过期，不复活 Closed 来源或丢掉终止标记 |

先前已作内存中的有限枚举：154 棵最多 6 节点的有根树、5,039 组 Origin/Start 与 44,484 个深度案例，
检查根处饱和、Scope 路径合成及相对命中读回；不包含真实权限、Store 或网络。
对首次 A pin、B pin、A 复核及各一次写入枚举 20 种合法次序，10 种通过复核的组合均有共同存在见证；
去掉 A 复核出现 1 种不存在共同状态的组合。此结果仅覆盖 Retained 首次捕获的有限模型，不覆盖可选配置或 CurrentVerified 的逐次验证，也不是任意并发的形式证明。

以上是设计反例与待实现测试要求，不是代码测试通过声明。

本次可选保留定稿另作有限枚举：对 A 捕获、B 捕获、B 复核、A 复核和各一次写入枚举 30 种合法次序，
10 种通过 CurrentVerified 复核的组合均在模型中具有共同存在见证。四种配置组合与两侧各自保留容量
组合共检查 16 个分支，未启用不被当成失败、开启但保留容量不足不降级。
该抽象模型不含真实授权、网络、Store、资源回收或任意并发，不能代替实现测试。
