# UCUPA 概念与主线基线

**状态：已接受的当前详细目标语义；受基本公理约束；不是实现合同或开发计划；不授权代码**

> 联系方式修订：用户已确认 Core／Facet 采用同一原则——每个需要跨服务访问的关系保存预期
> `FactAuthorityRef` 与权威指定的 `NextHop`，按该地址连接并核对目标，不建立 Ref → 地址目录。
> 本文已据此替换原统一 Ref Route 前置；这不是给每个 Ref 增加地址，也不是为每个 authority 指定唯一全球地址。
> 各实现合同按权威清单区分已确认目标与待检阅部分；代码仍须明确开始。本次只对齐文档，不授权代码或声称网络认证已实现。

> Realm 联系补充：沿用上述指定联系原则，以 `RealmAttachment` 表达 Core／Facet 到授权管理方的
> 联系，目标使用已有 `RealmID` 和指定 `NextHop`。这是既定授权模型的访问链补齐，不是 Realm
> 职责重定位或新授权模型；共同方向已确认，具体记录归位、接口及恢复机制仍须后续实现合同。

> P3 查询修订：用户已确认向上深度选择 Scope、完整搜索所选后代及可选快照的最终目标合同；
> 每 Core 启动时可选配置 SnapshotRetention，无默认保留时长；未填关闭，有效正时长启用保留。
> 读取按实际来源选择 CurrentVerified 或 Retained，写入仍可靠提交后确认。D9 的局部上行路径和 D10 的具体 profile
> 由已定稿的查询合同细化，不再要求重复逐条确认；实现授权、交付与验收状态仅由开发计划记录。

> 计划分工说明：第 6.2 节与第 9 节区分能力证明依赖和执行排序；用户已确认的 Stage／Phase
> 编排只由开发计划维护。计划重排本身不改变 D1–D12；本次查询语义修订单独如上声明，不把尚未实现能力声明为已完成，
> 也不因计划入库而接受候选实现合同的全部细节。

本文从已接受的[《UCUPA 基本公理》](ucupa-foundational-axioms.zh-CN.md)统一推导 UCUPA 的完整概念主线，并一次性裁决相互依赖的十二类下游问题。它不是十二份可以各自成立的局部设计，也不是 Go API、wire、Store 或部署计划。

本文的整体接受冻结目标语义，但不把概念决定偷换成当前 API、Store 或代码：

- [基本公理](ucupa-foundational-axioms.zh-CN.md)是上位约束；本文是唯一详细目标语义，任何 Platform、minimum-truth 或 capability view 都只能投影本文；
- [架构权威清单](README.md)唯一维护当前实现合同，[开发计划](development-plan.md)唯一维护当前阶段和下一动作；
- Range／空间根推导、能力重新判定账本、历史能力图谱和开放基础设施可行性评估是冻结证据，不参与当前状态对齐；
- 本文中的类型名只固定概念角色，不固定语言、序列化或进程边界；任何实现仍须独立合同与明确开始。

## 1. 一句话结论

> **UCUPA 的公共主线是：调用者从一个可验证但非中心化的空间入口进入明确的 Anchor 结构闭包，在有界授权下解析结构或发现未知 Element，读取 Core 的 `ElementUUID + Structure + Pose + Range` 与可选 Facet 的精确类型化记录，并以逐事实权威的证据观察或提出变更；联邦只连接这些独立权威，不合并它们。**

```text
可分享 SpaceReference
        │  分别验证空间入口 / authority / trust / 指定联系信息
        ▼
调用者临时 SpaceHandle
        │
        ├─ Resolve / ListDirect / QueryRegion ──► Core 空间事实
        ├─ Read / ListFacetTypes              ──► Facet 类型化事实
        ├─ Observe / Resume                   ──► 已提交事实的派生观察
        └─ Propose                            ──► 对应 fact authority

Core Element truth = ElementUUID + Kind + Structure + LocalPose + Range
                     + ElementRevision
Core control truth = delegation / per-Element FacetAttachment / relation contacts
Facet truth        = (ElementUUID, FacetTypeRef) -> exact payload + FacetRevision

独立关系：Structure ≠ Fact authority ≠ Access control ≠ Reachability ≠ Deployment
```

这个形态没有全局 World 对象、裸 UUID 目录、全局 Revision、中央空间索引、Realm 树或跨 owner 原子事务。体系通过增加有限的空间、权威和入口持续增长，而不是让一个中心节点承担“无限”。

## 2. 为什么必须整体裁决

十二类决策不是十二个可以孤立投票的问题，而是一张有向依赖图：

1. 球形 Range 只有在统一刚体坐标代数和确定性数值规则下才有跨实现含义；
2. Core／Facet 真相先决定事实 coverage，事实 authority、access 与 reachability 再决定如何可信进入；
3. SpaceHandle、查询 universe、未知发现授权和 `COMPLETE` 的含义必须互相一致。

因此可以按依赖顺序逐项审查；最终仍要通过一次整体一致性门，避免某项在局部正确、组合后却需要隐藏真相：

```text
已接受基本公理
        │
        ├─ D1 坐标代数 ─┐
        └─ D2 Range/Root ┴─► D3 数值安全 ─► D4 Core 空间真相 ─► D5 Facet 类型真相
                                                       │                 │
                                                       └────────┬────────┘
                                                                ▼
                                           D6 Fact authority / delegation / acceptance
                                                       │                 │
                                                       ▼                 ▼
                                              D7 Access/trust    D8 Contact/reachability
                                                       └────────┬────────┘
                                                                ▼
                                           D9 SpaceReference / address / handle
                                                                ▼
                                           D10 Resolve / discovery / query
                                                                ▼
                                           D11 evidence / observe / resume
                                                                ▼
                                           D12 transition / fencing
                                                                ▼
                                              一个一致的 concept successor
```

Cluster、Store、Transport、索引、Operator、Archive 和模块数量不进入这组公共空间语义；它们只能在后续生产合同中实现或消费这些结果。面向人的可视化是外部消费方式，不要求 UCUPA 内置 Console 模块。

## 3. 最小词汇与身份边界

| 概念 | 唯一含义 | 明确不表示 |
|---|---|---|
| `ElementUUID` | Anchor／Entity 稳定且不可复用的身份 | 路径、位置、owner、权限、endpoint |
| `FacetTypeRef` | Facet 内容键使用的稳定、不透明 32-byte 类型标识 | 服务地址、可变名称、Facet 已验证内容格式的证明 |
| `FactAuthorityRef` | 一个 Core 或 Facet 事实切面的稳定逻辑权威身份 | Realm、Principal、Cluster、Store、Pod、NextHop |
| mount Anchor | parent Core authority 仍拥有、但其直接内容 coverage 交由另一 Core authority 接受的 Anchor | 新 Element kind、远端 root、复制边界 |
| carrier role | 对一个 exact mount acceptance 负责其内容 coverage 的 Core-kind FactAuthorityRef | mount Anchor owner、Cluster、服务地址 |
| `PrincipalRef` | 由明确 trust context 建立的调用者身份 | fact ownership、write fence、联系信息 |
| `RealmID` | 身份与权限关系 owner 的身份 | 空间、Anchor 根、组织父级、Cluster |
| `ClusterID` | 部署与承载环境身份 | Element 或 fact authority 身份 |
| `BoundaryAnchorRef` | 已进入空间并验证后的精确二元组 `(AnchorUUID, owning Core-kind FactAuthorityRef)` | SpaceReference publisher、NextHop、credential、read cut、裸 UUID locator、永久路径 |
| `StructuralAddress` | 在明确空间边界内，从已知 Origin 按选定 Structure 上行／下行的结构路径 | 永久身份、人类名称、授权证明 |
| `SpaceReference` | 可分享的空间 bootstrap 描述 | 当前权限、当前可达保证、持久共同真相 |
| `SpaceHandle` | 某调用者当次进入空间后组合出的临时上下文 | 新空间对象、全局 locator、可转移凭据 |
| `FacetAccessContext` | 由一次 Core exact read 的 ACTIVE attachment 形成、供 Facet 选择 exact acceptance 及其 Core 联系记录的临时 AcceptanceKey | 持久 FacetKey、existence evidence、全局 UUID locator、地址解析器 |
| `NextHop` | 某一跨服务关系中由有权主体指定、供传输层直接使用的目标服务入口 | 目标身份、permission、delegation、全球唯一地址、当前可达保证 |
| 联系记录 | 绑定 exact 关系的预期目标身份、NextHop 与独立联系修订；事实服务使用 FactAuthorityRef，Realm 使用已有 RealmID | Ref → 地址目录、authority-wide 唯一地址、Element 空间记录或 Facet payload |
| `RealmAttachment` | Core／Facet 确定授权访问上下文后，用于联系预期 Realm 的记录；包含已有 RealmID 与指定 NextHop | PermissionBinding 副本、调用者自报的授权依据、新事实 authority 或每个 Element 必填的空间字段 |

`FactAuthorityRef` 这个稳定 owner 角色的必要性已经由四个独立需求证明：同一 Realm Principal 可以控制多个事实切面；事实权威必须跨 Cluster、endpoint 和 credential 变化保持可辨认；查询与观察必须限定各自 read cut；Core 与 Facet 具有独立 owner。它取代过窄的候选名 `SpatialAuthorityID`，但不创建全局权威目录，也不强迫新增一种全球可解析 ID；任何满足这些不变量的稳定 opaque reference 都可以承载该角色。

Ref 只作身份比较与 evidence 绑定。只有需要跨服务访问的关系才保存联系记录；同一 authority 在不同关系中可以使用不同入口，不能据此收敛成一张必须完整的 authority 地址表。`FacetTypeRef` 只是 Facet 内容键；调用方若需要解释或验证 Payload，自行持有相应规则，不能把这件事推给事实服务或另造事实 Ref 地址目录。

本文不新增 `WorldID`、`ConnectedSpaceID` 或 `SpaceRoot` 对象。一个可遍历空间由当前 Anchor Structure、有效 delegation／acceptance 与所选语义边界共同定义；其 root Anchor 已经提供足够的空间身份锚点。

## 4. 十二类整体决策

### D1. Core 只有一种权威坐标代数

1. Core Pose 是从 Element local frame 到直接父 Anchor frame 的右手 Cartesian 刚体变换，即三维 `SE(3)`；旋转和标量的规范编码由首个 interoperability successor 唯一确定，不属于空间代数本身。
2. 线性量纲统一为米；Pose 不含 scale、shear、投影参数或坐标系标签。
3. 每个有根空间的 root Anchor 没有父 frame，因此没有 `LocalPose`；它在自己的 root basis 中派生为 identity。
4. WorldPose、任意祖先 frame 投影和逆变换均由当前 Structure 与 LocalPose 派生，不持久保存为竞争真相。
5. polar、cylindrical、spherical、geodetic、天体尺度分块坐标及渲染坐标只能作为边界 projection/profile。投影必须声明来源、目标、误差和适用范围，不能让表示偏好进入 Core 记录。
6. 两个独立 root basis 之间不存在隐含变换。没有被某个明确 owner 持有并带 revision/evidence 的投影关系时，它们不属于同一个几何 Query universe；把两个查询结果并列展示也不等于建立共同坐标。

