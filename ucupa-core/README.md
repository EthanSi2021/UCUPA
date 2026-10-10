# UCUPA Core

Core 是 Element 空间真相的唯一 owner：它持久化稳定 UUID、Kind、局部位姿、
Range、Anchor 结构、Element Revision 与 owner-local read cut。Facet 之外的业务属性、
身份和权限决策都不属于 Core。

本目录实现[《Core 本地空间真相实现合同》](../docs/architecture/core-local-spatial-truth.md)。G1 与显式 Range 更新
修订均已通过验证和验收；后续执行状态见[开发计划](../docs/architecture/development-plan.md)。
本目录还实现[本地查询合同](../docs/architecture/core-local-query.md)对应的 QueryRegion，
其 P3 修订已通过完整验证并经用户接受，证据以开发计划为准。
其 current 分类只由[架构权威清单](../docs/architecture/README.md)维护。
Stage 1 · Phase 1 的最小基础已实现并验证：Core 稳定身份、owner-qualified scope 与提交历史。
相关证明见[基础测试](tests/instance/foundation_test.go)。P2 挂载控制增量按
[双 Core 挂载合同](../docs/architecture/core-mount-contract.md)实现，交付／验收状态由开发计划维护；
P3 新增启动配置、临时读取上下文及跨 Core 读取面也已完成实现、完整验证并经用户接受。
验收限于合同声明的可信本地双 Core，不表示网络、嵌套或完整开放联邦已完成。

## 代码组织

Core、Facet 按同一份[工程组织规范](../ENGINEERING.md)归位；模块根只作导航，
不保留旧 Go 包、类型别名或逐方法转发。迁移清单、固定 Store 证据及完整验收状态
统一见[开发计划](../docs/architecture/development-plan.md)，不在 README 维护第二份施工日志。

| 入口／职责 | 实际位置 |
|---|---|
| 创建、打开与关闭；取得本地操作入口 | [instance](instance/instance.go) |
| 公开参数、结果和 opaque 值；Connected/Federation 主题合同 | [contract](contract/spatial.go)、[connected](contract/connected/connected.go)、[federation](contract/federation/federation.go) |
| `c.Elements()`：元素创建、读写、投影与公开变更 | [element](internal/element/operation.go) |
| `c.Queries()`：本地查询、快照、续页与回收 | [query](internal/query/operation.go) |
| `c.Connected()`：可信本地跨 Core 查询 | [connected](internal/query/connected/operation.go) |
| `c.Mounts()`：挂载、对端接纳、地址维护和恢复 | [mount](internal/mount/operation.go) |
| `c.Facets()`：Core 侧 Facet 联系与控制 | [facet](internal/facet/operation.go) |
| `c.Writer()`：同原始 Store 的单写交接控制 | [handoff](internal/handoff/operation.go) |
| `c.Transfers()`：源端控制与恢复；父侧 carrier 控制及无外部边的三方本地恢复；尚非完整转移驱动 | [source](internal/transfer/operation.go)、[carrier](internal/transfer/carrier.go)、[carrier recovery](internal/transfer/carrier_recover.go) |
| `c.Access()`：公共请求封套 | [access/entry](internal/access/entry/request.go) |
| `c.Federation()`：有限来源读取与披露 | [federation/source](internal/query/federation/source/operation.go) |
| 同一事实聚合、候选提交、历史、发布与不确定结果恢复 | [commit](internal/commit/owner.go) |
| 现有数据库字节格式、写事务准备与资源生命周期 | [store](internal/store/transaction.go) |
| Element／关系的有限存储值编码；不持有数据库或正式状态 | [store/record](internal/store/record/element.go) |
| 转移 journal、清单与接续证据的唯一规范编码；不负责提交 | [transfer/encoding](internal/transfer/encoding/journal.go) |
| 公共上下文、观察与退休计量；不另存事实 | [registry](internal/access/registry/registry.go) |
| gRPC 配置、注册和网络宿主；实际 RPC 分组在通信私有包 | [transport/grpc](transport/grpc/server.go)、[entry](transport/grpc/internal/entry/service.go)、[admission](transport/grpc/internal/admission/service.go) |
| 指定地址的出站挂载／联邦连接；消费侧客户端 | [mountconn](transport/grpc/mountconn/mount_connections.go)、[federationconn](transport/grpc/federationconn/federation_connections.go)、[SDK](sdk/go/client.go) |

