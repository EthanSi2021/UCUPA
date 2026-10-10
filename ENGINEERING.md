# UCUPA 工程组织规范

**状态：公开源码快照中的唯一现行工程组织规范。Core、Facet、Realm 及通信、SDK、测试继续遵守已接受的职责与依赖约束。本规范不重新开启目录重构；当前范围与下一道 gate 只见开发计划，不授权新代码、提交或推送。**

本文是工程组织规则的唯一入口，不定义空间、权限、联系或内容语义。
业务依据由[架构权威清单](docs/architecture/README.md)指向的公理与合同决定；
当前实现范围、验证边界与下一道 gate 只在[开发计划](docs/architecture/development-plan.md)记录。
这次替换旧目录方案，不再把“公开类型、实例、提交与紧密相关实现全部留在模块根包”当作必须遵守的布局。
源码迁移按开发计划中的已授权范围推进；已验收的业务与可靠性约束继续有效。

下文保留的按增量说明用于解释已接受的包依赖与职责裁决，不是公开开发流水或待执行计划。
历史计数与源码位置不能替代当前检查器；它们不授权扩大包边界、增加额度或重复执行迁移。

## 1. 目标与工程责任

目标是让维护者能找到一项完整职责、判断它依赖什么，并在有限范围内修改和验证。
目录少、文件短、抽象多、测试通过，都不能单独证明结构合理。

- 先确定事实所有权、变化边界和依赖，再决定包与文件；不要按每个方法或操作步骤建目录。
- 一个逻辑 owner 只有一套正式状态、提交和恢复机制，但不等于只有一个 Go 包。
- 同一模型只有一个正式定义。同一条规则供正常执行与重放共同使用，不复制“新旧内核”。
- 公开面由实际消费者需要决定，不由“文件放在哪里方便”决定；私有实现不为迁移方便而公开。
- 新增文件前先确定归位；完成一个切片时同时删除失去用途的旧实现和过渡适配。
- 不让巨型根包原样搬到 `service/`、`runtime/`、`kernel/` 或 `internal/` 后被宣布为完成。
- Greenfield：采用确认后的唯一接口和实现，不加旧导入路径兼容层、双入口或旧协议回退。
  这不是静默改语义的许可；组织调整、公开接口调整、业务修订分开记录和验证。

工程方案、命名、拆分粒度、验证、审查和正常实施步骤由实现方负责，不要求用户选择目录名或替代技术审计。
用户确认工作范围及影响功能、调用方式、安全、可用性或成本的实质取舍。

P7.2 父侧 carrier 公开控制增量（2026-09-19）：`internal/transfer/carrier.go` 在原 Admission
捕获、复核 parent + mount 的 commit／inspect 权限，仍只持已装配的 commit.Owner。
`commit/public_carrier.go` 127 行只在原锁下比较当前／原输入、投影原结果与取消状态；
复用已有 mount 控制槽、意图和取消事务，不增加表、日志、私有编码、网络或第二份状态。
由于这些比较需要同一锁内的当前挂载、原操作与持久 signer，不能拆出另一状态 owner 来降低文件数。
`token.CarrierRevision` 使用固定域和原持久签发材料，绑定原 reservation 与当前 parent cut；
共享 token schema／Protobuf 同步，不暴露私有序号或新增认证机制。
持久提交前后故障、重开、原结果、slot 互斥和状态投影测试留真实 commit；Realm 撤权重授、
请求取消与原 caller 的公开链测试归 `tests/transfer`，不导入业务私有包。
新增依赖按四个具体文件登记，精确上限为 commit 62 份生产／8,427 行、113 份测试／17,052 行，
检查器 28 份生产／4,371 行；不为后续网络链或 codec 预留增长额度。

同日父侧网络增量复用 `transport/grpc/internal/transition` 与现有用户会话寿命，
`carrier.go` 仅转换公开请求、检查比较值表示与回应绑定，不导入业务 internal 或运行恢复流程。
SDK 的 carrier 校验只验证目标与原请求一致，不保存状态、授予权限或自动重试。
公开 TLS 反例留 `tests/transfer`，独立父侧产品重启测试留 acceptance；后者在启动产品前
通过真实公开 owner 建立挂载，随后关闭本地 owner，不用内部方法替代产品转移调用。
这四份新消费文件按依赖精确登记；检查器生产上限更新为 28／4,375，acceptance 为
71 份测试／10,253 行。Core commit、Facet 与模块根上限不变，不批准完整三方网络恢复已完成。
设计确认、实现完成、验证通过、用户验收、提交、推送分别记录，不互相替代。
讨论规范不自动授权代码、其他阶段、提交或推送。

同日三方 carrier 本地恢复增量：`internal/transfer` 按父侧恢复、源侧停止写入、目标接纳安装、
批准及证据读取分文件，仍只持原 Admission 与 commit.Owner，不创建流程表、锁或日志。
`commit/carrier_peer.go` 的 70 行仅在原锁内核对原计划、已存在 fence、已安装结果及尚未获得 hold 的外部边；
没有新增事务或私有编码。公开不透明封套复用 `contract/transition` → `transfer/exchange`，
不向调用方暴露可制造的完成证明。可信本地连接固定三方实例、指定地址、原调用者及批准视图，
反向调用由私有的来源绑定确定身份，不允许请求临时指定身份，也不自动刷新批准。
三方流程的公开黑盒测试归 `tests/transfer`，使用真实独立 Store 和独立目标 Realm；
丢失响应、三方重启、撤权重授和并发重试不通过私有 state 接口制造结果。
12 份新增消费文件逐项登记依赖；commit 的精确生产上限为 63／8,497，检查器为 28／4,387。
模块根、Facet、Store 格式与其他包上限不变；这不表示三方 gRPC 或外部边恢复已完成。

同日 carrier 父侧证据网络增量：请求形状校验复用 `transport/grpc/internal/wire/carrier.go`，
不在公开控制和服务回查中各写一套。`transitionwire` 转换原计划、决定与有限 cut／revision，
仍禁止标准库 I/O、状态、锁、Store、signer 与网络依赖；不复用持久化 JSON 作为协议。
`transition/parent.go` 只负责真实 peer 的双向 TLS 校验、原 owner 证据读取及披露前重验。
`transitionconn/parent.go` 复用原 Connections 的指定地址、并发和期限，不新增注册表或路由。
SDK 只核对请求与回应，不导入业务 internal、不代替 Realm 批准；真实组合测试仍归 `tests/transfer`。
12 份新增消费文件逐项登记依赖；检查器精确上限为 28／4,399，commit、Facet 及模块根上限不变。
该增量没有新增目录、公开业务操作或持久事务，不意味着三方网络恢复全部完成。

### S2P3c 网络 Realm 消费增量（2026-10-02）

Facet 的授权等待仍归原 `internal/namespace` 工作流：新增请求内 permit 文件只保留原 caller、
精确要求、联系代次及原 Realm view，不创建另一张许可表、owner、锁或日志。
原 namespace replacement 源侧与导入侧各成一个同包文件，共享现有状态与提交；
不为每一步转移建立包。access 只把原上下文释放回调排到锁外，不能拥有 grants 或业务决定。
transport 传递已认证 CallerContext，instance 维护入口消费同一身份，不通过主体名称远程授权。
产品显式 local／network profile；固定 TLS 接线复用 composition，不新增授权或连接内核。

新增生产依赖全部落在原已审查包边界；namespace 精确生产登记为 30 份／5,507 行，
较前一冻结集合净增 215 行。acceptance 新增一份共享网络 Realm 产品 fixture，
复用既有关联、恢复处置与转移场景而不复制流程，精确登记为 95 份／18,069 行，净增 270 行。
这是本片已实现职责的登记，不预留后续组合矩阵或规模增长；单文件和依赖限制不放宽。
检查器反例按现行上限构造超限一个文件，仍检查未注册生成物及 inactive 源不能隐藏规模；
这项自测净增一行，检查器测试精确登记为 58 份／11,272 行，不减少拒绝断言。
完整验证、验收与提交状态只在开发计划记录。

### S2P3d 有限组合验收增量（2026-10-02）

acceptance 新增四份同包测试，复用已有 Core／Facet 关系和跨 Core 查询产品流程：
组合入口、双方权限反例、跨 Core 网络 Realm 装配、字节级响应丢失各归一个文件。
显式共享使用一份原 Realm Store／产品及两个独立服务证书；独立组合保留各自 Store 与 grants。
不复制关系或联邦实现，不新建场景框架、事实状态表或管理 RPC。
该实际职责增量精确登记为 99 份／18,627 行，较 S2P3c 净增 558 行；不预留未来增长，
单文件规模门与依赖规则保持。P6 foreign Actor 与接收 owner 的接线修复仍在原 composition／Realm
服务边界，协议字段由固定工具生成，不改变持久格式或建立第二授权模型。
技术交付与完整验证结论只在开发计划记录。

## 2. 基线证据：这次解决什么

下表来自已恢复的 P6 完成提交 `a8e84ed8`，不是旧 P7 工作区的计数：

| 模块 | 含子目录全部文件 | 根包 Go 生产文件 | 根包 Go 测试文件 |
|---|---:|---:|---:|
| Core | 234 | 96 | 68 |
| Facet | 63 | 20 | 16 |
| Realm | 32 | 15 | 16 |

“生产”按非 `_test.go` 分类，生成物在全模块统计时另行识别。文件数量只是审查入口，不等于业务规模。
当前具体问题：

1. Core 的实例、空间值、元素操作、挂载、Facet 联系、快照、公共接入和 Store 共用根包。
   `convergence.go` 还同时定义公开 token、事实表、回执、提交与故障恢复。
2. Facet 的公开值、内容、接纳、回查 Core 和提交都在根包；简单分文件不会建立依赖边界。
3. 当前许多方法都绑定同一个公开接收者；Go 不能把这些方法直接分到不同包。
4. 如果仍固定公开类型和所有提交在根包，内部包反向依赖它就形成环；
   为绕环复制整套类型或建立逐方法回调，会让结构更复杂。
5. transport 同样需要按请求类别、会话和连接生命周期审查，不能只整理业务根包。

P6 是可执行回归基线，不是理想目录的证据；旧 P7 中已经提取的目录也不自动成为这次目标。

## 3. 模块外壳与公开范围

### 3.1 统一命名，按实际职责裁剪

下图是目标位置约定，不是必须全部创建的模板。Core、Facet、Realm 遵守同一规则，但内部业务包不要求同构。

```text
<module>/
  README.md                 模块职责、使用入口和实现状态
  instance/                 本地实例创建、打开、装配与生命周期入口
  contract/                 实际共享的公开 Go 值、请求、结果及必要接口
  internal/                 实际实现，按本模块职责分包
  api/                      跨进程协议源、schema、conformance
  bindings/<language>/...   唯一生成协议绑定
  transport/<protocol>/     服务端通信入口及其私有实现
  sdk/<language>/           消费方客户端及其私有实现
  tests/                    确有需要的本模块公开组合测试
```

目标模块根目录不承载业务源码或回归测试，也不保留旧根包的 alias／转发入口。
README、必要资源索引和工具要求的元文件可以留根。根目录清爽只是结果，不能用搬空根目录代替职责验收。
仓库根仍只有一份 `go.mod`／`go.sum`，不新增子 module、`go.work`、`replace` 或独立发布单元。

`instance` 是构造与装配入口，不是新的业务大包。它连接依赖，提供调用方实际需要的本地入口；
返回具体类型还是接口由真实用法与封装需要决定，不统一规定为“能力接口”。
请求处理代码属于相应职责，不能在这里重新写一百个代理方法。
生命周期所调用的内部释放和恢复流程归相应实现，不能把全部流程放进构造入口。

`contract` 不是万能 model 包：只放已有、需要对外消费的 Go 定义。
不放 Store 文档、可写事实表、锁、内部命令、重放事件或整份 owner 状态。
同一公开定义由服务实现和需要它的本地消费者直接使用，不在内部再造一份逐字段镜像；
协议生成类型仍由绑定生成，只在真正的传输边界转换。
公开定义按元素、查询、关系等主题分文件；只有不同消费与依赖边界确实成立时才进一步拆包。
消费侧需要的窄接口优先声明在消费侧；只有确需共同维护的公开接口才放进 `contract`，
不把各实现的全部方法列表集中到这里，也不复制一份内部状态模型作为“合同”。

`api` 是跨语言协议，`contract` 是 Go 本地调用合同，`bindings` 是生成表示，
`transport` 接收远端调用，`sdk` 帮调用方发请求，`instance` 构造本地实例。
这些不是必须依次经过的六层，也不是“SDK 对外、其他全对内”。
SDK 可以复用确有需要的公开纯值，但不得依赖服务实例或私有事实实现。
外部调用仍可直接按协议实现客户端；不强制通过 SDK。

