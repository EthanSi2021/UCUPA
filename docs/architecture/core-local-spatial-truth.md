# UCUPA Core 本地空间真相实现合同

**状态：G1 与显式 Range 更新修订均已验收；权威分类见[架构权威清单](README.md)**

本文把已接受主基线的 D1–D4 收敛为一个可直接实现和反证的 Core 切片。它只定义一个
owner-local、有根、无 federation 的当前形态；不建立版本、兼容输入或历史 Store 读取路径。
若本文与基本公理或主基线冲突，必须修正本文，不能用实现合同反向改写上位语义。

原 G1 实现已经通过全部黑盒 gate 和独立检阅，并完成用户验收。[架构权威清单](README.md)
将本文列为 current Core 实现合同。显式 Range 更新修订已单独通过验证并由用户接受，不建立平行合同；
当前 gate 与下一步只由 [Development plan](development-plan.md) 维护。
本文继续拥有基础空间值、写入与普通本地读规则；已验收的[本地查询合同](core-local-query.md)
接替 `Config` 启动读取配置、`Close` 的读取资源生命周期及新增查询行为，
[挂载合同](core-mount-contract.md)接替其明确的外部父、挂载控制与组合读取范围。
原 G1 的切片限制不再被解释成完整 Core 的现行能力上限；各条款只由对应 owner 合同定义。

已验收的 [P4 本地 Core／Facet 合同](facet-local-truth-and-core-lifecycle.md)只接替或扩充 Core self authority profile、
exact-read envelope、create／delete 的 attachment 原子副作用、attachment-only commit、owner-qualified scope、
附着与联系控制、typed evidence port，以及对应 Store／commit clause families。
本文继续拥有 D1–D4 `ElementRecord`、空间 create／update／reanchor／reparent／delete、ElementRevision、
ListDirect、空间 CAS／错误／收敛与 geometry oracle，不整体退出 current，也不保留一套不含附着副作用的旧写入口。

## 1. 本合同冻结什么

本切片唯一允许声明：

```text
D1  右手 Cartesian SE(3) 坐标代数
D2  Element Range 与 root 语义
D3  一种规范数值表示和安全几何判定
D4  owner-local Core 记录、读写、收敛和持久化
+ owner-local ListDirect
+ 非公共线性几何 conformance oracle
```

Core 在此范围内唯一拥有：

- ElementUUID 的 Core existence、封闭 kind、当前 parent、LocalPose、Range 与 Element Revision；
- root、直接包含、无环、非级联删除及 UUID 永不复用不变量；
- owner-local Core read cut、OperationID 绑定和 mutation retained outcome；
- 上述真相的内存与 durable Store 表示。

Realm 仍只决定 caller identity 与权限，Facet 仍独立拥有非空间事实。Core 可以向 Authorizer 提交精确
权限要求，但不持久化 Principal、Grant 或 policy。name、displayName、文本 path、业务对象、Facet payload、
Route、Cluster、deployment 和索引都不是 Core 真相。

## 2. 唯一记录形态

### 2.1 标识与种类

- `ElementID` 继续是 canonical lowercase RFC 9562 UUIDv4。
- `ANCHOR` 与 `ENTITY` 是封闭 kind；不存在自定义 kind 或 unknown-kind fallback。
- 同一个 UUID 只能声明一次 kind；成功删除后 tombstone 永久占用该 UUID。
- rename、Pose update、Range update、reparent、进程变化和未来被证明的 authority transition 都不改变 UUID。

### 2.2 空间值

```text
Vec3     { X Scalar, Y Scalar, Z Scalar }
Rotation { Pivot W|X|Y|Z, Ratios [3]Scalar }
Pose     { Translation Vec3, Rotation Rotation }

Range = BOUNDED_SPHERE { Radius Scalar }
      | UNBOUNDED_ROOT

SpatialTuple = ParentAnchorID + LocalPose + BOUNDED_SPHERE
```

`Rotation` 是 canonical projective quaternion，而不是“近似单位四元数”。`Pivot` 指定隐含为 `+1` 的
分量；其余三个分量按 W、X、Y、Z 去掉 Pivot 后的顺序保存。该表示把所有非零标量倍数视为同一个
旋转，天然消除 `q`／`-q` 双重真相。第 4 节冻结其唯一规范化规则。

### 2.3 Element record

```text
AnchorRecord {
  ID
  ParentAnchorID?   # 仅 root 缺省
  LocalPose?        # 当且仅当存在 parent
  Range             # root 为 UNBOUNDED_ROOT；其余为 BOUNDED_SPHERE
  Revision
}

EntityRecord {
  ID
  ContainingAnchorID
  LocalPose
  Range             # 只能是 BOUNDED_SPHERE
  Revision
}

ElementRecord = AnchorRecord | EntityRecord
```

缺省是真正的字段缺失，不用零 UUID、identity Pose、负数、NaN 或 Infinity 充当 sentinel。root 没有
parent 和 LocalPose；root 在自身 basis 中的 Pose 只能派生为 identity。

一个新 Core 可以为空。第一个成功 `CreateRoot` 建立唯一 root；此后该 Core 始终恰有一个 root，普通
行为不能更新、reparent 或删除它。本合同不允许一个 Core 持有多个独立 root，也不允许 rootless
carrier；这两种形态只能在 federation 合同中重新证明，不能预埋进本地记录。

### 2.4 owner 绑定、Revision 与 read cut

```text
Revision      # 绑定一个 Core owner 与一个 Element；opaque equality token
CoreReadCut   # 绑定一个 Core owner 的完整本地事实切面；opaque equality token
```

G1 的 owner 是调用已绑定的具体 Core 实例或 durable Store，不新增可传递的公开 owner ID。Revision 与
CoreReadCut 内部带有随实例创建、随同一 Store reopen 保持稳定的不可伪造私有 scope；来自另一实例或
Store 的 token 必须被拒绝。二者都不是 endpoint、Cluster、root UUID、时间戳或 `FactAuthorityRef`。
D4 在本地切片要求的 owner attribution 由这个调用边界满足；可跨进程披露和路由的最终 owner identity
只由 D6 的 `FactAuthorityRef` 定义，G1 不制造过渡身份。

空 Core 创建时就产生一个非零、合法且可返回的初始 cut；Go zero value 永远无效。每个真正改变 Core
fact 的成功 commit 原子产生一个新的 CoreReadCut 与一个新的 subject Revision；其他 Element 的 Revision
不变。两种 token 是不同语义，调用方只能分别做 equality，不得读取计数、比较大小或推断相互关系。
canonical final tuple 与当前 record 完全相同时返回 retained `UNCHANGED`，不推进 Revision 或 read cut。
retained negative outcome、重放和只读行为也不推进 read cut。read cut 不是 Realm-global revision、不是跨
owner 总序，也不要求任何子空间向“总 Realm”请求。