实例返回真实操作，不复制整套业务方法。空间、挂载、Facet 联系共同使用唯一提交 owner；
快照、访问资源、网络准入与客户端连接按各自原有生命周期管理，不变成第二份空间真相。
`c.Access().PublicCall(...)` 是本地公共封套，网络适配仍独立核对身份、caller、联系及响应。

协议源和字段约束见 [api](api/README.md)，生成表示在 `bindings/go/protobuf`；
独立宿主见 [cmd/ucupa-core](../cmd/ucupa-core/README.md)。
测试跟随实际 owner；[公开组合测试](tests/instance/foundation_test.go)使用真实实例和合同，
故障注入仅存在于相应 owner 的测试构建。单包通过或根目录清空不代替完整验收。

## 本地真相模型

- 一个 Core 可以为空；本地 CreateRoot 建立唯一且不可更新、移动或删除的 root。
  P2 也允许空 Core 经明确挂载接纳成为外部父形态，此时不再创建本地 root。
- root 没有 parent 和 LocalPose，独占 UNBOUNDED_ROOT。
- 非 root Anchor 与 Entity 都原子保存
  parent Anchor + LocalPose + BOUNDED_SPHERE Range。
- Anchor 组成无环树；Entity 由一个 Anchor 直接容纳。
- UUID 在 delete 后永久保留 claim，不能跨 Kind 或跨生命周期复用。
- 非 root Anchor 与 Entity 的 Range 均可由调用方显式保持、扩大或缩小。所有最终 tuple
  必须被直接 parent 精确包含；Anchor 还必须覆盖全部直接子 Element，否则原子拒绝。
- 不自动调整祖先或子元素；Range 是保守覆盖，不要求等于最小包围球。

空间数值采用唯一 binary64/projective-quaternion profile。Store 只保存 canonical
表示；包含、SE(3) 组合、QueryRegion 与私有相交 oracle 使用 exact rational 计算。公开
ProjectToAncestor 只在最终 Pose 边界舍入，不保存 WorldPose。

## owner-local 行为

Element 写面是 create、update 和非级联 delete。每次 update/delete 都使用 Element
Revision CAS；每个真正的 Element commit 同时产生新的 subject Revision 和 CoreReadCut。
完全相同的 update 返回 UNCHANGED，不推进任何 token。

ReadElement 是当前 owner-local exact record read。ListDirect 在同一 cut 上按 UUID 完整分页
列出直接 child Anchor 和 Entity；任一 fact mutation 会使旧 continuation 明确过期。
ProjectToAncestor 在一个 cut 上读取并授权整条必要 parent path。

QueryRegion 从 StartAnchorID 按 UpDepth 选择实际 Scope：0 留在起始锚，正数最多上溯指定层数，
-1 上溯至已证明的根；小于 -1 非法。在实际 Scope 及全部后代中，以独立 Basis 为坐标参考
执行只读球体相交查询，不限制向下深度。提升到共同祖先即可覆盖兄弟分支，Basis 不自动
缩窄搜索范围。返回实际 Scope、实际上溯层数、UUID、Kind、Revision 及相对 Scope 的路径。
每页完整扫描，以 UUID 排序分页；
数值工作界缺口明确返回 Partial，不能解释为没有元素。limit 只限制输出条数，不限制扫描成本。

New、CreateDurable、OpenDurable 都显式接收 Config。SnapshotRetention 不填即关闭旧状态保留，
填入正时长才开启，没有默认保留秒数。关闭时仍可查询、分页，但依赖 cut 变化后明确重查；
开启时后续页面固定同一旧视图，普通写入照常持久化，不延迟提交。需要完整记录时，
CurrentVerified 用同 cut 的 ReadElement；Retained 用 ReadElementSnapshot，并独立核验 exact read 权限。
快照有数量、逻辑字节及并发借用限制，到期、释放或关闭实例后不可续用，不偷偷转读最新状态。

每个 mutation 都携带 1..128 bytes 的 OperationID。相同 actor、operation kind、subject、
适用的 canonical input 与 expected Revision 的重试，重放原始结果及原始 cut；同 ID 的
不同绑定返回 `ErrOperationConflict`。成功、UNCHANGED、`ErrNotFound`、`ErrConflict` 和
`ErrRevisionConflict` 均被保留。

