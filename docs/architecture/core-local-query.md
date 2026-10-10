# UCUPA Core 本地空间查询合同

**状态：P3 已实现、通过完整验证并经用户接受；以下为唯一现行本地查询实现合同，current 分类由架构权威清单维护，交付证据与提交记录见开发计划。**

本文直接修订已有 QueryRegion，不维护两套查询规范或兼容入口。
本次验收覆盖包含深度和可选快照的现行完整查询形态；早先单 Core 查询提交未独立验收的历史
由[开发计划](development-plan.md)保留，不追认旧实现曾满足本合同，也不保留旧查询入口。

空间值、包覆、提交与普通读取沿用[本地真相合同](core-local-spatial-truth.md)，
语义由[主线 D10](ucupa-concept-mainline-baseline.zh-CN.md#d10-structure-发现与空间-query-使用同一个明确-universe)约束。
跨 Core 路径与快照组合只在[挂载合同第 5 节](core-mount-contract.md#5-读取路径与组合切面)定义，
不复制 Element 模型、写入裁决或另一套查询算法。[权威清单](README.md)区分目标与 current。

## 1. 范围与不变量

- 增改删仍经校验、可靠保存后才确认成功；不等待查询结束，也不人为延迟十秒保存。
- 普通 ReadElement 默认读取最新已提交状态；分页始终固定首次选择的事实切面，不混入后来的状态。
- 快照保留关闭时逐页确认原切面仍为 current；启用时才跨请求保留有期限的旧视图，允许跨写入续页。
- 不开放任意时间戳查询、永久历史 Store 或全局时间轴；关闭保留不关闭查询和分页。
- 查询不改变 Element、Revision、CoreReadCut、OperationID 或持久 Store；允许创建有界临时读取状态。
- 本地查询只处理能在本 owner 闭合的结构；遇远端 coverage 或需越过外部父时，
  沿用 P2 的 ErrMountContextRequired 防漏规则，不能把远端当空或外部父当根。
- 不增加 Realm 授权模型、独立 Route、网络服务、空间索引、Facet 过滤或实时仿真。

## 2. 一个深度：选择 Scope，不截断后代

请求区分三件事：Start 是上溯起点，Basis 是查询球的坐标参考，Scope 是解析后的搜索集合。

1. UpDepth 使用有符号整数：省略或 0 选择 Start；正数表示**最多向上走多少条真实父边**；
   **-1 表示不限制上溯层数，一直选择到本空间已证明的真正根**。小于 -1 非法。
   正数超过根时也在根停止；返回的 ActualUpDepth 始终是实际走过的非负层数，不返回 -1。
   不设置另一个向下深度。-1 是查询选项，不是 Range 无穷值或第二种空间真相。
2. Scope 是所选最高 Anchor；搜索它本身及全部后代，筛选与查询球相交的 Element。
   祖先直属 Entity、兄弟锚及其后代都属于该集合，不因从哪条分支进入而遗漏。
3. 到达深度上限，是已完整定义了一个较小的语义范围；Scope 之外不是本查询的缺口。
   Complete 只针对返回的 Scope；选择 -1 且成功证明根时，Scope 才是本空间的根闭包。
   无限制上溯不意味着无限执行资源；无法建立必要根上下文时必须失败，不把中途停下的 Anchor 当 Scope。
4. 无法确认上一条父边，不能冒充“已经到根”；跨 owner 需路径上下文，缺连接／无权按相应失败处理。
5. Basis 必须是 Scope 本身或其后代 Anchor，可以不在 Start 子树内。
   深层 Basis 不隐式缩小 Scope；Start、祖先链、Scope、Basis 全部来自同一选定切面的读取视图。
6. 跨 Core 不新增结构边；由部署产生的转发次数不是 UpDepth。数值证明工作界也不是语义深度。
7. 修改 Start、UpDepth、Basis、球或 selector 都是新查询，不能接旧游标继续翻页。

已确定 -1 为无限上溯、0 为缺省、正数根处饱和，并显式区分 Start 与 Basis。
在当前本地 profile 中终点是真正根；未来若有明确 SpaceHandle boundary，则不得越过该已声明边界，
具体路径规则见挂载合同。接收 Core 或任意连接入口本身不构成这种边界。

## 3. 本地调用与结果

目标公开面如下；沿用已有值类型，具体 Go 声明在实现时按工程规范核对：

```text
RegionQuery = {
  StartAnchorID, UpDepth: int64, BasisAnchorID, Sphere, Selector
}
QueryRegion(actor, query, at *CoreReadCut, limit, continuation)
  -> QueryPage, error

QueryReadMode = CurrentVerified | Retained
QueryPage = {
  Matches, Cut: CoreReadCut, ReadMode: QueryReadMode, Snapshot: CoreReadSnapshot?,
  ScopeAnchorID, ActualUpDepth, Status, Gap, Continuation?
}
Match = { ElementID, Kind, Revision, Path }
ReadElementSnapshot(actor, ElementID, snapshot CoreReadSnapshot)
  -> ElementRead, error
ReleaseReadSnapshot(actor, snapshot CoreReadSnapshot) -> error
```

- Sphere 仍为有限 Vec3 Center 与有限非负 Radius，使用现有 canonical 规则，-0 归一为 +0。
  Selector 仍为 Anchor／Entity／Both；limit 仍为 1..256，只限制当页输出数量。
- 首次 continuation=nil：at=nil 捕获 current；显式 at 只允许在该 cut 仍 current 时捕获，
  不凭裸 cut 从日志搜索历史。QueryRegion 的这条入口不开放任意旧 cut 查询。
- 续页 at 必须 nil。读取模式、固定 cut／Scope、可选 snapshot、查询身份和位置唯一来自 continuation；
  query 必须规范等价，limit 可以在合法范围内改变。
- continuation 保留请求的 UpDepth；-1 与某个恰好到根的正数仍是不同请求，不能因当前 Scope 相同而互换续页。
  先校验 UpDepth >= -1 再处理计数，不把 -1 转为无符号大整数来模拟遍历。
  实际层数与结构地址上行次数均检查累加溢出，不让计数回绕成为另一条路径。
- CoreReadCut 只标记事实切面。CoreReadSnapshot 是本次运行中保留视图的私有句柄，
  关联 cut、固定有效期与资源占用；不是新的事实 Revision、网络 bearer token 或授权证明。
- ElementRead 沿用现有 ReadElement 的结果类型，附带同一 cut；
  ReadElementSnapshot 明确选择保留状态，现有 ReadElement 的 at/current 语义不被暗改。
  此接口只读能在保留 Scope 中证明归属的 Element；未知 ID 与 Scope 外 ID 统一 ErrInvalid，
  不用没有 parent 上下文的裸 UUID 证明历史缺失。已验证视图内部自相矛盾则 ErrStore。
  Retained 下，发现后仅带 page.Cut 调普通读取不足以获得旧状态，须使用 page.Snapshot。
  CurrentVerified 下 Snapshot 必为空；发现后用普通 ReadElement(at=page.Cut)，cut 仍 current 才能成功。
  不返回零值假句柄，也不把空 Snapshot 解释为可供历史读取的句柄；相关 Snapshot 接口拒绝零值 ErrInvalid。
- Release 只释放调用者自己的合法形状及绑定的保留句柄，重复释放和到期后清理成功；错误 Core／caller 绑定拒绝。
  释放不披露事实，不要求调用者仍持有 query grant；共享底层视图的其他句柄不受影响。
- 本地 ListDirect、ProjectToAncestor 不因此获得历史读取接口；跨 Core 列举的快照规则见挂载合同。

已有 Element 原点仍可作为探针：先普通 ReadElement 取得 Parent、Pose 和 cut，
以 Parent 为 Basis、Pose.Translation 为 Center，再用该 cut 发起 QueryRegion。
两步间发生提交则明确过期并重新开始；仅 Retained 在成功保留后不因普通写入而过期。
搜索半径由调用方指定，不自动等于被读 Element 的 Range；Start 仍由调用方明确选择。

### 3.1 可选保留与固定读取模式

每个 Core 在创建／启动时接收可选 SnapshotRetention 时长配置。Go 构造参数可用 *time.Duration
表达“未配置”；实现须复制值，不能保留由调用方可变指针控制的运行策略。本轮配置在该运行期固定，
不增加热切换。未来配置文件映射相同语义，不要求本轮先建设服务配置文件系统。

| 配置 | 读取模式与行为 |
|---|---|
| 未配置 | CurrentVerified：不跨请求保留旧空间状态，查询与分页照常可用；每页验证原 cut 仍为 current |
| 明确配置有限正时长 | Retained：按该时长保留本次读取视图，后续写入不改变原分页依据 |
| 明确填写零、负值或无法表示的时长 | 创建／启动配置错误，不自动当未配置，也不补默认值；文本转换溢出须在转换前拒绝 |

没有默认秒数，没有“留待实现选择默认值”，也没有无限保留。此处的模式是同一 API 的运行行为，
不是版本兼容入口。快照保留能力与资源准入分开；已开启但容量不足明确拒绝，不能改以 CurrentVerified 成功。

- ReadMode 在首页前确定，后续各页绑定同一模式。Retained 必有有效 Snapshot，且其 cut 等于 Page.Cut；
  CurrentVerified 必无 Snapshot；两者都固定 query、Scope、ActualUpDepth 与排序位置。
- CurrentVerified 每次调用只借用临时一致视图，不在调用间保留旧记录或计算结果；
  续页先对游标固定 Scope 授权，再核对 current 等于原 cut，计算后再次授权并核对同一 cut。
  任一核对已证实不同，零结果 ErrReadCutExpired；不重新选择 Scope 或从新 cut 接着排序。
- 未配置本身不是错误、过期或完整性缺口；没有写入、运行仍有效且检查通过时应正常完成分页。
  单次调用仍有临时内存及计算额度，不因关闭历史保留而取消一致性或资源边界。
- CurrentVerified 游标使用有大小界的私有值式元数据，不登记历史视图；绑定本次运行以拒绝重启前的游标。
  没有保留句柄可释放，也无历史 TTL。普通 ReadElement 的裸 at 仍按现行事实 cut 规则，
  不因这种游标运行绑定而改变重启后的普通读取语义。
- 跨 Core 的全来源模式选择、组合元数据与故障处理由挂载合同定义，不由单个 owner 擅自切换模式。

## 4. 命中、顺序与完整性

- 使用闭球精确相交／相切；中心在球外但 Range 相交仍命中。
  Anchor 使用自身保守 Range，无界根在 selector 包含 Anchor 时命中。
- selector 只过滤输出 kind，不能截断通向所选后代的遍历。
- Match.Path 是从解析后的 Scope 向下的 UUID 链：不重复 Scope，包含终端；
  Scope 自身 Path 为空。路径每条边在同一选定 cut 的视图中真实成立，不是稳定身份。
- 按 canonical UUID 的 16 bytes 升序输出；Kind、Revision、Path 与 Cut 同源。
  不返回完整记录、Facet、Metadata 或舍入后的投影 Pose。
- ReadElementSnapshot 仍独立检查 exact read 权限；query grant 不自动授予完整记录读取。
  授权后成功读回的 ID／Kind／Revision 必须与原命中一致；已删除或换父的新状态不能混进旧读。
- 每页可在同一选定 cut 的视图重算结果：Retained 借用旧视图，CurrentVerified 重新取得仍为 current 的原切面。
  不要求保留完整命中数组或引入结果 Store。

| 已验证结果是否还有下一页 | 是否存在精确数值缺口 | Status / Gap / continuation |
|---|---|---|
| 否 | 否 | Complete / None / 无 |
| 是 | 否 | Partial / None / 有 |
| 是 | 是 | Partial / ExactWorkLimit / 有 |
| 否 | 是 | Partial / ExactWorkLimit / 无 |

Complete 说明**同一分页链的并集**完整，不是末页包含所有结果。
只有首页 Complete 且 Matches 为空，才证明所选 Scope 在该选定切面上没有相交元素。
Gap 描述完整 query/cut，而非上一页 UUID 后的后缀；超界候选排序更早，也不能在末页清掉缺口。
返回项始终为确定命中；无 continuation 的 Partial 是终止缺口，不制造无进展的游标。

## 5. 权限与披露

PermissionQueryRegion 仍为 ucupa.core.anchor.query-region，
但目标取 **CoreElementScope(本 Core, 实际 ScopeAnchorID)**，不是 Start 的授权自动向上扩大。

- query grant 允许该 Scope 完整闭包内的匹配摘要及必要路径，不授予 exact read、写入或 Facet 权限。
  不增加逐元素隐式隐藏；不能静默略去未授权分支再报完整。
- 本地先私下解析实际 Scope，仅用于构造授权要求；未获许可不披露 Scope、祖先链、cut、存在性或匹配。
  成功解析只检查实际 Scope 的 query grant，不额外强加 Start grant。
  若无法形成本地 Scope（Start 缺失／非 Anchor、需要外部父、Core 不可读等），先检查请求中
  exact Start 的 query grant，作为披露该失败的权限；无此许可一律 ErrDenied，不能提前返回
  Invalid／MountContextRequired／Store 暴露状态。有许可才披露相应失败，披露前重复检查同一 grant。
  这只是失败诊断门，不允许借 Start grant 搜索祖先；外部父仍交路径合同，不做全网 UUID 查找。
- 通过实际 Scope 授权后才准入计算，并仅在 Retained 分配正式保留句柄。若解析视图与捕获 current 已不同，
  零结果过期，不在当前调用悄悄改 Scope。续页直接使用游标固定的 Scope 重新授权。
- 每页与快照精确读均在锁外初次授权，借用不可变视图后计算，披露前再授权；
  最后检查 Core 可读及运行／读取上下文有效。Retained 检查句柄和期限，不要求 current 等于旧 snapshot cut；
  CurrentVerified 必须再次检查 current 等于原 cut。外部授权返回后再作本方最终检查。
- 快照精确读限于该查询已授权并保留的 Scope；范围外统一 ErrInvalid，不变成任意历史 record 读取器。
- 外部 Authorizer 永不在 Core 锁／Store transaction 内运行。
  bool false 仍映射 ErrDenied，不假造权限后端的区分能力或 Realm 授权版本。
- 权限不是被冻结的事实。持有快照不绕过持续撤权；但 bool 端口不证明撤权又恢复的 ABA，
  也不提供 Realm/Core 跨 Store 原子授权。
- 旧结构的 scope 含义由快照固定，新授权检查仍针对同一 exact Scope；不能用 current 的新 parent
  改写原授权闭包。独立授权的历史读取不声称对象目前仍存在。

## 6. 有界快照生命周期

只有 Retained 使用下列 **owner 内存中的按需保留视图**，不改变 Store 编码或写入流程。
CurrentVerified 按第 3.1 节使用单调用临时视图；返回、取消或失败即释放借用，临时计算仍受有限准入限制。

1. 在本地提交发布边界捕获不可变空间记录及所需挂载／联系状态，带 cut。
   可以共享不可变数据，但不能保留含完整回执、操作和提交历史的整个 coreState；
   不在整个分页期间持有 Core 锁、bbolt 读事务或阻挡写入的数据库快照。
2. 句柄绑定本 Core、本次运行、caller、允许的 Scope、固定 cut 和有效期。
   同 cut 底层状态可共享，不共享授权；另一个 caller 不能借同 cut 取得其句柄。
3. 启用时固定 TTL 取可信装配明确提供的有限正时长，不接受无限值，不把十秒硬编码为语义；未配置不进入本生命周期。
   每个句柄有独立期限，同 cut 的新句柄不刷新旧句柄；共享视图到所有句柄结束且借用退出才回收。
   首次捕获起按 owner 本地单调时钟计时，首轮不续租、不因翻页延长；
   每次借用及最终披露均检查到期。无全局墙钟或跨 owner 时间戳比较。
4. 同时限制活动句柄数、保留记录／路径字节计量和并发借用／计算。
   已到期但尚有计算借用的视图也计入占用；共享存储只计实际保留一次。
   实现须声明计量口径，不把逻辑字节数宣称为精确进程内存上限。
5. 分配前检查并预留容量，超限拒绝新查询 ErrSnapshotCapacity，不驱逐尚在承诺期限内的句柄，
   不扩大限额、不改写入语义。准入与捕获失败释放预留；取消／到期最终回收借用和临时结果。
   续页、快照精确读也须取得有限借用／计算额度；不足同样零结果 ErrSnapshotCapacity，
   不无限排队，不释放原句柄、不重 pin、不改变原 TTL，可在未过期时重试同一请求。
   这不是 scan-count／CPU deadline 的通用预算系统。
6. 成功末页**不自动释放**：随后还需同快照读取完整 Element。调用者结束使用后显式释放，
   未释放由 TTL 回收。释放先使该句柄对新借用和最终披露失效，借用安全退出后回收；
   等待不能持有 Core／registry 锁，也不能延长可读期限或使其他独立句柄失效。
7. 正常 reopen 仍恢复同一 AuthorityRef 与事实 cut，却不恢复运行内句柄。
   到期、释放、关闭后重启或句柄丢失，旧页／精确读明确失败；不得重放日志恢复句柄或切换 latest。
   同一合法 owner 的过往运行句柄归 ErrReadCutExpired，不能因运行标记不同归为新的参数错误；
   错 owner／caller 等非法绑定仍按 ErrInvalid 且遵守授权顺序。
   本次运行绑定只是私有读取状态寿命，不是持久 writer epoch、复制 fencing 或 Authority 身份变更。
   Close 同样结束内存 Core 的本次运行：释放读取资源，普通后续操作返回 ErrStore，
   合法旧读取句柄／游标返回 ErrReadCutExpired；不再把内存 Core 的 Close 当作无操作。
8. 失败／无授权不发布可用句柄；未交付的新句柄及时释放。
   已交付句柄的使用失败不隐式扩展其期限。已经交付的旧页仍是旧观察，不能与新链拼成 Complete。

continuation 私有绑定 Core 运行、caller、规范化 query（含 Start／UpDepth／Basis）、
实际 Scope／祖先依据、固定 cut、ReadMode、可选 snapshot 及上一页末 UUID。
重放在相同权限与有效读取依据内稳定，不能重复计入集合。
Retained 的普通写入不使其过期，持续写入也可完成**已经捕获并未到期**的分页；
CurrentVerified 只在原 cut 仍 current 且运行有效时续页，写入后必须重新查询。
不保证任意大查询能在有限 TTL 内完成，也不把短期保留说成无限资源或永久历史。

## 7. 数值与成本

继续使用完整扫描、精确过滤、UUID 排序的保守实现，无持久索引或后台预计算。
分页只限制输出，不保证 O(limit)、固定扫描量、路径长度、CPU 或总时延。

- 精确工作界保持每条相关投影链 1024 edges、约分 rational 分子／分母 65536 bits。
  这是证明成本界，不限制合法结构深度；跨部署不重置，不作为向下搜索深度。
- 必要 Scope/Basis 坐标上下文无法建立：整体 ErrUnrepresentable，不返回页。
- 上下文已建立，个别 selector-relevant 候选超界：标记全查询 ExactWorkLimit，继续其他确定命中。
- 未选 kind 可以不计算自身相交，但必须保留所选后代需要的结构与坐标链。
- 保留视图损坏或 Core 不可读：整体 ErrStore，不用 Partial 包装不可信事实。
- 完整可计算案例必须等于同一选定切面视图的无剪枝 oracle；不把只支持全成全败的 oracle 当公开实现。
  无法输出有限 binary64 Pose 不等于不能精确证明相交。

## 8. 失败规则

任意非 nil error 返回零值页／读结果，不携带暂存 matches、路径或 cut。

| 情形 | 结果 |
|---|---|
| 非法 query（包括 UpDepth < -1）、limit、token 形状／绑定；续页另传 at | ErrInvalid；资源绑定检查遵守授权顺序 |
| 无法形成本地 Scope，且未获 exact Start 的失败诊断许可 | ErrDenied，不区分缺失、kind、外部父或 Store 状态 |
| 已获失败诊断许可，但 Start 缺失／非 Anchor | ErrInvalid |
| 实际 Scope query 或 exact read 当前未获准 | ErrDenied，无存在性结论 |
| Basis 不存在、非 Anchor 或在实际 Scope 外 | 授权后统一 ErrInvalid，不探测其他范围 |
| 首页指定 cut 不再 current；正式捕获前依赖切面已变；CurrentVerified 调用前后原 cut 不再 current | ErrReadCutExpired |
| 未配置 SnapshotRetention，原切面仍可验证 | 正常 CurrentVerified 查询／分页，不报错误或 Gap |
| Retained 所需句柄已到期／释放／披露前到期；任一模式的旧游标发行运行已结束 | ErrReadCutExpired，不读 latest |
| 快照准入／资源预留超限，或续页／精确读借用额度不足 | ErrSnapshotCapacity；不新建句柄，已有句柄与期限不变 |
| 需要外部父或远端 coverage | ErrMountContextRequired |
| 必要坐标上下文无法表达／Core 不可读 | 分别 ErrUnrepresentable／ErrStore |

Retained 的普通写入后旧句柄仍有效，不以“current 已变”返回过期；CurrentVerified 必须拒绝已改变的 cut。
查询失败的优先级遵守无状态验证 → 可信绑定与授权 → 状态相关结果 → 披露前检查；
不能先解释他人的 token 内容或返回容量细节再检查 caller。

## 9. 必须通过的验收

1. 深度 -1、0、未填、1、多层、超过真实根以及小于 -1：-1 到已证明根，正数在根饱和，
   ActualUpDepth 为实际计数；根上请求 -1 返回实际 0，非法负数无状态拒绝，不把外部父当根。
   包含最小 int64 负数的拒绝、-1 与同 Scope 正数的游标混用，以及上溯中断／到期不截短为成功。
2. 各层祖先直属 Entity、兄弟及深层后代、大 Entity 中心在外但相交；selector 不截断遍历。
3. Basis 不在 Start 子树但在实际 Scope 内；改变深度得到新链，旧 UUID 游标不能复用。
4. Retained 中 Start／祖先／元素在首页后改 Pose、Range、Parent、创建或删除：旧页和快照读保持旧状态，
   新查询／普通读取看到新提交；不复制未持久化状态，不延迟写入确认。
5. 两模式多页并集无漏项无重复；可改 limit；有效上下文重放稳定。
   Retained 末页后仍能快照精确读，显式释放后失败；CurrentVerified 末页后的普通读仍验证原 cut。
6. Entity 原点探针的初次 read/capture 竞争明确过期；Retained 成功 pin 后不再因后续写入失败。
7. 向上不是扩权；Scope 无权不泄露解析结果；Start 缺失／非 Anchor／外部父在无诊断许可时
   与无 Scope grant 同为 ErrDenied；有 query 无 read 时仅返回摘要。
   长扫描／精确读期间撤权在披露前拒绝；Authorizer 全程锁外。
8. TTL 边界、计算中到期、正常重启、句柄丢失、错误 Core/caller/query 均不回退 latest；
   真正 Store 不可读保留原故障语义，不把所有失败伪装成过期。
9. 容量满、并发准入、共享视图、借用中释放／到期、失败清理均有确定测试；
   新查询拒绝不影响事实写入，旧承诺句柄不被无声挤掉。
10. 数值缺口四种分页形态、较小 UUID 超界候选的 sticky Gap、无进展 continuation 拒绝。
11. M=MaxFloat64：根下 Anchor x=M/range=M，其 Entity 局部 x=M/range=0；
    根 Scope/Basis、查询 Center.x=M/radius=M 必须命中精确中心 2M 的 Entity，不强求可输出有限 Pose。
12. 完整案例同视图 oracle 一致；Partial 使用独立精确向量证明返回项正确且其他可计算命中未漏。
13. 查询不改变 Store 编码／内容、Revision、cut、回执或 OperationID；
    临时快照可观测生命周期不冒充新的持久真相。
14. 配置未填、显式正值、零／负值／溢出、外部配置指针被改均有测试；未配置不创建跨请求空间 pin，
    无提交时多页可完成，提交发生在计算前／中／最终校验前均明确过期。
    两模式 Snapshot 有无、cut、游标模式和运行绑定一致；关闭不被误报为过期，容量不足不降级。
15. 公开面 allowlist、文件规模、原 Core／Facet／Realm 回归及全仓 test／race／vet／依赖／示例通过后，
    再提交用户验收；设计审查不代替运行验证。

目标合同确认与实现授权分别记录在开发计划中；曾通过的 current-cut 查询测试不代证上述快照测试。