### 3.2 调用方式的显式变化

当前集中在 `*Core`／`*Facets`／`*Realm` 上的方法可以按实际职责重新归位，
但不默认把所有业务方法接口化。若具体类型已能清楚表达调用，且不暴露私有状态或引入反向依赖，
就不为拆包额外包接口或逐方法 facade；职责实现也不必机械地一包对应一个公开对象。

建立接口必须指出真实消费方、实际使用的方法，以及需要隔离的依赖或可替换实现。
不为预想的未来扩展、方便 mock、目录对称或绕过未理清的依赖而预造接口。
已经具有真实用途的授权、联系等窄端口继续保留；“按需”不是取消已有隔离或权限检查。
需要接口时由实际实现直接满足，不另造实现全部同名方法的转发层。
不要求小而内聚的 Realm 按 Core 的数量拆分；接口多少不是整理完成的指标。

本规范允许为职责归位进行必要的本地 Go API 调整，包括导入路径、公开类型定义位置以及取得调用入口的方式。
具体签名与调用映射仍须在实施边界中明确并验证；规范确认不等于尚未写出的签名或迁移已经验收。
已有方法数量和可观察业务不因此增加。接入程序、Composition、transport 和测试必须一次性迁到确认后的接口；
不保留旧调用形态适配层。仅移动 import 与改变接收者／构造签名必须分栏记录。
网络方法、请求字节、权限与 Store 格式不因 Go 包调整而变化。

特别是 Revision、ReadCut、Continuation 等 opaque 值：必须先证明唯一表示、构造、签发、解码和归属验证
在新依赖下仍成立，才可移动。不得公开私有字段、任意铸造能力或相信调用方实现的比较方法来消除依赖环。
不把 token 的结构有效性当成授权或来源认证。若保持这些边界需要调整具体接口，应明确列出；
不能借规范文字提前认定已解决。

已通过受限工作区验证的一项归位方式是：需要隐藏表示的值在本模块 `internal/token` 定义，
`contract` 以 alias 提供同一个类型；这不是旧根包兼容层，也不增加逐字段镜像。
构造、拆取和结构校验是内部包函数，不能作为该值的公开方法被 alias 一并暴露。
既有公开比较／证据投影方法按原边界保留；来源、关系和权限仍由实际操作核对。
这只批准已记录的具体值，不自动批准迁出全部 Revision／Continuation 或新增对外构造器。

已验证的挂载值包括 MountKey／NextHopRevision：前者保持原 Valid，后者只保留原 Equal；
内部重建和字段拆取不成为公开方法。公开请求／结果迁入 contract，原根定义不保留 alias。
运行期 CoreReadSnapshot／ConnectedCut 也使用同一归位：Snapshot 不新增公开方法，
ConnectedCut 只保留原 Equal。token 只存原绑定值，查询 owner 保留实际签发、注册、
caller／run 核对、TTL、容量、释放与快照状态；结构校验不是保留状态存在性的证明。

查询条件和使用这些条件的三个 opaque continuation 统一归 `internal/query/request`，
QueryReadMode 一并保持唯一表示，由 contract alias 暴露原公开值；请求叶子仅依赖
shared/values、既有 geometry／token 与标准库，不反向依赖 contract 或实际读取流程。
该包只负责值形状、规范化、复制、比较与原绑定校验；不持有事实、快照表、权限或期限。
扫描内部的部分位置不能被当作已签发凭据；原查询 owner 仍在原时点补齐正式绑定。
未登记的 query 子包仍须按实际职责单独裁决，不能据此放行整棵查询实现。

可信本地跨 Core 的封套归 `internal/query/exchange` 的唯一表示，主题合同 `contract/connected`
仅 alias 外层 Request/Reply 并保留原两个窄端口；外层不导出字段或方法，内部封装/拆取为包函数。
exchange 复用根 contract 与有限 facts View，不导入实际 query owner、commit 或 Store；根 contract
不能反向导入该主题合同。内部 payload 字段不是外部构造接口，不能把 alias 扩为可读写的公开 DTO。
原单次调用 Cleanup 仍只做借用结束/pin 丢弃，不新增通用执行或事务回调，也不持有另一个快照注册表。
实际流程归 `internal/query/connected`，共用已装配的 query.Operations 与 commit.Owner；Core 只提供
取得真实 Connected 操作的入口，在原关闭顺序下调用内部生命周期函数，不逐方法代理。
这些 trusted-local 封套不是网络 wire，也不把本地调用者引用提升为远端身份认证。

原 Connected 回归按被测边界归位：`tests/connected` 只通过真实 Core／contract 操作，
以启动配置启用保留、在测试注入的 Authorizer 中切换权限，不能穿透业务 internal 或修改私有状态。
私有 world 合并测试只组装有限 facts；私有过滤测试必须读取实际 query.Lease，因此仅其
`filter_test.go` 可用同一个 commit.Owner、Element 写入口与 query Capture 建立原始读集，
Finish／Close 正常释放。不得为测试增加 Lease 构造后门、复制过滤算法或放宽整个包依赖。
原并发屏障、资源清理与披露断言仍须随所属实现保留，不能以公开成功场景替代。
跨 Connected 的 query 资源检查使用 `query_test` 与实际组件装配；同包 `export_test.go`
只返回锁内读取的四项数值计数，不暴露表、指针或 setter，且正常生产构建不能使用该测试符号。
这些精确登记的测试文件不扩大生产 query 的依赖范围，尤其不能造成 query 到 connected 的反向依赖。
query 的四项资源计数与 connected 的上下文/lease 计数分别跟随实际 owner 检查；共享原场景
不等于复制生产状态。connected 仅 `review_fixture_test.go` 可装配真实 Element/Mount/query，
断言文件仅使用既有 exchange 值和本包私有计数，不把装配依赖放开给生产实现或其他测试文件。

原 Foundation 候选、提交顺序、完整历史、快照元数据与 Store 损坏反例跟随 `internal/commit`；
夹具通过实际 owner 的完整绑定提交建立事实，不直接写事实表。运行中身份被替换的反例使用
外部 `commit_test` 和真实 Element 写/读/重试，保持同一打开数据库上的完整故障链。
仅 `foundation_fault_export_test.go` 可注入这项数据库身份故障、报告 reload 是否返回空状态、
读取持久身份与指定操作是否存在；只返回错误和有限值，不返回数据库、可写表或状态指针。
这三个辅助函数只属于测试构建，逐符号验证生产 `go build` 无法使用；不扩大生产提交 API 或依赖。

挂载写门的全状态不变性测试使用 `commit_test` 与真实 Element／query 操作。
`mount_guard_proof_export_test.go` 仅在测试构建内保存不可穿透的 clone proof，比较仍调用原
`sameCompleteCoreState`；不降级成只比 cut 或磁盘。配套 inspection 只返回原断言所需的
四项计数、root 存在性、指定元素 claim／kind、指定操作是否保留及当前 admission 半径。
不返回状态／表／数据库指针，不增加 setter 或生产 API；测试类型和函数逐项验证生产不可用。
五类故意破坏结构的反例留提交同包，不为了移测试向外导出任意破坏模型的能力。

原线性完整扫描参考实现归 `internal/query/oracle`：持原 owner 读锁，先验证当前状态、
选择原 cut、核对 boundary／basis 和挂载完整性，再执行原 scan／geometry 算法。它是模块私有
对照参考，不是新公开发现接口；不持另一套事实或快照注册表，不以 QueryRegion 的输出
代替独立对照。它的测试 fixture 可装配真实提交／Element，其他断言文件按实际有限值依赖登记；
contract、transport、SDK 和其他 owner 不得因此穿透私有参考或访问提交状态。
余下的损坏/精确深度 oracle 反例在提交外部测试包运行，只有 `oracle_fault_export_test.go`
提供原五种固定损坏和固定深树的三个测试函数；无任意状态回调、可写表返回或生产符号。
原 direct adjacency 与 259 条实际分页对照跟随 `element/read/adjacency_test.go` 的外部测试包，
只该文件可装配 commit/Element，生产 read 叶子仍不能依赖 owner。公开挂载覆盖场景在
`tests/query` 用 Composition 与真实 MountPeer 建立，不穿透业务 internal；公共权限在启动时装配。

投影路径损坏的三条授权/交错反例使用提交外部测试包与实际 Element 操作；固定 parent 损坏
只经 `projection_fault_export_test.go` 的唯一测试函数注入。原授权调用次数、发现路径的目标顺序、
非法 parent 不进入 Authorizer，以及授权期间写入后拒绝混合 cut 均保留，不新增生产故障接口。
Core 侧 Facet 生命周期/恢复测试同样装配真实提交、Element 和 facet 操作；权限变化发生在同一个
测试 Authorizer 内，不替换已装配的实例字段。仅 `facet_fault_export_test.go` 的两个测试函数
报告事件数/是否占用以及注入原五种固定 Store 损坏；完整状态相等与写入故障继续复用既有测试接缝。
这三项辅助均须证明生产 consumer 不可调用，测试文件按实际依赖逐项登记。Facet 历史快照的
公开测试留 `tests/query`，不能借此穿透提交或 Store；不向生产提交包引入操作包依赖。

原 Anchor Range 持久化和当前 Store 二十种损坏测试归提交外部测试包；同一真实 Element
夹具保留原主体、固定 ID、操作与两项拒绝回执，不以直接写事实表替代。原回滚完整状态证明
和写入故障接缝继续复用；新的两项 `_test.go` helper 仅能破坏固定历史缩小或原二十种已命名
Store 情况，不接通用 mutation callback、不返回数据库/状态。生产 consumer 必须不能引用。
Store 私有 JSON shape 随提交编码检查，格式 marker 检查随 store，扫描本模块全部生产 Go；
公开文件独占、创建不覆盖、打开不建文件测试归 `tests/instance`，仍用真实构造和生命周期入口。
公开关系的声明/opaque 字段/方法检查归 `contract`，不因离开根目录放宽原公开面断言。

挂载持久化、no-cut 回执与地址结果的七组原测试同样归提交外部测试包；真实 Element 和
Mount 操作共用唯一 durable owner，双方指定地址的测试连接不绕过 Prepare/Finish。
原 26 个逐阶段提交故障、18 个日志损坏、五种不可达历史和第三合法 Store 结果均保留。
仅测试构建可设置第 N 次提交失败、检查地址回执存在性、破坏固定日志/候选或注入固定第三结果；
计数句柄字段私有、只读 Calls，候选与完整状态不对外返回。生产 consumer 必须不能调用。
可信连接夹具随实际嵌套/联邦测试归位；最后根消费者已迁走，旧根共享夹具已删除，
不恢复旧根生产入口、测试包或兼容层。

原 QueryRegion 的七组白盒对照/数值极限/损坏/持久化测试随 commit 外部测试包归位，
使用同一真实 Element、query.Operations 与提交 owner；独立 query/oracle 不是被测查询的替身。
固定的 1,025 层边数树、40 层极小旋转树及七项损坏仅由两个同包测试文件构造，
不公开任意命令/状态回调。完整历史 proof 和精确磁盘字节检查保留；no-cut 比较只返回
操作数差值与原完整事实谓词结果，不返回历史对象。所有五个新测试符号须在生产构建中不可用。

原挂载控制/交错十三组测试共用已迁入的真实 Element/Mount/commit 和指定地址 transport；
fixture 的授权开关、并发屏障、原始 caller、ID 与命令不变，结果检查使用原有限事实/回执值。
同包测试仅额外提供事件数和两种固定坏历史的 current/full 验证结果；不返回克隆状态，
不新增生产 API，也不弱化并发精确重试、拒绝优先级、旧关闭隔离和写门清理断言。

嵌套挂载的七组祖先/子级交错与持久重开反例同样跟随提交外部测试包；五个实际 owner
只复用已有 Element/Mount 操作、指定地址连接和完整历史 proof，不新增导出测试钩子。
暂停在 reserve、Prepare 后及 denied-decision 的原交错时点保持不变；固定提交注入只在
fixture 内通过已存在的具体 owner 操作完成，不开放通用事务 callback 或可写状态。

现有跨网络联邦的 17 个服务值归 `internal/query/federation/value` 的唯一表示，
`contract/federation` alias 同一类型并声明原两个联系端口与 Range 构造入口。
该值叶子只依赖 shared/values、根 contract 与既有 geometry；根 contract 不反向导入主题合同。
ProjectionPage 的原 traversal 保持私有、无公开方法，只由原查询写入和 coverage 计算的包函数接入；
wire 不编码它，也不能导入该业务 internal。实际消费方统一使用主题合同，不留旧根 alias。
value 不持有事实、授权、source lease、registry 或 Store，Denial 仍跟随原 capture/check 流程，
不能为了移包把其授权证据并入可构造服务值。API 字段顺序、原条件摘要与 wire 验证顺序不变。