Core 用自身稳定身份构造 exact `CoreElementScope(CoreRef, ElementUUID)`，并按合同的披露顺序向 Authorizer 提交 exact all-of
requirements：

- create-root、create-Anchor、create-Entity；
- exact read、ListDirect、exact Scope 的 QueryRegion；
- update subject 与 place final parent；
- delete Anchor、delete Entity。

既有 Element 写入与 exact read／ListDirect 各使用一次 all-of 判断；`ProjectToAncestor` 先对 request 已知的 subject/ancestor
做首轮 all-of，再对同一 provisional cut 解析出的完整 path 做一次 all-of，最后复核 cut。
QueryRegion 对同一 exact Scope 在求值前后各授权一次，再复核 cut；整段查询许可不自动授予 exact read。

授权发生在状态披露、OperationID replay 和 mutation 裁决之前，且外部 Authorizer
永不在 Core 锁或 Store transaction 内调用。实例装配把该 Authorizer 绑定到本 owner
当前持久 Realm 联系：断连、错 Realm 或无法核验返回不可用，不压成拒绝或不存在。
Core 不导出无 actor 的 existence probe。
Composition 以显式 service actor 调用 ReadFacetProposal／ReadFacetAttachment，只投影实际 Core cut、存在性及附着证据，不披露空间记录。

## 双 Core 本地挂载控制

P2 只支持一条挂载：A 拥有空的普通锚 M，空 B 接纳 M 下方内容。
M 本身仍归 A；B 只保留外部父上下文与已接纳 Range，不复制 M、不占其 UUID，也不造伪根。
B 对直接内容继续做精确包覆校验。

控制接口分为四类：`ApplyMount` 提交建立／修改／撤销；`ReadMount` 与
`ReadMountOperation` 只读状态及原始结果；`RecoverMounts` 推进已有未完成义务；
`UpdateMountNextHop` 维护本方向指定地址。恢复是显式的一轮推进，没有后台循环或超时自动解锁。
可信装配使用 `ConfigureMountTransport` 和受限 `MountPeer`，应用不能以这些本地端口冒充网络认证。

范围修改先关闭 B 的新写门，再由 A 作出终局，B 确认终局后开门；
断连保留待处理状态。精确旧回执仍可重放，因写门拒绝的新请求不占用 OperationID。
普通 M Pose／Parent 更新不改 B 的局部事实；M 的 Range 变化必须走控制操作。
撤销明确区分 A 关闭入口与 B 停写，Detached 保留 B 内容，不等于删除或迁移完成。

双方使用各自已建立的 caller 与 owner-qualified scope。新提交重新检查原调用者权限；
精确匹配的既有收尾义务不因随后撤权而遗失。地址与稳定 Ref 一起限定目标，
只用关系指定的下一跳；改址可在控制等待中执行，不改变 Element、MountKey 或 Admission。
Element、挂载意图、地址操作共用本 Core 的 OperationID 命名空间和 CoreReadCut；
纯控制提交不增加 Element Revision。

owner-local 读取面不自动转成组合读取：本地 ListDirect 遇到挂载内容、QueryRegion 的闭包包含远端覆盖，
或把 B 的外部父作为本地查询边界时，明确返回 `ErrMountContextRequired`，不误报为空或完整。
P3 用单独的沿路径读取面明确请求连接空间。

跨模块证明见[双 Core 控制](../acceptance/mount_control_test.go)、
[并发与恢复](../acceptance/mount_recovery_test.go)；各持久步骤的故障与损坏验证见
[提交故障](internal/commit/mount_store_atomicity_test.go)、[历史损坏](internal/commit/mount_store_corruption_test.go)。

## 沿挂载路径读取与查询

P3 的 ResolvePath、ListDirectPath、QueryRegionPath 从显式入口锚出发，沿真实父子关系
读取路径、列举直接内容或执行空间查询。挂载本身不增加结构深度；M 的记录始终来自 A，
其下内容来自 B。只连接挂载关系指定的 NextHop 并核对目标稳定身份，不按裸 Ref 查目录。
可信装配用 ConfigureConnectedTransport、ConnectedPeer 接线，不把本地身份绑定当成网络认证。