## 3. 空间与结构不变量

### 3.1 坐标代数

- `LocalPose` 是 Element local frame 到直接父 Anchor frame 的右手 Cartesian 刚体变换，线性单位为米。
- Pose 不包含 scale、shear、projection 参数或坐标系标签。
- polar、spherical、geodetic、天体分块和渲染坐标只能在 Core 边界外投影。
- `WorldPose` 不再是独立 API 或持久字段；到指定祖先 frame 的 Pose 由当前 parent chain 派生。
- 两个独立 root basis 没有隐含变换。本合同只有一个有根空间，因而不声称跨 root 投影。

### 3.2 Range 语义

- Entity Range 是领域中立 occurrence coverage，不是 Collider、LOD、可见距离、权限区或业务影响范围。
- 非 root Anchor Range 是该 Anchor 完整结构闭包的稳定保守 zone；允许多报，禁止漏报。
- root 独占 `UNBOUNDED_ROOT`。所有有父 Element 都必须使用有限、非负的 `BOUNDED_SPHERE`。
- 兄弟 Anchor 可以重叠；Core 不把空间组织误写成无重叠分片。

当直接 parent 是有界 Anchor 时，最终 tuple 必须精确满足：

```text
distance(parent.origin, child.origin) + child.radius <= parent.radius
```

实现必须使用第 4.4 节的无平方根精确谓词。边界相切合法；rotation 不改变球半径。直接 parent 是 root
时只做类型包含，不读取不存在的 root radius。

普通非 root Anchor 与 Entity update 都允许 Range 半径保持、扩大或缩小，只要最终 tuple 被直接 parent
包含。Anchor 的最终 Range 还必须包含同一提交状态中的全部直接子 Anchor 与 Entity；孩子保持原有
local tuple，不能通过自动移动或缩小孩子使请求合法。任一包含失败即原子拒绝。
Range 是外部显式声明的保守覆盖，不要求最小化。子 Element 增改删不自动调整任何祖先 Range、record
或 Revision；更新 Anchor 也不改写后代 record 或 Revision。root retirement 和 mount retirement 不属于普通行为。

### 3.3 最终 tuple 原子性

所有 create 和 update 都对最终 `parent + LocalPose + Range` 一次性验证以下不变量，并只产生一次 commit。
本列表是不变量集合，不表示可观察错误的裁决顺序；唯一裁决顺序由第 7 节冻结：

1. parent 存在且是 Anchor；
2. UUID、数值表示与 Range kind 合法；
3. Anchor update 没有形成环；
4. 最终 subject 被直接 parent 包含；Anchor 的最终 Range 同时包含全部直接子 Element；
5. expected Revision 与当前 subject record 匹配。

任一步失败都不留下中间 parent、Pose、Range、Revision、read cut 或 operation success。Anchor reparent 只
改写该 Anchor 自身最终 tuple；后代 record 和 Revision 不改写，所有投影重新派生。

Entity 可直接非级联删除。Anchor 仅在非 root 且同一 commit domain 中证明直接 child 集合为空时可删除，
也不级联。本合同中不存在 delegation state；挂载后继合同若允许删除曾承载远端内容的锚，必须先增加独立 retirement 证明，不能继续
套用本地 empty-delete 规则。

## 4. 唯一数值 profile

### 4.1 Scalar

`Scalar` 的公开权威编码是 IEEE 754 binary64 的 64 位值：

- 接受完整有限域，包括 subnormal 和最大有限值；拒绝 NaN 与正负 Infinity；
- 所有零输入保存并返回 `+0`，因此 `-0` 不成为第二真相；
- position 与 radius 的绝对上限就是 binary64 最大有限值 `0x1.fffffffffffffp+1023`；
- radius 还必须非负；`+0` 是合法点球；
- Store 和测试按精确 bit pattern 判断 canonical equality，不以 decimal 文本或 epsilon 判等。

Core 不设置一个与 UCUPA 尺度目标无关的经验米制 ceiling。任何派生结果若不能正确舍入为有限 binary64，
公开投影返回 `ErrUnrepresentable`；不得饱和、截断或静默降精度。

### 4.2 Rotation 规范化

Core record 只接受 canonical `Rotation`。边界构造器可以接收任意四个有限且不全为零的 binary64
quaternion 分量，并按以下唯一算法产生它：

1. 把四个 bit pattern 解释为精确 dyadic rational；
2. 取绝对值最大的分量为 Pivot，相等时固定按 W、X、Y、Z 选择最先者；
3. 整体变号，使 Pivot 为正；
4. Pivot 分量不保存并定义为精确 `+1`，其余分量除以 Pivot 后正确舍入为 binary64
   round-to-nearest, ties-to-even；
5. 若 Pivot 之前的某个精确比值满足 `abs(r) < 1`、但舍入后 `abs(round(r)) == 1`，保留正负号并向零取
   相邻 binary64，以维持“Pivot 是第一个最大分量”的唯一性；所有正负零 bit 再规范为 `+0`。

Pivot enum 只允许 W、X、Y、Z。canonical record 必须满足：Ratios 有限且绝对值不大于 `1`；Pivot 之前
的 ratio 绝对值严格小于 `1`；所有零 bit 都是 `+0`。
因此表示可机械验证且幂等；精确成比例的输入、`q` 与 `-q` 必然得到相同 bit pattern。Core 不同时保存
unit-quaternion、Euler angle 或 matrix 作为第二种旋转真相。

内部 Hamilton product 产生 exact rational 分量时，仍使用同一 Pivot、符号和舍入规则形成公开 Rotation。
SDK 可以提供 unit-quaternion、Euler angle 或 matrix 投影，但它们都不是 Core record。

### 4.3 精确 SE(3) 约定

令 quaternion 分量顺序固定为 `q=(qw,qx,qy,qz)`，Hamilton product `a ⊗ b` 固定为：

```text
w = aw*bw - ax*bx - ay*by - az*bz
x = aw*bx + ax*bw + ay*bz - az*by
y = aw*by - ax*bz + ay*bw + az*bx
z = aw*bz + ax*by - ay*bx + az*bw
```

`conj(q)=(qw,-qx,-qy,-qz)`，`norm2(q)=qw²+qx²+qy²+qz²`。Core 使用 active、右手旋转；
local vector 到 parent frame 的唯一含义是：