联邦有限读投影归 `internal/query/federation/projection`：原路径、记录、查询、遍历与关系投影
只接同一次借用的 Element／Mount／Facet View 及原 call-local signer，不接完整 commit.ReadView、
Core、Store、授权或 registry。原 PathIDs 有界算法也供同次 observation 绑定使用，不暴露事实字段。
借用结束、cut／权限复核、资源计数、dependency 绑定和 coverage 签发仍在原受控流程及锁序执行。
共享错误映射只维持原 Federation read 语义，未知错误原样传递；不与一般变更错误混用。
`internal/query/federation/transcript` 保存唯一既有摘要字节编码，固定 SHA-256／HMAC-SHA256 构造；
底层 hash 不公开，不接受任意 hash／回调，不拥有密钥、上下文、事实或权限。摘要本身不构成授权。
MountAdmissionKey 的纯形状校验归 mount/fact；只复用有限 commit/binding 的 OperationID 字节规则，
不依赖 commit.Owner，不复制校验，不增加公开合同方法。上述叶子均按实际文件和消费者登记依赖。

`internal/query/scan` 登记同一有限读集上的 closure、精确投影 memo、candidate、path 与原分页 coverage 扫描；
它依赖 contract、element/fact、mount/fact 的当前 View、query/request、geometry、shared/values 和标准库，不依赖 owner、挂载控制、
Store、权限或 registry。调用方保持原锁／不可变读集；各查询流程保留原错误与完整性策略。
扫描参数使用 element/fact.View，仅可读取原 live Anchor／Entity 两表；完整 State 嵌入
这份唯一 View，并继续持有历史 claims／tombstones 和修改／恢复操作。两者不是两份事实，
View 也不自行提供快照一致性；正式状态只通过原候选提交发布，借用与回收仍由查询 owner 管理。
原 UUID 字节表示／排序在 element/fact 只有一份定义，供读取与既有 Store 写序使用，不改变身份校验。
原当前挂载内容谓词在 mount/fact 共用，查询扫描和提交写门不各复制一份；
普通／公开扫描保留原 basis 优先级与挂载缺口表达，不能以扫描位置代替正式签发的续页凭据。

`internal/element/read` 是当前读取与历史查询共同使用的有限投影叶子：精确 Element/Facet 同 cut
读取、直接邻接和分页边界只定义一次。它读取 element/fact 与 facet/fact 的 View，
不导入 commit、Element 操作、query、Store 或权限，不自行承诺快照一致性或签发续页。
调用方持原锁或已保留的不可变视图，保留同一 cut、原错误和披露时点；不建第二份长期事实或索引。
Element 实际读写归现有 internal/element.Operations，不通过 query owner 转发。

## 4. 内部实现：完整职责与唯一提交

### 4.1 推荐职责集合

下表是基于 P6 实际代码确认的职责归位基准；是否独立成包仍须按实际依赖验证，不是现在就建立所有目录的施工清单。

| 模块 | 业务实现职责 | 本模块唯一的协调／存储职责 |
|---|---|---|
| Core | element：结构与空间写读；geometry：精确计算；query：查询、快照、续页与跨来源读取；mount：挂载控制；facet：Core 侧联系；access：请求上下文与事实观察 | commit：跨职责原子提交、修订、历史、发布及不确定结果恢复；store：现有数据库的字节读写与资源生命周期 |
| Facet | namespace：一个 Element 下内容、接纳、反向联系的完整生命周期；access：请求上下文与事实观察 | commit 与 store 只服务本 Facet，不能复用 Core 的可写状态或另起一套接纳裁决 |
| Realm | policy：主体、授权、凭据与相关校验；只有确有独立边界才拆出更小职责 | 其规模允许把协调与 policy 放同一内聚实现包；存储技术边界独立论证，不为了与 Core 对称凑包 |

Realm 在 S2 网络职责落定后的实际归位：根目录只导航；公开值、错误与权限常量归 `contract`，
构造／打开和身份组件装配归 `instance`，返回实际内核 handle，不逐个业务方法做代理。
`internal/policy` 保留唯一 realmState、锁、binding／credential 事务、Store 编码与重放；
`internal/identity` 拥有独立的运行内 proof 状态与签发／验证，不导入 policy 或持久 Store。
两种 revision 的唯一私有表示归 `internal/token`，contract alias 不暴露字段、拆取或铸造方法；
policy 内部函数读写同一表示，溢出检查与原持久编码不变，不把结构有效性当授权。
`transport/grpc` 只留配置、TLS 注册与主机生命周期；其 `internal/admission` 管原有服务准入、
challenge 与有限句柄，`internal/entry` 直接注册实际 RPC 并消费窄 Kernel／Verifier 端口，
`internal/wire` 仅编解码。三者不导入业务私有包或 instance。SDK 两文件保持内聚，不造空子包。
私有反例随实际 owner 归位，宿主保留真实 TLS 与入口装配测试，`tests/instance` 覆盖公开构造与重开。
位置登记精确到文件，依赖规则覆盖上述所有层级；不得在 contract、instance、transport 或 SDK
换个名字重新堆放业务。具体进度与验证证据仅见执行计划，本条本身不授权提交或升级合同效力。

未获相应切片授权前，不预建 transfer／handoff／chunk 目录。
P7 按实际切片合同确定归位；合法的字节分块等技术能力不因名称被先验禁止，也不凭旧代码自动恢复。
Facet 不为消费者业务类型校验规划 schema 包，相关已确认语义差距由开发计划先行对账，不混进纯目录迁移。
Core 内部的 `facet` 表示 Core 侧关系，不是另一份 Facet 内容模块；文件夹名字不决定公开关系类型的最终命名。

当前 Core 侧关系的本地与公开控制流程统一归 `internal/facet.Operations`，由 `Facets()`
取得真实入口；单次配置的接纳源与控制往返留同一职责，仍借用唯一 commit.Owner 的事实锁。
私有固定控制请求保存原 caller/View/requirements/context，不导出事务 callback、不反调根包。
原公开 View 校验与唯一并发 active 计数归 `internal/access.Admission`，所有 public 入口
共享同一实例，disabled 仍明确不可用；该组件不拥有事实、Store、上下文 TTL 或观察队列。
上下文 TTL 与观察队列已归 `internal/access/registry`，观察与入口分别归相应私有职责，不由 Admission 接管。Facet public 与联邦记录共用
`facet/fact.PublicInspection` 的有限签名投影，不读取新状态、不自行授权或提交。

Core 侧 Facet 的服务读取/准备检查同样归实际 facet 操作，接入唯一 Registry 而非新建观察队列。
本地反向联系显式绑定 `LocalFacetCoreEndpoint{Read: c.Facets(), Control: c.Facets(), Access: c.Access()}`：
关系读取与 JSON 观察调用使用各自真实入口，Control/Access 同时提供或同时关闭，调用前后都核对同 owner。
gRPC 宿主向 `Config.FacetControl`、`Config.AttachmentControl` 接入同一 Core 的实际关系操作；
固定服务启用时仍要求完整 AttachmentControl 端口，不从 JSON Owner 猜测关系能力。
可选功能不存在时保留原不可用/Unimplemented 或启用时配置拒绝结果，不合成能力；
装配核对同 owner，不扩展网络协议或要求请求携带新字段。

本地挂载、公开控制、地址维护、对端接纳与恢复流程一起归 `internal/mount.Operations`，
由 `Mounts()` 取得实际入口，仍共用上述 Admission 与唯一 commit.Owner；不保留根逐方法转发。
原始授权与 reservation 是该包私有固定记录，不开放事务回调。缺少原始 View 的网络恢复不能
为 pending 操作重新授权；已持久决定的收尾仍走原规则。命令摘要只有该操作内的一份算法：
入站 `Config.MountControl` 与出站 `MountConnections` 显式接收同 owner 的实际窄端口，
不让 transport 导入业务 internal，也不在 contract 中复制摘要或命令规范化规则。
公开 Pose JSON 表示唯一归 `access/value.PoseJSON`，记录投影与摘要共用，编码语义不变。

### 4.2 避免“流程调用提交，提交又调用流程”的环

每项职责先区分：确定性规则／事实表示，以及带外部依赖的操作流程。二者可以同包，
只有事务协调需要调用前者、而后者又需要调用协调者时，才在该职责内部提取下层 `fact/` 包。
不在所有职责里机械生成一套层级。

以 Core 元素为例，按 P6 行为基线完成归位的依赖是：

```text
instance ──装配──> element 操作实现、commit、store
element 操作实现 ──> commit、element/fact、contract
commit ──> element/fact、所需的其他事实规则、store
element/fact ──> contract、geometry、shared/values
store ──> 自身字节格式、标准库、现有数据库依赖
transport ──> contract 中实际使用的公开值、协议绑定
```

箭头表示允许的依赖，不是网络跳转。commit 不导入调用它的操作流程，事实规则不导入 commit；
transport 不导入事实实现，instance 之外的业务实现不反向依赖装配入口。
transport 使用的窄调用接口可在消费侧声明，由装配代码接入实际实现；不要求统一从 `contract` 取得接口。
同一规则只在事实职责中实现，正常提交与历史重放调用它，不各写一次。
如果某个拆法只能产生大量完整模型转换，应调整边界，而不是继续补 adapter。

commit 是本 owner 的明确事务协调，不是通用交易框架：
维护原操作去重、全局 cut／journal 顺序、候选状态、原子保存、失败加载和发布顺序。
只持有一套正式事实聚合；领域组件操作的是该事务的受限候选或不可变读取依据，不保存长期同步镜像。
不提供跨模块可调用的 `Execute(any)`、全状态 getter、任意回调事务或任意写 map。
允许为确有需要的跨职责事务设置具体操作入口，不为每个内部检查步骤再造一层端口。
不把内存已发布与磁盘已确认拆成两个事实负责方。

`internal/access/observe` 承接当前已实现的发布相关性投影：输入为同一发布边界的前后 Element、
Mount、Facet 只读 View，以及本次观察的有限选择条件；不接收完整 State、journal、回执、
Store、权限对象或生命周期回调。它只判断已有 spatial／attachment 分类是否受影响，
不拥有会话、队列、TTL、容量、锁或发布时点。实际队列更新仍在原唯一提交锁内，
后续迁出资源 owner 时也不能把这种纯投影当成完整观察生命周期已经迁出。
该包只依赖标准库、shared/values、Core contract 和本 Core 的三个事实 View；其他模块、transport、SDK 不得穿透。

store 只读写现有的规范存储表示，不判权限、包含关系或接纳有效性；相关验证留在 owner 的规则／恢复链。
Store 编解码与业务模型的转换属于真实持久化边界，可以存在，但不维护第二份长期可写对象图。
若拆 store 只多一层无意义的数据库转发，应保留内聚实现并记录理由，不为模板强拆。
跨 owner 仍通过消费侧端口与 Composition 装配，不增加共享事务、全局锁或路由目录。

### 4.3 三项边界检查

对每个拟拆边界记录以下数量与实际用途，不能只报告搬了多少文件：

- 正式状态：由谁保存、修改、发布；是否新增了镜像或缓存一致性负担。
- 公开面：新增／迁移了哪些类型、构造和方法，谁真实使用；仅为拆包而公开的能力应删除。
- 适配面：每条真实调用链新增几次转发、转换或反向端口；每次必须说明边界价值。

包装只为切换包路径、逐字段转换完全相同对象、每个小步骤都需回调原大对象，均视为需要改设计的信号。
允许协议、持久化、跨 owner 证据等真实适配；禁止以减少根文件为理由把这些区别抹掉。

### 4.4 全层边界与增长裁决

同样的职责审查递归适用于 `instance`、`contract`、业务 internal、通信 internal、SDK internal 和测试。
下表规定允许留下什么，不是六个必须创建的目录，也不是先搬进去、超过门槛再想办法：

| 位置 | 内聚边界 | 不得借迁移混入的内容 |
|---|---|---|
| instance | 构造参数、依赖装配、打开／关闭的对外入口 | 整份 Core／Facet 接收者及其逐方法代理、事实表、业务裁决、会话算法 |
| contract | 消费方实际共用的值与请求结果，按主题组织 | Store 编码模型、提交命令、重放事件、可写内部状态；为绕依赖环新造的通用接口 |
| 业务 internal | 元素、查询、关系等完整职责；必要时分出供提交与重放共同调用的确定性事实规则 | 所有 `public_*` 文件集中搬入 access；流程反调、平行事实模型、任意事务回调 |
| transport 的 internal | 实际 RPC 组及其帮助函数；同一会话／准入资源由一个组件拥有 | 从 Handler 直接访问业务 internal、复制会话表、每个 RPC 一层空转转发 |
| SDK 的 internal | 确有独立边界的响应验证、编码等客户端职责 | 服务端权限／事实裁决；把请求关联状态拆散后再同步；为小客户端预建层级 |
| tests／acceptance | 按真实场景归组的组合测试与最小 fixture | 私有状态全部公开后迁入的白盒测试、统一大测试包、跨场景万能 helper |