这个选择使球形 Range 在任意 Anchor 旋转下仍保持球形，并让 Pose composition、reanchor 和空间相交形成一个封闭数学模型。若未来需要 Core 原生 scale 或非刚体 frame，必须同时重开 D1、D2、D3 和 D10，而不能添加一个旁路字段。

### D2. Range 是结构闭包的保守发现覆盖

权威 Range 只有两个形态：

```text
Range = BOUNDED_SPHERE(radius) | UNBOUNDED_ROOT
```

1. 普通 `Range(E)` 是在 `E` local frame 中、以 frame 原点为球心的有限非负球。
2. Entity 是叶 occurrence，其 Range 覆盖该 Entity 的领域中立空间存在；它不是 Collider、LOD、可见距离、权限区或业务影响范围。
3. Anchor Range 是以该 Anchor 为根的完整 Structure closure 的稳定保守 zone；它既允许 false positive，也必须禁止 false negative。
4. 只有无父 root Anchor 使用 `UNBOUNDED_ROOT`。它取消固定根半径 ceiling，但不表示无限精度、无限容量或一次查询整个宇宙。
5. 每个有父的直接子 Element 都使用 `BOUNDED_SPHERE`。当直接 parent 也是 `BOUNDED_SPHERE` Anchor 时必须满足：

   ```text
   distance(parent.origin, child.origin) + child.radius <= parent.radius
   ```

   当直接 parent 是 `UNBOUNDED_ROOT` 时，任意合法有限 child Range 都按类型被包含，不读取不存在的 `parent.radius`，也不做有限球数值比较。只验证这两类直接父子关系即可归纳覆盖任意后代；边界相切合法，orientation 不改变球半径。
6. Element 的 Structure、Pose 与 Range 只由调用方显式提交改变。UCUPA 不为使请求合法而自动改动任何祖先或子元素；子 Element 的增改删也不自动扩大、收紧祖先 Range 或推进祖先 Revision。
7. 普通非 root Anchor 与 Entity 的 Range 都允许保持、扩大或缩小。最终球必须被直接父 Anchor 包含；Anchor 的最终球还必须包含全部直接子 Element。这些约束在同一提交状态中验证，任一不成立则原子拒绝，不保存中间 Parent、Pose 或 Range。
8. Anchor Range 是调用方声明的保守覆盖，不要求等于最小包围球；缩小是普通显式更新，不另设 zone-governance 行为。只读坐标换算、包含校验与空间相交计算不改变存储真相。若将来内容跨越 owner 边界，覆盖证明及提交协调仍须由后续合同定义，本地原子校验不构成跨 owner 原子承诺。
9. `SubtreeEnvelope`、Octree、BVH、children index 和查询 cache 均为可重建优化。它们可以多报候选；缺失、dirty、过期或 coverage 不明时必须回退到稳定 Range 或报告不完整，不能用于 complete miss。

兄弟 Anchor 可以重叠。强制不重叠会把通用空间组织误写成分片规则；病态重叠由预算和 `PARTIAL` 结果处理。

### D3. 数值不确定性必须向安全方向收敛

1. position、orientation 与 radius 的公共权威表示必须是有限、规范且可互操作的值；不使用 NaN、无穷或其他 numeric sentinel 表示语义状态。`UNBOUNDED_ROOT` 是类型化变体，负半径永远非法。
2. 数学上等价的旋转或数值编码不能成为不同真相。owner 接受后只保存和返回首个 interoperability successor 规定的一种 public canonical 表示；调用方输入编码不是第二真相。
3. 坐标、半径、Pose composition 和投影必须受明确 profile 上限约束。超出可表示或可证明范围是稳定失败或 `UNAVAILABLE`，不能溢出、截断或静默降精度。
4. 权威 containment 写判断不能因舍入错误接受数学上越界的 tuple；无法证明包含时拒绝写入。
5. 查询剪枝不能因舍入错误排除数学上可能相交的 Element；无法证明不相交时保留候选。
6. 深链投影必须使用确定性精确谓词、区间／误差包络、向安全方向舍入，或在证明不足时回退遍历。经验 epsilon 不能决定权威写入或 complete miss。
7. 不要求独立 owner 共享一个全局数值执行过程；每个结果携带其权威值和投影证据，跨 owner 组合遵守同一保守规则。首个 interoperability successor 必须只选择一种标量与旋转规范编码、判定算法和 conformance corpus；不能同时保留历史 profile 或多套权威表示。

D3 冻结的是可观察安全语义，不冻结数学库、SIMD、索引布局或 wire 字节。

### D4. Core 的最小真相与本地操作闭环

目标 Core 记录是：

```text
Anchor {
  ElementUUID
  ParentAnchorUUID?  # 仅 root 缺省
  LocalPose?         # 当且仅当存在 parent
  Range              # root-only UNBOUNDED_ROOT；其余 BOUNDED_SPHERE
  ElementRevision
}

Entity {
  ElementUUID
  ContainingAnchorUUID
  LocalPose
  Range              # BOUNDED_SPHERE
  ElementRevision
}
```

必须成立：

1. Anchor 与 Entity 是封闭 kind；UUID 在 create、Pose update、Range update、reparent、reanchor、route change、deployment change 和已证明的 owner transition 中不变，删除后永不复用。
2. root 的 parent、LocalPose 和有界 Range 均不存在；普通 mutation 不能 reparent 或删除 root。整个空间的退役属于另行批准的治理行为，不是 Element CRUD。
3. 所有会共同决定合法性的修改按最终 tuple 原子验证：

   ```text
   final spatial tuple = parent/containing Anchor + LocalPose + Range
   ```

   API 可以提供 create、update、reanchor、reparent 等方便行为，但不能要求调用者经过一个暂时非法的中间状态。
4. 同一 authority 内的 reanchor／reparent 是一个语义提交；后代记录和 Revision 不因祖先移动而改写，WorldPose 重新派生。
5. Anchor 普通删除不级联，并且只在其 owner 能在同一 commit domain 证明直接内容为空且不存在 `ACTIVE`／`DETACHED` delegation coverage 时成立。mount Anchor 不执行普通删除：未来显式 mount-retirement 行为必须先 fence admission／writer，再以 matching delegation／acceptance 与 final carrier cut 证明直接 coverage 已空，或证明全部内容已完成显式 transition；carrier 不可达、acceptance 撤销或 detached 永远不能证明 empty。Element 删除终止 Core existence；Facet 立即按 Core existence 三态（`PRESENT`／已确认 `NOT_FOUND`／`UNAVAILABLE`）隐藏，物理清理由 Facet 独立完成。`DENIED` 是在披露 fact state 之前返回的正交授权结果，不是第四种 existence state。
6. 每个 Element Revision 只标识该 Element 当前空间记录的权威切面，不获得时间、祖先、Facet attachment 或跨 owner 顺序含义。空间 tuple 的真实变化推进 Element Revision 与 Core read cut；Facet attachment 使用 D6.3 的独立 revision。每个 mutation 使用 owner-local OperationID、明确 CAS 前置条件和可收敛保留结果。
7. Core 至少公开以下所有者结果，名称和 DTO 另行决定：
   - 按当前 owner 上下文精确读取已知 Element；
   - 以稳定读切面分页列出一个 Anchor 的全部直接 child Anchor 与 Entity 的空间记录；该结果不披露 FacetAttachment，后者必须由对应 Element 的 exact read 返回；
   - 读取／验证当前父链并投影到指定 Anchor frame；
   - 原子创建、更新最终空间 tuple、reanchor／reparent 与非级联删除；
   - 返回 owner、record Revision、read cut 与三态存在性证据。
8. `ListDirect` 是 Structure 真相的必要读面，不是可选管理便利；没有它，消费者只能建立竞争的影子目录。它的 parent list 权限不能隐式升级为所有 direct child 的 Facet 入口读取权限。
9. 当前 Core 不定义 Metadata、专用 name／displayName／文本 path 字段；Element 空间记录不保存 WorldPose、ancestry、索引、Endpoint、Cluster、权限或业务关系。Core 的挂载及 Facet 附着联系记录可以按 D6／D8 权威保存 NextHop，但它不是空间字段。Metadata 与 displayName 只保留为未来候选能力；当前不预留字段、API、Store 或 Revision 语义。

D4 只定义 owner-local 语义。跨 authority 解析、查询和迁移分别由 D6、D9、D10、D12 组合，不能污染本地记录。

### D5. FacetTypeRef 是不透明内容键，不是名称或服务地址

Facet 是可选的专用扩展面。一个只有 Core 空间事实的 Element 已经是完整合法的 UCUPA Element；需要由独立 owner 保存内容、权限、Revision 或生命周期时，才建立 Facet 联系。Facet 保持一个最小且可开放互操作的真相形态：

```text
FacetKey(ElementUUID, FacetTypeRef) -> ExactPayload + Revision
```