```text
R_q(v) = vector_part(q ⊗ (0,v) ⊗ conj(q)) / norm2(q)
Pose(t,q)(v) = R_q(v) + t
```

因此不要求 q 为单位长度，且结果仍是严格刚体旋转。对于 inner local→parent Pose `I=(ti,qi)` 和
outer parent→ancestor Pose `O=(to,qo)`：

```text
O compose I = (R_qo(ti) + to, qo ⊗ qi)
inverse(t,q) = (-R_conj(q)(t), conj(q))
```

所有式子先把 canonical binary64 bits 解释为 exact rationals，按括号和 W、X、Y、Z 顺序求值；不得让
FMA、编译器重排或中间 binary64 舍入决定权威结果。Hamilton 输出只在第 4.2 节指定的 Rotation 边界舍入，
translation 只在公开 Pose 输出边界 round-to-nearest, ties-to-even。

以下 exact vectors 是最低 golden corpus：

```text
Rotation(Pivot=W, Ratios[X,Y,Z]=[0,0,1]) maps +X to +Y
that rotation compose itself maps +X to -X
outer=(translation [0,2,0], +90deg Z), inner=(translation [1,0,0], identity)
  => composed translation [0,3,0] and rotation +90deg Z
inverse(+90deg Z) maps +X to -Y
```

任何实现若得到反向旋转、`qi ⊗ qo` 或 `[1,2,0]`，即违反合同。

### 4.4 权威包含谓词

所有存储的 finite binary64 在谓词中解释为精确 dyadic rational。对于有界 parent 与 child，令 child
translation 为 `(x,y,z)`、半径为 `rc`、parent 半径为 `rp`，只在以下两式都精确成立时接受：

```text
rc <= rp
x*x + y*y + z*z <= (rp - rc) * (rp - rc)
```

规范语义是 arbitrary-precision integer/rational comparison，不绑定某个语言库，也不执行 binary64 平方、
平方根或经验 epsilon。这样数学上
相切的 tuple 被接受，紧邻边界外的 tuple 被拒绝，极值计算也不溢出。无法完成证明时拒绝 mutation，不能
猜测为 inside。

### 4.5 派生 Pose、相交与 exact-work 界

parent-chain composition 从 subject 沿当前 parent 顺序应用第 4.3 节的 `outer compose inner`；球心从
Element frame 投影到 query basis 时，先求 `T(boundary<-element)` 与 `T(boundary<-basis)`，再计算
`inverse(T(boundary<-basis)) compose T(boundary<-element)`。全部在精确 rational 上计算，只在公开 Pose 输出
处正确舍入为 canonical binary64／Rotation。链断裂、环或非 canonical record 是 `ErrStore`；有限真值
无法表示为输出 Scalar 是 `ErrUnrepresentable`。

球形相交同样比较精确平方距离与 `(query.radius + element.radius)^2`。root 的 unbounded Range 按类型
相交。实现不得用“中心是否落入查询球”替代球体相交，也不得让数值不确定性产生 complete miss。

为使 exact 语义物理可执行，本合同同时固定数值工作界：

```text
MaxExactProjectionEdges = 1024
MaxExactRationalBits     = 65536
```

这两个名字只表示本合同的规范工作界，不属于第 5 节冻结的 public Go API。

edge 数包括一次 Projection/oracle 中任一 subject 到所需共同边界的最大 parent 边数。每次 primitive
rational operation 约分后，分子或分母绝对值的 bit length 都不得超过 `MaxExactRationalBits`。直接父子
containment 对全部 finite binary64 输入天然低于此界，因此写入不能用工作界逃避裁决。

Projection 或 oracle 一旦需要超过任一界，整体返回 `ErrUnrepresentable`，不返回截断 Pose、部分 match
或 complete 标志。Core 仍可保存更深的合法 Structure，并继续 exact read／ListDirect；工作界只限制该次
派生数值操作，不改变查询 universe，也不把查询 depth 写进 Core truth。future implementation 可以扩大
资源上限，但不能改变界内 bit-exact 结果或把超界伪装成成功。

## 5. owner-local API

以下 public Go 表面由本合同冻结。规范数值与 token 的字段保持 private；DTO 字段按下文导出，但 Core
仍验证所有 input，并且只返回满足 discriminant invariant 的值。operation、参数顺序、结果 discriminant
与错误映射不能另造平行入口：