Core 的公共请求必须区分本地事实操作、查询来源读取、跨来源收集与观察生命周期。
`public_` 是目前的文件前缀，不是一个可以直接整体迁移的职责；请求入口归对应操作，
访问上下文、有限资源与观察接续按实际共享状态归位。正常准入与历史重放共用的请求解析／校验
应处于二者都能依赖的确定性规则边界，不能让 commit 反向导入整个 access 流程。

共享访问资源的下一边界是 `internal/access/registry`：唯一上下文索引、容量、TTL、父子与
派生 basis 释放、观察队列、collector 退休退款须统一迁移，不能按 token 种类分别建表或计量器。
原 Admission、query 快照 owner 和 collector 保持各自职责；registry 只提供受限操作、opaque 引用和
必要的有限只读结果，不开放可写 Context、全表或 mutex。到期取消与摘除在原 owner 锁内，
collector 退休在锁外，退款回原锁；同步发布只接原 before/after，不反向获取另一份当前事实。
来源读取算法归位前先完整接通此生命周期边界，不把所有公共操作原样搬进去。

来源流程实际归 `internal/query/federation/source`：Open、Project、Check、Release、Observe
及 Capture／Check Denial 使用同一 commit.Owner、query、Admission、Registry 和 observation。
不持另一张来源表／快照表、不反调 Core、不逐方法代理；实例 `Federation()` 返回真实入口。
完整本地来源连接随其归位，由 collector 既有端口调用，来源只返回原披露值与有限证据。
gRPC `Config.Federation` 显式绑定同 owner 的五方法来源端口；nil 仍不可用，不从 JSON Owner
动态查找；denial 不因此扩入网络协议。打开／替换／重开实例的实际宿主必须同步绑定该入口。

共享本地／来源观察流程归 `internal/access/observation`，只持同一 owner、Admission 与 Registry：
普通调用取锁、来源调用在原锁内执行相同流程，保留原 JSON 形状、权限、关系、队列和 limit 的校验顺序；
不因拆包新增一轮授权、另一套队列或先解析 limit 来改变失败优先级。纯 limit 规则归既有 access/value。
根公开 envelope 在 Acquire 成功后生成的私有准入 context 只绑定这份 Admission；
来源只识别同一组件，不认可其他 owner／外部请求字段。该标记不是身份或权限证据。
本地 collector 退休只调用 source 私有释放，普通服务 Release 仍走准入；
原 nil context 前置条件、取消／关闭和释放幂等行为保留，不开放通用“已准入执行”入口。
闭包授权 requirement 的原构造规则由 access 共用，不在来源与公共入口复制。
原根来源证据与观察反例随 source；零资源计数断言跟随 Registry，外部测试不得取得其私有表。

跨来源公共协调归 `internal/query/federation/coordinator`，只引用同一 commit、Admission、
Registry、source 与 observation 五个组件。collect 继续拥有实际来源图与租约，Registry
继续拥有公开 receipt、衍生 basis 与退休退款；协调不复制它们，也不反向调用 Core。
原 JSON 到 typed collect 请求及有限 Page/SourceGraph 到 JSON 的表示与协调一起归位，
保持 source 打开先于后置请求解码、两次混合保留尝试、锁外 I/O 与回锁披露复核的顺序。
公共 envelope 已归 `internal/access/entry`，直接调用 Read/Observe，不保留根入口或旧逐方法代理。

原根、source 和 observation 使用的相同 token 解析/归属/查找顺序唯一归
`internal/access/resolve`，所有函数要求调用者已经持有原 owner 锁，不建状态、不签发
凭据或自行准入授权。Registry 的有限返回值保持同一表示；这些包函数只对精确的当前
消费者开放，不让 transport、SDK、其他 owner 或提交层反向使用。续页条件的规范编码
归既有 access/value，由本地与联邦共用，仍只排除 limit 与 continuation。
WithAdmission 的精确使用位置与成功 Acquire 形状由实际源码扫描检查，不能只靠静态
测试中“某个 Acquire 文本出现在前面”来声称任意新增调用也安全。

本地公开 read/list/query 归 `internal/query/local`，仅引用同一 commit、Admission、
Registry 与 query 四个组件，实际执行原 borrow/capture、读取投影、固定分页条件、
派生 basis、准备观察及返回前检查。原结构地址检查随该有限读投影归位，不开放 UUID
裸查或改变空间范围。原公开 JSON 变更/recover-outcome 归既有 element.PublicOperations，
只用同一 commit、Admission、Registry；本地 typed Element 操作无需为了构造而依赖公开资源。
两类入口执行各自原有授权合同并共用唯一提交，不另建 Element 事实或重放状态。
Intent/Reference 仅在 access/value 解析原权限种类与联系值，不自行授权或查事实。

实际公开请求封套归 `internal/access/entry`，仅装配同一 owner、Admission、Registry、
local、federation coordinator、Element PublicOperations 六个组件。它保留统一容量与
JSON 分派，不吞并各业务流程；Acquire 成功后才附加同组件 marker，两项 defer、reservation
及错误映射次序不变。它直接满足网络/Composition 所需的公共窄端口，不增加第二次同名转发。
Core 构造/打开、随机运行 scope 与 Close 归 `instance`；`c.Access()` 返回实际入口，
原 Core.PublicCall[WithFederation] 不保留代理。实例只存真实组件与稳定身份，不持事实表，
关闭继续按原 BeginClose、资源关闭、锁外退休、查询关闭及 Store 关闭顺序执行。
原根公共调用八组组合测试归 `tests/access`，使用真实 instance、Realm 和 API schema，
不导入业务 internal；其余原私有根测试已随实际 owner 归位，模块根生产与测试均为零，覆盖证据仍须独立核对。

原 public_resource 两组队列溢出、执行准入、内存拒绝和过期退款反例归 Registry 外部测试包，
仅其 public_resource_fixture_test 装配真实 entry/local/source/coordinator/Element/commit，
一套 owner 与发布链贯穿请求；不把公开请求替换成直接操作 Registry 表。
public_resource_export_test 只在测试构建提供三个固定场景（Groups=1、Bytes=1<<19、
时钟前移两小时）和 entries/bytes 数值计数；无任意设置器、可写状态返回或生产符号。
按实际文件限制依赖，两个测试函数逐项验证生产 consumer 不能引用；其他 Registry 测试与
生产实现不能因此导入 entry 或业务 owner。旧根 shared helper 仅在尚有原消费者时保留。

公开联邦组合回归归 `tests/federation`，通过实际 instance、公开合同与指定地址 Composition
建立原多 Core 挂载，不导入业务 internal、Store、transport 或其他测试包。可变授权策略在
构造前注入同一实例，不在装配后替换 owner 字段。跟踪连接仅计数真实 Release，并以真实
Check 证明被释放来源不可再用；该证据不替代 Registry 的 entries/bytes 或快照 owner 的账本。
来源过期、容量拒绝、取消、非法模式及缺失 Purpose 的原六种资源反例归 Registry 外部测试；
丢失保留快照后的来源失败归 source，快照精确借用/退款仍由 query 自身测试验证。
Registry 的组合退休反例使用真实 source/collect 和显式屏障，分别验证锁内取消、锁外释放、
释放未完成时不提前退款、完成后精确归零及重复关闭不重复释放；不新增生产故障接口。

迁移联邦断言必须保持其原调用层：来源接纳的范围不匹配由 collect 的直接 follow 拒绝；
完整 query 对无法验证的 frontier 保留已验证来源并报告 UnresolvedCoverage。这两项是
P6 原本不同的合同，不得把后者改成整次失败来满足误放在上层的断言，也不得只保留
partial 成功而删除下层接纳拒绝、未加入来源图及资源释放的反例。

来源与 Facet 地址的同-cut 组合归 tests/query/source_attachment_test，只使用真实 instance、
Core contract 和 federation 主题合同：旧来源快照保持旧地址、当前读取立即见新地址，
Pending 控制值不能通过记录读取泄漏；不增加内部状态接口或修改写入持久化规则。

原 FederationDenial 唯一私有表示归 `query/federation/denial`，主题 `contract/federation` alias
同一 opaque 值，不保留旧根 alias 或额外 wrapper。该叶子仅存原拒绝联系证据，包函数不随 alias
公开；Capture/Check 仍由真实流程在原时点核对主体、请求、入站关系、期限和授权拒绝视图。
它不是许可或公开认证 token，不进 wire，不因能匹配结构就跳过授权。

唯一事实 owner 不等于全模块只能有一把锁。事实提交、快照保留、远端连接与会话准入可以有各自的
资源锁，但必须列清状态归属、锁序、锁外 I/O 及回到锁内的重验证；不得拆出第二份可写事实。
Core 的发布投影必须继续以明确的前后状态为依据；Facet 的投影必须继续以原命令／结果为依据。
存储确认、恢复安装、cut 推进与观察通知保持原子边界和既有顺序，不改成无保证的异步通知，
也不让观察组件在发布中途回调 owner 取得可能不匹配的“最新状态”。

新增包前在开发计划记录：原文件／声明、目标职责、状态 owner、允许依赖、实际消费者和测试去向。
混合职责文件先按声明拆清，不按文件名整批移动。每片复核所有受影响的直接包，而非只数根目录；
若一个候选包在搬迁前就超过控制线，先调整职责裁决，不将其作为已经批准的目标。

## 5. transport、SDK、生成代码同样遵守职责规则

`transport/grpc` 根包只做服务注册、配置、启动停止和必要的 RPC 入口。
会话、身份握手、明确的业务 RPC 组、对端联系应按实际耦合组织私有实现，
例如 `transport/grpc/internal/session`；不把模块业务裁决重新搬入 Handler。
RPC 分派只跨越必要的职责边界，不制造 Handler → adapter → driver → root 的空转链；
实际身份认证、协议校验和业务执行不能为了减少层数而省略或合并裁决。
RPC 组与其私有帮助函数一起归位，不只迁出常量和小工具。

同一会话状态只有一个 owner；各 Handler 使用受限会话操作，不各维护 sessions map。
权限检查、容量准入、锁外网络、返回前再验证的既有先后顺序必须保留。
“统一 middleware”不能成为调整错误优先级、重复授权或绕过披露检查的理由。
通信内的 `internal` 与模块业务 `internal` 分属不同私有边界，不能因为 Go 允许导入就互相穿透。

SDK 按连接／会话、实际业务请求与响应校验组织。相同共享协议值可以复用唯一编码，
但服务端授权、跨 owner 事实检查和客户端响应验证不是同一职责，不能为消重取消其中任何一步。
SDK 不能直接读取 Store、实例状态或服务端会话表。
binding 按生成器、语言、协议边界放置，不为每条消息创建目录；生成代码不手动拆分，
也不为了文件数好看随意重划协议源。必须有固定版本的生成工具和重生成检查。

仓级 `internal/protovalue`、`grpcbound`、`grpcdial` 只保留当前已经证明的具体共享用途。
依赖检查要区分业务私有包、transport 私有包和 SDK 私有包，不能统一用一个 internal 通配豁免。
未来所有实现目录都适用规模与职责检查，不能只约束模块根。

服务注册可直接注册实际 RPC 组的实现，不强制先在 transport 根包补一份同名方法。
会话容量预留、父会话有效性、锁外调用及返回后重新校验是一条完整准入链；
不能把不同 Handler 各自“验证过”当成这条链的等价物。
客户端正在等待的挑战、会话与请求对应关系同样属于客户端状态，不按“看起来是响应校验”迁到无状态工具包。
较小的 Facet transport／SDK 可以保留内聚布局，不机械复制 Core 的分组数量。