1. 同一 `FacetKey` 在一个有效切面最多存在一条记录；没有通用 `slot`、`subjectKind` 或自由 metadata map。
2. `FacetTypeRef` 是任意、固定 32-byte 的不透明键。同一个 Element 与 TypeRef 的组合精确选择一条内容记录；它不要求可解析的类型制品、content-address、签名或中央 schema service。
3. Facet 对有界 Payload 按原始字节保存和返回；Put 不解析 Payload、不执行 schema 或业务类型校验。TypeRef 的有效表示、权限、容量、接受关系和 Revision 前置条件仍由各自 owner 检查。
4. 上层若为 TypeRef 约定领域含义、格式、版本或校验规则，应自行管理与验证；Facet 保存记录不构成内容符合该规则的证明。若上层改变同一标识的解释，其后果由该上层承担，UCUPA 不提供隐含迁移或自动重解释。
5. 不为类型合同再增加 Facet 内部 resolver、artifact、pin／mirror 或第二个 `contractRef`；TypeRef 也不是事实 authority 的地址或路由入口。
6. Facet owner 至少提供 exact Get／Put／Delete，以及按 Element 完整枚举当前 `FacetTypeRef`。类型枚举是开放消费者发现稳定扩展的必要行为，不是可选调试 API。Batch 是多个 item-level read 的优化，change read 属于 D11 Observe；二者只有在相应后继合同冻结证据、分页和失败语义后才进入实现，不扩大本地最小真相面。
7. 每个单一 subject 的 Facet Get／enumerate／Put／Delete 都必须把 Core existence 与 `FacetAttachment` 绑定到该 subject 的同一个明确 Core read cut，并同时确认：Element 在该 cut 上 `PRESENT`，且 attachment 为 `ACTIVE(exact responding FacetAuthorityRef, matching proposal/acceptance evidence)`。调用携带由 Core exact read 得到的 AcceptanceKey 作为 `FacetAccessContext`；Facet 用它选择已保留的 exact acceptance 及对应 `CoreNextHop`，直接联系并核对 exact CoreAuthorityRef 后 fresh-read current Core，不调用 Ref 地址目录。该 key 和联系记录都不是持久 FacetKey、existence evidence 或 Facet 保存的 current-Core pointer。`NONE` 表示没有已接受 namespace；`DETACHED`、authority mismatch 或无法验证只产生 `UNAVAILABLE`／not-current-owner，旧 authority 不得返回或提交当前 payload。只有通过 `ACTIVE(self) + matching acceptance` 后的 PRESENT／KEY_NOT_FOUND／complete-empty 响应才同时携带明确 Facet cut、Core-existence 与 attachment evidence；在进入 Facet namespace 前已由 Core 证明的 NOT_FOUND／NONE／DETACHED／authority mismatch 只携带 Core evidence，不能伪造 Facet cut。Get／enumerate 先在 Facet cut F0 捕获结果候选、acceptance 及联系 token，在锁外回查 Core，再验证 F0 与联系仍成立；变化则重新开始或明确失败，不能把 Core current check 之后才提交的 Facet fact 拼进同一结果。mutation 的接受依据也必须绑定同一 active attachment revision 与经验证的联系上下文。Core 与 Facet 不伪造跨 owner 线性化点：并发 detach／delete 后，fresh read 必须隐藏只绑定旧 Core cut 的记录；已在线性化于旧 cut 的在途结果则携带旧 evidence。
8. reanchor、reparent、Pose／Range 更新和 NextHop／Cluster 变化不改变 FacetKey 或 Facet Revision。Element 删除 cut 之后新建立的 Facet read 不再返回 payload；已在线性化于删除前 Core cut 的在途响应可以携带原 evidence 完成，删除不倒改历史切面。orphan 的物理清理是 Facet owner 独立、可重试的 Reconcile。
9. 外部资产、账户、物理状态、工作流和实时仿真仍由各自系统拥有。Facet 可以保存不透明引用或记录，但不吸收外部资源本体。

公开表示与互操作规则可以规定 TypeRef 的 32-byte 编码，但不借此给 Facet 增加内容格式解释或验证职责。

### D6. FactAuthorityRef、delegation 与 acceptance 共同证明唯一 owner

#### D6.1 稳定事实权威身份

`FactAuthorityRef` 只标识稳定的逻辑事实 owner。与它绑定的 `AuthorityEvidence` 才声明 Core／Facet kind、当前 coverage、authority epoch 和可用 read-cut 空间；这些可变属性不混入身份。Ref 跨进程、Pod、Cluster、endpoint、credential 和同一权威的 writer handoff 保持不变；真正的 owner／coverage replacement 可以建立新 Ref，并由 D12 给出连续性证据。

每个有效事实切面恰有一个 active authority：

```text
Core fact slice  -> one Core-kind FactAuthorityRef
Facet fact slice -> one Facet-kind FactAuthorityRef
```

`FactAuthorityRef` 不是权限主体本身。一个或多个已建立的 PrincipalRef 可以被授权操作该 authority，但 Realm Allow、credential、Cluster hosting 和联系信息都不能据此成为事实 owner。

#### D6.2 Core mount 权威边界

一个 mount 必须同时存在两份独立、revision-bounded 的控制事实：

```text
parent Core authority:
  DelegationBinding(mount ElementUUID, carrier Core-kind FactAuthorityRef, delegation revision)
  outgoing contact(exact relation, carrier Ref, carrier NextHop, contact token)

carrier Core authority:
  MountAcceptance(exact delegation, accepted admission context, acceptance revision)
  outgoing contact(exact relation, parent Ref, parent NextHop, contact token)
```

1. parent Core authority 唯一拥有 mount Anchor 的 Element 记录、Pose、Range、Revision、Facet attachment 和 delegation。
2. active carrier Core authority 唯一拥有放置在该 mount 内的直接 Entity、直接子 Anchor 及其尚未再次委派的后代；它不复制或声明 mount Anchor。
3. 每个 delegation revision 最多有一个 active carrier coverage。一个 mount 的多 carrier 分片不进入 baseline，不能用“查到了哪些路由”猜 owner 集。
4. carrier 写入使用独立的 `MountAdmissionContext`：mount UUID、Range cut、delegation／acceptance evidence 与 admission epoch。祖先 WorldPose 不参与 local containment。
5. 跨空间投影另用 `CoordinateProjectionContext`：mount UUID、WorldPose 及覆盖整条祖先 Structure/Pose 的 read evidence。祖先移动使投影 context 失效，但不会自动使仍然有效的 local admission 失效。
6. `MountAcceptance` 使 mount UUID 成为 carrier 的外部 adjacency root：carrier 可以在该 context 下完整列出和查询自己拥有的直接内容，但不拥有、复制或修改 mount Anchor record。
7. nested mount 可以存在；同一 Structure／coverage ancestry 必须无环且每个切面 owner 唯一，但 authority 节点图不要求全局无环。A 在一个独立 branch 委托 B、B 在另一个 branch 委托 A 可以合法；route loop 检测不能替代单条事实 ancestry 的防环。
8. 两方向的 NextHop 由各自保存联系记录的一侧在本地授权与 CAS 后维护；地址变化只推进该侧联系 token 与 owner-local cut，不改 ElementRevision、delegation 身份、acceptance 内容或 admission epoch。指向同一目标的新地址不能建立新挂载、改换 carrier 或免除写门协调。

#### D6.3 Facet attachment 是 Core 记录的逐 Element 权威入口

Core 可达不推出 Facet 可达。为了在没有全局 Facet 目录或广播的情况下唯一定位一个 Element 的扩展面，每个 Element 在一个有效 Core cut 上必须有一个由当前 Core authority 保存并返回的 `FacetAttachment` 控制事实：

```text
FacetAttachment lifecycle(ElementUUID)
  -> NONE(AttachmentRevision)
   | ACTIVE(AttachmentRevision,
             one Facet-kind FactAuthorityRef,
             exact accepted ProposalRevision)
   | DETACHED(AttachmentRevision,
               former Facet-kind FactAuthorityRef,
               former accepted ProposalRevision)

Core-side contact(exact pending / ACTIVE relationship)
  -> expected FacetAuthorityRef + FacetNextHop + contact token

Facet-side contact(exact AcceptanceKey)
  -> expected CoreAuthorityRef + CoreNextHop + contact token
```

它是 Core Store 中的逐 Element 控制真相，但不属于 Element 的空间记录；Core 侧 pending proposal 及 ACTIVE 附着联系记录保存目标 FacetAuthorityRef、权威指定的 `FacetNextHop` 与独立联系 token，不复制 Facet owner read cut 或 payload。AcceptanceKey 仍为 `(FacetAuthorityRef, CoreAuthorityRef, ElementUUID, ProposalRevision)`，不含地址；对应 immutable acceptance 由 Facet owner durable retained。Facet 同时按该 AcceptanceKey 保留可维护的 `CoreNextHop` 联系记录，供 D5 的 source-Core fresh check；该记录不覆盖 immutable acceptance，也不是 current-owner 指针。网络连接使用 D8 的关系指定地址并独立验证预期身份，不能把连接成功写成新 ownership。Core 的 exact Element read 可以在同一 Core cut 返回 Element record、attachment 与该关系的联系信息；内部最小披露 port 也可以从同一切面只投影 existence 与 attachment，但不得维护第二份 existence。`ACTIVE` authority 唯一拥有该 Element 的完整 `FacetKey` namespace，因此同时回答完整 TypeRef enumeration 和每个 exact key 的权威读取／变更。`DETACHED` 的 former AcceptanceKey 与当前 Core cut 已足以保留“曾经 ACTIVE、内容没有被证明删除、但当前入口不可用”的区别，不能折叠为 `NONE`；保留的旧联系信息不能使其重新 ACTIVE，successor continuity evidence 只在 D12 transition 中增加。

Element create 在同一 Core commit 中初始化 `NONE`。AttachmentRevision 只标识该 Element 的 attachment 控制 lineage；proposal、cancel、activate、detach 推进它与 Core read cut，但不伪造 ElementRevision 变化。Pose、Range 或 Structure 更新推进 ElementRevision 与 Core read cut，但不改变 active FacetAuthorityRef 或 AttachmentRevision。Element delete 在一个 Core commit 中终止 existence 与 attachment 当前效力，且不跨 Store 删除 Facet bytes。

纯联系变更由保存该方向记录的一侧独立授权、CAS 并持久提交：Core 维护 FacetNextHop，Facet 按 AcceptanceKey 维护 CoreNextHop。它只推进联系 token 与该 owner 的 cut，不推进 AttachmentRevision、ElementRevision 或 FacetRevision，不改 ProposalRevision、AcceptanceKey 或 immutable acceptance。变更地址不会隐式替换 owner、重新接纳 namespace 或授予权限；已保留的旧 acceptance 可以继续作为历史被核对，当前可服务性始终以 exact source Core 的 fresh evidence 为准。

这是必须可观察的逻辑关系，不冻结私有 Store 布局。baseline 不把一个 Element 的 Facet namespace 再分片或转委派；否则“完整枚举”会重新依赖另一层 coverage 证明。若 Core 在明确 read cut 上确认 `NONE`，含义是该 Element 当前没有被 UCUPA 接受的 Facet namespace，而不是要求广播所有 Facet 服务；`DETACHED` 则按 D6.4 返回 unavailable evidence。

`NONE -> ACTIVE` 不能由 Core 单方面写入一个 Facet authority 指针。当前 Core authority 先持久化绑定 exact CoreAuthorityRef、ElementUUID、AttachmentRevision 与目标 FacetAuthorityRef 的 proposal，并保存 FacetNextHop。首次 Accept 的已授权调用方显式给出 `InitialCoreNextHop`；目标 Facet 按该地址联系预期 Core、验证其现时 exact proposal 后，在一个 Facet 本地事务中同时保存 immutable acceptance 与该 AcceptanceKey 的初始 CoreNextHop 联系记录，以 Facet cut 作为 `AcceptedAt`。旧 acceptance 的重放不能暗中覆盖已维护的联系地址。Core 只有在 proposal 仍有效且 acceptance 仍匹配时才能提交 `ACTIVE`，目标 authority 也只有观察到该 active attachment 后才能按 D5 对外服务。

Core cancel 与在途 Accept 不构成跨 Store 原子排斥：Facet 在先前已验证 proposal 后仍可能提交迟到的历史 acceptance。它不能让已取消 proposal 再次 ACTIVE，也不能凭该 acceptance 对外返回当前 namespace；Core 在最终激活提交点重验 proposal，Facet 在每次服务时执行 D5 fresh check。acceptance 不再增加含义重复的独立 AcceptanceRevision。该握手是两方控制事实，不复制 payload，也不引入中央 Facet 服务或 authority-wide 固定 peer。