```go
type ElementKind uint8       // closed: Anchor, Entity
type RotationPivot uint8     // closed: W, X, Y, Z
type RangeKind uint8         // closed: BoundedSphere, UnboundedRoot
type MutationStatus uint8    // closed: Committed, Unchanged, Rejected
type ExistenceState uint8    // closed: Present, NotFound
type OperationID string

const (
    ElementAnchor ElementKind = iota + 1
    ElementEntity
)
const (
    PivotW RotationPivot = iota + 1
    PivotX
    PivotY
    PivotZ
)
const (
    RangeBoundedSphere RangeKind = iota + 1
    RangeUnboundedRoot
)
const (
    MutationCommitted MutationStatus = iota + 1
    MutationUnchanged
    MutationRejected
)
const (
    ExistencePresent ExistenceState = iota + 1
    ExistenceNotFound
)

type Rotation struct { /* private canonical Pivot + ratio bits */ }
type BoundedSphere struct { /* private canonical radius bits */ }
type Range struct { /* private closed variant */ }
type Revision struct { /* private owner/Element-scoped token */ }
type CoreReadCut struct { /* private owner-scoped token */ }
type ListContinuation struct { /* private Core/parent/cut/position binding */ }

type Authorizer interface {
    Authorize(values.PrincipalRef, ...values.Requirement) bool
}

type Vec3 struct { X, Y, Z float64 }
type Pose struct { Translation Vec3; Rotation Rotation }
type SpatialTuple struct {
    Parent values.ElementID
    LocalPose Pose
    Range BoundedSphere
}

type AnchorRecord struct {
    ID values.ElementID
    ParentAnchorID *values.ElementID
    LocalPose *Pose
    Range Range
    Revision Revision
}
type EntityRecord struct {
    ID values.ElementID
    ContainingAnchorID values.ElementID
    LocalPose Pose
    Range BoundedSphere
    Revision Revision
}
type ElementRecord struct {
    Kind ElementKind
    Anchor *AnchorRecord
    Entity *EntityRecord
}

type MutationResult struct {
    Status MutationStatus
    Record *ElementRecord
    TombstoneRevision Revision
    OutcomeCut CoreReadCut
}
type ElementRead struct {
    State ExistenceState
    Record *ElementRecord
    Cut CoreReadCut
}
type DirectPage struct {
    Records []ElementRecord
    Cut CoreReadCut
    Continuation *ListContinuation
}
type Projection struct {
    Pose Pose
    SubjectRevision Revision
    Cut CoreReadCut
}
func New(authorizer Authorizer, config Config) (*Core, error)
func CreateDurable(authorizer Authorizer, path string, config Config) (*Core, error)
func OpenDurable(authorizer Authorizer, path string, config Config) (*Core, error)
func (core *Core) Close() error

func NewRotation(w, x, y, z float64) (Rotation, error)
func NewPose(translation Vec3, rotation Rotation) (Pose, error)
func NewBoundedSphere(radius float64) (BoundedSphere, error)
func (rotation Rotation) Pivot() RotationPivot
func (rotation Rotation) Ratios() [3]float64
func (sphere BoundedSphere) Radius() float64
func (value Range) Kind() RangeKind
func (value Range) BoundedSphere() (BoundedSphere, bool)
func (left Revision) Equal(right Revision) bool
func (left CoreReadCut) Equal(right CoreReadCut) bool

func (core *Core) CreateRoot(
    actor values.PrincipalRef,
    id values.ElementID,
    operationID OperationID,
) (MutationResult, error)

func (core *Core) CreateAnchor(
    actor values.PrincipalRef,
    id values.ElementID,
    final SpatialTuple,
    operationID OperationID,
) (MutationResult, error)

func (core *Core) CreateEntity(
    actor values.PrincipalRef,
    id values.ElementID,
    final SpatialTuple,
    operationID OperationID,
) (MutationResult, error)

func (core *Core) UpdateAnchor(
    actor values.PrincipalRef,
    id values.ElementID,
    final SpatialTuple,
    expected Revision,
    operationID OperationID,
) (MutationResult, error)

func (core *Core) UpdateEntity(
    actor values.PrincipalRef,
    id values.ElementID,
    final SpatialTuple,
    expected Revision,
    operationID OperationID,
) (MutationResult, error)

func (core *Core) DeleteAnchor(
    actor values.PrincipalRef,
    id values.ElementID,
    expected Revision,
    operationID OperationID,
) (MutationResult, error)

func (core *Core) DeleteEntity(
    actor values.PrincipalRef,
    id values.ElementID,
    expected Revision,
    operationID OperationID,
) (MutationResult, error)

func (core *Core) ReadElement(
    actor values.PrincipalRef,
    id values.ElementID,
    at *CoreReadCut,
) (ElementRead, error)

func (core *Core) ListDirect(
    actor values.PrincipalRef,
    parent values.ElementID,
    limit uint16,
    continuation *ListContinuation,
) (DirectPage, error)

func (core *Core) ProjectToAncestor(
    actor values.PrincipalRef,
    subject values.ElementID,
    ancestor values.ElementID,
    at *CoreReadCut,
) (Projection, error)
```

对含 `at` 参数的方法，`at == nil` 表示选择 current cut；非 nil 的 Go zero value 是 `ErrInvalid`。
`Revision`、`CoreReadCut`、
`Rotation`、`BoundedSphere`、`Range` 与 `ListContinuation` 的内部字段不可构造或解析；它们的 Go zero value
无效，只能由构造器/Core 产生并通过上述 accessor/`Equal` 使用，不提供 ordering、时间或 wire
serialization 承诺。`ElementKind`、`RotationPivot`、`RangeKind`、`MutationStatus` 与 `ExistenceState` 各自
导出上述 closed constants；unknown numeric value 是 `ErrInvalid` 或 `ErrStore`，不能 fallback。

每个 returned record 与 slice 都是不会 alias Core mutable state 的 value snapshot。`ElementRecord` 恰有一个
与 Kind 匹配的非 nil variant；root Anchor 的两个 optional pointer 都为 nil，非 root 两者都非 nil。
`MutationResult` 与 `ElementRead` 的 optional/zero fields 必须与 discriminant 满足下述
规则，调用方不得通过“哪个字段碰巧非零”猜状态。

public DTO 是以下封闭 discriminated values：

```text
ElementRecord = ANCHOR(AnchorRecord) | ENTITY(EntityRecord)

MutationResult {
  Status = COMMITTED | UNCHANGED | REJECTED
  Record?               # create/update 的 COMMITTED，或 update 的 UNCHANGED
  TombstoneRevision?    # delete 的 COMMITTED
  OutcomeCut             # retained outcome 的 CommitCut 或 DecisionCut
}

ElementRead = PRESENT { ElementRecord, CoreReadCut }
            | NOT_FOUND { CoreReadCut }

DirectPage = { ElementRecord[], CoreReadCut, continuation? }

Projection = { Pose, SubjectRevision, CoreReadCut }
```

每个 DTO 必须先读取 closed discriminant；不允许用 nil record、零 Revision 或零 cut 猜状态。
`MutationResult` 的 COMMITTED 返回提交后的 cut；只有 Update 可以返回 UNCHANGED，并返回未改变的 record/Revision 与作出决定时的
cut。retained failure 返回 `REJECTED`、原始 DecisionCut 和对应 sentinel error。非 retained error 返回
zero `MutationResult`。exact replay 永远返回原始 result/error，不能附上伪装成当前事实的新 cut。

COMMITTED create/update 只有 Record 有效；COMMITTED delete 只有 TombstoneRevision 有效；UNCHANGED 只有
Update 的 Record 有效；REJECTED 两者都无效但 OutcomeCut 有效。PRESENT ElementRead 恰有一个 Record，
NOT_FOUND 没有。Create 对已占用 UUID 只能返回 retained `ErrConflict`，不能把重复创建解释为 UNCHANGED。
任何其他组合都是 Core bug 或 `ErrStore`，不能向 caller 返回。

`ReadElement` 是唯一普通 exact record read；不保留平行的 typed `Anchor`／`Entity` readers。authorized
absence 返回带当前 cut 的 `NOT_FOUND`，使“不存在”也是可归属的读取事实。Store 无法建立一致读取时返回
zero `ElementRead` 与 `ErrStore`，其公共含义是 `UNAVAILABLE`，不伪造 cut；`ErrDenied` 和
`ErrReadCutExpired` 同样不返回 existence outcome。