当前 Core 已直接注册 `transport/grpc/internal/attachment` 的四个 AttachmentControl RPC，
根包不保留同名转发。该组只持有构造时固定的服务联系和四方法消费端口；
`internal/session` 目前只拥有 caller／grant 的私有 context key 与访问函数，
实际 sessions／grants／pending／challenges／approvals 准入状态现已统一归 `internal/admission`。
这些共享状态继续共用原 mutex、失效规则和停止顺序，不按 RPC 组各建一份。
`internal/rpcfailure` 是原控制错误到协议错误的唯一映射，其他组直接消费，
不能反向导入 Server、业务 owner 或 SDK。FacetControl 两个 RPC 同样直接注册于
`internal/facet`；不支持该端口的 owner 仍在 RPC 时返回 Unimplemented，不新增构造失败。
原 mount peer 错误映射直接归 `internal/rpcfailure`，旧根包装已删除；该叶子只增加既有
contract 错误的依赖，不访问事实、会话或数据库。四个精确包的依赖与禁止反向／跨模块引用均由检查器登记。

应用侧 MountControl/MountContact 的五个 RPC 及命令/联系表示现直接归
`transport/grpc/internal/mount`；一个 Service 只保存原六方法消费端口、预期身份和
固定 mountconn，Server 直接注册同一 Service，不保留根同名方法或转发。
配置未启用（包括缺连接）仍先于 caller、delegation 和请求字段解析拒绝；Apply 仍先
Resolve，再解析本次 approval 并调用原提交端口，批准副本不修改已装配的连接。
该组不拥有会话、批准表、锁或事实；仅 service 声明和最小 fixture 引用 mountconn，
其余只消费已认证 context、原 wire/错误映射与公开值。精确文件、方向、禁用/错误顺序、
根同名实现与 Service 非依赖状态均有反例检查。审批与 peer 协商由上述共享准入统一拥有，
不能把这组完成称为整个 transport 完成。

`internal/admission` 直接实现六类原入站服务：Session、ReadDelegation、IdentityProof、
MountApproval、MountPeer、FederationRead。其五张原表、mutex、active/running 与 listener
注册/停止状态只有一份；TLS、真实网络关闭和 gRPC 宿主留 Server。Serve/Stop 仍经同一
准入锁登记，锁外 I/O 后按原顺序复核，不拆成两套锁或可独立续期的授权镜像。
普通 Core RPC 用有限 BindEntrySession/CheckEntrySession/CheckDelegatedResult 复用这份
准入；EntrySession 字段私有，只暴露原期限，不持第二张表，也不是新身份 token。
原配置定义移到实际消费者，宿主引用同一类型，不保留旧 Server RPC 代理；原 protobuf/JSON
编码直接归唯一 wire 叶子，不向业务内核或 SDK 开放服务端状态和 codec。
该包当前为 16 个生产文件/1,932 行、6 个测试文件/854 行；每个文件有精确依赖登记，
只许可原共享准入完整生命周期，不能借此吞并普通业务 JSON 分派或出站连接。

普通 CoreService 的 15 个 RPC 整体归 `transport/grpc/internal/entry`，当前四个生产文件
共 290 行、两个测试文件共 553 行；Server 直接注册实现，不保留方法代理。
该组只处理协议请求、原公共操作选择、结果表示与有限会话复核，不保存授权表或业务状态。
只有构造文件持有既有 federationconn；其他生产文件只消费协议/公开值、wire、session
及 admission 的三项 BindEntrySession/CheckEntrySession/CheckDelegatedResult 操作。
不得取得、别名化或转交完整 admission 句柄，也不得调用其停止、建会话或批准操作。
真实 admission 会话/委托返回前复核测试只在指定 fixture 文件中使用 SDK、metadata 与屏障，
生产代码不因此获得这些依赖；完整网络验收仍由 acceptance 覆盖，fixture 不替代它。
Core gRPC 宿主当前只保留一个 252 行生产文件，负责校验配置、直接注册服务、TLS 与启动停止。

## 6. 文件、命名、测试与规模

文件名说明职责，不用编号拆片；不建立 utils／common／manager 等无边界容器。
按 Go 工具链使用 gofmt 和既定换行规则。注释说明不变量、锁序、失败与取舍，不删除注释来降低行数。
同包多个文件是正常组织手段，但“都能访问同一对象的私有字段”不是必须同包的充分理由。

延续既有单文件门槛，并新增包级增长审查。以下数值是项目工程控制线，不是 Go 标准或正确性定理：

| 对象 | 触发人工职责审查 | 未完成边界裁决前停止继续扩张 |
|---|---|---|
| 单个手写生产 Go 文件 | 超过 500 行 | 超过 800 行 |
| 单个 Go 测试文件 | 超过 800 行 | 超过 1,000 行 |
| 单包手写生产代码 | 超过 25 个文件或 5,000 行 | 超过 50 个文件或 10,000 行 |
| 单包测试代码 | 超过 50 个文件或 10,000 行 | 必须给出测试职责与 fixture 依赖审查结论 |

物理行含空行和注释；包级只计算该包直接文件，不把全部后代误算成同包。
生产、测试、生成物分别统计；低于门槛却混合独立职责仍须处理。
控制线不要求自动拆包或拼大文件：可以批准有事实依据的内聚大包例外，
但必须记录精确路径、现状、理由和有限上限，不能给全部旧代码永久豁免。
不得改名、伪造生成标记、隐藏路径或搬进 testdata 规避检查。

**P7.2 编码职责收敛（2026-09-18，现行边界）：** 一个提交 owner 不要求其纯编码也在提交包。
`internal/store/record` 唯一拥有 Element、tuple、Revision、挂载和 Facet 联系的有限存储值编解码；
`internal/transfer/encoding` 唯一拥有转移 journal、manifest、lineage、父边及终局证据的规范编码与校验。
两者不得持有正式状态、锁、数据库、签名器、网络或授权入口；不反向依赖 commit 或操作层。
清单解码使用独立的有限 Element builder，不再构造临时 `coreState`；这不是第二个事实 owner。
`commit` 仍独占正式状态、候选、锁、原子提交、发布与恢复；正常提交和重放使用同一份编码规则。
不得为归位复制存储类型、改变字段／字节格式、增加公开接口或保留旧路径转发。

本次 commit 生产上限从 76 份／10,886 行收紧到 **61 份／8,300 行**（施工前实际已达 77 份／10,981 行）。
仍超过 50 文件控制线的部分，是同一事实聚合的确定性提交、重放和关系不变量；这不是无限增长许可。
新独立包分别为 `store/record` 5 份／709 行、`transfer/encoding` 18 份／2,046 行，均无规模豁免。
原 112 份 commit 故障／恢复测试保留，新增实际导入后为 16,817 行；存储形状指纹测试随编码包。
检查器仍为 28 份生产／58 份测试，有限上限为 4,343／11,239 行；增加的是精确依赖登记与反向／I/O／锁拒绝反例。
新包和调用者逐文件登记，不允许通过标准库 I/O、新增未审查文件或通信／SDK 引入私有表示绕过边界。
完整运行验证和实际施工进度只记开发计划；工程检查通过不能代替全仓验证或 P7.2 验收。

### P7.2 源证据通信边界（2026-09-19）

`transport/grpc/internal/transition` 的源读取组只从受限 owner port 读取已保存证据；
`internal/transitionwire` 只做有限网络值转换，不导入 Store/journal 编码、锁或业务操作；
`transport/grpc/transitionconn` 负责指定地址的 pinned client、有界收块和摘要核验，不拥有事实。
新位置和依赖逐文件登记，未登记文件继续拒绝；Core/Facet 根及 commit 上限保持不变。
检查器因 7 项精确文件登记与 wire 反向／运行态依赖限制净增 14 行，生产上限为 28 份／4,357 行；
58 份测试／11,239 行不变，在原反例表中增加新路径，未放宽通配依赖。

同日目标通信增量：`internal/transition/target.go` 负责固定 peer 的接纳／安装 RPC；
它在锁外拉取实际源证据，调用 admission 的 `AcceptFrozenTransition` 再进入原 target port。
admission 只保留原批准／会话生命周期，本次 `frozenSource` 是调用内的不可变单次回复，
不读网络、不读写 Store、不保留另一份转移状态，不允许任意执行或事务回调。
唯一实际提交、当前状态与恢复仍归 commit；finite receipt 转换继续归 `transitionwire`，
按指定地址调用的单操作连接归 `transitionconn`。只开放实际所需的窄接纳端口，不导入业务实现。
检查器新增 7 项精确文件登记及 1 行 admission 文件登记，当前生产上限为 28 份／4,365 行；
58 份测试／11,239 行、commit 61 份／8,300 行与 Core/Facet 根零源码上限均不变。
新增反例归本模块 `tests/transfer` 与各自通信／准入／wire／SDK 包，不扩张 acceptance 例外。

随后公开恢复增量：`internal/transition/recover.go` 只装配单次原操作到实际 transfer owner，
不另写状态机；同一 `transitionconn.Connections` 同时承接源读取与目标调用，不保留旧单向名。
`admission/entry_context.go` 的取消源属于既有有限 session 项，闲置时没有 timer／goroutine；
有 RPC 时才绑定期限和取消通知，释放、过期和停机沿原 session 生命周期传播。
最终 owner 检查同步读取消状态，不在 owner 锁下获取 admission 锁，不为每次恢复建立新 registry。
具体产品配置归 `cmd/ucupa-core/transition_connections.go`，没有 Store 或业务裁决。
本增量新增两行精确登记后，检查器生产上限为 28 份／4,367 行；已有跨 owner 产品测试净增
87 行，acceptance 上限为 70 份／10,096 行。两个实际可执行程序、SDK、指定地址及重启场景
没有调用私有 owner 辅助步骤；不把这条受限路径等同全部 P7.2。其它上述规模和依赖上限不变。

以下为前序施工的边界依据与规模记录；其中计数不是新的增长额度，现行计数以本段及检查器为准。
前序检查器测试包的有限例外为 `cmd/verify`：58 个测试文件／11,196 行。
这些测试共同检查未导出的源码解析、依赖规则和命令编排，fixture 是有限的注入源码及
临时编译目录，不穿透业务 owner 状态，也不是把业务白盒测试搬进工具目录。
保留一个测试包避免为了文件数公开 checker 私有函数或复制规则；按责任主题分文件，
精确计数上限登记在检查器中，后续增长需复核，不给其他 tests 包通用豁免。
检查器生产部分当前及有限上限为 28 个文件／4,168 行，超过文件数审查线但未超过生产控制线；
这些文件是同一检查流水线的具体职责规则，不持业务状态，不为计数合并无关规则或导出私有解析器。
collect 和 transport 的精确位置登记与各自依赖策略放在同一文件；通用路径成员判断留分类文件，
不让中央分派继续吸收具体主题登记。此归位不改变允许边或检查顺序。
本次新增的是根退出、真实 collect 消费方、应用侧 Mount RPC、共享准入和普通入口的精确注册/AST 反例；
仍为上述同一测试包，没有增加业务 fixture 或向工具包搬业务状态。
commit 的 59 个测试文件／8,603 行是同一个持久 owner 的事实、历史、提交故障与恢复反例，
P7.1 新增一份 writer 事务反例，覆盖同原始 Store 的关闭、安装、不确定结果和并发提交，仍使用同一真实提交链。
P7.2 首批仅增加真实 Store 下的锁内预检、观察 cut、原结果、重启与旧 writer 拒绝反例；
闭包及 placement 纯规则测试随 `internal/transfer/fact`，不集中到提交包。
该有限测试例外不允许继续把普通查询或其他 owner 测试集中到 commit。

P7.2 持久控制再增加意图／取消与存储故障两份提交测试：同 ID 并发、原始非 UTF-8 身份字节、
精确取消重试、容量满后仍可取消、日志损坏、提交前回滚／提交后响应丢失和实际 writer 交接。
这些测试只需要该 owner 原有 Store 故障接缝；不向公共 contract、SDK 或 instance 暴露状态。

P7.1 的生产 commit 上限为 28 个文件／4,927 行；P7.2 首批预检为 29 个文件／4,985 行。
持久意图增量为 32 个文件／5,385 行；源 fence 增量为 34 个文件／5,650 行；
源写入保护增量为 35 个文件／5,724 行；有限读取效力增量为 36 个文件／5,807 行；
完整清单解码与私有目标接纳增量为 38 个文件／6,132 行，提交职责测试为 62 份／9,101 行；
源唯一决定、目标原子安装与源收尾增量为 41 个生产文件／6,644 行、66 份提交测试／9,907 行；
返回迁入增量为 43 个生产文件／6,891 行、69 份提交测试／10,379 行；
Core root 接续增量的实际上限为 44 个生产文件／7,054 行、75 份提交测试／11,144 行。
carrier 父侧准备增量为 47 个生产文件／7,464 行、79 份提交测试／11,752 行；
控制方身份与源持久拒绝增量的当前实际上限为 49 个生产文件／7,800 行、81 份提交测试／12,126 行。
后续 carrier 源关闭增量的有限上限为 50 个生产文件／8,071 行、83 份提交测试／12,481 行。
新增 `carrier_fence.go` 只承接原 owner 的持久关闭、原结果恢复与当期有限证明投影；
其与原拒绝事务复用同一个 controller-qualified 履约记录、候选和提交域，不另建 fence 表或日志。
`transfer/fact/carrier_source.go` 负责外部父锚下的完整内容选择和纯证据形状，空闭包也须成立，
不把父侧锚或伪根写进源内容；外部 outgoing 边未完成接续前仍明确拒绝，不能默默省略。
原 manifest codec 在真实内容 profile 边界共用成员、Facet 前态与 lineage 编解码，不复制一套 carrier 存储模型。
incoming 的 fenced 门与普通 detached 门区分；原匹配关闭证明进入有限 read projection，
仍在现有每个 incoming 64 KiB 的读取计量内，不保存 manifest 或整本历史。当前与保留披露继续核对源效力。
两份新 commit 测试只使用真实持久 owner 与原故障接缝；纯闭包、空空间观察反例留各自规则包。
这次依赖新增逐文件登记，检查器生产上限同步为 28 份／4,206 行，不给后续增长或整个 transfer 通用豁免。

