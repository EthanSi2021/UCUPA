# P7 归属连续性与生命周期总合同

**状态：已确认的 P7 领域合同；P7.1–P7.4 的首批支持 profile 已实现、验证、验收并提交。current 精确范围由架构权威清单声明；outgoing 外部边正向接续仍为按需条件合同，不能由首批交付推断已支持。不保留并行方案。**

本合同在 `12502b35` 获接受，完整正文以历史 `b9a1a96f` 保留版本为恢复依据；撤回当时的精简重写稿。
2026-09-23 的范围修订区分首批与按需支持，不删去已支持形态的命令职责、持久证据和失败语义；
外部边正向接续的规则保留为未来启用时的条件合同，不再是首批 P7 验收门。代码从重构完成提交 `35f9e6f3` 重新实现，
不恢复旧 P7 源码，也不沿用其实现状态或验证结论。当前基线与实施顺序由开发计划维护。
文中的 FacetAttachment、AttachmentRevision、ProposeAttachment 等描述关系、修订与动作，
不要求恢复旧公开符号；Core 侧关系采用已确认的“Facet”命名，实际声明随相应切片同步。

本文落实[主线 D12](ucupa-concept-mainline-baseline.zh-CN.md#d12-支持稳定-uuid-的-owner-transition但接受明确不可用窗口)，
并接续 D4 的提交／原操作恢复、D7 的独立授权、D10 的完整性和 D11 的观察连续性。
网络复用[P6 合同](network-access-contract.md)，阶段状态只见[开发计划](development-plan.md)。
本文定义 P7 全部业务边界、命令角色、转移证据、失败语义和验收门；Go 声明／Protobuf 字段号由实现映射，不另立语义。

生效说明（2026-10-02 文档对账）：第 6 节的同原始 Store writer profile、第 7 节首批四种 Core
转移、第 8–9 节的有界完整 namespace replacement／重新接纳／关系恢复／退役，以及第 10 节
对应读取和终末观察闭环，已按 P7.1–P7.4 交付。共同命令、授权、持久化与验收条款只在这些
支持形态内生效；第 1.2 节未启用的外部边正向接续仍不在 current 范围。证据见唯一开发计划。

## 1. 目的与范围

P7 使事实更换写入实例或负责方时，保持 ElementUUID、已确认事实和原写入结果的可恢复性。
允许明确且可能持续的不可用窗口；不承诺零中断、自动失联接管、任意副本恢复或全局原子迁移。

| 转移 | 保持不变 | 必须改变或重新建立 | 唯一切换事实的 owner |
|---|---|---|---|
| 同 owner writer 交接 | AuthorityRef、UUID、空间／内容、原操作历史 | writer epoch、运行期凭证；地址可不变 | 该 owner 的原 durable commit domain |
| Core Element／coverage 转移 | ElementUUID、已确认事实；仅按明确最终 tuple 改空间 | CoreAuthorityRef、placement、目标授权和读取依据 | 原 Core 的 release／successor |
| carrier replacement | parent 挂载锚 UUID、转移内容 UUID；结构可不变 | carrier、matching acceptance 和联系 | parent Core 的 DelegationBinding |
| Facet namespace replacement | ElementUUID、完整当前键集合及 exact payload | FacetAuthorityRef、matching acceptance 和联系 | Element 当前 Core 的 FacetAttachment |
| 同 Facet 重新接纳 | FacetAuthorityRef、已有 namespace 内容 | 新 Core／proposal 对应的 AcceptanceKey | Element 当前 Core 的 FacetAttachment |

endpoint 改变、普通 reanchor、mount revoke、Element delete 仍各自独立，不隐式触发转移。
不新增全局 transition service、总 Realm、独立 Ref 解析器或跨 owner 共享数据库。
完成 writer 首个切片不等于整个 P7 完成；全部交付门见第 13 节。

### 1.1 与既有合同的接替范围

已交付的首批 profile 只接替以下明确排除项；未启用的按需 profile 不能借此宣布支持：

- 挂载合同中“无 writer handoff／replacement、撤销后不可重新挂接或处置”的限制；原建立、改范围和撤销语义不变。
- Facet 本地合同中“无 content adoption／re-acceptance／authority replacement”的限制；原 F0 与普通写入收敛不变；类型职责遵循 P7.3a 已交付的不透明 TypeRef／Payload 合同。
- 公共读取合同中转移造成的来源失效与 terminal successor 增量；不把普通写入后的快照读改成不可用。
- Core／Facet Store 的 writer／transition 控制记录及唯一现行编码；正常 Element／Facet 真相模型不变。

Realm 原身份、凭据、grant 裁决和 P6 网络认证不被替换；网络只是承载本合同新增命令。

### 1.2 首批支持 profile 与按需扩展

P7.2 的首批 Core 归属转移支持单个 Entity、无外部挂载边的 owner-local Anchor 闭包、root successor，
以及内容中不含未接续外部挂载边的 carrier replacement。源或目标本身处在已有 incoming 挂载关系中时，
仍须按本合同核对相应 parent 关系、权限和 placement；不得把它当作无关系的独立空间。
上述 Anchor、root 与 carrier 首批 profile 均不包含所选内容内部还需跨 owner 重新接续的 outgoing mount 边；
这不排除已按本合同核对的源／目标 incoming 父关系或 carrier 自身受 parent 控制的挂载关系。
这些是公开可用的支持承诺，不是只有私有事务或单侧 RPC 的演示。每个已支持 profile 都必须完成
源 fence／final cut、目标接纳与安装、唯一 successor／binding、原操作恢复、读取闭合和必要的故障验证。

若所选内容含尚未得到完整接续的外部挂载边，首批公开入口必须在撤回源内容前明确拒绝，
不得把部分成员、旧关系或未完成接纳报告为转移成功。外部边逐关系关闭和新父接续的正向支持
属于后续按实际场景单独启动的 profile；现有私有实现和测试可以保留，但不自动扩大首批公开承诺。
第 7.3 节仍约束未来一旦启用该 profile 时的正确行为；不能以“按需”作为绕过外部边的理由。
本次分层不降低已支持 profile 的身份、授权、单写、持久恢复和错误披露要求。

## 2. 责任与转移身份

- 有授权的外部调用方决定转移对象、目标并提交精确意图；UCUPA 不自行选择迁移策略。各 owner 独立认证／授权，入口服务身份不能替代用户。
- 旧 owner 保存源事实、关闭原提交资格、固定 final cut，保留原结果和不可回退连续性记录。
- 目标 owner／writer 接收并验证事实，明确接纳；切换成立前不得公开或接受普通写入。
- 控制 owner 只对上表已有控制事实作唯一决定，不拥有另一方的事实副本。
- 执行服务传递字节、查决定、完成既有义务；不自行选目标或另起授权。

角色可重合，证据不可替代。writer 是运行期提交资格，不是新 Element 容器；epoch 是提交代次，不是 API 版本。

### 2.1 精确操作绑定

转移以（控制 owner，OperationID）唯一定位；OperationID 沿用非空、最多 128 bytes 的约定。
控制 owner 保存规范输入：种类、原 caller identity、源／目标、精确资源范围、expected Revision／epoch、
最终 placement 与目标限定批准。首次依赖联系时绑定方向和当时 contact Revision，后续地址修复另记。
同 ID／同输入返回原进度或结果；同 ID／不同输入冲突；目标按原转移身份去重，不把重发当新导入。
超时后按原 ID 查决定／恢复，不换 ID、更新 CAS 或重算 tuple 猜测重试。
manifest 和公开证据不含长效凭据、会话、委托秘密、原 Store 签发密钥。

## 3. 公共状态和不可回退决定

    OLD_ACTIVE → 源 durable fence + final cut → UNAVAILABLE
    UNAVAILABLE → 唯一 successor/binding 决定 + 匹配接纳 + 目标安装 → NEW_ACTIVE

这三个状态描述更换 writer／事实承载方的可用性：OLD_ACTIVE 仅旧方有效；UNAVAILABLE 两方都不能作为该事实的 active 来源；NEW_ACTIVE 仅新方有效。
旧方可保留受保护的转移证据／恢复记录，不把旧 placement 作为当前元素。
没有双方先写、稍后删除、shadow Element 或依靠 timeout 推断切换成功。

以下是按转移类型适用的持久里程碑，不要求每类操作都有六个状态、六份记录或六次调用。
同一 owner、同一事务内能够原子成立的里程碑可以合并记录与调用，但须保留精确输入、决定及原结果证据；
不同事务域的控制决定与目标安装不能合成一次已完成，也不能用省略状态掩盖未完成义务。

| 内部持久进度 | 含义 |
|---|---|
| INTENT | 控制 owner 固定输入，允许预检目标；旧事实仍有效 |
| FENCED | 源在唯一提交事务内关写门并固定 final cut；旧事实不再作为 active 来源 |
| ACCEPTED | 目标持久保存完整候选数据、精确接纳及 placement 保留；仍不公开 |
| COMMITTED | 控制 owner 不可回退地提交唯一 successor／binding 决定，不代表目标已经上线 |
| INSTALLED | 目标核对 COMMITTED 后在自己事务中启用，不重选输入 |
| SETTLED | 必要各方记录最终效果、有限清理完成；不授权擦除唯一连续性记录 |

COMMITTED 但未安装仍不可用，不能提前报告 NEW_ACTIVE。
接纳是对精确转移的持久承诺，不是普通内容写权限；承诺后 grant 撤销不抹掉已承担义务。
新增修改、换目标或另一项转移仍须当前授权；新方普通数据读写继续逐次独立授权。

| 类型 | 必要流程 | 不额外引入 |
|---|---|---|
| 同域 writer 交接 | 关闭旧 epoch、固定 final cut 和指定安装；新运行在同一 Store 核对后安装更高 epoch | 数据导出／分块导入、新 owner placement 或跨 Store 接纳，见第 6 节 |
| Core slice／carrier／root 转移 | 源 fence → 完整内容与目标 placement 接纳 → 唯一控制决定 → 安装与结果收敛；首批遇未接续外部边先拒绝，未来启用对应 profile 时逐项接续 | 全局原子迁移；控制 owner 按第 1 节各自确定 |
| Facet namespace replacement | 源 fence → 完整 namespace 接纳 → Element Core 切换 → 新 Facet 安装与结果收敛 | 空间 placement 裁决或只搬部分键，见第 8 节 |
| 同内容 owner 的重新接纳／关系恢复 | 证明旧关系失效，核对保留内容与当前接纳条件，建立新 proposal／acceptance 和唯一有效关系 | 内容重新搬运、旧批准复活或重复执行一次 owner 转移，见第 8.1、9 节 |
| 内容退役／释放控制 | 显式处置、封闭范围的最终证明、核对剩余义务后释放 | 虚构目标 owner、目标安装或 NEW_ACTIVE，见第 9 节 |

重新接纳／恢复从已有失效关系继续；退役以处置完成结束，不套用完整交接状态链。
下文 Fence／Accept／Commit 的规则只约束本类型实际承担的决定；不适用不等于免除当前关系、内容或权限校验。

### 3.1 取消、恢复与授权时点

1. fence 前允许原授权调用者取消，旧方继续有效；fence 后没有通用 Abort、恢复旧 writer 或自动换目标。
2. 控制方在许可源执行 fence 之前关闭自己的取消窗口。其后取消必须取得源的 durable 结果：
   “尚未 fence，且永久拒绝为此转移 fence”。源记录中取消与 fence 同一事务域串行；失联时不能宣告取消成功。
3. fence 前须有源和目标的目标限定预检批准；预检不代表最终内容接纳。各方在外部等待前后及自己的
   Fence／Accept／Commit 事务前重验原 caller/view，撤权重授不能继续旧未决定动作。
4. 源 Fence、目标 Accept、控制 Commit 分别是本方精确授权的不可回退提交点。之后可按已确认义务恢复；
   运行期批准过期不能删除义务，也不能据此扩大批准范围。
5. 已 ACCEPTED 的目标保留不单方超时释放；只有权威取消结果或最终完成／处置证据才能解除。
6. 未决定的用户动作在重启后需重新认证的原调用者精确重试；持久 Principal 字符串不是凭据。
   已决定的安装／收尾可由有限执行身份完成，不伪装用户重新授权。
7. 不确定是否落盘，先查相应 owner 的 durable 结果；断连、取消等待和 RPC 超时都不是回滚证据。
8. 无法继续原目标就保持不可用，不通过“换地址”改目标。后续若需要更换已接纳目标，必须另有能证明
   原目标永久不能启用的转移，不在本轮提供自动重定向。

## 4. 命令、证据和权限

以下区分调用方控制与受限对端协作。Core／Facet 只提供本类型需要的语义入口，不建立通用转移管理模块。
调用方提交意图、查询进度、精确重试或请求恢复，不必逐步编排分块、fence、接纳和安装。
各 owner 推进并持久记录自己负责的步骤，通过受限对端命令衔接；不把流程状态移交给 SDK 或另设协调 owner。

调用方控制：

| 命令角色 | 输入／结果 | 处理方 |
|---|---|---|
| InspectTransition | 精确资源／转移 → 允许披露的进度、比较 Revision、缺失义务 | 各记录 owner |
| BeginTransition | 原 ID、种类、源／目标、范围、expected、最终 tuple → INTENT | 控制 owner |
| CancelTransition | 原 ID、expected → 仅 fence 前的权威取消结果 | 控制／源按第 3.1 节串行 |
| RecoverTransition | 原转移 → 已决定义务的有限一次推进，不新建或换目标 | 对应 owner／已授权执行服务 |
| UpdateTransitionContact | 原转移、方向、contact CAS、新地址 → 独立联系 Revision | 保存该联系的 owner |
| RestoreRelation | former relation、内容 lineage、新 proposal、expected → 精确重新接纳／恢复决定 | 关系控制 owner 与内容 owner |
| RetireContent | exact 封闭范围、显式删除意图／已转移证据、expected → 最终处置证明 | 内容 owner |
| ReleaseControl | matching disposition、expected、无剩余义务 → 解除控制占用 | 挂载／附着控制 owner |

受限对端协作（同域步骤可以直接在 owner 内完成，不为对称增加 RPC）：

| 命令角色 | 输入／结果 | 处理方 |
|---|---|---|
| FenceSource | exact intent、源批准、expected → fence、final cut、适用的内容依据 | 源 owner |
| ReadTransferChunk／StageTransferChunk | 转移、manifest、位置、exact bytes → 有界传输／去重结果 | 源／目标服务，仅实际内容传输 |
| AcceptTarget | 完整内容依据、目标批准、本类型接纳条件 → durable acceptance | 目标 owner |
| CommitTransition | 原转移、fence、matching acceptance、expected → immutable decision | 控制 owner |
| InstallTransition | exact COMMITTED decision、已接纳对象 → 原安装结果 | 目标 owner |

这些命令不是让调用方自行签发 fence、acceptance 或安装许可的入口；未决定动作仍按第 3.1 节取得原调用者有效批准。
跨机器沿用 P6 gRPC／TLS、指定地址、预期目标认证、原请求者目标限定批准和接收方独立授权。
固定 peer 身份只执行精确转移端口，不能夹带任意用户修改。
hash、Ref、OperationID、successor bytes 均不是认证、访问凭据或离线许可。

Realm 仍使用 PrincipalRef + PermissionID + owner-qualified scope，至少分开源 export/release、
目标 import/accept、控制 binding change、状态读取和固定 peer 执行。writer 使用 authority 范围，
Core slice 使用精确 boundary／closure，Facet 使用 exact namespace。
普通 CRUD、Query、MountRead 或 Facet Get 不自动允许导出、交接或退役。
具体 PermissionID／codec 在原 shared values／Realm 登记，不增加权限 owner。
公开 Inspect／恢复证据依照原操作及资源权限披露，不允许枚举别人的转移任务或隐藏目标。

### 4.1 结果与错误

转移进度、业务可用状态和请求 outcome 分开返回；Begin 成功不表示转移完成，COMMITTED 不表示目标已安装。
查询到的状态带签发 owner、转移身份、比较 Revision、已完成步骤和可披露的恢复责任。
INVALID 表示输入不合法，DENIED 表示权限未成立，CONFLICT 表示原输入／CAS／lineage 或保留冲突；
容量不足用现有 RESOURCE_EXHAUSTED，证据不能建立用 UNAVAILABLE，运行期读取连续性丢失用 GAP。
状态名称是本合同语义，wire 复用各 owner 已有错误分类并精确映射，不新增笼统“迁移失败”吞掉区别。
可能已经派发的写入断连保留结果未确定；只有对应 owner 的 retained outcome 能判定是否提交。
原 exact 操作结果可恢复但当前事实已转移时，不把旧成功结果作为当前可写／可见资格。

## 5. final cut、manifest 与原结果

更换 writer 或转出内容时，源 fence 和 final cut 在同一提交锁／事务中确定：先成功的写包含于 final cut，后到写拒绝。
数据 final cut 与 fence／successor 控制 Revision 分开，控制记录不伪装成普通数据改动。
跨 Store 传输的 manifest 绑定转移、源、范围、final cut、完整有序条目和 canonical 内容摘要；
摘要覆盖条目种类、ID、长度及 exact bytes，仅一个无成员清单的文件 hash 不能证明范围完整。
同域 writer 直接核对原 Store 与交接记录，不为交接重新生成全库传输清单；同内容 owner 的重新接纳也不重新导入已有内容。

| 类型 | 必须保存／校验的对象 |
|---|---|
| 同 owner writer | 原 Store 全部事实、claimed UUID／tombstone、控制日志、确认及 retained 非变更结果、scope／签发材料；不经公共导出格式搬运 |
| Core slice | 成员、UUID／kind、Parent／Pose／Range、范围依赖、源 Revision／cut、附着前态及连续性；只转移本 owner 事实 |
| Facet namespace | 完整 live TypeRef 集合、exact payload、相关 tombstone／操作 lineage、原 namespace acceptance／cut |

跨 owner 不复制源签发密钥、Realm grant、运行期 token 或无关资源历史。目标使用自己的 owner／Revision。
源保留旧 OperationID→原 outcome 的恢复权威；目标保存转移与 source-operation lineage 关联用于去重和查来源，
不把源 CAS／回执变成新 owner 的可写凭证。涉及多个 slice 的源操作仍由源按原权限恢复，不泄露无关条目。
“不丢确认写”要求 final 数据与原结果可核查，不保证源失联时每个旧结果都能在目标查询。
唯一原结果恢复证据不得因内容转移而删除；新写使用新 owner context／CAS，不自动重放源命令。

传输总大小、条目数、分块大小、并发及暂存配额显式有界，未知总量不能无限接收。
同位置／同 bytes 可重试；不同 bytes、缺段、重叠、重复 UUID／键、摘要错或额外条目拒绝。
未接纳临时分块可按有限期限回收；已承担 durable 义务的数据不随会话 TTL 清除。
容量不足明确拒绝或暂停，不截断清单、不拿部分 namespace 宣称完整、不回退旧方。
Facet 接纳验证完整键集合、精确 payload 的传输摘要、容量、来源和 lineage；不解析 payload 或执行类型合同。
移除 artifact／pin 不允许跳过集合完整性、重复键、tombstone、原请求／结果或内容摘要验证。
本条保留已确认的不透明 payload 决定；此前以类型制品作为 Facet 写入与持久化前提的条款已被
主线 D5 和公共合同的不透明内容规则取代。各切片的实施与验收状态仅见开发计划。

## 6. 同 owner writer handoff

### 6.1 支持 profile

Core persist／Facet save 继续使用各自的唯一事务和历史校验，本 profile 在同一 Store 内持久保存 writer epoch 与交接决定。
bbolt 独占锁只保护同一个数据库文件，不能 fence 另一份复制文件。

首个 profile 是**同一原始 durable commit domain 内的显式顺序交接**：Store 不复制、不回滚、不替换。
旧运行在 Store 内关闭当前 epoch、固定 final cut 和精确目标安装绑定，拒绝全部后续业务写，停止新披露。
退出前取消在途请求；新运行取得同一 Store 独占访问，校验记录与授权装配，原子安装更高 epoch。
目标安装绑定是受控启动的资格，不是新 AuthorityRef／地址，也不能仅凭自报身份获得。

普通 Open 遇到 closed 且未 installed 的交接记录，只能做受控恢复检查，不能自动成为 writer。
每个提交事务检查运行持有的 epoch，不只在启动或 RPC 入口检查。
空间、附着／挂载控制、Facet 内容／接纳、联系更新和恢复写都受该 gate 约束；
安装所需的控制事务是窄例外，不能夹带普通写入。安装至多一次、epoch 溢出拒绝，旧安装资格不能拿下一代。

正常重启保留有效 epoch、重建运行期上下文；不制造交接。显式 handoff 有不可回退闭门和指定安装，
不能用同一个“自动抢占”分支处理。同 owner 不偷偷换 Realm／授权归属；
首轮明确受控装配核对原依赖，不把仅比较 RealmID 当作恶意装配防护或 Stage 2 联系机制。

### 6.2 不支持的副本接管

两份 Store 副本具有相同 Ref／epoch、各自文件锁，也不能互相 fence。
未经能约束全部提交路径的原 durable fence，不允许启用同身份的另一份副本。
P7 跨独立 Store 的数据移动走下节不同 owner 转移，不复制身份后称为 writer handoff。
备份回滚、灾难自动接管、多副本一致性需具体运行合同，不能由本 profile 代证。

## 7. Core Element／coverage 转移

### 7.1 成员与源闭包

Entity 为最小 slice；Anchor slice 必须列出 owner-local 全部后代与直接边界，不能遗留指向旧 parent 的本地子元素。
外部 mount 内容归另一 owner，不作本地条目复制。含外部边界的 slice 须逐边接续 matching 关系；
不能提供新父侧／承载侧合法证据就不得启用，不能省略边界后宣称 subtree 转移完成。
首批公开支持限于无外部挂载边的闭包；含外部边时遵循第 1.2 节的安全拒绝。
逐边正向接续只在后续明确启动该 profile 后进入对应交付验收。

fence 同时禁止改变成员、祖先配方、外部关系或使 final cut 失效的相关操作；无关 slice 可继续写。
已有未完成挂载／附着控制先收敛或拒绝开始转移，不随 Element 私自搬走未决义务。
源父邻接和 release 在源事务中一致更新。源 slice 从 active 查询退出，保留 successor 与转移进度证据，
不把 fence 时的退出解释为已完成迁出。新查询按第 10 节区分未完成转移、原空间换承载和已完成迁出；
历史连续性记录不等于永久查询缺口，其对外披露仍受权限约束。
claimed UUID 的旧 lineage 不清成“从未存在”，普通 Create 不能绕过 successor 重用身份。

### 7.2 placement、保留与安装

发起者提交最终 Parent、LocalPose、Range；UCUPA 验证，不自动改位姿、扩大 Range 或保持世界位姿。
目标已有 active UUID 或该 lineage 已经被永久删除则拒绝，不允许覆盖或复活。
仅有先前转出的 inactive lineage 不等于永久删除：A→B→A 或多次转移后返回，须验证从已知 predecessor
到当前源的连续 successor 链、当前源 fence 和这次新操作，才能建立新的合法 placement generation。
旧 claimed／successor 不擦除；相同转移重试只返回原结果，不能误当返回迁入，普通 Create 仍不能重用 UUID。
目标在明确 parent、祖先 Structure／Range cut 上校验父存在、种类、无环、完整包覆和 direct-content authority。
parent 为 mount 时另验 exact DelegationBinding／MountAcceptance／联系与写门，不能绕过 parent Core。

durable acceptance 对 UUID、placement 及相关关系建立持久控制保留。
相关删除、Range 收缩、reparent、revoke 或另一转移不得破坏已承诺 placement，冲突明确拒绝。
不持跨网络 mutex 等待；用本地记录及全部相关提交路径校验保证。无关写不全局暂停。
重启恢复保留；不按 timeout 擅自释放，完成或有效取消证明后解除。

原 Core 根据 fence／acceptance 提交唯一 successor；目标只能据此安装。
安装前原转移、目标接纳和唯一决定的签发方／角色必须匹配；网络接收者从指定控制方认证读取决定，
不能凭客户端给出的 hash、自报成功或旧联系缓存启用。目标已持久安装的精确结果可本地重放，
后续关系有效性检查仍遵守原挂载／Facet 合同；重放不获得新提交资格。
目标以自身 Revision 表示新 placement，源 token 不跨 owner 用作 CAS。
Structure 可按明确最终 tuple 改变，也可保持；owner 连续与空间合法两份证据缺一不可。

### 7.3 carrier、外部边与 root

carrier replacement 由 parent 挂载控制串行化：旧 carrier final cut／fence、新 carrier 接纳都成立才切 binding；
挂载锚仍归 parent，不随内容移动。同一关系的 range-update、revoke、replacement 不能分别并发提交。
若后续启用含外部 mount 的 slice 转移，还须旧父关系永久失效、新父 exact delegation 与原承载方重新接纳；
承载方内容不复制，任一边没接好整体只能未完成／不可用。新结构环、重复接纳和越界均拒绝。
承载方须在自己的控制记录中停止以旧父关系提供该分支的写入／有效查询，并固定与新父的匹配接纳；
不能仅由搬迁中的父方改一个指针就让承载方默认授权或继续使用旧关系。

root replacement 必须有旧 root successor，保留 root UUID 和无 Parent／无界根规则。
目标为新 authority 的受控未激活接收状态，不能覆盖已有活动根或拼接独立空间。
未激活数据只是私有传输状态，安装时原子建立目标唯一根与完整接纳空间。
旧根失联且无证据时 unavailable；另建空间不是继承旧根。

## 8. Facet namespace replacement 与重新接纳

namespace 是一个 Element 的完整 TypeRef 键空间，不能移动几个 key 就切换整个 Attachment。
源 Facet 同事务关写门／固定 final cut；Put、Delete、接纳变化和迟到 F0 回查都不能继续提交。
目标验证完整清单、exact payload 的完整性和 lineage，接纳前不 ListTypes／Get／Put。
acceptance 绑定源 namespace、final cut、ElementUUID、目标 Facet 和当前 Core 的 exact 新 proposal。

Element Core 在 old fence、target acceptance、expected AttachmentRevision 都匹配时提交唯一切换。
替换中的新 proposal 保存在精确转移控制记录里，不通过旧的普通 ProposeAttachment 覆盖 ACTIVE／DETACHED；
fence 以前旧关系继续有效，fence 以后明确不可用，只有唯一 Commit 才改变 current binding。
与 Delete、Detach、其他 proposal 及 Core 转移串行化，不能复活已删除元素。
新 Facet 安装后仍作 F0 capture／fresh-check／recheck，不用旧 final cut 代替当前附着效力。
Core 已切换而目标未安装时 unavailable；旧 Facet 永久非 current。

### 8.1 Core 转移后的同 Facet re-acceptance

Core 转移默认不移动 Facet payload。新 Core 初始附着为有 predecessor evidence 的 DETACHED；
只有源确为 NONE 才表示 NONE，不把“内容未恢复”解释为“没有内容”。
新 Core 创建新 proposal。同一 Facet 核对旧 Core successor／existence final cut、新 Core existence／proposal 和
namespace lineage 后给出新 matching acceptance，新 Core 再提交 ACTIVE，不能复用旧 AcceptanceKey。
Facet 在自己事务中将同一 namespace lineage 唯一关联到新 acceptance，不生成第二份可写内容。
仅 UUID 相同不能收养已有内容；Core transfer 可先结束，Facet 失败只使其 detached，不回滚 Core 归属。
恢复 proposal 尚未完成时，普通 CancelProposal、Element Delete 或再次 Core 转移不得抹掉该义务。
因为 Facet 可能已经接纳而响应尚未到达，不能仅凭 Core 仍为 DETACHED 推断取消安全；
应继续精确恢复并激活，或由明确的最终处置协议解除，不能用普通删除代替处置证明。

## 9. 撤销后恢复与退役

revoke／detach 不证明内容已删除、归还或转移。

- 恢复原内容关系：使用新 proposal／acceptance 和原内容 lineage，核对原撤销、未被替换／退役、
  当前范围、权限及联系。不清除撤销历史来复用旧批准／写门／查询依据。
- 内容退役：发起者必须显式选择删除，不由恢复 timeout 触发。内容 owner 在封闭范围中确认所有内容
  已按授权删除，或已交接到有完整 successor 的目标，给出 exact final disposition evidence。
- 释放控制占用：控制 owner 核对处置证据、无未完成义务及外部子关系后才释放挂载锚／namespace。
  非空、失联、处置未知都拒绝；普通空锚删除不能绕过 detached carrier 未处置内容。
- 保留证据：内容字节清理与永久防重复／连续性记录分开。P7 不回收唯一 tombstone、successor、原操作结果，
  不引入通用历史 GC；临时分块和已终止运行凭证可有界回收。

退役不使 UUID 可再分配。内容 owner 永久失联又没有处置证明，保持 detached／unavailable，不编造完成。

## 10. 读取、发现、旧快照与观察

fence 后旧事实不再作为有效当前来源；Retained 保存数据但不冻结 owner／writer 的效力。
相关 receipt、分页、ReadAt 不能跨 fence 继续；明确上下文失效、GAP 或不可用，不返回伪空集。
在途读取在最后披露门核对 fence；已交网络栈的旧响应无法撤回，不承诺按网络到达时间排列，
但旧证据不能据此获得新提交或续页资格。无关资源读取不被无故停止。

新查询以本次解析的 Scope、查询球和已验证 cuts 判断完整性，不以元素曾经属于哪里决定搜索范围：

| 本次查询所依赖的状态 | 读取规则 |
|---|---|
| 转移尚未完成，相关分支仍可能相交且不能安全剪枝 | 根 context 有效时按 D10 返回 PARTIAL；根依据无法建立时 UNAVAILABLE，不因源邻接退出就宣称空集 |
| 原语义范围内更换承载方 | 沿已确认的当前关系／successor 与指定联系验证新来源；接纳、安装、结构、权限及读取依据闭合后才可 COMPLETE |
| 已完成转移，元素已不属于本次 Scope 或不与查询球相交 | 当前结构／空间证据可安全排除时，不因旧 lineage 保留而留下缺口；其他必要来源闭合即可 COMPLETE |

迁出缺口的解除须由源将精确控制决定、经认证核对的目标安装结果及源邻接变更关联为持久完成证据，
并绑定到本次查询所验证的控制 Revision／证据；COMMITTED、自报成功、超时或仅改变源邻接都不足以解除。
完成后新 placement 若仍在本次 Scope 且可能相交，必须纳入匹配来源，只按新路径计入一次。
若已证明迁出本次 Scope，目标随后失联不会使旧 Scope 重新出现该迁出缺口；包含目标 Scope 的查询仍独立校验目标可用性。
仍在 Scope 内而仅位于查询球外时，须以本次 cuts 的有效 coverage 证据安全剪枝，不能凭转移时的位置永久免查目标。
只允许沿有效结构／联系继续当前 Scope 的发现，successor 是连续性证据，不是扩大 Scope 或跳过权限的通行证。
Start、Basis 或根依据因转移失效时须重新建立查询上下文，不能默换范围或退化成空结果。

旧 continuation 不插 successor、不换 Scope／cuts；上述新查询规则不恢复旧 receipt、ReadAt 或分页资格。
允许披露的 successor 给出 source／target／final cut／决定绑定及指定联系；目标还须独立验证身份、
placement／acceptance 与当前权限。无权披露时仅不透明失败，不泄露隐藏 owner、地址、范围或数量。

源观察在 final cut 后提供 terminal transition／successor evidence，不再混入该旧范围普通变更。
在有效原授权及保留窗口内可读终止证据；窗口丢失或不能证明连续性则 GAP，不假装没变化。
新 owner 从自己的明确 cut 重建 observation，不接受旧 cursor，不承诺两个流的序号天然连续。
同 owner writer 重启也使运行期 cursor 失效；durable 交接记录能解释终止，不是无限期历史回放。

## 11. 持久化、联系、资源与工程边界

各 owner 在自己事务内只保存本类型实际承担的 intent、fence、acceptance、decision、安装及原输入／结果，
不为补齐统一状态链复制其他 owner 的记录或增设空步骤。
数据与控制历史有可验证因果关系；load 拒绝缺 final cut、无来源安装、重复目标、回退 epoch。
同 owner handoff 可以有独立控制 journal，不改写业务历史，不把重放当新提交。
外部等待不持写锁；返回后和最终事务前重核原记录、角色、expected 和证据。

任务容量、并发、传输期限显式有界；durable 义务不随 session TTL 消失。
任务满时拒绝新任务，不丢旧任务腾容量；资源不足／失联不自动放开 fence。
联系修复由保存方向的 owner 校验身份、权限及 contact CAS，不访问坏旧地址，也不等待控制槽解锁。
换地址不换目标、owner 或 manifest，不复活旧凭据；迟到旧更新不能覆盖新联系。

Core／Facet 各自维护 gate 和状态，不导入对方内核；Composition／transport／SDK 只接线。
按[工程规范](../../ENGINEERING.md)拆职责，不造 shared writer 管理器、通用工作流或第二份事实索引。
当前代码归位：Core 的 writer／控制记录进入 `ucupa-core/internal/commit` 唯一持久 owner，
Facet 进入 `ucupa-facet/internal/namespace` 唯一内容 owner；Store 不裁决 placement 或授权。
按真实职责安排新增实现，不在 `instance` 堆业务代理，不预建空 transfer／chunk／handoff 目录。
上述归位不缩减本合同范围，也不允许流程另开第二套提交／恢复机制。
Protobuf 唯一生成，公开 schema 同步；本地／网络共用 owner 裁决，不留旧接口 alias 或旧 Store reader。
P7 是业务事实的 owner 转移，不是历史软件版本的兼容迁移。

## 12. 必须验证的正反例

以下反例按所声明支持的 profile 验证；涉及尚未启用的外部边正向接续时，首批须验证安全拒绝，
不得把内部单项测试当作公开支持或删除该项未来启用时的正确性要求。

| 反例 | 必须得到的结果 |
|---|---|
| 写与 fence 并发 | 写包含于 final cut 或被拒绝，不丢确认提交 |
| INTENT 取消与源 fence 交错 | 只有一项获 durable 许可；源未知不能宣告取消成功 |
| fence 后旧实例或克隆重新上线 | 原域旧 epoch 拒绝；无真实 fence 的跨副本接管不在支持范围，不自报成功 |
| 每个 durable 边界前后崩溃／响应丢失 | 只有旧有效、不可用、新有效；原 ID 判定／恢复，不双写双显 |
| 接纳后 parent 收缩、删除、reparent／revoke | 相关保留拒绝破坏 placement，不使用过时空间证明安装 |
| slice 漏成员、遗留子元素、跨 owner 边漏接 | 不接纳／不安装，不将局部复制称完整 subtree |
| 目标 UUID 冲突、重复 namespace、不同输入同 ID | 冲突，不覆盖、不建第二个有效 lineage |
| A→B→A 返回、重放旧 successor 冒充返回、已删除身份再导入 | 只接受完整连续性和当前源 fence 的新转移；拒绝旧证明及永久删除后复活 |
| 撤权重授、批准换目标、服务借用户权限 | 未决定动作拒绝；已决定恢复只按原绑定 |
| payload 篡改、缺键、丢尾块、配额满 | 不接纳部分数据、不切 binding、不回退旧方 |
| Core Delete 与 Facet replacement／re-acceptance 并发 | 串行约束或冲突，不对已删除元素重新 ACTIVE |
| 旧 acceptance／CAS／cursor 在目标使用 | 不作新写／观察凭据；原结果恢复与新写分开 |
| 仅有源邻接退出／COMMITTED，目标尚未完成安装 | 未闭合的相关查询不能声称 COMPLETE 空集；旧分页不得换来源继续 |
| 完成迁出后查询旧 Scope；目标随后失联 | 持久完成与当前范围依据可证明该元素已不属于范围时，不留永久迁出缺口；其他来源闭合即可 COMPLETE |
| 完成转移后新旧位置仍在同一查询 Scope | 在匹配 cuts 上按新 placement 判断相交、至多计入一次；不漏项、不双显 |
| 安装时位于查询球外，后续在同 Scope 内移动／扩大 Range | 以本次有效 coverage 判断，不能用安装时的位置永久剪枝；缺少必要证据不能报 COMPLETE |
| 空间不变只换承载，或 successor 指向 Scope 外 | 前者必须闭合新来源或报告缺口；后者不得扩大查询范围纳入结果 |
| 同域 writer／同 Facet 重新接纳被强制导出再导入 | 直接核对原 Store／内容和适用决定，不新增无用传输；仍验证交接或关系接纳证据 |
| 调用方直接请求内部安装、合并跨 owner Commit 与 Install | 缺少受限执行资格或精确决定就拒绝；不能把控制已决定伪报为目标已启用 |
| detached 后普通删除挂载锚或重建同 UUID | 无处置证明拒绝；旧身份不重用 |
| 断连及联系修复与未完成转移相遇 | 保持正确状态，允许授权的独立修复，不自动换目标或路由 fallback |
| 隐藏分支转移／未授权 successor 查询 | 不泄露身份／地址／数量；unknown 不是不存在 |
| 旧 root 失联即让新 Core 继承 | 拒绝连续性声明，新空间不是旧空间恢复 |

## 13. P7 整体验收门

按已声明的支持 profile 分批验收。首批不可用局部私有结果冒充完整公开能力；
后续按需 profile 不因已有内部代码自动成为 P7 Baseline 的必做项：

1. Core／Facet 在同原始 durable domain 的 writer handoff，覆盖全部提交路径、fence、重启和安装。
2. Core Entity、无外部挂载边的 owner-local Anchor closure、root successor 与简单 carrier replacement
   完成公开转移和恢复；含未接续外部边的输入在撤回源内容前安全拒绝。外部边界 slice 的逐关系
   正向接续只在后续明确启动该 profile 时验收；始终拒绝跨多个 owner 的隐含原子迁移。
3. Facet 完整 namespace replacement、Core 转移后同 Facet re-acceptance、撤销关系恢复及内容退役。
4. 相应授权、公开控制命令、原结果恢复、指定联系维护、查询失效、terminal observation 和重新接入闭环。
5. 对首批支持 profile 验证单元、确定性并发交错、每个不可回退持久边界故障、独立 Store／真实产品进程、
   P6 网络重试／权限矩阵及公开正反例；不要求穷举未启用形态与所有故障条件的笛卡尔积；
   协议变更不破坏已声明独立实现 corpus。
6. 本次全仓 go run ./cmd/verify 通过，工程／生成／race／依赖／示例均有证据，并逐项标明支持 profile。

未来若启动外部边正向接续，须按第 7.3、12 节追加其公开调用、授权、持久恢复和逐边故障验收；
不能凭当前私有代码或首批 P7 验收直接宣称该 profile 已支持。

同机独立进程不证明真实多机网络或复制存储安全，P6 真实多机待验边界继续保留。
同身份跨副本接管、P8 生产 profile、Stage 2 Realm 联系和下游产品不被写成 P7 已交付能力。
合同确认与实现授权不表示能力已实现或验收，不自动提交后续代码或启动后续 Phase。