Core 不再导出无 actor 的 `ConfirmElement` 或任何 authorization-free existence oracle。G1 维持现行 Facet
owner boundary 的方式是：Composition 显式绑定一个已经建立的 service `PrincipalRef`，由非权威 adapter
调用唯一的 `ReadElement(actor, id, nil)`，再投影到 Facet 当前消费的 `(exists, available)`。service actor
必须拥有 exact `ucupa.core.element.read`，不得由 Core/Composition 猜测或默认授予；DENIED/ErrStore 映射为
`available=false`，不能伪造 absence。该 bool 只留在尚未实现类型化生命周期增量的 Facet-owned consumer port，不是 Core
public API、兼容 facade 或新证据合同；Facet 本地闭环由独立合同定义 typed existence/cut lifecycle。

### 5.1 ListDirect

`ListDirect` 是 Structure 的必要完整读面，不能由调用方影子目录替代：

- parent 必须是当前 Anchor；页面同时包含直接 child Anchor 与 Entity 的完整 record；
- 以 ElementUUID 的 16 个 canonical bytes 升序排列；`1 <= limit <= 256`；
- continuation 缺省时在一个一致 transaction／锁切面上选择 current cut 并返回第一页；
- continuation 非 nil 时，它是后续页唯一 cut/position 来源；API 不再接收第二个 `at`；
- G1 只保留 current snapshot：token cut 已不是 current 时，不返回任何 fact 并返回 `ErrReadCutExpired`；
- continuation 是 opaque value，至少绑定 Core 私有 scope、parent、首次 cut 与最后一个 UUID；
- 后续页绝不偷偷切换到 current；无法继续服务首次 cut 时返回 `ErrReadCutExpired` 且不返回任何 item，
  调用方必须丢弃此前全部页面并从第一页重启；
- malformed token 在授权前返回 `ErrInvalid`；语法合法 token 的 Core-scope／parent binding 只在 exact-parent
  授权通过后检查，不匹配再返回 `ErrInvalid`；最后一页没有 continuation，因而在该 cut 上证明直接集合完整；
- 不做逐 child 静默权限过滤。调用方要么拥有整个 parent 的 `ListDirect` 权限，要么得到 `ErrDenied`。

该 owner-local 全事实 cut 是最小正确基线，不是中心化 Realm revision。未来可以用 snapshot、MVCC 或更细
cut 改善并发并继续服务旧 cut，只要不改 API 和结果语义。G1 不保证持续写入期间多页读取一定完成；它
保证的是要么得到同一 cut 的完整集合，要么明确失败并整体重启。

### 5.2 ProjectToAncestor

投影读取 subject、每个中间 Anchor 与目标 ancestor 的同一 cut record，返回 subject frame 到 ancestor
frame 的确定性舍入 Pose、subject Revision 与读取证据。subject 与 ancestor 是同一个 Anchor 时返回
identity Pose；同一个 Entity 同时作为 ancestor 仍因 ancestor kind 非法而返回 `ErrConflict`。
结果不持久化，也不披露中间 UUID。调用者对内部 path 的每个 Element 都必须拥有
exact read 权限；任一项未授权只返回 `ErrDenied`。ancestor 不存在返回 `ErrNotFound`，存在但不是 Anchor
或不是当前祖先返回 `ErrConflict`。该 API 不寻找其他 root、不跨 authority，也不把文本路径变成地址。

公开 Projection 的 binary64 translation 是表现值，不是可用于安全 query pruning 的 derived envelope。
把它与原 Range 半径直接拼成投影球，可能因球心舍入而失去保守覆盖；因此本 DTO 不返回 projected Range。
需要同 cut 原 Range 时可用 `ReadElement(at=cut)`，但空间否定判断仍必须使用 exact oracle 或未来 D10
明确携带误差包络的结果，不能自行用舍入 Pose 制造 complete miss。

caller-facing `ReadElement` 与 `ProjectToAncestor` 对 `at` 使用同一规则：nil 选择 current；Go zero token 是
授权前可判定的 `ErrInvalid`；语法合法但来自另一 Core scope 的 token 在当前 exact scope 授权后返回
`ErrInvalid`；属于本 Core 但已不能服务的 cut 返回 `ErrReadCutExpired`。`ListDirect` 第一页产生 cut，后续页
只从 continuation 取同一 cut。这样可以把后续 exact read/projection 固定到页面 cut，不能拼成虚假快照。

## 6. 权限与披露顺序

本合同固定以下 PermissionID：

| 行为 | PermissionID | exact Element scope |
|---|---|---|
| 创建 root | `ucupa.core.anchor.create-root` | candidate root |
| 创建 Anchor | `ucupa.core.anchor.create` | final parent |
| 创建 Entity | `ucupa.core.entity.create` | final parent |
| exact read | `ucupa.core.element.read` | subject／projection path 每一项 |
| `ListDirect` | `ucupa.core.anchor.list-direct` | parent |
| 更新 tuple | `ucupa.core.element.update` | subject |
| 放入 zone | `ucupa.core.element.place` | final parent |
| 删除 Anchor | `ucupa.core.anchor.delete` | subject |
| 删除 Entity | `ucupa.core.entity.delete` | subject |

update 即使 parent 不变，也同时要求 subject update 与 final-parent place；两者一次 all-of 决定。Core 不要求
旧 source parent 的独立权限。create 的 kind-specific parent permission 已包含首次 placement。

所有调用遵守：纯语法与 canonical value 验证 → 对输入中已知 exact scope 授权 → OperationID replay 查找
或状态读取 → 必要的 path 追加授权 → CAS／结构／几何验证 → 原子 commit。不得先用存在性、kind、parent、
continuation staleness 或冲突类型向未授权 caller 泄露状态。权限撤销后，历史成功 OperationID 的 replay
仍先得到当前授权；`DENIED` 从不绑定 OperationID。

外部 Authorizer 不在 Core state lock／Store transaction 内调用。完成 input-known scope 的首轮授权后，
`ProjectToAncestor` 在一个 provisional cut 上内部解析 path，释放 Core 锁后提交完整 path 的 all-of
requirements，再回到完全相同的 cut 求值；期间发生任何 fact mutation 就完整返回
`ErrReadCutExpired`，不使用新旧混合 path。G1 只证明每个必需的当前授权判断按上述顺序先于相应状态披露、
最终求值或 commit，不声称 Realm cut 与 Core cut 跨 owner 原子。

G1 只提交 exact requirements。后代 closure scope、授权视图、隐藏分支和远端 trust 属于公共发现及开放联邦后继合同，不编码
在 Core record 或 PermissionID 中。