carrier 目标接纳增量再次完成职责审查后，`internal/commit` 有限上限为 52 个生产文件／8,259 行、
85 份测试／12,855 行；检查器为 28 份／4,212 行。新增的一个接纳事务文件和一个私有证据 codec
都依赖原 owner 的候选状态、日志与 Store，不另设接纳表、锁、事务或当前真相。接纳结果作为
原 `targets` 保留索引的 carrier 变体；同用容量限制和建根／建挂载／转移的保留检查。
完整闭包与未激活接收位置的纯规则分别留在 `transfer/fact`，不为文件数量把提交裁决搬进它。
测试按接纳语义、故障／并发／存储证据分组；纯 placement 测试在规则包内。原竞争挂载 fixture
改为真正无本地子内容的合法请求，避免以无效请求冒充并发反例。这是本增量的明确上限，
后续 parent binding、安装及公开链仍须逐项审查，不获得继续扩张或自动豁免。

carrier 父侧唯一 binding 与目标安装增量的职责审查上限为 57 份生产文件／8,727 行、
90 份提交测试／13,472 行；检查器为 28 份／4,223 行。binding 与 install 两个事务仍只使用原
owner 候选、提交与 journal；私有 codec、有限 origin 校验和 lineage 证据随同一履约模型归位，
不把 workflow、授权或 RPC 放入 commit，不新建第二份状态／提交器，也不复制安装算法。
原 `targets` 索引统一区分是否已安装；普通写不再误把已安装 carrier 当成待接纳的保留。
当前 incoming 的 carrier origin 用真实安装结果证明，不制造普通 MountPrepare／PeerAttempt；
查询只携带有限证明，完整日志和 manifest 仍不进入 read view。原日志重放在转移提交后登记实际
签发的挂载／联系 revision，使后续普通地址修复可恢复，不接受由请求者自行构造的旧 token。

carrier 收尾与返回增量复核后，`internal/commit` 上限为 58 份生产文件／8,970 行、
93 份测试／13,987 行，检查器为 28 份／4,227 行。新增一个收尾事务文件共用原 owner、
control slot、转出记录与 journal；父侧释放原槽，源侧只保留已完成转出，不重新开写门。
返回所需的关系接续规则归 `transfer/fact`；原 UUID lineage 验证扩展到 parent 控制的 carrier，
不新增另一套 claims、查询历史或恢复器。收尾、循环返回和日志损坏三组测试分别归位，
精确依赖逐文件登记；上限不授权其他流程、公开接口或通信实现在 commit 内继续膨胀。
父侧日志直接绑定 acceptance 中的原 actor 与 input，不重复序列化 actor；最长合法编码反例检查
原 8 KiB 控制事件与总 journal 的收尾预留，不以放大容量掩盖重复存储。
新增测试按父侧决定、目标安装、真实多成员及前态、故障／并发／损坏、codec 容量分组。
parent/source settlement 与 carrier 返回已在上述收尾增量完成；外部边接续仍按下述实际增量核对，
不将内部事务通过等同于公开转移链或整个 P7.2 已完成。

外部边捕获及承载侧持久关闭增量的有限上限为 `internal/commit` 60 份生产文件／9,292 行、
96 份测试／14,434 行，检查器为 28 份／4,235 行。新增 `transfer_edge_close.go` 仅处理原 owner
内的控制事务，`transfer_edge_store.go` 复用原 manifest／journal codec；不是独立 workflow、
Store 或第二份可写空间。原 transferState 中按 controller／operation／mount 保留的有限索引
只由原日志重放重建，仍计入原意图、事件和清单容量，不扩张资源上限。完整清单只在关闭时
验证，普通读取只带当前关系的一份有限闭门证明；与普通 detach 和内容已迁出的 carrier fence 区分。
确定性边界选择和闭门证据归 `transfer/fact`，当前为 13 份生产文件／1,148 行、10 份规则测试／962 行。
三份提交测试分别检查普通／root、嵌套 carrier、故障与并发，使用真实 owner 和原提交路径；
观察 GAP 反例仍归 observe。target 在新父关系尚未匹配接续前继续拒绝安装，不能凭源闭门
自动启用。精确依赖逐文件登记，不预建包、不将 RPC、授权或工作流加入提交包。

外部边目标私有接纳增量的复核上限为 `internal/commit` 61 份生产文件／9,447 行、
98 份测试／14,782 行，检查器 28 份／4,240 行。新增的关闭证明 codec 只负责原接纳事件
内的规范编码，共用既有 admission、cut、浮点表示和原 8 MiB 保留容量；不重复存储每条边的
完整源输入，不另建数据库或恢复入口。逐边完整匹配与目标绑定归 `transfer/fact/edge_acceptance.go`，
提交包只负责在原锁和事务下保存同一组证明、成员及 placement 保留。新增两份提交测试检查
私有接纳、故障、精确重试和损坏恢复，规则测试跟随纯事实包。接纳不等于新父关系已经启用；
含边界的安装仍须等待后续匹配 delegation 接续，不以接受证明绕过该门槛。

外部关系接续与目标完成增量（2026-09-18）再次复核后的有限上限为 `internal/commit`
68 份生产文件／10,038 行、101 份测试／15,213 行，检查器 28 份／4,255 行。
超过生产 10,000 行控制线的新增内容限定为同一 owner 的新父关系安装、子侧重新接纳、目标完成事务，
以及这三者的原 journal codec 和有限 incoming 证明；不加入 RPC、授权装配或跨 owner 工作流。
这些事务必须与原 cut、成员保留、挂载记录、候选及发布原子更新，拆出另一可写事务 owner 会破坏边界；
纯安装／完成证据形状仍归 `transfer/fact`，该包当前 15 份生产文件／1,330 行、11 份测试／1,060 行。
接纳时在原 8 MiB 保留量中同时预留逐边收尾回执，完成后用实际规范字节消费该预留，不重复完整输入；
每个目标增加一个有限的紧凑完成事件额度，原 64 MiB journal 字节上限不变。
原 targets／edges 索引共用唯一提交与恢复；读取只借用有限可用性投影，不读取整本 transfer journal。
已安装但外部边未全部接续的成员不作为匹配、列举项或路由入口；相关查询保留缺口，旧快照不升级为新状态。
查询／观察与纯规则反例跟随各自包；三份新增提交测试只覆盖真实事务、重启、回复丢失、损坏和容量预留。
新增依赖逐文件登记，不批准整个 transfer 子树，也不将本次审查视为后续公开调用实现的增长许可。

父侧关系保留与普通 slice 两侧 incoming 增量复核（2026-09-18）：提交包有限上限为
75 份生产文件／10,716 行、111 份测试／16,642 行，检查器 28 份／4,274 行。
新增五份生产文件仅负责原挂载控制槽的保留／释放、同一快照的事实校验、源端 incoming
匹配及两类既有 journal／manifest 编码；占槽、空间 cut、提交历史和恢复不得分成不同事务。
父侧持久承诺是临时的关系保留，不是新 owner、锁服务、授权或另一套转移数据库。
证据形状、有限闭包与 placement 选择归 `transfer/fact`，当前 16 份生产文件／1,528 行、12 份测试／1,184 行。
源 manifest 保存一次精确的父侧保留证明，仍受原 1 MiB 单清单和 8 MiB 总保留上限约束；
父侧保留共用原 1024 个意图及 64 MiB journal，已终止记录不腾出身份，不增加清理或回退入口。
新增五份提交测试分别检查真实关系与源转移、故障恢复、非 UTF-8 精确字节和容量；
协议形状反例留纯事实包，不把授权、网络和跨 owner 编排移入提交包。
目标 incoming 再增加两份生产文件，仅负责目标真实 admission／父侧保留匹配和保留证明的有界编码；
源 manifest 与目标 acceptance 复用同一父侧证明 codec，关系保留进入既有 acceptance 索引与 journal。
目标证明单份最多 8 KiB 并计入原 8 MiB 总保留证据，不挤占安装／收尾的控制封套空间；不增大原日志上限。
现有 placement 规则直接检查外部父范围与本地祖先，不生成伪父元素；原事务同时保留 UUID 和 placement。
新增五份提交测试按目标约束、故障／损坏、返回 lineage、容量及双侧关系组合分组，复用真实 owner fixture。
这些检查、保存、恢复属于同一个原子承诺，不为文件数另立 acceptance owner 或逐方法回调包。
此项只覆盖私有事务及持久证据，公开入口及跨进程链仍须后续接合与验证。

父侧保留读取接合增量（2026-09-18）：提交包仍为 75 份生产、111 份测试，精确上限分别为
10,741／16,665 行；检查器 28 份生产／4,278 行。本次不新增提交文件，只从现有占用槽和同一
父关系校验器投影精确当前关系，未改变原 mount facts、journal 或控制占用。最终披露只检查当前
挂载槽，不扫描迁出成员或整本历史。纯读取证明归 `transfer/fact/visibility.go`，不携带 actor、
manifest、授权或控制记录；不是写入许可。每份保留关系额外计入快照 64 KiB 逻辑容量，不扩大上限。
联邦投影和凭据检查使用同一规则，仍精确比较原 revision、admission、slot 与 contact；普通变更槽
不能借此开放。Connected 的有限授权投影不传出这份不需要的父侧控制证据，公共合同不新增字段。
真实独立 Store 的联邦读取、续页、观察、撤权及地址修复反例归 `federation/source`；重启故障的
只读投影断言补在原提交测试，容量测试归 query。各新增依赖逐文件登记，不放开整个 transfer 包。

源端公开转移控制增量（2026-09-18）：`internal/transfer/operation.go` 只持同一 commit.Owner 和
Admission，负责原 caller 的独立 export／inspect 授权、准入、上下文与原 view 复核；
`instance.Transfers()` 返回实际入口，不代理各动作，不增加独立转移服务或恢复表。
`commit/public_transition.go` 在原锁下完成公开 CAS 与实际当前／原输入的比较、原取消结果恢复及
有限状态投影；调用原意图／取消事务，不另写 journal、不换 Store 格式、不保留第二份公开输入。
Begin 此时只记 INTENT，不读取目标或导出内容；目标限定批准及 fence 前的双方 view 复核仍由后续
真实推进链完成，不能拿此入口代替完整转移。公共结果不携带 manifest、私有 cut／控制序号或批准。
签发 `transition-revision` 的唯一固定用途函数归原 token 包，复用 owner 的持久签发材料，
比较值绑定 Store scope、operation、boundary 与控制修订，不是凭据。公共值、JSON token 定义及
共享 Protobuf token 种类同步。真实 Realm／instance 场景归
`tests/transfer`，不导入业务内部或 Store；注入持久提交前后故障的投影测试归原 commit。
本增量 commit 精确上限 76 份生产／10,874 行、112 份测试／16,801 行。新增一份提交表示文件的原因是原 MAC／保留结果只归唯一 owner，
不为降低行数把密钥或完整状态交给流程包。通信与 SDK 禁止直接导入 transfer，实例仅开放此一条装配边。

源端 gRPC 增量（2026-09-18）：`transport/grpc/internal/transition` 以 `service.go`／`wire.go`
承接三个公开源控制动作及有限协议转换，测试随同组归位。它只依赖公开合同、原 session 准入和
wire／failure 适配，不持有事实、提交表、授权副本或网络重试循环；仅 transport host 装配。
原会话在调用前后均核对，原 caller 交实际 transfer 操作再次授权。SDK 只使用生成 stub 并核对
返回 source／operation／boundary／kind／target／comparison，不能接收错配结果或代替服务编排。
`ErrStore` 在源控制写入映射为 INDETERMINATE，精确检查映射为 UNAVAILABLE；断连不证明未提交。
产品进程测试归 `acceptance/transition_product_test.go`，复用既有真实 TLS／Realm／启动与重启夹具，
不是完整多 owner 转移验收。检查器精确上限为 28 份生产／4,299 行、58 份测试／11,211 行；
acceptance 为 68 份测试／9,799 行。只增加实测职责用量，不给其它职责可复用的空额。