更换 `ACTIVE` FacetAuthorityRef 是 D12 authority transition；同一 Core authority 内的 reanchor 不触发更换。一个 Core authority 的不同 Element 可以 attach 到不同 Facet authorities，一个 Facet authority 也可以接受来自不同 Core authorities 的 Element。这个最小入口只解决 attachment、唯一 owner 与完整性，不把 Facet existence 或 payload 复制进 Core。

#### D6.4 Revoke 不是删除

当 delegation、immutable acceptance 的当前效力或 FacetAttachment 被撤销时，已提交的 acceptance 本身仍保留、不删除也不改写；此后：

- 旧 carrier／Facet authority 的后续本地写不再被该关系接受为 connected-space 事实；撤销关系本身不能代替 durable writer fence；
- 已持有内容不会被解释为已删除，也不会自动转归 parent、Realm 或联系记录管理者；
- 控制关系 owner 必须保留 detached／retirement evidence，使后续 source-set read 不能把“曾有但尚未处置的 coverage”折叠成“从未存在”；
- 在成功 handoff、恢复原关系或明确退役前，相应 branch／Facet namespace 对通过该入口的读取是 `UNAVAILABLE`／detached evidence；已在明确 Core read cut 上确认 `NONE` 的情况除外，它表示没有已接受的 Facet namespace；
- `ACTIVE`／`DETACHED` delegation 都阻止 mount Anchor 的普通删除；unreachable、revoked 或 detached 不证明 carrier coverage 为空，只有完成 D4 所述显式 retirement／transition 后才可处置该 Anchor；
- 已确认写入和 UUID 必须保留，具体恢复由 D12 处理。

### D7. Access-control／trust owner 管身份与权限；Realm 是当前实现角色

1. trust boundary 先把 credential／签名／会话证据建立为一个当前可信的 `PrincipalRef`；access-control owner 再计算精确关系：

   ```text
   PrincipalRef + PermissionID + OperationScope -> Allow / Deny
   ```

2. 当前 UCUPA 模块架构由 Realm 承担这个 access-control／trust owner 角色。Core 与 Facet 定义自己操作所需的 PermissionID 和 Scope 语义，并在最终 read／mutation boundary 根据当前资源事实执行；Realm 不读取或复制 Core／Facet 真相。
3. baseline Scope 至少支持：
   - `CoreElementScope(CoreAuthorityRef, ElementUUID)`；
   - `FacetElementScope(FacetAuthorityRef, ElementUUID)`；
   - `FacetKeyScope(FacetAuthorityRef, ElementUUID, FacetTypeRef)`；
   - exact Realm administration target；
   - `AnchorClosureScope(BoundaryAnchorRef)`，表示对一个已进入空间中的明确 Anchor closure 的操作权限。
4. exact Core／Facet scope 必须绑定事实 authority，避免同一 ElementUUID 发生真正 owner transition 后旧 Grant 被新 owner 自动接受；Ref 只限定 Grant 的适用 owner，不使 access-control owner 成为 fact owner。`AnchorClosureScope` 不是裸 UUID locator、文本 wildcard 或 Realm 树。access-control owner 只比较精确 scope 值；Core 必须在对应 Structure read／commit cut 上验证候选 Element 当前位于该 closure，调用方提供的 ancestry 不具权威性。
5. `BoundaryAnchorRef` 按上述二元组精确相等，可作为 durable Scope 值：同一 authority 的 writer handoff、NextHop／credential 变化和 boundary reparent 不改变它；Anchor delete 使该 Ref 永久 retired；跨 Core authority transition 产生新的 Ref，旧 Grant 不自动迁移。closure scope 在每次操作 cut 上求值，因此默认覆盖当时已位于 boundary 下的未来新后代；移出 closure 即不再覆盖，移入则进入覆盖。
6. 一个 scope 跨越 mount 不等于 permission 传递。每个 contributing fact authority 独立建立 caller 并授权其本地 coverage；delegation、acceptance、联系信息、Cluster membership 和远端 issuer trust 均不自动授予读写权限。
7. 未知结果的 Discover 必须使用 closure-bounded discovery permission；要求调用方先拥有每个未知 UUID 的 exact Grant 是逻辑矛盾。
8. request-level authorization failure 返回不声明资源存在与否的 `DENIED`，绝不伪装成 fact-owner-confirmed `NOT_FOUND`。内部遍历与结果披露分离：每个 contributing fact authority 可以在自己的 coverage 内执行必要剪枝／转发，但不得泄露被拒绝节点的标识、数量或拓扑；不存在一个跨 owner 的全局特权协调器。
9. baseline 发现结果必须携带可验证 StructuralAddress／projection evidence，因此披露一个 Element 也要求披露解释和继续寻址它所必需的 Anchor 链。若链上任一必需节点不可披露，该 Element 就不属于对应 per-owner authorization view 的可披露结果；不能一边把它算作“允许结果”一边静默省略。opaque descendant handle 若未来被证明，必须是单独合同，不在 baseline 中假定。
10. `COMPLETE` 总是相对于 caller-visible recursive source graph、可披露的 per-owner authorization-view vector 与 aggregate opaque closure commitment。每个 access-control owner 都在本地建立自己的 view；只有允许披露的 owner 才公开 view stamp，不可披露分支的 owner-local 授权与遍历结果统一折叠进不泄露 owner、节点、数量或拓扑的 aggregate commitment。missing evidence 只能披露调用者被允许知道的 source。若无法确认一个潜在 branch 已由对应 view 和 closure evidence 闭合，结果只能 partial／unavailable；任一 view 改变都使旧 continuation 失效，不能沿用已撤销授权。
11. access-control owner 不拥有角色继承、组织树、资源目录、空间 root、工作流或通用 policy DSL。若上层产品需要这些能力，它们生成明确 PermissionBinding，而不是改变 UCUPA 权限内核。

#### Realm 授权访问链补充

`RealmAttachment` 只补齐接收方怎样联系已经确定的授权管理方，不改变上述主体、操作、目标的
精确授权关系。它沿用 `RealmID + NextHop`：按指定地址联系，核对预期 Realm，再在已证明的
身份建立边界内校验调用者和当前权限。RealmID 相等本身不是远端认证；调用者填写的 Realm 或
地址不能替换接收方受控的授权联系。Core／Facet 获得允许后仍独立校验各自业务不变量。

授权记录、凭据校验记录仍由 Realm 拥有，不因 attachment 被复制到 Core／Facet；Realm
也不因此成为 Element／Facet 的事实 owner。该名称不要求照搬 Facet proposal／acceptance 状态机，
不预设每个 Element、每条 Facet payload 都新增字段，也不新造 `RealmAuthorityRef` 或身份地址目录。

联系的持久归位、与受保护事实上下文的绑定、同 owner 更换承载后的恢复、地址维护及跨 owner
转移时的核验，由后续合同落实；实现必须保持本节已有的 owner-qualified scope 和旧 Grant 不自动
转移规则，不能通过部署配置或联系变更静默重新解释授权。联系缺失、错目标或无法完成必要校验时
不得放行，也不能把不可达伪装成已确认无权限或事实不存在。

多个独立 Realm 各自管理其身份和授权，不要求全网总 Realm 或自动同步。补齐这条访问链不要求
Realm 多副本、共识或高可用。现行本地装配已能接入 Realm，但尚未实现 RealmAttachment；
不能把未来联系机制当成现有本地授权能力的必需前置。实现安排只由开发计划维护。

### D8. Core 与 Facet 都沿关系指定的联系方式访问

Core mount／Facet attachment 的事实服务联系明确持有：

```text
exact relationship contact {
  expected target FactAuthorityRef
  authoritative NextHop
  owner-local contact token / read cut
}
  -> call NextHop
  -> verify expected target + exact relationship + required caller authorization
```

指向 Realm 的授权联系沿用“预期目标身份＋指定地址”的原则，目标类型使用 D7 的 `RealmID`，
不是 `FactAuthorityRef`；不把两类身份合并，也不由本示例提前冻结 RealmAttachment 的 Store 或 API。

1. 预期 Ref 回答“应当联系谁”，NextHop 回答“这条关系指定怎样联系”。调用按 NextHop 进入传输适配器，不以裸 Ref 查询地址目录；没有地址时不得偷偷调用另一套 resolver。Ref 本身不嵌入地址，也不是全网 locator。
2. Core mount 与 Facet attachment 使用同一原则：parent Core 的 mount 联系指向 carrier，carrier 的对应联系指向 parent；Core 的附着联系指向 Facet，Facet 按 exact AcceptanceKey 的联系指向 source Core。每个方向由其本地联系记录 owner 在自己的授权与提交边界维护，不要求一份 authority-wide peer pair、一个 authority 唯一全球地址或所有 Ref 均有联系字段。
3. NextHop 的服务入口由部署环境提供，例如稳定服务名、网关地址或本地传输入口；关系 owner 决定本关系采用哪个入口并持久提交。部署可以更新入口背后的进程或 IP，但不能替关系 owner 改写事实归属。底层 DNS、IP 路由或按地址的本地端口装配不等于 UCUPA Ref 地址目录。
4. 联系更新有独立 contact token 与 owner-local cut，使用授权、CAS、OperationID 和可收敛结果。纯地址变化不改变关系代次、FactAuthorityRef、ElementRevision、FacetRevision、AttachmentRevision、immutable acceptance 或 writer fence，也不能激活 pending／detached 关系。地址的持久指定不可作为缓存任意丢弃；实际连接对象、解析所得 IP 和可达性缓存可以重建。
5. 发送方按已确认关系携带预期目标、精确关系／操作及 caller context；接收方独立核对，返回来源也须与预期匹配。可信本地装配只能证明该本地 profile 的目标绑定；Ref 字符串相等不是远端认证。网络认证、credential、服务能力与 freshness 的公开证据须由后继合同证明。
6. 地址缺失、过期、连接中断或目标／安全绑定不匹配返回 `UNAVAILABLE`，有效聚合查询按 D10 返回明确 partial；它们不返回 `NOT_FOUND`，也不撤销 delegation、permission、Element 或 FacetAttachment。旧地址只能造成失败或经验证的在途结果，不能决定新 owner。
7. 本主线不再以 `ResolveRoute(FactAuthorityRef)`、RouteRegistration、独立 Ref 目录、referral 链或 lease 为 Core／Facet 的强制前置；这不是把旧系统延期。网络传输、地址更新传播、多地址、负载均衡与只读副本若需要，须另行证明它们如何消费关系指定的联系而不控制 ownership、permission 或 writer activation。当前共同联系原则不宣称这些运行能力已实现。
8. 结构路径逐跳访问可受 hop、fan-out 和 deadline 限制，耗尽时显式失败、continuation 或 partial，不改变事实拓扑。`FacetTypeRef` 只选择内容记录；它不是事实服务的 Ref → 地址解析，也不要求 Facet 解析类型合同。

### D9. SpaceReference 可分享，SpaceHandle 临时；结构地址是一条相对 UUID 链

#### D9.1 语义空间边界