`Authorizer` 只有一个 bool 决策：`false` 对 Core 一律是 fail-closed `ErrDenied`，无论 Realm 明确拒绝还是
policy backend 当时不能作出许可；G1 不从该 bool 猜测 unavailable 原因，也不把 Realm failure 变成 Core
existence evidence。

## 7. 错误、CAS 与 OperationID 收敛

唯一错误集合是：

```text
ErrInvalid
ErrDenied
ErrNotFound
ErrConflict
ErrRevisionConflict
ErrOperationConflict
ErrReadCutExpired
ErrUnrepresentable
ErrStore
```

- `ErrInvalid`：ID、OperationID、limit、continuation 或数值表示在读取状态前已非法。
- `ErrDenied`：当前 exact requirements 未全部获批；不披露 fact state。
- `ErrNotFound`：authorized mutation／List／projection 的目标不存在。`ReadElement` 用 `NOT_FOUND` outcome。
- `ErrConflict`：kind 不匹配、UUID 已占用、第二 root、root mutation、环、非空删除、final parent containment
  或 Anchor direct-child coverage 失败，或 ancestor 关系不成立。Range 缩小本身不是冲突。
- `ErrRevisionConflict`：expected Revision 不属于该 owner/Element 或不等于当前 record Revision。
- `ErrOperationConflict`：同一 OperationID 已绑定不同 actor、operation kind、ID、canonical final tuple 或 CAS。
- `ErrReadCutExpired`：合法 owner cut 已无法由本实现继续服务；不返回该 cut 的部分事实。
- `ErrUnrepresentable`：合法有限真值无法形成本 profile 的公开派生输出。
- `ErrStore`：Store、事务、锁、codec、随机 owner 建立或持久历史验证失败；结果可能未确认。

每个 mutation 的 OperationID 是 `1..128` bytes 的非空 opaque value。绑定包含 actor、操作 kind、subject、
全部 canonical input 和 expected Revision。相同绑定 replay 返回原始成功 receipt 或原始 retained negative
outcome；不同绑定返回 `ErrOperationConflict`。

必须原子持久保留：`COMMITTED`、`UNCHANGED`、`ErrNotFound`、`ErrConflict` 与
`ErrRevisionConflict`；每个 outcome 都保存其原始 `CommitCut` 或 `DecisionCut`。`ErrInvalid`、`ErrDenied`、
`ErrOperationConflict`、`ErrReadCutExpired`、`ErrUnrepresentable` 和 `ErrStore` 都不是 retained outcome。
明确发生在 commit 前的 `ErrStore` 不建立绑定；但 response loss 或 commit 边界故障可能以 `ErrStore` 返回，
而原子 transaction 实际已经建立 binding/outcome。caller 无法区分时只能用完全相同的 OperationID 与
binding 重试：若没有绑定则重新执行，若已经绑定则 replay 原始结果，二者都不能双重提交。

authorization 和 retained-operation lookup 之后，mutation 状态裁决顺序固定为：

```text
update: subject existence/kind -> expected Revision -> root immutability
        -> unchanged tuple -> final parent existence/kind -> cycle
        -> exact parent containment -> Anchor exact direct-child coverage -> commit

delete: subject existence/kind -> expected Revision -> root/empty rule -> commit

child create: final parent existence/kind -> candidate UUID unclaimed
              -> exact containment -> commit

root create: candidate UUID unclaimed -> no root exists -> commit
```

CAS 先于 destination 与 geometry 检查，防止 stale caller 借失败类型探测 destination。所有 quaternion、零值
和 tuple 在进入 OperationID binding 前已经 canonicalize；数学等价重试不得变成 `ErrOperationConflict`。

内存与 durable Core 必须串行化彼此重叠的 read／mutation 临界区。CAS 验证、直接 child empty/coverage 证明、
parent containment、cycle 检查、fact cut 推进、record/tombstone 和 operation outcome 是同一个逻辑 commit；并发调用不能产生两个
root、越界 child、孤儿、环、漏删或混合 cut。

## 8. 非公共线性几何 conformance oracle

实现必须提供 package-private／test-only oracle，逻辑输入为：

```text
ScanIntersecting(
  boundaryAnchorID,
  queryBasisAnchorID,
  at CoreReadCut?,
  QuerySphere { Center, Radius },
  selector = ANCHOR | ENTITY | BOTH,
)
  -> complete ordered Range-intersection matches at one CoreReadCut
```

它不是 `Discover`，不得导出为公共产品 API。其唯一作用是给未来索引和 discovery 提供不可放宽的正确性
参照：

1. `boundaryAnchorID` 定义包含自身在内的完整 Structure closure；`queryBasisAnchorID` 只定义查询球坐标
   基底，两者不是同一个参数。二者都必须是当前 Anchor，且 query basis 必须等于 boundary 或为其后代。
2. query basis 可以是 boundary 的任意深层后代。oracle 仍遍历 boundary 的完整闭包，所以查询可以命中
   query basis 的叔伯 Anchor 及其后代；不得把“共同祖先”误写为只遍历某一兄弟分支。
3. selector 只过滤最终输出，不过滤 Anchor traversal。空 Anchor 仍有自己的 Range，是否返回取决于
   selector 与球体相交。
4. 在单个一致内存快照／Store read transaction 中，以 `O(current facts)` 校验当前发布的 spatial snapshot，
   并线性扫描 closure；不读取 BVH、Octree、child cache、persisted WorldPose 或调用方影子目录。oracle 不在
   每次 scan 中重放 retained OperationID history；完整 history validation 属于第 9 节的 Store 边界。
5. 对每个 Element 把 Range 球心投影到 query basis，再做第 4.5 节的精确球体相交；中心在查询范围外但
   两球相交或相切的 Entity 必须命中。
6. 输出按 ElementUUID 排序并携带同一个 cut。它返回的是权威 Range 球的精确相交集合，不把数值
   不确定 candidate 冒充 match。若无法证明完整结果，整个 oracle 返回失败，不能标记 complete 后漏项。

oracle 可以借助同一快照内的临时 parent map 和 memoized transform 保持结构访问线性；这些临时结构不是
Core truth。查询深度、预算、continuation、authorization view、SpaceHandle、远端 source 和 `PARTIAL`
属于 D9–D11 的公共 Discover 合同，不在这里提前设计。

非公共失败矩阵固定为：

| 条件 | 整体结果 |
|---|---|
| 非 canonical ID/cut、非有限 center、负/非有限 radius、unknown selector | `ErrInvalid` |
| boundary 或 basis 已确认不存在 | `ErrNotFound` |
| boundary/basis 不是 Anchor，或 basis 不在 boundary closure | `ErrConflict` |
| 指定同一 Core 的 cut 已不能服务 | `ErrReadCutExpired` |
| oracle 当前持有的 cut/scope、parent graph 或 current record 损坏 | `ErrStore` |
| 超过 exact-work 界或派生值不可表示 | `ErrUnrepresentable` |