目标有限批准增量（2026-09-18）：`internal/transfer/approval.go` 使用同一 Admission 与 commit.Owner
捕获／复核目标原 caller/view，检查 import 与 slice 最终 parent 的 place 权限，不存第二份授权事实。
`commit/transfer_preflight.go` 只补充十二行持久目标可用性检查，不新增事务或改存储格式。
运行期 token、原会话、精确 binding 与到期清理归既有 transport admission，Create／Check／Revoke
直接注册，不转发到根目录；有限容量独立配置，重启清除，不与 durable Accept 混为一谈。
同一个 command 的边界解码归 `wire/transition.go`，源控制和目标批准共用；SDK 只核对原输入和
结果归属，回显的原 source 比较 token 不冒充 target 签发，且不能给其它响应放开 owner 检查。
真实 Realm 的本地授权反例归 `tests/transfer`，会话并发与寿命反例归 admission，产品进程与实际
TLS 调用归两份 acceptance 测试。新增依赖逐文件登记，admission 禁止业务内部、Store 与外呼依赖。
本增量检查器生产／测试精确上限 28 份／4,304 行、58 份／11,213 行；commit 为 76 份／10,886 行；
acceptance 为 70 份／10,009 行。Core／Facet 模块根源码继续为零。

新增源 fence 事务与规范 manifest 两份文件，仍由原 owner 的锁、publish、persist、reload 管理。
manifest 复用原空间及 Facet 关系 codec，只保留所选成员与祖先，不持另一份长期可写状态。
源活动记录退出、历史 departure 占用、原结果、final cut 和 fence journal 必须在同一事务中核对；
新纯 departure 规则及测试留在 element/fact，不把这些规则堆入协调包。
新增的源 fence 测试仅检查真实 Store、并发写入、响应不确定与清单/提交序/迁出记录损坏。
`transfer_guard.go` 只在原提交 owner 内复用非活动成员与有限控制记录，保护源祖先配方，
不逐次解码 manifest 或重放历史，不引入持状态的另一个规则 owner。
Element 更新/删除、Mount 新占槽、另一源转移与 Facet 新决定共用其必要校验；
既有回执先恢复，无关兄弟的写入不被全局禁止。对应一份真实持久事务测试覆盖重启、
祖先写入竞态、空父锚重挂、较早意图的重采和伪造 fence 后历史，不向本包搬普通查询测试。
超过 5,000 行审查线的原因是同一事务必须同时验证原结果命名空间和新增控制记录，
不能把完整 owner 状态暴露给第二个“转移事务”包；纯输入与空间规则留 transfer/fact。
它不是对后续 fence／接纳／安装代码的无限上限许可，每批新增仍须精确复核。
P7.1 的三份 writer 记录、事务与安装代码保持原职责，
持久日志仍由同一个 owner 管理，不为减少文件数拆出第二套写入权威。调用者准入归
`ucupa-core/internal/handoff`，RPC 直接归各模块 `transport/grpc/internal/writer`，实例只返回实际入口。
这些包及其消费者逐文件登记；共享值不持状态，通信不能导入业务内部包，未登记的后续包仍拒绝。
P7.2 当前 transfer/fact 为十一份生产文件／974 行、九份规则测试／884 行；它不持锁、Store、
授权或另一个状态 owner。commit 的预检、意图／fence／控制重放、私有 codec，以及实际有限
读取消费者逐文件登记；只有 codec 文件可引用原 Store 字节工具，只有故障测试可引用 bbolt。
fenced 意图的 Origin 空间元组从同一规范清单推导并由原历史重建，不由调用方指定。
它包含原父锚和包围范围，支撑祖先保护与尚未闭合的 coverage，不复制整个闭包或另存授权。
`transfer_read.go` 只从当前 departure 与有限意图构造只读 Visibility，并在唯一 owner 锁内
校验最后披露时的源效力；它不解码 manifest、不重放历史、不保留控制日志或增加事务 owner。
快照仅增加非活动 UUID 到原边界的映射及有限空间包围证据，纳入原容量计量；
本地、Connected 和 Federation 查询共用覆盖排除规则。Connected 只过滤原授权范围内的证据，
Federation 继续输出现有 Gap／Complete 与绑定后的 coverage，不把内部标记直接暴露为新 wire。
观察发布只借用同一有限证据：未有有效 successor 的源退出使相关队列 GAP，不能成为普通删除事件。
普通查询、联邦、观察与隐私反例分别随实际职责归位，不搬入 commit；不新增公开状态或测试 setter。
`transfer_receive.go` 只在源证据传输／目标接纳时执行有界规范解码，复用原 Element codec、
图校验、闭包与 Facet 关系值；不复制 writer key、原操作历史或活动目标状态。
`transfer_accept.go` 只在原 owner 事务中保存精确 source fence／完整清单与目标承诺，仍不安装或公开。
UUID 保留按 Source＋Operation 绑定；有限成员索引由日志重建，不冒充永久删除或抢占本地 OperationID。
此保留仅限制冲突写入，不向查询 Visibility 加标记，不把原有效空结果变成源迁出错误。
目标 parent／祖先配方与保留 UUID 受原提交 guard 保护，无关兄弟及共用 parent 的不相交转移可继续。
清单解码、接纳／回滚／响应丢失、父删除竞态和日志损坏测试随 commit 的实际持久 owner 归位。
SourceFence／TargetAcceptance 只是证据值，不是凭据；现阶段仍为私有构建入口，后续操作层必须取得
指定来源的认证证据及原调用者目标限定批准，不得把收到相同 hash 当作授权或安装许可。
意图不是源 fence、接纳保留或安装；INTENT 阶段仍允许正常读写。
检查器增量只登记这些实际边和拒绝反例，不提前批准后续 transfer 文件。
`transfer_successor.go` 在原源 owner 中绑定精确目标接纳，唯一决定不修改数据 cut、原结果或源不可用状态；
固定 FenceRevision 保留原 fence 结果，不随最新控制 Revision 被替换。
`transfer_install.go` 只在原目标事务安装完整已接纳闭包、签发本方各元素 Revision、推进一次 cut，
仍走原 Store／重放／发布。确定性成员赋值归 `transfer/fact/install.go`，不是另一个提交或 I/O owner。
读集只接收从有界安装记录推导的修订数上界补量，完整历史仍证明每个实际签发；
不能以原先“一次 cut 最多一个元素 Revision”的假设拒绝合法原子批次，也不伪造中间 cut。
target accepted 与 installed 原结果分别保留；安装才释放本方 placement 保留，source 未收尾仍保留 gap。
首批安装原先仅允许 NONE；下面的 Core-side Facet 前态增量明确接续 ACTIVE／DETACHED，
不能静默删除或复活旧 Core key，也不能因此把 Facet 内容转移算作已完成。
两份新提交测试跟随真实持久 owner；纯赋值规则与真实分页反例分别留 transfer/fact 与 query，依赖精确登记。
`transfer_settle.go` 只核对本源唯一决定、原目标安装结果与源闭包退出证据，在原事务推进一个 coverage cut；
不修改 Element Revision 或原普通操作结果，不在本地事务中查询网络。恢复返回原收尾结果，而非最新 cut。
DecisionRevision 与 FenceRevision 分别固定原事件；SourceClosed 保留历史失效，SourcePending 只管理未完成保护／缺口。
既有快照失效不随收尾解除，当前 source departure 不变为普通 NOT_FOUND；只有新 cut 可去掉已闭合迁出缺口。
转移日志同时为已接纳任务保留事件与字节上限：最多 6,144 个有界控制事件及 8 MiB 原始清单，
64 MiB 私有日志上限覆盖控制输入与编码膨胀，不挤掉终局义务；不新增事件系统或历史 GC。
真实故障／重启和容量封套反例归 commit；Query 的真实缓存 cut、收尾前后完整性与旧分页反例继续归 query。
返回迁入沿用上述原安装事务，不建立另一套返回 owner、业务结果或 claimed 表。
`transfer/fact/lineage.go` 只校验和复制所选 Element 的有界连续性值；整份 slice 最多 4,096 步，
还受原清单字节上限约束。线性记录包含历史 owner、原操作身份、源修订／cut、目标安装 cut 和原决定摘要，
不携带无关旧成员、旧空间元组、调用者凭据或完整历史清单；摘要不是身份认证或授权。
`commit/transfer_lineage.go` 仅在转移控制捕获时从本 owner 实际 installed 记录派生连续性；
接收侧与本地最后一次 settled departure 及其不可改历史前缀核对，再交原目标事务恢复准确 inactive generation。
普通 Create 仍不能重用 claimed；active UUID、永久删除、旧轮次、断链或错配证据拒绝；
较早源收尾尚未完成则保持 unavailable，不超时抹掉义务。新旧成员可在一个合法闭包内原子安装。
`transfer_lineage_store.go` 是原规范清单的私有 codec，保留非 UTF-8 OperationID 字节；不新增数据存储域或兼容入口。
`element/fact` 只执行已验证候选中 exact departure 的移除，claimed 与原结果不清除，不能单独发布残缺状态。
三份返回测试归真实 commit owner；纯链及赋值／departure 反例归各自 fact 包，旧查询依据不会因 UUID 返回复活。
Core-side Facet 前态沿用原转移 manifest、journal 与原子安装，不另立 attachment 事务。
`transfer_facets.go` 捕获原 Core 签发的有限公开比较值；已有前态继续保留，返回原 Core 时用其原 signer 核对。
`facet/fact/predecessor.go` 只执行 NONE→NONE、ACTIVE／DETACHED→DETACHED 的候选赋值：
目标自己的关系修订与原 key 分开，清除可用联系和 pending；不接触 Facet namespace 或 payload。
`token.PublicSigner.FacetProposal` 仅表示原比较值，不签发另一 owner 的 token；格式层仍不依赖事实状态包。
公开检查以 KeyCore 表示历史 key 归属；当前 Revision 属本 Core，DETACHED 不带联系，SDK 不准其扩大来源、权限或当前 proposal。
纯前态反例、持久多 Core／返回与故障重试、同 cut 分页读取、wire 和 SDK 反例分别归实际职责包。
这些新增测试仍不代替公共／网络认证转移操作链，不能以内部哈希核对替代后续指定源认证。
root 接续使用原 transfer intent 的显式 profile，不以零值 bounded tuple 猜测 root；
目标只允许未激活空空间或经原连续性证据验证的同根返回，不覆盖独立 root，也不拼接其他旧空间。
唯一 root-slot 保留同时约束 CreateRoot、incoming Mount prepare 和竞争 root 接纳；不建立可独立修改的第二张 root 表。
source 的 root-role 标记随原 departure 编码并由原提交历史重放核对；转出后不能普通创建另一 root，不能报告完整空空间。
目标安装仍保留 root UUID、无 Parent／Pose 和无界 Range，整份本地闭包只发布一个本方 cut；旧快照不因此复活。
四份 root 提交测试只覆盖真实 owner 事务、接纳竞争、持久前后故障／重试／重启与坏数据；纯规则和分页反例在 fact／query。
本增量不冒充 carrier 或 external Mount 边接续完成，仍禁止无接续证据把外部内容合入清单。
carrier 父侧准备复用原 MountOperation 的互斥记录形态和挂载控制槽，控制事件进入原 transfer journal，
不新增另一张挂载状态表、提交日志或恢复 owner。完整历史仍由原 Core replay 推导；
快照只保留当前占用槽的有限证据，不把所有历史操作带入普通读取。
reservation 与 fence permit 分开：前者可取消；后者只关闭本地取消窗口，不冒充源已 fence 或目标已接纳。
地址修复不被占槽阻塞；尚未 permit 时，旧联系修订不能继续发出 permit，须取消后按已修复联系重新准备。
原始请求者、预期元素／挂载修订、三方身份与地址不可被同 OperationID 的重试改写。
新三份 commit 生产文件仅负责父侧原子控制、有限事实校验与私有 journal codec；纯关系值在 transfer/fact。
四份 commit 测试分别覆盖真实事务、持久故障／竞争、坏记录与有限投影，不借整理扩张公开接口。
普通 Mount 的重试、决定、收尾与恢复只处理原类型，不能以空 binding 把 carrier 记录当成普通挂载回执。
这不代替后续旧承载方关闭、目标接纳、父侧唯一绑定和认证 public/gRPC/SDK 转移链。
转移的操作身份使用（实际控制 Core，OperationID）：普通 slice／root 由源控制，carrier 由父侧控制，
不能把后者的源 Ref 当作控制方。departure 与 lineage 明确保留控制方；缺少此项不能在加载时猜测或兼容补齐。
源对外来 carrier 操作的有限履约记录按此二元组放在原 transferState 中，与本地调用结果命名空间区分；
它和本地源意图、父侧控制共用同一容量，不增加第二个日志、Store、锁或恢复入口。
`carrier_refusal.go` 在原提交 owner 中保存不可反悔的“不执行本次 fence”结果；父侧只能用匹配的源结果取消已 permit 的操作。
拒绝不修改空间事实或冒充源 fence；原切点和操作结果不变。父侧取消只释放自己的原槽，不替源恢复写门。
`carrier_refusal_store.go` 复用已有身份字节、revision、cut、挂载 key／联系 codec；证明值不等于网络认证。
两份新增测试归真实提交与恢复职责，覆盖原结果、容量、持久故障、并发重试和损坏历史；纯身份/证明反例归 fact。
检查器生产有限上限为 28 个文件／4,201 行；新增依赖逐文件登记，不放开整棵 transfer 或 commit。
Facet namespace 维持同一内容与提交 owner，当前 23 个生产文件／3,520 行、20 个测试／2,886 行。
跨 owner 的 acceptance 有限上限为 67 个测试文件／9,681 行，新增两份真实 Realm／产品进程验证，
不承接业务私有状态或通用 fixture 库。检查器的相应增量只实现这些精确边界与拒绝反例，不放宽整棵目录。