一次接入必须明确一个 `boundary Anchor`。本次结构解析与默认空间查询的 semantic universe 是该 Anchor 在指定 Structure/read cuts 下的 closure，而不是调用方猜出的共同祖先，也不是“从当前位置固定向上 N 层”。boundary 可以是整个有根空间的 root，也可以是被明确发布和授权的有限子空间 Anchor。

一个有根 Structure closure 恰有一个 root Anchor。不同 `UNBOUNDED_ROOT` 之间没有隐含共同坐标、共同 Structure 或完整查询；若消费者同时持有多个 SpaceReference，它得到的是多个独立 universe 的结果集合，而不是一个伪造的全局 World。

#### D9.2 两种接入 artifact

```text
SpaceReference  # 可分享、无 caller secret、允许 stale hint
  boundary Anchor reference
  initial Core FactAuthorityRef/evidence candidate
  trust bootstrap reference
  initial Core NextHop + source / freshness evidence

SpaceHandle     # 调用者临时建立、不可当作永久真相
  resolved boundary
  established PrincipalRef + root authorization request/view context
  validated authority/delegation/acceptance contexts
  current reachability contexts
  independent freshness/read evidence
```

SpaceReference 不携带可转移的调用者凭据，也不保证其中 authority／NextHop 当前有效。任意组织都可以发布 reference；它只能携带或指向 bootstrap evidence，不能创建、修改或替代 authority binding。调用者按明确入口建立连接，根据自己的 trust policy 验证实时 authority chain 后进入，不把缺失地址补成裸 Ref 查询，因此不需要 UCUPA 全球入口目录。

SpaceHandle 不拥有一个共同 Revision，也不预装尚未发现的远端 PrincipalRef／authorization view。authority、authorization、关系联系 token 与实际连接各自验证；遍历到新 owner 时，从对应关系取得预期身份与 NextHop，该 owner 的 trust／access-control owner 根据调用者提交的 identity／trust evidence 独立建立本地 PrincipalRef 和 view evidence，root 已建立的 PrincipalRef 或 issuer trust 不自动传递。任何一项更新都只重建相应 context；Handle 可以被丢弃并从 SpaceReference 重新建立。

#### D9.3 StructuralAddress

`StructuralAddress` 是在已接入空间内，从已知 Origin Anchor 到 target 的结构路径；可以先沿父边上行，再沿 ElementUUID 链下行。每条边由选定 read cut 的 parentage／placement 及必要 mount evidence 验证，非终端项必须是 Anchor，终端可为 Anchor 或 Entity。默认 Origin 可取 handle boundary，纯下行是其常见特例。Origin 不自动成为语义边界；显式 handle boundary 仍须遵守，不能因上行跨过它。

```text
{ OriginAnchor, upward parent edges, downward ElementUUID chain }
```

因此：

- 地址中的 UUID 组织了路径，但 UUID 本身仍只表示身份；
- reparent／reanchor 后地址改变，ElementUUID 不变；
- 旧地址可以返回经相应 owner 确认的 `NOT_FOUND`，或可选的短期 stale/referral；不要求永久 redirect；
- 解析从已知 Origin 开始逐边验证，上行也依赖真实 parent evidence；跨 mount 的正反方向都读取该方向指定 NextHop，并分别验证 authority evidence、authorization、联系 token 和响应来源；
- 路径上行次数是地址结构，不是查询 UpDepth；查询返回从原入口到实际 Scope 的可复读路径，不能用原起点直接拼接 Scope 相对命中；
- 只给裸 ElementUUID 且没有 authority／space context 时信息不足，系统不广播整个开放体系。

`AnchorUUIDPath` 只是 StructuralAddress 中 Anchor 前缀的解释性名称，不再成为第二种地址真相或另一套持久类型。

### D10. Structure 发现与空间 Query 使用同一个明确 universe

UCUPA baseline 提供三种互补的发现行为：

1. **Resolve：** 已知当前 StructuralAddress，逐跳验证并读取终端 Element；
2. **ListDirect：** 在一个 authority read cut 上分页列出 Anchor 的完整直接邻接；
3. **QueryRegion：** 由已知 Start 及向上深度解析出明确 Scope，再在该 Scope 的完整闭包内按空间相交发现未知 Element。Scope 不越过已声明的 SpaceHandle boundary。

最小空间请求语义是：

```text
SpatialQuery {
  SpaceHandle
  Start + UpDepth      # -1 到已证明边界，非负值限制上溯层数；无向下深度
  resolved Scope       # 由选定结构切面解析；不是另一个独立过滤器
  QueryRegion          # 一个在明确 Anchor basis 中的有限球
  selector             # Anchor / Entity / both
  result basis
  execution budget
}
```

1. `QueryRegion` 是临时请求输入，不是 Element Range。其 basis 必须由 handle 内的当前 StructuralAddress／projection evidence 解析并能投影进 semantic boundary；输入本身不属于该 context 时拒绝为 `INVALID`，所需 evidence 当前无法取得时为 `UNAVAILABLE`。
2. selector 只决定哪些 Core kind 可以成为结果，不决定遍历哪些 Anchor。即使只返回 Entity，也必须遍历所有可能含有 Entity 的相交 Anchor zone。
3. 对普通 Element 的最小命中是两个闭球相交或相切：

   ```text
   distance(query.center, element.origin) <= query.radius + element.radius
   ```

   因此，Element 中心在查询球外但 Range 相交时仍必须命中。
4. selector 包含 Anchor 时，Anchor 以自己的 closure Range 命中；`UNBOUNDED_ROOT` 对任意有限 QueryRegion 命中。默认产品 UI 可以不请求 root，但不能改变其语义。
5. 查询沿已验证父边从 Start 上溯，选出明确 Scope，然后搜索 Scope 自身及完整后代。不再设置向下深度；祖先直属 Entity、兄弟分支及其后代不可遗漏。Start 与 query basis 独立，深层 basis 不隐式缩小 Scope。上溯必须由 Structure 证明，不由 carrier 猜父方或把接收服务当空间根。
6. structure scope 与空间 region 是独立约束：UpDepth 选择前者，查询球定义后者。用户确认 UpDepth=-1 表示不限上溯层数，一直到已证明的空间根或已声明的 SpaceHandle boundary；不是全网遍历、越权或跨独立根。小于 -1 非法，非负值按层数选择。选定 Scope 之外不属于本次搜索，不因此产生 PARTIAL；Complete 明确只针对该 Scope。profile 明确零／缺省及根处行为，未知父边、拒绝或断连不能冒充到达边界。查询入口服务与显式 SpaceHandle boundary 不混淆。
7. **UpDepth 是语义 Scope 选择参数，不是执行预算。** 转发 hop、fan-out、deadline、page size、result limit、内存／成本配额及精确证明工作界另属执行约束，不能改变所选集合。查询 context 已成立后未完成其后代只可 continuation／PARTIAL 或合同明确的失败，绝不把尚未访问分支当空。部署边界不增加或重置结构深度。
8. 空间索引只用于产生候选与安全否定。优化结果必须与同一 cuts 上不剪枝的线性 oracle 一致；任何可能的 false negative 都推翻该索引 profile。
9. Core spatial query 不按 Facet payload、资产、Collider、物理或产品字段过滤。消费者先发现 Element，再通过 D5 读取 Facet；未来跨平面组合查询必须作为明确的、带双 owner evidence 的新行为证明，不能暗中耦合 Store。
10. 一个 verified match 不能只是裸 UUID；它至少关联 ElementUUID、kind、相对本次 boundary 的 StructuralAddress、source FactAuthorityRef／read cut 与相交／projection evidence。完整 Core／Facet payload 仍由后续 `Read` 取得，Discover 不复制另一份记录真相。

#### D10.1 完整性不是一个布尔装饰

`INVALID`、`DENIED` 与 `UNAVAILABLE` 是产生 `DiscoveryResult` 之前的 request／execution outcome：`INVALID` 表示请求不能在给定 context 中形成有效的 normalized query；`DENIED` 表示 request-level authorization 未建立；`UNAVAILABLE` 表示 semantic boundary、query basis 或其 root fact-authority cut 当前无法建立，因而连一次有确定语义的查询都不能开始。三者都不返回 matches，也不伪装成空结果；只可携带本 outcome 已被允许披露的诊断 evidence，不能携带尚未建立的 source graph。`DENIED` 尤其不能确认目标、branch 或匹配是否存在。

有效查询的结果至少携带：

```text
DiscoveryResult {
  verified matches
  status: COMPLETE | PARTIAL
  caller-evidence identity + authorization request identity
  normalized query identity {
    handle boundary + Start + UpDepth + resolved Scope / ancestor evidence
    QueryRegion + query basis
    selector + result basis
  }
  caller-visible authorization evidence {
    disclosable per-owner authorization-view vector {
      fact/access-control owner
        + locally established PrincipalRef/evidence + view stamp
    }
  }
  disclosable contributing Core FactAuthorityRefs + per-authority read cuts
  caller-visible recursive source-evidence graph {
    disclosable visited parent + complete ListDirect cut/pages
    disclosable safely pruned edge evidence
    disclosable mount delegation + matching acceptance cuts
  }
  aggregate privacy-preserving opaque closure commitment
    # 绑定内部不可披露的授权与遍历结果，不暴露 owner、节点、数量或拓扑
  disclosable projection/range coverage evidence
  authorized missing or dirty branches/sources
  relation contact revisions + independent connection evidence for disclosable sources
  continuation?
  opaque observation/resume state?  # 仅在请求持续观察时
}
```

query planner／gateway 可以在各 contributing authority 的本地授权边界内使用更完整的内部 traversal graph，但绝不能把该内部图原样序列化给调用者。公开 `DiscoveryResult` 只包含 authorization views 允许披露的 owner vector 与子图；不可披露分支的 owner-local 授权及遍历结果统一折叠进与本次 request／cuts 绑定、且不暴露 owner、Anchor、数量或拓扑的 aggregate opaque closure commitment。`COMPLETE` 证明的是“caller-visible evidence graph + aggregate opaque-closed branches”已经闭合，不是调用者取得了内部全图。

- `COMPLETE`：normalized query identity、semantic boundary/root authority cut 与 root owner 本地建立的 Principal／authorization view 已成立；递归发现的每个 contributing owner 都独立建立本地 PrincipalRef。允许披露的 owner 提供绑定本次 request identity 的 establishment evidence + view stamp；不可披露分支的 owner-local 判定只折叠进不泄露 owner 身份、数量或拓扑的 aggregate opaque closure commitment。最终 caller-visible per-owner vector、source-evidence graph 与该 aggregate commitment 一起闭合，所有可能相交且允许披露的分支都被安全剪枝或完整处理。只有此时，空 matches 才能表示确认没有结果。
- `PARTIAL`：上述查询根 context 已建立，但至少一个后代 source 因 missing／dirty／budget exhausted 而未闭合；已有 matches 均可验证，matches 也可以为零。某个后代 branch 不可用只使聚合结果为 `PARTIAL`，不能把整个已定义查询改称 `UNAVAILABLE`。missing evidence 可以是经授权的不透明摘要，不能借错误详情泄露被拒绝 branch 的标识、数量或拓扑；调用者不得把未返回项当成不存在。