任何失败都返回零 matches 和零 cut；oracle 没有授权行为，也不得被 Composition 当作绕过 Realm 的运行时入口。
retained operation／Store history 不是每次 oracle scan 的输入；其损坏必须在 Store create/open/persist/reload
边界完成全量验证并以 `ErrStore` 阻止发布，不能为了 scan 内重放历史而破坏上述线性工作界。

## 9. durable Store 与 Greenfield 切换

内存与 bbolt durable 形态必须实现同一可观察语义。durable Store 的唯一识别 marker 固定为：

```text
ucupa.core.local@5747950a9ca74cbcae658dde84f87b80
```

marker 只是精确 Store kind 标识，不是公开协议版本。创建 Store 时原子建立 marker、随机私有 token
scope 与非零初始 read cut；空 Store 可以合法 reopen。
识别范围包括记录与 retained-operation 裁决语义，而不只是字段布局；本次显式 Range 更新使用本文唯一 marker。

Store 至少持久化并交叉验证：

- 私有 token scope、当前 opaque read cut、Element current record 或 tombstone 及其 opaque Revision；
- canonical numeric bit pattern、kind、parent、Pose、Range 与 opaque Element Revision；
- OperationID 完整 binding、retained outcome，以及成功 receipt 的 record/tombstone Revision 与 commit cut。

每个 `COMMITTED` mutation 在一个 bbolt transaction 中写入 record/tombstone、永久 UUID claim、Revision、
fact cut、operation binding 和 outcome。`UNCHANGED` 与 retained negative outcome 只写 operation 事实及其
DecisionCut，不推进 Element Revision 或 fact cut。

Store 初始创建、每次 persist、reopen 与失败后的 reload 都必须在发布前 fail closed 验证完整 current state
与 operation history；reopen 还必须验证 marker、scope/cut、canonical codec、UUID、root cardinality、封闭
kind、parent 完整性、无环、Range kind、全部直接包含、每次历史 mutation 的最终包覆与 Revision 来源。任何
missing／empty／任意不匹配 marker、缺字段、unknown semantic field、损坏记录、不可能历史、Store lock 或事务失败
统一返回 `ErrStore`；不得部分打开。

实现不读取、转换或迁移任何旧 Core Store，不保留旧 FormatID fallback，也不返回“旧 profile”专用错误。
测试 fixture 直接创建本文形态的新 Store；Git history 足以保存过去。

## 10. 同一实现切片必须删除的旧表面

本合同的完成要求并已经证明，以下 predecessor 表面被替换而不是并存：

- root `LocalPose` 参数与持久字段；
- typed `Anchor`／`Entity` readers、独立 `WorldPose`、`ReanchorAnchor` 与 `ReanchorEntity`；
- `PermissionReanchorSubject`／`PermissionReanchorDestination`；
- `ErrUnsupportedProfile` 与任何旧 Store reader／marker 分支；
- 原 in-memory `DelegationBinding`、`MountContext`、`MountAcceptance`、`AdmitMount`、`AcceptMount`、
  `MatchesDelegation`、`MatchesMountAcceptance` 及 mount PermissionID；
- 依赖上述 mount DTO 的 `composition/routes*`、原 `acceptance/` federation fixture、demo 及当时的
  acceptance README 声明。

旧 federation proof 没有 durable OperationID/CAS、Range cut、完整 parent-chain evidence 或多 owner 合同。
把它适配到新 record 会在原 G1 范围偷渡当时 G4 的能力，并制造第二种 Core profile；因此直接删除，不提供兼容 facade。
开放联邦后继设计从 D6–D11 和独立互操作合同重新推导 federation。

`docs/concept-v0.md`、历史合同与冻结证据正文不因这次代码减法而重写；本次 authority 切换只更新权威
清单和必要的最小 predecessor status notice，历史论证继续由 Git 与原文保存。

Facet 与 Realm 的 current contracts 不因本切片归零；它们只通过现有明确 owner boundary 继续组合。G1
只对暴露出的 Facet compile/test dependency 做了维持现有 Core-existence port 所需的最小调整，没有提前
实现 G2 语义。

## 11. 黑盒完成 gate

只有以下测试均以公开行为、第 8 节的 package-private linear oracle，或仅用于证明第 4 节算法与工作界的
package-private exact-numeric conformance helper 通过，才能声称 G1 完成。数值 helper 不得代替 API
状态、授权、收敛或 Store gate。

### 11.1 记录与结构

- UUID canonicality、跨 kind 冲突、删除后 reopen 仍不可复用；
- 空 Store、唯一 root、root 无 parent/LocalPose 且只能是 `UNBOUNDED_ROOT`；
- Anchor／Entity 只能使用合法最终 tuple；第二 root、root update/delete、孤儿和环全部原子拒绝；
- 非 root Anchor 与 Entity update 均可保持、扩大或缩小 Range；空 Anchor 可缩至零；
- Anchor 合法缩小覆盖直接子 Anchor、Entity 及其后代；相切接受，少一个可表示值导致越界时拒绝；
- Parent、Pose 与 Range 联合更新只按最终 tuple 校验；不能被新父包含或不能覆盖任一直接孩子时原子拒绝；
- 成功或拒绝都不自动改动其他 Element 的 record/Revision；child 内移或删除不自动收紧祖先 Range；
- 父 Anchor 缩小与孩子外移并发时不能同时提交出破坏包覆的状态；
- canonical final tuple 完全相同且 Revision current 时返回 UNCHANGED，record/Revision/cut 均不推进；
- reparent 后 UUID 不变、只更新 subject Revision，所有后代 record/Revision byte-for-byte 不变；
- Entity delete 与 empty non-root Anchor delete 成功，非空 Anchor delete 无副作用。

### 11.2 数值与几何

- NaN/Infinity/负 radius 拒绝，`-0` 读回 `+0`，subnormal 与最大有限 bit pattern 可 round-trip；
- `q`、`-q`、精确正负标量倍数得到同一 Rotation；Pivot tie 与临界舍入产生唯一幂等表示；
- 第 4.3 节的 +90°Z、composition、translation 与 inverse golden vectors 得到 bit-exact 方向，不接受反向约定；
- boundary tangent 被接受，向外一个可表示值被拒绝；极大/极小 dyadic predicate 不溢出；
- 深 parent chain 的投影、inverse 和 non-ancestor 失败行为确定；超出公开 Scalar 输出返回
  `ErrUnrepresentable`，Store 不损坏；