测试随实际被测职责迁移：

- 私有规则、故障与算法测试留实现包；公开 API 测试优先使用外部测试包。
- 单模块多个职责的公开组合测试可放 `<module>/tests`，跨 owner 黑盒仍放仓级 `acceptance`。
  不能把所有测试堆到新的 tests 包，也不能为测试公开事实状态。
- helper 留最小使用范围；静态数据进 testdata，测试 Go helper 不伪装成数据。
- 保留每条行为断言和反例；如果删除重复测试，说明剩余覆盖证据，不为降低文件数删测试。
- 并发优先用明确屏障；权限、取消、断连、重试、过期、回收和重启继续按原合同验证。
- 模块 README 只导航到本规范，不复制目录规则和门槛。

迁移测试时同时核对 fixture、相对路径、进程工作目录和独立消费者的构建入口。
公开组合测试不能只因缺少私有故障钩子就给生产 API 增加注入接口；
需要访问 Store 故障接缝的测试随实际提交／恢复实现归位，跨 owner 场景仍从公开入口验证。
全仓门必须实际覆盖全部新包，不能只把测试文件搬到不会执行的目录。

## 7. 用实际调用链检验方案

以下已对照 P6 源码做了**静态职责核对**，不是已编译的新接口或迁移验证：

| 链路 | P6 当前关键位置 | 新边界必须保留的行为 |
|---|---|---|
| Create／Update 元素 | Core core.go → convergence.go 的 executeMutationLocked → mutation.go → publishStateLocked | 准入顺序、操作冲突、挂载写门、唯一候选与裁决、原 cut／回执；Store 不确定时加载有效前后态，否则停用 owner，不能先返回成功 |
| 激活 Core 侧 Facet 关系 | Core attachment_control.go | 先保留原错误顺序和重放路径；需要接纳证据时解锁读取；回来重新授权、核对关系并在原锁下提交，不能把旧许可变永久许可 |
| Facet 内容写入 | Facet content.go／mutation.go 的 commitLocked／store_operations.go | 回查 Core 与 F0／视图约束、一次历史规则、提交后观察；不确定结果仍走原恢复和明确错误；P6 类型验证的业务修订须另行处理 |
| Realm 授权与凭据 | Realm realm.go、binding.go、credential.go、store_commit.go | 主体、授权、凭据仍共享一套适当的事务与生命周期；复制公开 handle 不得复制可写状态或锁 |

至少用一条“元素更新＋Store 不确定结果恢复”小切片验证拟议接线，
并同时验证涉及跨职责的删除／附着关系与公开读取，不能只验证一个纯数学包就宣布整体方案成立。
内部包的依赖 DAG、opaque token 归位和本地 API 映射是实施前的必交工程结果，由实现方完成；
如果需代码原型，应在得到相应实施授权后进行，不以文档推导冒充编译通过。

## 8. 验收与规范落地

规范已确认；源码与检查器按实际职责归位，不能把根目录清空或单包通过说成全部验收完成。
每个迁移切片须有“之前 → 之后”的声明、状态、包依赖与调用链映射，不能只附目录树。

实施交付的共同门：

1. 一个事实 owner、一套正式状态、一条最终提交／恢复路径，原有锁序和授权时点不变。
2. 原请求、结果、错误、精确字节、Store bucket／key／value／sequence、回执与历史重放一致。
   本地 Go 接口调整按单独确认的映射验证，不假称“公开接口完全不变”。
3. 没有旧包兼容层、完整镜像模型、无用途转发、空目录；新增适配逐一说明，根以外的大包也过审查。
4. 原有测试一一有去向；固定输入／身份的存储证据与故障反例通过，随机新建数据库不要求整文件字节相同。
5. 最后冻结文件集合后完整执行 `go run ./cmd/verify`；检查器、全仓 test／race／vet、依赖、协议生成、示例均通过。
   `-checks-only` 不代表完整验证。未运行、失败、旧基线证据分别说明。
6. 文档反映实际完成范围；接受、提交和推送仍需要相应授权，不自动恢复 P7。

**检查器落地状态（2026-09-13，已随 `35f9e6f3` 提交）：** 已增加按包、按生产／测试／注册生成物分类的规模检查，
以及 Core／Facet `contract` 与 `internal/token` 的精确依赖规则、真实外部 consumer 编译反例。
Core 修订值与服务证据已使用同一 token 定义；为保留证据解析和空间值校验的原错误身份，
`internal/failure` 仅定义既有 ErrInvalid／ErrUnrepresentable 两个 sentinel，只依赖标准库。
geometry 与 Core token 可引用这一叶子；它不反向导入值、规则或 owner，其他模块及通信／SDK 不得穿透。
这不是通用错误框架，也不自动放行其他 failure 子包或额外业务职责。
Core `internal/store` 已登记字节工具与现有四桶写事务准备边界：只允许标准库与
`shared/values`；现有 `go.etcd.io/bbolt` 仅放行 transaction.go 及其测试，纯字节工具不能因此访问数据库。
准备函数只接收原事务上下文与持久身份头，返回固定字节写口；不接收完整事实对象、
操作回调，也不重新裁决业务。原 owner 保留 db.Update、锁、最终提交、恢复与发布顺序；不为了拆包
先构造整份字节镜像。未登记的 store 子目录仍拒绝，其他模块与通信／SDK 不能穿透。
Facet 的 instance、namespace、access 及 request 叶子已按实际依赖逐包登记，旧 Facet 根源码／导入拒绝；
外部不得导入 namespace，通信生产代码只用窄端口、不得导入 instance。
Core 的 instance、实际业务职责和通信组也按具体消费方登记，不放行整棵 internal。
`transport/grpc/federationconn` 与 `transport/grpc/internal/wire` 是两个独立精确包：
前者持有原出站连接／有限联邦读取租约，后者持有唯一的共享编解码。
通信根与 federationconn 可以使用该 wire 包；wire 不能访问 SDK、连接 owner、通信根或业务 internal，
SDK 及其他模块也不能借 Go 的导入可见性穿透它。其他通信子目录均按第 5 节独立登记。
共享通信值直接使用实际 contract，旧 Core 根包导入已拒绝，没有临时旧路径许可。

Core `internal/commit/binding` 只登记原 Element 重试输入、规范化 public 请求、绑定比较与格式校验；
它同时供实时准入、Store 格式转换和重放使用，不持有可写事实、权限、锁、日志或数据库。
原 `publicExpectedRevision`／closure／MAC 准入仍留唯一提交 owner，不移入解析器；
未准入解析结果不能直接作为已验证操作。其下层 `internal/access/value` 仅持有现有字段形状与表示，
不保存上下文或授权。现有 Element/Path/Facet 联系记录的纯 JSON 投影也唯一归此处；
仅 `record.go` 及其测试可依赖原 `token.PublicSigner`，使用调用方传入的既有 signer，
不持有提交 owner、密钥管理、可写事实或注册表。逐文件登记，不给其他表示文件开放 token 依赖。
两包按精确路径、方向与外部不可访问性检查，禁止事实/value 反向导入 binding。
绑定片的历史验证不能替代最终工作区验证；具体声明映射、固定 Store 等价、
当前规模和最后完整验证要求统一在开发计划记录，不在规范中维护第二份施工状态。

`transport/grpc/mountconn` 同样只登记现有的指定地址出站适配，拥有原客户端索引及逐次 approval 副本，
不拥有 Server 会话／准入表。构造方直接使用该包，不保留旧根 alias；Server 只检查它是否为预期来源的
无 approval 装配实例。入站和出站共用既有 wire 叶子的 binding 形状校验，不复制授权或命令承诺规则。
Core `internal/element/fact` 已按精确路径登记：只有本模块事实、提交与恢复所需的消费者可引用；
事实规则依赖既有 contract、geometry、token 和 shared/values，不反向依赖 Core 根、instance、
Store、关系流程、Realm 或通信实现。外部 consumer、SDK、transport 及其他 owner 不能穿透；
事实表保持私有，查询投影只承接原来的有限读集，不携带操作／控制历史。
Core `internal/facet/fact` 只登记当前关系记录、确定性 Plan／Apply 和有限读取投影；
允许依赖标准库、shared/values、Core contract／token，不依赖 Element 事实、提交、Store、
Realm、Facet 内容或通信实现。元素存在性由原 owner 在原时点传入；操作回执、控制事件、
cut 签发和持久提交不在该事实包中，跨 owner、SDK 和 transport 不得穿透。
Core `internal/mount/fact` 只保存当前 outgoing／incoming 关系及有限读取、候选替换；
只依赖标准库、shared/values 与 contract。操作／地址回执、peer attempts、journal、cut 签发、
Element 协调、权限、Store 与恢复仍由原 owner 管理；事实包不能反向访问这些职责，
也不提供可写 map 或来源不明的修订签发。跨 owner、SDK 和 transport 不得穿透。
这两个关系 fact 包现与 Element 一样，由 State 嵌入唯一的只读 View：
View 只提供当前记录的值读取／迭代，不提供 Plan、Apply、Replace、Clone 或恢复方法。
读取这份 View 不是自行取得一致快照；持锁、不可变发布、有限证据校验与回收仍由原 owner 保证。
当前快照读集已不再持有 `coreState`：只保留 live View 与当前挂载所需的有限证据。
首次构造仍验证原 claims／tombstones，完成后释放历史输入；后续每次读取仍校验 live 空间事实，
不能以“已经是不可变快照”为由删除原有检查。完整事实和 live View 共用同一套确定性校验规则。
现有公开 Mount／Facet 控制值及受信服务结果直接归 contract，业务校验不随类型迁入，
不增加公开 Valid 方法；服务 cut 的证据解析也只改变定义位置，不能被当作来源认证。
Core 和 Facet 根生产与测试均已收紧为零文件／零行；最后旧消费者已迁出、无用途共享夹具已删除，
检查器拒绝根源码与旧根导入，不保留 alias、RPC 代理或测试入口。每个迁移区域只维持一套有效实现。
完整实现、transport／SDK 及其测试都必须按上述实际边界验收，不能仅凭根目录和检查器通过判断完成。

## 9. 参考与边界

[Go 项目组织说明](https://go.dev/doc/modules/layout)允许多包与私有实现；
它没有规定所有公开入口必须在根目录，也没有给出一个普适的文件数上限。
[Go 方法声明规则](https://go.dev/ref/spec#Method_declarations)要求接收者基类型属于同包；
因此不能把一个大对象的方法直接分散到多个目录而不改变设计。
[internal 导入规则](https://pkg.go.dev/cmd/go#hdr-Internal_Directories)只限制包的导入范围，
不是网络身份、权限或运行时安全边界。
[Go 接口审查建议](https://go.dev/wiki/CodeReviewComments#interfaces)强调先有真实使用场景，
接口一般由消费侧定义；本规范不把所有实现统一包装成接口作为默认目标。

本文给出 UCUPA 已确认的工程规则与目标归位，不把 Kubo、etcd、Pebble 的文件数或目录形态当作本项目正确性的证明。
最终判断依据是实际状态与依赖、调用成本、可定位性和完整验证，不是模仿某棵目录树。