查询开始时只冻结 normalized query identity、调用者提交的 identity／trust evidence identity、authorization request identity、semantic boundary/root authority cut，以及 root trust／access-control owner 本地建立的 Principal／authorization-view evidence；然后递归构造 caller-visible source-evidence graph、可披露的 per-owner authorization-view vector 与 aggregate opaque closure commitment。它既不假定存在一个能在开始时枚举全图的全局 source snapshot，也不假定 root PrincipalRef、root issuer trust 或尚未发现的远端 authorization view 可以全局复用：

1. 每个被访问的 parent 都必须在其 authority 内部的稳定 cut 上通过完整分页的 `ListDirect` 证明直接邻接闭包；只有对应 view 允许时，parent、pages 与 edge evidence 才进入 caller-visible graph；
2. 每条候选 edge 要么由保守 Range evidence 安全剪枝，要么递归处理；
3. 每条未被 parent-owned mount Range evidence 安全剪枝、仍可能相交的 mount edge，都必须绑定 exact `DelegationBinding` cut 与 matching `MountAcceptance` cut，随后在 carrier authority 自己的 cuts 上继续；已被安全否定的 mount 只保留 prune evidence，不要求连接、acceptance 或远端 carrier 可用；
4. 递归首次到达一个 fact authority 时，其 trust／access-control owner 必须针对调用者提交的 identity／trust evidence 和同一 authorization request identity，独立建立该 authority 语境中的本地 PrincipalRef 与 authorization view；允许披露时，把 principal-establishment evidence + view stamp 绑定进 caller-visible vector；不可披露时，只把 owner-local 判定折叠进 aggregate opaque closure commitment。无法取得或验证时，该 branch 不能被静默当成已授权空分支；
5. 只有所有可能相交且按各 owner view 允许披露的分支都在 caller-visible graph 中闭合，且所有不可披露分支的 owner-local 结果都被纳入不泄露 owner／节点／数量／拓扑的 aggregate opaque closure commitment，结果才能是 `COMPLETE`；仅记录“计划访问了哪些 owner”不能证明 source 或 authorization closure；
6. parent cut 之后新增的 edge 不属于该 cut。Retained 有期限保留已选 facts/evidence，current 的普通事实变化不使旧 continuation 自动失效；CurrentVerified 不跨请求保留旧视图，续页必须证明原 cuts 仍 current，已改变就明确过期，不能换成 latest 继续。保留依据到期／丢失、当前许可或合同要求的连接效力不能建立时，必须按失败来源失效、重验或在仍可验证根 context 上降为 PARTIAL，不能偷偷读 latest；
7. continuation 必须绑定同一 normalized query identity、caller-evidence identity、authorization request identity、已取得的 caller-visible per-owner principal/view vector、source-evidence graph、aggregate opaque closure commitment 与可披露 cuts。不可披露的 owner、view、cut 和 traversal state 只能由不泄露其身份、数量或拓扑的 privacy-safe opaque continuation state 继续绑定，不能原样序列化。在构建尚未封存的来源图时，递归发现新 owner 可以独立建立 Principal／view；分页开始后的来源接纳规则须由 profile 明确。P3 先封存固定来源与缺口再返回首页，后续不可补入原 Excluded 来源，否则新增的较小 UUID 会被遗漏。任何方式都不能悄悄换 Scope、既有 cuts 或调用者证据；保留事实不冻结 Allow，每页仍重新检查当前权限。

#### D10.2 可选快照与固定切面分页

写入经本 owner 校验、可靠持久化后确认；分页一致性不通过延迟提交实现。
普通读取默认 latest committed。每 Core 在启动时独立配置可选的 `SnapshotRetention`：
未填即关闭旧视图保留，没有默认保留时长；显式填写可表示的有限正时长才启用，无效值使配置失败。
关闭不是关闭查询或分页，也不妨碍单次调用捕获临时一致视图。

`ReadMode` 在首页确定并公开，同一链不能自动升级或降级：

| ReadMode | 固定读取依据与后续行为 |
|---|---|
| CurrentVerified | 不跨请求保留旧事实视图；查询、分页和范围内精确读仍可用，复用时必须证明原 cuts 仍 current。已改变就明确过期，不把新结果接到旧链；不承诺持续写入下旧分页能够完成 |
| Retained | 按配置的有限期限保留选定视图；普通事实写入不使其失效，旧页及范围内精确读使用原 cuts，直到释放、到期或合同规定的失效 |

CoreReadCut 标记事实，Retained 的运行内句柄负责旧视图寿命；不是任意时间戳查询，也不是另一份永久真相。

同一分页链固定 ReadMode、Start、UpDepth、Scope、Basis、条件、来源 cuts 与排序位置；改变深度须新查。
短期保留有有限 TTL、容量与释放规则，过期不回退 latest；两种模式的精确读都须独立授权。
来源所有者重启后的上下文效力由合同明确，P3 不恢复已丢失的运行内保留句柄。

跨 Core 根据本次实际来源决定模式，包含只提供关系、邻接或投影证明的 owner；任一方未启用保留，
整链使用 CurrentVerified，所有实际来源都启用才使用 Retained。未启用不是不可达、过期或 coverage 缺口；
Excluded 不计入已取得来源且不能在旧链后补。资源不足按合同处理，不能自动改用另一模式，
也不能将无法联系误报为已确认 cut 改变。
跨 Core 仍需匹配关系／接纳／投影依据，并给出所承诺组合的一致性见证，不能仅凭时间戳宣称一致。
P3 双 Core 首次括读证明两份状态共同存在；CurrentVerified 在复用时核对原 cuts；
Retained 使用保留视图，不要求每页各方 current 等于旧 cut。两者都不扩展成全网原子快照，
也不在首页后补入未取得的来源。
当前撤权、挂载关闭及指定联系变更如何阻止旧链继续披露，由 owner 合同明确检查，不由缓存代替。

多 owner 没有一个合成 `WorldRevision`。每个 owner 选择自己的 read cut，mount／projection evidence 证明这些 cuts 如何连接；无法证明根连接时返回 `UNAVAILABLE` outcome，无法闭合某个后代连接时重启或返回 `PARTIAL` result。

### D11. Read evidence 与 committed observation 构成可恢复链

#### D11.1 精确读取证据

读取证据必须保留三类不同职责，不能因为同一服务参与多个职责就压成一个通用证明：

- **Core／Facet 事实及其 delegation／acceptance／namespace-attachment 与联系控制事实：** Core-或 Facet-kind `FactAuthorityRef`、`AuthorityEvidence`、owner-local `ReadCut`、记录 Revision／retirement evidence；权威指定 NextHop 的修订属于保存该方向联系记录的一侧 cut，但不改变空间记录、payload 或关系身份；
- **授权：** access-control owner identity（当前实现使用 `RealmID`）、Principal establishment evidence、Allow／Deny 与 authorization-view stamp；
- **实际可达性：** 按上述指定联系建立连接所得的目标认证、endpoint binding 与独立 freshness；它不替代持久联系修订，不混入空间／payload Revision，也不证明 writer activation。连接缓存丢失可以重建，联系控制历史丢失则是 Store／恢复问题。

exact read outcome 至少区分 `PRESENT`、`NOT_FOUND`、`UNAVAILABLE` 与 `DENIED`。`DENIED` 不声明存在性；`NOT_FOUND` 只有在 exact fact authority、scope 和 read cut 均已确认后才成立。Core 在明确 cut 上确认 `FacetAttachment = NONE`，可以证明不存在已接受的 Facet namespace；但 attachment 状态无法确认、联系失效、权限证据无法验证、Store 故障或 source-set 不完整都不是 `NOT_FOUND`。

聚合读取的内部 cuts 是一个向量，但公开结果只披露 authorization views 允许的 owner／cuts；不可披露分支仍使用 D10 的 opaque closure commitment。这个向量可以描述“可披露 owner 分别在什么切面被读取”，但不声称全局同时刻、全局总序或跨 owner 事务。

#### D11.2 Observation／resume

每个 authoritative owner 必须能从一个已返回 read cut 衔接其后已提交变化：

```text
owner snapshot/read cut
  -> committed observation cursor
  -> ordered owner-local changes
  -> resume(cursor)
  -> explicit GAP when retained history is insufficient
```

1. observation 只描述已提交事实或控制事实；事件不是事实本体，消息系统也不是 owner Store。
2. 顺序只在一个 authority stream 内成立。公开跨 owner 观察只返回可披露 owner 的 cursor vector，不合成全局序列，也不列出隐藏 owner。
3. query／observation coordinator 可以在各 owner 的授权边界内维护更完整的内部 observation graph 与 hidden cursors，但这些状态绝不能原样序列化。调用者只获得与 normalized query、caller-evidence identity、可披露 per-owner view vector 和 aggregate opaque closure commitment 绑定的 privacy-preserving opaque resume state；它不能泄露隐藏 owner、stream、节点、数量、拓扑或 change frequency。
4. delivery 可以 at-least-once；每个可披露 change 具有稳定 owner-local identity／cursor，消费者能够去重。不可披露 branch 的事实变化在其 aggregate opaque closure validity 仍可证明时不产生公共 change event。
5. cursor／opaque-state retention 有限。可披露 cursor 过期、丢段、source epoch 无法衔接，或 opaque closure／view validity 无法继续证明时，必须返回 `GAP` 并要求重新取得 snapshot/query；不能从“最近看起来一样”推断没有遗漏。
6. 一个持续空间观察由 `initial COMPLETE/PARTIAL query + disclosable owner streams + opaque observation state` 派生。Enter、Exit、Move、TypeAdded 等只是可重建投影，不写回 Core／Facet 真相。
7. parent `Structure/ListDirect`、`DelegationBinding` 与 matching `MountAcceptance` 控制流共同决定内部 Core source graph；涉及 Facet 时，还必须跟踪 `FacetAttachment` proposal／acceptance 控制流。关系改变时，为新增或替换的可披露 source 建立 snapshot+cursor；不可披露 source 只更新 privacy-safe opaque state，不公开 source-set change。无法从既有 cursor、opaque state 与控制事实证明连续时返回 `GAP`，聚合观察按仍可定义的根 context 降为 partial，根 context 或 exact FacetAttachment 本身无法建立时为 unavailable。
8. 任一 per-owner authorization view 改变都可以撤销后续披露并使 continuation／subscription 失效；新可披露 source 必须从新 snapshot+cursor 开始，不能从 hidden cursor 泄漏历史。授权变化不会倒改已经提交的 Core／Facet 事实。
9. polling、outbox、NATS、数据库 change feed、SDK watcher、gateway state 或不可解读的 signed resume token 均可实现该行为；只要 snapshot-to-stream 无缝、gap 可检测、隐私边界成立且 bus／token 不成为真相。