- exact-work edge/bit 界的界内、恰好到界与首次越界结果确定；Projection/oracle 超界返回零结果，
  不截断 Structure；
- oracle 命中“中心在查询球外但 Range 相交”的巨大 Entity、相切球、空 Anchor；
- query basis 位于深层兄弟分支时，仍命中 boundary 下另一兄弟及其后代；selector 不截断 traversal；
- oracle 与同一 cut 上的穷举预期完全一致，任何 future index corpus 都不得比它少报。

涉及非平凡舍入、相邻边界、subnormal 或极值的数值向量，以 hexadecimal binary64 bits、标准库精确命名
常量，或 `Float64frombits`／`Nextafter` 的精确构造提供，并使用明确 UUID；结构 fixture 可以使用可精确
表示的简单整数与 deterministic literal UUID helper。任何向量都不能用测试 epsilon 把错误判为相等。
golden corpus、独立 exact reference 与 fuzz/differential test 共同提供可证伪证据；测试通过不被夸大为
对全部 binary64 输入和任意深度的穷尽数学证明，规范算法本身才定义全域语义。

### 11.3 读取、并发与收敛

- `ReadElement` 的 PRESENT／NOT_FOUND／UNAVAILABLE 与 cut 证据不混淆；
- 空 Store 的初始 cut 非零且 reopen 相等；另一实例/Store 的 Revision、cut 和 continuation 均被拒绝；
- Core 源码不存在 authorization-free `ConfirmElement`；Composition adapter 以显式 service actor 调用
  `ReadElement`，而现行 Facet bool consumer port 不被误称为 G2 typed evidence；
- `ListDirect` 混合列出两种 kind，顺序、limit、continuation、最终完整性和空列表确定；
- 任意并发 fact mutation 使旧 continuation 得到 `ErrReadCutExpired`；从第一页重启得到一个新完整 cut，
  永不拼接不同切面；
- `ReadElement` 与 `ProjectToAncestor` 指定同一 cut 时要么共同成功，要么 cut expired；持续写入下允许分页
  饥饿但不允许虚假完成；
- 清空所有可重建临时结构后，`ListDirect`、parent chain 与 projection 仍只从 owner truth 得到相同结果；
- 七类 mutation 均覆盖自身 codec、适用的 COMMITTED 结果与 reopen exact replay；共同收敛类别
  （UNCHANGED、different-binding conflict、stale/foreign Revision、retained negative、原始 outcome cut、
  lost response retry）各至少有一个语义适用的代表；kind-specific 决策顺序与 invariant 由对应 operation
  覆盖，不要求无意义的七类乘全类别矩阵；
- stale subject Revision 与 missing destination 同时成立时稳定先返回 `ErrRevisionConflict`；语法合法 cursor
  binding mismatch 只在 parent authorization 后返回；
- 授权先于状态披露与 replay；update all-of permission 缺一即无副作用；
- controllable fake Authorizer 阻塞一个调用时，另一个获批调用仍能进入 Core 临界区并完成，以证明
  `Authorize` 不在 Core state lock/Store transaction 内执行；false 只验证 fail-closed `ErrDenied`；
- 并发 create/update/reparent/delete 在 race-enabled test 下保持唯一 root、CAS、无环、empty proof 与 cut 原子性。

### 11.4 Store 与减法证明

- 每类 COMMITTED／UNCHANGED 与 retained negative 在进程退出/reopen 后 outcome/cut 不变；每个 commit
  边界故障注入不产生半状态；
- 合法 Anchor 缩小在 reopen 后保留；因孩子越界而拒绝的缩小在孩子后来内移后仍重放原始拒绝，
  新 OperationID 才重新裁决；缩小的 precommit rollback 与已提交但丢失响应均可精确重试；
- missing／empty／任意不匹配 marker、locked Store、unknown field、non-canonical numeric、越界图、错误 Revision/cut 与伪造
  operation history 全部 fail closed 为 `ErrStore`；
- source/static test 对机械封闭集合提供证明：公开 Core method allowlist、唯一 current marker、`storedElement`／
  `storedPose` schema allowlist，以及 predecessor identifier/file/permission-prefix denylist；
- code review 与 invalid/predecessor Store fail-closed 行为共同证明没有替代 open/route、旧 reader/index、public
  Discover 或 predecessor compatibility path；有限关键词扫描不得被称为对所有兼容分支的穷尽证明；
- `go test -race ./...` 与 `go vet ./...` 通过，且 acceptance README 只声明 owner-local Core truth。

## 12. 明确不覆盖

本合同不实现或暗示：

- D5 Facet successor、FacetTypeRef/namespace attachment 完整生命周期；
- 公共 `Enter / Discover / Read / Observe / Propose`、SpaceHandle、StructuralAddress、查询 depth/budget、
  Discover/query continuation 或 `COMPLETE/PARTIAL`；
- FactAuthorityRef、delegation/acceptance、Route/referral、rootless carrier、nested mount、多 owner cuts；
- Realm closure authorization、authorization view、hidden branch 隐私或远端 trust；
- D12 writer handoff、owner replacement、跨 authority reanchor 或不可用窗口；
- mount retirement、root retirement；
- BVH、Octree、child index、空间 cache、事件流、网络协议、进程拓扑、恢复编排或生产容量；
- polar/geodetic 权威记录、displayName、文本路径、Collider、LOD、资产、上层应用的业务对象。

这些不是被否定的 UCUPA 能力，而是后续 gate 必须基于本合同真相继续证明的独立行为。G1 完成只说明
Core 的本地空间真相可运行、可重启、可收敛、可穷举验证；不能把它宣传成开放 federation 或完整产品。

## 13. 接受与生效

原 G1 合同及其实现已经接受。[架构权威清单](README.md)将本文列为基础空间行为的 current Core 实现合同，
[Development plan](development-plan.md)保留原 G1 本地空间能力的完成记录；显式 Range 更新修订也已单独验证并由用户接受。
后续执行状态仍以该计划为准。
`core-minimum-model-correction.md`、`core-anchor-structural-mutations.md`、
`phase-11-element-id-decision.md` 与 `element-id-profile-name-correction.md` 退出 current 表并仅保留历史。

任何测试若迫使 Core 增加第二数值表示、root Pose、影子目录、旧 Store fallback 或未证明 federation，
应判定实现违反合同，而不是把例外写成兼容逻辑。