首请求固定一个 ConnectedCut 的范围、来源和读取方式，后续页面与同 cut 的路径读取不能扩大
原有可读范围或悄悄加入新来源。必要来源全部启用保留才使用 Retained；混合配置整体使用
CurrentVerified。每次披露仍独立核验各 owner 的权限、当前挂载效力和所用地址关系。
数据快照不是授权快照，Query 许可也不等于完整记录读取许可。

必要来源失联使请求明确失败；非必要覆盖无法验证时明确标记 Partial／Gap，不返回伪空集。
快照或原 cut 过期要求重查；已成立的上下文失去非必要覆盖时终止续页，不用恢复后的内容拼补旧结果。
ReleaseConnectedCut 释放上下文；未主动释放的保留状态按期限安全回收。
这只是单条挂载、两个可信本地 Core 的实现范围，不是跨机器或任意嵌套联邦。

## 同原始 Store 的单写交接

P7.1 已实现并通过完整验证，交付／验收状态见开发计划；依据为
[P7 总合同第 6 节](../docs/architecture/owner-transition-contract.md#6-同-owner-writer-handoff)。
`c.Writer().InspectWriter/HandoffWriter` 返回实际控制结果，SDK 的 `Writer()` 使用同一 gRPC 服务。
控制依赖已启用的 PublicAccessPolicy 与可信授权 view；每次独立检查
`AuthorityScope(c.AuthorityRef())` 上的 `ucupa.core.writer.inspect` 或 `ucupa.core.writer.handoff`，
普通元素、closure 和只读委托权限不能替代。检查状态与精确重试不会因源已 fence 而被一并关死。

`Config.Writer` 只含交接开关与可选安装资格；开关在新建 durable Store 时随初始事务记录，普通重开必须保持一致。
交接在原事务内关闭当前 epoch 并固定 final cut，后续写和旧读取／快照／观察不再披露事实。
指定实例独占打开同一原始 Store、验证私有安装资格后推进一次 epoch；身份、UUID、事实、签名材料和
原业务操作结果保留，普通重启不推进 epoch。新建内存 Core 不接受 Writer 配置。
交接关闭的 Store 不能事后开启，不靠启动时自动补值；这不是复制库、回滚或失联自动接管。
启动字段、秘密保管和网络调用见 [Core 宿主](../cmd/ucupa-core/README.md#单写交接)。

## Durable Store

New 建立内存 Core；CreateDurable/OpenDurable/Close 管理同语义的 bbolt
Store。创建时由可信初始化给出 RealmID＋local 地址并先行核验后绑定身份；重开只从
原 Store 恢复并核对联系，配置不能覆盖或补建。唯一精确 marker 是：

    ucupa.core.local@9c4f2a6e8b1d7350f6a3c9e2b7d4f1a8

New／CreateDurable 各生成一次独立 `FactAuthorityRef`；OpenDurable 只恢复它，不重新生成。
`AuthorityRef()` 供可信本地装配读取身份，不是可用性证明或远端认证。
Store 原子持久化 self Ref、私有 scope、持久 Realm 联系、current cut、记录、tombstone、Revision、OperationID binding、
retained outcome 与按 cut 排序的提交序列。Element 提交和控制推进共享该序列；
控制事件持久保存原输入与外部结果，使用同一转换器恢复控制状态，不另维护可变事实副本。
拒绝、不变及精确重放不追加 fact commit，但需要保留的原始结果仍入库。
reopen 重放同时验证全部成功与非变更回执，
并在每次 persist／reload 核对原 self Ref、私有 scope 与持久 Realm 联系，缺联系记录的 Store 拒绝打开。不存在旧 marker
reader、迁移、兼容分支或 predecessor-profile error。

包内线性 scanIntersecting 是 QueryRegion 与未来空间索引的 conformance oracle，不是公共 API。
P3 的快照和连接查询上下文只存在于内存，不增加 Store 字段或持久游标；Close 对内存和持久化
Core 都结束本次运行，重新打开不会复活旧查询句柄。P6 的公共联邦已支持按指定挂载联系嵌套发现、
读取、列举与有限观察，网络由 transport／SDK 和独立宿主适配，不进入 owner 内核。
真实跨机器环境尚未验证；不实现独立 route/referral、Facet successor、永久事件流、空间索引、
displayName、文本路径或生产运行面。当前网络交付与测试边界见[开发计划](../docs/architecture/development-plan.md)。