### D12. 支持稳定 UUID 的 owner transition，但接受明确不可用窗口

本文明确支持同一 Element 或一个 authority 明确拥有的 coverage slice 在事实权威变化后保持各 ElementUUID；同时拒绝 shadow Element、双写、双显和跨 authority 全局原子事务。跨多个独立 authorities 的 subtree 不是一个隐含原子迁移：每个 contributing slice 必须分别保持或转换其 owner evidence，否则整体只可报告不可用／未完成。

Core 的 Structure 变化与 fact-authority 变化是两个正交轴：reanchor 可以不换 owner，owner replacement 也可以不改 Structure；“跨 authority reanchor”只是两轴同时发生。下表列出组合结果，Facet authority transition 在 D12.2 单独说明：

| 变化 | FactAuthorityRef | StructuralAddress | ElementUUID |
|---|---|---|---|
| endpoint／Pod／hosting Cluster 变化 | 不变 | 不变 | 不变 |
| 同一 authority 的 writer／durable owner handoff | 不变，commit epoch 前进 | 不变 | 不变 |
| 同一 authority 内 reanchor／reparent | 不变 | 按新 Structure 改变 | 保持不变 |
| 跨 authority reanchor／reparent | 改变，并带 continuity evidence | 按新 Structure 改变 | 保持不变 |
| Structure 不变的真正 authority／carrier replacement | 改变，并带 continuity evidence | 不变 | 保持不变 |

#### D12.1 同一 authority 的 writer handoff

1. 旧 writer 的 commit fence／epoch 必须在 authority 自己的 durable commit domain 中先被永久推进或关闭；unreachability、更新 NextHop、Operator assignment 或新 lease 都不能证明 fencing。
2. 新 writer 只有在持有更高且已安装的 fence 后才能提交；相关关系的指定联系由各自 owner 随后更新。改地址不能替代 writer 安装 fence，也不要求一个全网地址更新点。
3. 若无法确认旧 fence，新 writer不得启用，调用方得到 `UNAVAILABLE`。这比可能双写更正确。

#### D12.2 跨 authority 的语义连续性

跨 owner reanchor、carrier replacement、Facet authority replacement 或其他已接受的 coverage replacement 必须有唯一 authority-binding 切换点。D12 不新增全局 transition service；切换点必须是一个已经存在且只有一个 owner 的控制关系：

- carrier replacement 修改 parent Core authority 拥有的 `DelegationBinding`；
- Facet authority replacement 修改当前 Element Core authority 证明的 `FacetAttachment` 关系；
- 单个 Element 或一个 Core authority 明确拥有的 coverage slice 跨 owner reanchor，由当前 Core authority 唯一提交不可回退的 release／successor evidence；
- root 的 FactAuthorityRef replacement 只有在当前 root authority 提供 successor evidence 时才能声称连续；更新 SpaceReference 本身不能证明 owner 连续。旧 authority 永久失联而又没有预先存在的 fact-authority control binding 时，旧空间只能 `UNAVAILABLE`，新 reference 是新的空间入口，不能自行继承旧 root 的身份。

跨 owner reanchor 必须同时证明两件正交事实：

1. **identity／owner continuity：** 旧 Core authority 或既有 binding owner 已不可回退地释放该 UUID／coverage，并给出 final read cut 与 successor evidence；
2. **target topology placement：** 目标 parent 的 active direct-content coverage owner 已在明确 parent／Range／Structure cuts 上接受最终 spatial tuple，验证 containment 与无环。若目标 parent 是 mount，这份 placement evidence 还必须匹配 parent 的 exact `DelegationBinding` 与 carrier 的 `MountAcceptance`。

只有两份 evidence 都成立，目标 authority 才能进入 `NEW_ACTIVE`；联系信息、目标方自报或单独一份 continuity proof 都不能替代 placement acceptance。

Facet authority replacement 使用同一正交结构，但 target acceptance 是“新 Facet authority 接受旧 final cut 后的完整 namespace”，不涉及 spatial placement；Core 持有的 `FacetAttachment` 只有在旧 namespace fenced 且新方 acceptance 成立后才切换。

概念层只冻结下面三个可观察状态和单向顺序；operation identity、字节传输、持久化记录与恢复协议留给 transition successor：

```text
OLD_ACTIVE
  -- old owner durable fence + final read cut -->
UNAVAILABLE
  -- unique control owner commits successor/rebinding evidence
     + matching target acceptance -->
NEW_ACTIVE
```

- `OLD_ACTIVE`：旧 owner 唯一可见、可写；
- `UNAVAILABLE`：旧 owner 已 fenced，新 owner 尚不可见、不可写；
- `NEW_ACTIVE`：新 owner 唯一可见、可写；旧 owner 永久拒绝把旧 placement 作为 active Element 提交或披露，但可按私有 retention 保留非权威历史字节与 continuity evidence，二者都不是查询结果或第二真相；
- 任一失败或恢复只能停留在、或向前收敛到这三个状态之一，不能让同一 UUID 同时出现在旧、新 Structure 中。

目标方可以在不可发现、不可写的私有传输区准备字节，但那不是 `shadow Element`、第二真相或查询结果。系统不承诺零中断；它承诺 UUID 连续、最终 tuple 合法、已确认写入不丢、任一时刻最多一个可见 owner 和 writer。

同一 Core authority 内 reanchor 不改变 FacetAttachment。跨 Core authority transition 时，Facet authority target 与 Facet records 默认不迁移，旧 Core-owned attachment 随旧 Core fence 失效；新 Core 可以继续指向同一 Facet authority，但不能静默复用绑定旧 Core proposal／existence cut 的 namespace acceptance。该 Facet authority 必须对新 Core attachment proposal 给出 matching re-acceptance，新 Core 才能提交 `ACTIVE`；若 re-acceptance 尚未建立，Core transition 可以独立完成，但新 Core 必须返回 `DETACHED`／unavailable attachment evidence，直到恢复 acceptance。若 Facet authority 本身 handoff，使用同一单写原则。Observation 在旧 stream 以 successor evidence 结束，并从新 authority 的明确 cut 继续；不能假装 cursor 在两个 owner 间天然连续。

delegation revoke 若没有完成上述 transition，只会使 branch detached／`UNAVAILABLE`，绝不等于内容删除或自动归还 parent。

## 5. 五类公共行为如何由十二项决策闭合

| 公共行为 | 最小输入 | 必须返回／保证 | 主要决策 |
|---|---|---|---|
| `Enter` | SpaceReference + caller trust evidence | 临时 SpaceHandle；分别验证 boundary、authority、authorization 与指定联系及连接 | D6–D9 |
| `Discover` | SpaceHandle + StructuralAddress 或 QueryRegion | `INVALID`／`DENIED`／`UNAVAILABLE` outcome，或带 source cuts、可披露 per-owner authorization-view vector、aggregate opaque closure commitment、completeness 与隐私化 missing evidence 的已验证 `COMPLETE`／`PARTIAL` result | D9–D11 |
| `Read` | 已发现的 Core／Facet reference | exact record／absence／unavailable／denied、owner、Revision、ReadCut | D4–D6、D11 |
| `Observe` | owner ReadCut，或 DiscoveryResult 的 caller-visible cuts + opaque observation/resume state | 可披露 committed changes／cursor、privacy-safe resume、gap 与经授权 source-graph change | D11 |
| `Propose` | identity、authority evidence、final tuple／payload、CAS、OperationID | committed／denied／conflict／unavailable／indeterminate 的可收敛结果 | D4–D8、D12 |

`Resolve`、`ListDirect` 和 `QueryRegion` 是 Discover 的三种形态；create、update、reanchor、delete、Facet Put 等是 Propose 的 owner-specific 形态。公共薄腰不要求所有模块共用一个通用 CRUD interface。

## 6. 层面与阶段是两个独立维度

### 6.1 四个层面

| 层面 | 本文冻结什么 | 本文不冻结什么 |
|---|---|---|
| 真相模型 | Core 的 `Structure + Pose + Range`、可选 Facet exact payload record、Element／Facet 身份不变量 | authority、permission、联系控制、Store schema、缓存、索引、事件投影 |
| owner-local 操作面 | exact read、ListDirect、最终 tuple mutation、Facet enumeration，以及 delegation／acceptance／authorization／reachability 等控制关系的唯一 owner 与可观察结果 | Go 方法、RPC、数据库事务和控制记录私有布局 |
| 业务操作面 | Enter／Discover／Read／Observe／Propose、完整性和失败语义 | 应用玩法、业务工作流、物理实时查询 |
| 生产运行面 | 单写 fencing、联系／连接 freshness、容量显式、可恢复 transition、可重建派生层 | Kubernetes、etcd、NATS、具体部署拓扑 |

authority、delegation、permission、关系指定联系和 fence 具有各自的权威控制状态，但它们是支撑空间真相的正交控制面，不是 UCUPA 世界真相。StructuralAddress、索引、查询结果、事件和连接缓存则是派生层。任何能力都要先问“属于哪个层面”，再问“在哪个阶段实现”；把联邦执行后置不允许删掉它对当前 truth／query 设计的约束，把机制留到生产阶段也不允许提前把机制写成世界真相。

### 6.2 能力证明依赖与执行排序

不可省略的是被声明能力所依赖的事实与证据，不是让所有机制按一条整阶段链等待：

1. Core 空间操作、挂载与查询始终以 D1–D4 的合法事实为基础；挂载还需 D6 的双侧控制、
   独立授权及联系，组合查询再证明各来源 cuts 和 coverage。Facet 可选，不是 Core 挂载的功能前置。
2. D5 Facet 服务必须满足 D6 attachment／acceptance 与 D7 授权，并取得 source Core 的现时证据；
   它不以 Core 已完成挂载或公网部署为前置。
3. D11 owner-local Observe 依赖相应 owner 的 read cut、提交顺序、cursor 与缺口证明；
   聚合观察另需 source graph、各 owner streams、授权 views 和隐私化恢复，不能用单流替代。
4. 跨机器访问先证明所用操作的公共编码、D8 指定联系、目标认证、caller establishment 与独立授权；
   完整开放互操作再证明 D6–D11 的公开行为和第 7 节的独立实现门，不能由同机或单厂商实验代证。
5. D12 转移依赖真实 fence、final cut、唯一控制绑定切换与目标接纳；涉及观察连续性时另需 D11 衔接。
   不能用更地址、不可达或部署规模代替这些证据，也不要求先实现所有无关转移形态。
6. 生产与规模证明必须限定所使用的能力、故障和容量 profile，并具有相应正确性证据。
   受限实验不必等待无关能力；宣称支持开放联邦、交接或高可用时，相关证明则不可缺少。
   索引只能在所服务的公共查询合同闭合且同 cut 保守 oracle 正确后替换执行策略。

完整目标仍须覆盖 D1–D12 与相关公理反例；允许小范围先行不等于将观察、联邦或 transition 降为可选。
阶段编号、默认顺序、当前进度和切片安排只在[开发计划](development-plan.md)维护，
不得从局部完成推定整组语义已经证明。

## 7. 整体反例与后继可证伪门

这组场景分两层使用，不能形成“先实现才能接受、先接受才能实现”的循环：

1. **概念接受审查：** 已逐项完成逻辑反例审查；本文能够仅依靠已声明的 truth、control fact 和公共结果给出唯一且不矛盾的预期，不依赖隐藏目录、第二真相、全局事务或未声明 owner。这个结论不声称当前代码已经证明结果。
2. **后续实现合同：** 把相应场景固化为公开 conformance gate。每个阶段只在声明覆盖的能力上声称完成；第 16 项属于 transition 阶段，第 19 项属于开放互操作阶段，第 20 项属于明确 scale profile，均不阻塞更小的前置实现阶段。

统一预期如下：

1. **坐标封闭：** 多层旋转／平移后，任意 Element 可以投影到任意祖先 frame；root 无 LocalPose，外部 polar 输入不会产生第二真相。
2. **Range 无漏查：** 巨大 Entity 中心位于 query sphere 外但两球相交／相切，线性 oracle 必须命中。
3. **父子覆盖：** 随机深树满足所有直接包含时，任意后代都位于祖先 Range；bounded parent 的越界 tuple 原子拒绝，`UNBOUNDED_ROOT` 的合法有限直接 child 按类型接受且不读取 root radius。
4. **数值边界：** 深链、极值和接触边界上的优化查询与保守 oracle 一致；不确定时多报或 unavailable，不漏报。
5. **结构可读：** 清空所有 children/path/index cache 后，只用 Core records、owner-local ListDirect 及跨 mount 所需的 delegation／acceptance evidence 可重建 Structure 与 StructuralAddress。
6. **明确 scope：** Start + UpDepth 选择 Scope，深层独立 basis 不截断该闭包；祖先直属 Entity 与更高层兄弟后代都能命中。深度改变重查，跨 Core 不改变其含义，返回实际 Scope 和可复读路径；不把执行中断当作新的边界。
7. **来源图并发：** 查询期间新增、撤销或改变 mount／acceptance 时，旧 source-evidence graph 只能按其绑定的 Structure／delegation／acceptance cuts 完成、失效或 partial，不能错误 `COMPLETE`。
8. **发现授权：** 只有 closure-bounded discovery Grant、没有目标 UUID Grant 的 caller 可以发现未知结果；递归发现的新 owner 根据同一 caller evidence 独立建立本地 PrincipalRef／view evidence，root PrincipalRef 不自动复用。被拒绝 branch 不泄漏详情，且任何因必需 Anchor 链不可披露而排除的后代必须在对应 per-owner authorization view 中被判为不可披露，不能先算作允许结果再静默消失。
9. **Facet 可选性、开放性与接纳：** Core-only Element 在无 Facet 时完整成立；启用 Facet 后，未预装业务代码的第二实现仍可按 Element 与不透明 TypeRef 枚举键并读取 exact payload，不要求理解其业务格式。Core 不能在缺少 matching acceptance 时从 `NONE` 激活 target，target 也不能在 Core 提交 `ACTIVE` 前对外服务。
10. **Facet 生命周期：** reanchor 后 FacetKey 与 Facet authority target 不变；跨 Core owner transition 不搬迁 Facet records，但同一 target 必须 re-accept 新 Core attachment，未完成时 namespace 为 `DETACHED`／unavailable。Element 删除 cut 之后建立的新 Facet read 不再返回 payload，删除前已确认响应保留原 cut evidence，Core unavailable 不被解释为 absence。
11. **独立 owner：** 一个 Realm／Cluster 承载多个 FactAuthorityRefs，或一个 authority 更换 Cluster／credential，查询仍能区分 owner continuity 与联系变化。
12. **rootless carrier：** carrier 无本地 root 且不复制 mount Anchor；从 parent boundary 规划的查询仍能进入 carrier 并返回独立 cuts。
13. **联系失效：** NextHop 不可达或目标不匹配只产生 `UNAVAILABLE/PARTIAL`，不改变 Element、Facet、permission、delegation 或 owner；更新某一方向的地址不能隐式更新另一侧关系。
14. **Revoke／mount delete：** delegation revoke 后 contents 不消失、不归 parent；旧本地写不再被连接关系接受，durable fence 另行证明；没有完成 transition 时明确 detached／unavailable。carrier 不可达、acceptance 撤销或 detached 时删除 mount Anchor 必须失败，不能把未知 coverage 当成 empty。
15. **观察恢复与隐私：** snapshot 后断开、重复 delivery、cursor 过期和新增 source 均能去重或报告 GAP；公开 cursor／source-change 只含可披露 owner，隐藏分支仅通过不泄露 owner、数量、拓扑或 change frequency 的 opaque state 维持，不能静默漏事件或反向泄密。
16. **handoff 故障注入：** 后续 transition successor 在每个持久状态边界中断时，均只能得到 `OLD_ACTIVE`、`UNAVAILABLE` 或 `NEW_ACTIVE`，永不双写／双显／丢确认写。
17. **多 root：** 两个独立 root 没有 projection evidence 时，系统拒绝把结果伪装在同一 basis 或声称跨 root `COMPLETE`。
18. **派生层丢失：** 删除索引、连接 cache、query cache、Client projection 和 event transport 后，权威事实及持久联系记录不变且可重新进入、重建和观察。
19. **开放实现：** 至少两个不共享内部 Go 类型、Store 或 gateway 代码的实现通过同一 contract corpus。
20. **病态规模：** 深链、高 fan-out、大量重叠 Range 和不可达 owner 只导致 bounded cost、continuation、partial 或 unavailable，不导致错误空结果。

概念接受冻结这些预期结果，不等于宣称它们已经被运行证明；任何 successor 或实现只有通过其阶段相关的 gate，才能声称相应能力完成。

## 8. 明确排除的非必要真相与机制

- 当前 baseline 的 Metadata／displayName 字段、API、Store 和 Revision 语义；它们只有在真实消费者证明共同最小需求后，才可由独立合同作为可选能力引入，且不得成为身份、结构、空间查询、寻址、路由或授权的前提；
- 文本名称解析、alias、DNS 项目或名称注册中心；
- 裸 UUID 全宇宙 locator、全局 Element／owner／Cluster directory；
- 全局 root、World、WorldRevision、全局时钟、全局事务或跨 owner 原子 snapshot；
- 把 Anchor tree、authority graph、Realm trust／permission、联系／网络拓扑和 Cluster topology 合并成一棵树；
- WorldPose、StructuralAddress、SubtreeEnvelope、索引、query result、event 或 cache 作为第二真相；
- 在 Core 内保存 polar／geodetic、scale、Collider、资产 bounds、LOD、渲染或高频物理状态；
- 用执行截断、转发 hop、timeout 或 page limit 偷改 semantic universe；UpDepth 是明确选择 Scope 的语义参数，不在此禁止之列；
- 把 exact UUID Grant 当成未知发现授权；
- 要求 Facet 依赖中央 schema service、类型制品或类型合同才能保存内容；
- NextHop、连接成功或底层路由控制 ownership、permission 或 writer activation；
- shadow Element、dual write、delayed delete、以 unreachability 证明 fencing；
- 为已删除原型保留 `/1`、`/2`、`/3`、旧 Store reader、转换器或兼容分支；
- 把 Go package、模块数量、数据库、消息总线、Kubernetes 或当前 Composition fixture 写成空间公理。

## 9. 投影与实现合同边界

- UCUPA 不提供 Console 产品模块，不把内置可视化面板作为基础设施职责或后续建设项。浏览与管理界面由外部客户端按 owner 的公开操作、明确权限和结果语义实现，不直开 Store、不引入第二份真相。可编程管理、权限校验、事实观察及必要运行诊断仍属于各自原有职责；不因移除面板而删除，也不转化为 Operator 或新模块的 UI 任务。
- Platform 中的 product／minimum-truth 视图与开发计划中的 capability／stage 视图都是本文的非规范投影，不得独立新增或修改 owner、truth、outcome、完整性或第 6.2 节的语义证明前置；发生冲突时本文优先。执行阶段的编号、分组与默认排序由开发计划维护，不再被一条整阶段链冻结。旧同名文档只是冻结 predecessor，不参与投影同步。
- [架构权威清单](README.md)只导航当前规范与已实现合同，不复制它们的语义；[开发计划](development-plan.md)只维护进度与 gate。
- bounded implementation contract 必须声明它实现哪些 D1–D12 条款、没有实现哪些条款及相应黑盒门；完成一个切片不能把未覆盖语义宣称为已实现。
- Greenfield 实现直接定义唯一当前形态，不增加 predecessor reader、旧数据迁移、转换器、版本路由或兼容分支；私有 Store 格式只在相应实现合同中决定。

## 10. 已接受内容与后继边界

本文的整体接受已经冻结：

- 十二项决策的概念语义和跨项不变量；
- `Structure + Pose + Range` 的目标 Core Element 真相；
- 可选 Facet 的不透明 32-byte TypeRef、exact Payload 与类型枚举；
- FactAuthorityRef 以及 Core mount／逐 Element FacetAttachment 的唯一 owner 入口；
- RealmAttachment 沿用已有 RealmID 与指定 NextHop 的授权联系方向，不改变 D7 权限语义；
- closure-bounded discovery authorization；
- SpaceReference／SpaceHandle／StructuralAddress 的职责分离；
- complete／partial／unavailable／denied、invalid request、per-owner cuts、resume／gap；
- 本次另行确认：向上深度选择 Scope（含 -1）、完整搜索后代、可选 SnapshotRetention 与 CurrentVerified／Retained 双模式，以及不延迟可靠写入；P3 目标合同已定稿，具体实现授权与交付状态仅由开发计划记录；
- 允许不可用但禁止双写的 owner transition。

本文的整体接受仍然没有冻结：

- Go API、protobuf／HTTP、错误枚举的具体拼写；
- RealmAttachment 的具体记录粒度、持久归位、接口、联系维护和恢复合同；
- Store schema、FormatID 字面值或数据库；
- Octree／BVH／缓存／query planner；
- credential、签名、TLS、lease clock 和具体安全产品；
- Operator、Archive、Partition、StoreGroup 或 Cluster 部署 profile；
- polar／geodetic projection API、产品 Facet 合同或上层应用行为。

P3 的零／缺省与根处行为、结构路径、可选保留、容量／释放及当前连接门细则已由用户确认的
[本地查询合同](core-local-query.md)与[挂载合同第 5 节](core-mount-contract.md#5-读取路径与组合切面)定稿，
不属于上述待设计项；当前实现、验证及验收以开发计划和 current 权威清单为准，本文不复制执行状态。

本文已经整体接受；若 D1–D12 任一项未来被新反例推翻，必须修改依赖项并重新通过全部反例门，不能只把矛盾推给实现。当前动作与代码授权不在本文维护。
