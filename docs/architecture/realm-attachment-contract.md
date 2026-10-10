# RealmAttachment 授权联系合同

**状态：S2P1 设计已由 S2P2 本地闭环与 S2P3a–d 网络消费／有限组合实现、验证并按授权验收提交；为该明确范围内的 current 增量合同。精确效力由架构权威清单声明，环境边界与 Baseline 整体接受状态见开发计划；不授权新代码、提交或部署变更。**

本文落实[主线 D7／D8](ucupa-concept-mainline-baseline.zh-CN.md#realm-授权访问链补充)，
只补齐 Core／Facet 如何持续联系其授权 Realm。身份、凭据与精确 PermissionBinding 仍由
[Realm 合同](README.md#4-realm-contracts)拥有；事实提交与生命周期仍由各自 owner 拥有。
阶段、进度与验证结果只见[开发计划第 6 节](development-plan.md#6-stage-2realm-授权访问链)。

**接替范围：** 本合同接替 P4／P5／P6 的本地授权依赖装配限制及运行时任意注入的接线方式；
身份、凭据、PermissionBinding 的原裁决和 P6 原调用者接力不变。联系属于 Core／Facet 原 Store，
Realm 网络服务只承载已有授权内核。S2P3 的证据为同机独立产品进程／真实 gRPC/TLS，
不能借用 P6 两机事实服务或 S2P2 双机工具链复测，宣称真实多机 Realm 或生产验收已完成。

## 1. 最小闭环与明确不做

首批 profile 是：一个 Core 或 Facet 逻辑 owner 使用一个明确的授权 Realm；多个事实 owner
可以各用不同 Realm，也可以显式使用同一个 Realm。共享 Realm 不代表共享权限或提交事务。

```text
调用者 → Core／Facet
            ├─ 从自己的 RealmAttachment 取得 RealmID + NextHop
            ├─ 核对目标并建立调用者身份，向该 Realm 检查精确要求
            └─ 校验本地事实与当前执行资格，再披露／提交
```

联系不是授权本身；Realm 不代理空间请求，也不替 Core／Facet 决定业务是否合法。
不新增全局 Realm、Ref 地址解析、授权复制、离线 Allow 缓存、自动切换 Realm 或多副本接管。
不引入 Element 级多 Realm、proposal／acceptance 状态机、RealmAuthorityRef、K8s 或统一管理模块。
首批只允许维护**同一 Realm 的地址**；更换授权 Realm、合并权限或迁移授权库不在本合同范围内。

## 2. 记录归属与唯一性

每个事实 owner 保存恰一份有效联系；所属 Core／Facet 的原始 Store 是该联系的唯一持久 owner。
内存 owner 使用相同规则但不承诺进程退出恢复。元素、内容、类型键和传入转移包不重复携带此记录。

| 语义字段 | 约束 |
|---|---|
| Owner | 当前 Core／Facet 的 FactAuthorityRef；必须与所在 Store 的真实 owner 精确一致 |
| Realm | 已有 RealmID，表示负责本 owner 授权判断的 Realm；本 profile 建立后不变 |
| NextHop | 唯一明确联系地址；复用现行 local／网络 profile，不从 RealmID 推导或解析地址 |
| ContactRevision | 所属 owner 签发的 opaque 比较依据；只比较相等，绑定 Owner 与联系代次，不与元素／内容 Revision 混用 |

这些是必要语义字段，不要求把 Store 头部已有 Owner 冗余存入每个编码对象。
初始建立、地址变更和保留回执都进入原 owner 的唯一事务／历史体系；不新建联系数据库或提交引擎。
联系代次不得因地址从 A→B→A 而复用；重启后原比较依据仍有明确效力，不靠内存计数器重置。

以下不进入 RealmAttachment：凭据 secret、PermissionBinding、Realm 授权私钥、授权 view、会话、
连接对象、DNS 结果和 Realm Store 路径。信任材料与运行资源配置由受控部署提供；它们不能覆盖记录的 RealmID。

授权 Realm 与 `PrincipalRef.Realm` 不是同一概念：后者标识主体的身份命名空间。
接收方不能按请求主体所属 Realm 改选授权源。外来主体仍须符合 P6 的显式身份信任和活性验证，
然后由本联系指定的 Realm 独立授权；联系存在不自动建立跨 Realm 信任。

## 3. 建立、打开与可信连接

### 3.1 明确初始化，不在普通启动中补建

创建事实 owner 时，受控初始化明确提供预期 RealmID、NextHop 和适配所需的信任配置。
先证明适配连接的是指定 Realm，再与事实 owner 初始状态原子保存联系；失败不能留下可对外服务的无联系 owner。
初始化属于既有可信引导，不接受普通业务请求自报 Realm，不自动创建主体或授予管理权限。
初始管理权限由现有 Realm 引导／管理流程显式配置，不能因建立 attachment 自动产生。

打开既有 Store 时，先验证 Store 身份、完整性、writer 资格和持久联系，再按记录连接。
配置可以声明预期 Realm 以检查误装配，但不提供高于持久记录的覆盖值；不匹配即拒绝启用。
缺记录、错 Owner、非法地址或损坏历史为 Store／配置错误；不以默认 Realm、任意注入 Authorizer 或重新 seed 修复。
普通服务启用前须成功核对授权依赖；离线检查或第 4 节的受控修复不能开放普通读写。

### 3.2 local 与网络使用同一联系语义

- 本地适配按指定 `local:` 地址取得可信装配对象并核对 RealmID；不得扫描实例寻找匹配 ID。
  这证明受控本地装配正确，不证明恶意对象只凭返回相同 ID 就可信。
- 网络采用现行 gRPC／Protobuf／TLS 原则：明确 NextHop、标准证书和 hostname 验证，另有受控
  `expected RealmID → server SPKI` 绑定；在发送凭据或授权正文前完成核验。
  RealmID 与 FactAuthorityRef 不互相转换，不能沿用只接受事实 Ref 的目标字段来冒充 Realm 身份。
  Realm 服务还须核对请求的预期 Realm 与自己打开的原始 Realm Store 身份。
- Core／Facet 到 Realm 的受信服务访问须绑定明确的服务身份与允许的事实 owner；网络采用 mTLS
  及受控绑定。不能仅因一个客户端知道 Realm 地址，就允许其自报用户或查询全部授权关系。
  这项服务接入资格不是用户对事实的权限，两者分别检查。
- 不跟随对端返回的地址，不降级，不把不可达解释为改用本地 Realm；DNS 只解析指定 host。
  同一稳定 NextHop 背后的进程／IP 变化不自动构成联系更新，重新连接仍核对身份。

信任、证书与凭据不能由普通请求传入替换。信任配置改变需受控重装配，不靠旧连接继续绕过撤销。
本 profile 不防御拥有同一 Realm Store 与服务私钥的恶意可写克隆，也不实现 Realm 单写交接或高可用。

## 4. 联系维护：读取与同目标地址更新

只增加两个 owner 控制动作的语义，不扩展为通用 Realm CRUD：

| 动作 | 输入／结果 | 必需权限 |
|---|---|---|
| 读取联系 | 当前 Owner → RealmID、NextHop、ContactRevision | `ucupa.core.realm-contact-read` 或 `ucupa.facet.realm-contact-read` + exact AuthorityScope(Owner) |
| 更新地址／恢复原结果 | 原 OperationID、Owner、expected ContactRevision、原 RealmID、新 NextHop → 原裁决及联系比较依据 | `ucupa.core.realm-contact-update` 或 `ucupa.facet.realm-contact-update` + exact AuthorityScope(Owner) |

权限归所属 owner 定义，Realm 仍仅比较精确要求。普通元素读写、writer 管理或 Realm binding 管理权限
不隐含授予联系维护权。联系信息不随普通 Element／Payload 读取披露。

地址更新先捕获原联系并校验新 NextHop 的形状，按以下两种路径之一取得**原 Realm** 的当前维护授权：

- **正常维护：** 当前联系仍可核验时，通过当前地址认证原 Realm 并检查维护者的精确权限。
  新地址不必已经在线；保存地址不等于证明端点可用。首次使用新地址仍须按第 3.2 节认证目标，
  失败只保持不可用，不向错误目标发送凭据，也不自动退回旧地址。
- **失联维修：** 当前联系无法完成核验时，可使用本次明确提交的新地址联系原 Realm。
  必须先认证候选仍代表持久记录中的 Realm，再向其提交身份证明、建立维护者身份并取得当前授权。
  仅凭相同 RealmID、自报 Allow 或旧授权缓存不能维修；候选也不可验证则不改联系。

两条路径共用同一个更新命令与本地提交规则，不增加备用地址列表或业务调用的自动寻路。
已核验的明确拒权不能改走另一条路径寻找 Allow。候选地址只为显式维修取得授权，
提交前不成为普通请求的授权源；本合同仍不允许更换 RealmID。
授权后在本地原事务内检查 writer gate、Owner、RealmID 和 contact CAS，原子提交联系与回执。
成功回执只证明联系已持久更新，不证明新地址可达；不得因新端点尚未上线把已提交结果改写成未执行。

失联维修不要求旧连接参与批准，但不绕过原 Realm 的授权。
若普通服务无法启用，受控维护入口独占打开**原始** Store，仅执行同一更新规则，不能同时运行第二个 writer。
不能依靠失效的旧业务会话维修。原 Realm 已不可认证或拒绝维护者时，
修复失败并保持不可用，不提供 emergency Allow、替换授权库或编辑数据库的产品后门。

同 actor／OperationID／规范请求重试返回原裁决；不同绑定冲突，不自动换 ID 或更新 CAS。
返回保留回执仍需原 Realm 的当前维护权限；正常通过当前联系核验，离线维修则通过已认证的候选联系核验。
授权后优先查询原结果，不要求已被后续更新取代的历史地址仍可达，也不重新执行旧地址更新。
只有尚未裁决的新更新才继续完成对应路径的必要核验、联系 CAS 和写入。
崩溃后联系与其回执必须同时为前态或后态；响应丢失按原请求恢复，
回执记录当时的结果，不冒充后续当前联系。原地址相同的有效更新可为 unchanged，不推进代次。
畸形输入、失联维修无法认证候选、授权依赖不可用不伪造持久 DENIED；已受理裁决按原 owner 的保留结果规则收敛。
历史校验使用已提交的决定及其绑定，不在重放时重新连接 Realm 用今天的权限重判昨天的提交。
容量、代次溢出等明确拒绝，不能删回执或回退代次来腾出成功路径。

纯地址更新不改变 RealmID、AuthorityRef、空间／内容 Revision、Facet 关系或 writer epoch。
它推进联系代次，但不直接撤销 caller、授权 view 或会话；已持久完成的业务结果不被撤销。
联系变更后的继续使用须证明连续性，而非仅证明新端点仍自报同一个 RealmID：

- 通过当前指定地址重新认证原 Realm，核验**原 caller establishment 和原 view**，并确认接收 owner、
  精确要求集合、本地事实依据及相关权限代次仍有效，才可在当前联系代次下继续原上下文／续页／观察。
  不把新建立 caller 或新取得的 Allow 填进旧 token；有效期不延长，不另存一份授权。
- 暂时无法核验返回 UNAVAILABLE，本次不得披露／提交；不因这一次断连自动销毁原上下文，
  但其原有到期、容量回收和释放规则继续生效，之后只有重新核验成功才能继续。
- 已确认 caller 失效、Realm 重启、相关权限撤销／重授或依据失效，按原规则拒绝或返回
  INVALID_CONTEXT／GAP，并在需要时重新建立身份与读取依据，不能为了续接而忽略不连续。

在途执行 guard 仍须检查联系代次。若最终本地检查时已变，不能直接消费旧核验结果；
允许在原请求期限与有界尝试内完成上述重验并重新检查本地依赖，否则本次停止，不强行重写旧比较依据。
这只收敛 Realm 授权联系的连续性，不放宽 Facet 关系、空间快照或其他已有 token 的失效条件。

## 5. 身份、授权与最终业务裁决

1. 请求先进入指定 Core／Facet；由该 owner 已验证的 Realm 联系建立 caller。
   普通请求不能提供另一 Realm／地址来决定这次授权。主体声明本身不是身份证明。
2. owner 根据当前操作与事实边界派生完整 PermissionID／Scope 集合，交给指定 Realm。
   Realm 原子检查相关授权并返回现有的 decision／opaque view；无关 binding 不使本次 view 失效。
3. owner 在最终读／写边界校验授权连续性、联系代次、caller 有效性及本地事实；通过后才披露／提交。
   Realm 不判祖先关系、挂载合法性、Facet lineage 或 payload，也不保存第二份空间／内容事实。

现有消费者不能只接一个 `bool Authorize` 并把远端错误压成 false。
公共链必须保留 VALID、CHANGED、DENIED、UNAVAILABLE；已确认身份无效与无法校验身份也分别表达。
授权拒绝不披露对象存在性；联邦使用的 denial view 仍只证明某项要求当前被拒绝，验证成功不代表 Allow。
撤销后重授即使最终 Allow 相同，旧 view 仍失效；断连不延长 view、session、delegation 或查询依据的寿命。

### 5.1 本地端口不能直接当远程协议

现有 CallerContext、AuthorizationView／DenialView 是 Realm 运行内受信端口值，不能把它们的内存字段
序列化成远程身份证明。S2P2 先落实本地联系；S2P3 通过服务专用、调用者及接收 owner 绑定的有限远程
句柄衔接 Realm 内的 caller／view，再由本地适配供事实 owner 消费。句柄不是离线授权，不向最终客户端披露。
不得复制 Realm view 密钥／binding 到 Core／Facet，也不以客户端自报 Principal 调用 TrustLocalCaller。

P6 的终端 session、受限委托、issuer challenge、身份活性与目标独立授权全部保留。
同 Realm 的直接凭据接入沿用原 audience：接收服务属于明示的可信凭据处理边界，可把凭据送到
已认证的指定 Realm 建立身份，但不保存 secret，不向另一个 Core／Facet、日志或回执转发。
跨 Realm 身份继续走既有显式签发信任；不把 foreign Principal 的 secret 送给本地授权 Realm 碰运气。
新增 Realm 网络服务的通信身份只保护服务间调用，不把服务权限冒充用户权限。

远程句柄数量／期限、请求 deadline 和消息容量必须显式有界；沿用 P6 资源与断连原则，
不新增自动续期或断网宽限期。Realm 重启使其旧运行内证据失效；重新连接不恢复旧会话或续读证明。

### 5.2 网络等待与裁决时点

当前 Core 的 `internal/access/admission.go` 明确要求锁内 ValidateView 是有界本地工作；
Facet 也有 capture／解锁回查／recheck 的既有边界。S2P3 不能把网络 RPC 塞进这些锁内调用。

远端组合必须采用有界的捕获／核验／本地重查过程：

1. 在本地捕获本次精确命令／读取范围、caller、联系代次、writer epoch 和所依赖的事实依据。
2. 不持有事实提交锁等待 Realm；通过已认证联系建立／检查 view，并完成本次当前授权核验。
3. 回到本地唯一裁决边界，只做本地检查，确认该核验属于同一次请求、同一要求集合、当前已核验的联系代次／运行，
   且业务依赖仍成立。联系变化按第 4 节重验；事实或权限要求变化则失败／重新核验，不能把旧 Allow 套在新范围上。
4. 仅本次在途请求可消费该结果；不持久保存为许可、不跨下一页／重试／另一个操作复用。
   deadline、取消、联系变更或 writer 闭门均不能因持有结果而失效。

这不建立 Core／Facet 与 Realm 的分布式原子事务。授权时点是 Realm 对本次最终核验的裁决点：
先完成撤权再核验必须拒绝；在核验之后并发发生的撤权不撤销已进入执行的这一次操作，
后续请求必须重新核验。读取／观察的连续 view 仍须发现相关授权代次变化，不能用新的 Allow 续接旧依据。
已经提交后丢失响应不证明未提交；原结果恢复仍需当前权限，不因 Realm 后来不可达回滚事实。

此过程已由 S2P3 的调用内 guard、远程句柄和 typed RPC 落实到现有消费者，包含 mutation、
outcome recovery、分页、观察、denial view 及 P6 身份接力；本地 proof 仍不直接充当 wire 值。
后续变更不能只跑通一个 Get 就宣布原授权面已贯通。若无法保持上述语义，停在该边界复核，
不改为锁内无限等待、周期缓存 Allow、全局撤销锁或新的授权模型。

### 5.3 S2P3 的网络职责与有限句柄

Realm 服务承载原 Realm Store 与授权内核，不维护另一份 grants／credentials 镜像。
本轮公开网络面只覆盖现有身份建立与授权消费，不增加远程主体、凭据或 grants 管理接口。
协议使用 typed Protobuf；以下按职责分组，不要求每组独立成包，也不在计划中冻结 RPC 数量：

| 职责 | 输入与输出边界 |
|---|---|
| 目标核对 | 按配置的 RealmID、服务证书绑定与指定 NextHop 核对实际 Realm；核对完成前不发送用户凭据 |
| 身份建立／派生 | 接收原 audience 下的凭据，或既有 P6 的签发证据、challenge 与显式信任；返回经核验主体与有限 caller handle。派生沿用原身份资格，不凭自报 Principal 建立 caller |
| 当前授权／连续 view | 按原 caller handle 和精确 Requirement 集合核验；返回原四态结果及需要的有限 view handle。旧 view 的核验不能被新 Allow 替代 |
| 拒绝依据 | 建立／核验 denial view handle，保持“当前仍拒绝”与“当前允许”不同，不将 denial 核验成功解释为授权 |
| 释放 | 显式释放不再使用的运行内句柄；过期最终回收。释放失败不产生离线权限，也不要求无限重试 |

所有受保护服务请求绑定经 TLS 认证并获准接入的接收 owner，目标 RealmID 来自 owner 的当前持久联系，
不是终端请求字段所选的授权源。RealmID 与 FactAuthorityRef 保持不同类型，不以强转类型复用目标认证；
可复用已有 TLS／gRPC 有界传输机制，但不得另建 Ref 路由或自动换源。
首次创建时，目标核对只确认 Realm 身份，不建立 caller／view，不读取 grants 或注册新 owner。
后续身份／授权消费必须具备已生成 owner 与服务证书的显式可信绑定；缺绑定拒绝服务，
不能因通过目标核对就自动获准接入。可信引导与正常请求分离，不借创建扩展远程管理面。

P6 foreign 接力的 Actor 是接收 Core 已经通过服务 TLS 核验的发起 Core 身份，
并非接收 Core 自身的 owner。`EstablishForeignCaller` 显式携带该 Actor，原签名必须与它精确匹配；
接收服务 owner 仍由 Realm 的 mTLS 接入绑定确定，challenge 与返回句柄也仍绑定该 owner。
不能用接收 owner 替代签名 Actor，也不能仅凭请求中的 Actor／Principal 建立身份。
签发者信任、在线活性、单次 challenge 和等待后期限核验保持原规则。

句柄是不可猜测的运行内引用，唯一注册表位于 Realm 服务接入层，关联 RealmID、Realm 本次运行、
接收 owner、原 caller 及对应 view 的要求集合。不得向终端客户端披露，不能跨 owner／caller／运行复用。
caller、view、denial view 的本地 proof 字段不成为 wire 格式；Core／Facet 的适配不获得 Realm 签名密钥。
句柄只引用 Realm 内核证据，不缓存可跨请求兑现的 Allow；每次操作仍完成第 5.2 节的当前在线核验。

句柄 TTL 不超过其父身份／P6 证据的有效期；派生不刷新原证据寿命。数量、TTL、RPC deadline、
消息及要求集合容量均须在实际接口中有有限上界，并验证临界值、过期与容量拒绝；禁止零值代表无限。
同 Realm 临时断连不会自行注销仍有效的原句柄，联系维修后只可核验同一原 caller／view；
重启、失效或撤销重授不能通过重新建立 caller 来伪装旧连续性。

跨 Realm 身份仍须核验原签发证据、challenge、显式 issuer 信任与在线活性。
不得仅因 Core／Facet 自称已经验证 foreign Principal，就在远端 Realm 调用 TrustLocalCaller。
签发者网络回查也不得持有 Realm 状态／提交锁；完成后本地重查相关依据，不形成新的分布式事务。
服务通信身份不能代替原用户身份，P6 原调用者接力和各目标独立授权不变。

本地与网络是明确配置的两种联系 profile，不是互为失败回退。联系的创建、重开、在线维护、
原 Realm 离线维修和原结果恢复均须覆盖网络 profile；引入网络地址时，逐项核对历史决定的重放规则。
若新规则会改变旧持久拒绝／结果，更新对应唯一 Store 标记并拒绝旧格式，不能只以字段布局未变为由保持标记。

## 6. 恢复、交接与转移

| 情形 | 唯一行为 |
|---|---|
| Core／Facet 正常重启 | 恢复原联系及回执，按其地址核对原 Realm；重建运行内 caller／连接，不自动重 seed |
| Realm 暂时不可达 | 保留联系与事实，受保护调用明确不可用；恢复同一 Realm 后重新核验，不改成 Denied／NotFound |
| Realm 重启 | 稳定 RealmID 不变，但旧 caller／view／远程句柄失效；重新建立依据，不能续用旧观察 |
| 同 owner 更换承载／P7.1 handoff | 原 Store 内联系跟随 owner；检查原 Realm 与 writer 资格，不以机器配置替换。已关闭 epoch 不允许顺带更新联系 |
| NextHop 地址维护 | 按第 4 节独立 CAS 提交；不改变事实归属和权限，原上下文经当前联系重验连续后可继续，旧在途结果不能未经重验越过新代次 |
| P7.2 Element／闭包转给新 Core | 目标使用自己的 RealmAttachment；转移包不导入源联系、源 grants 或 caller；目标按新 owner-qualified scope 检查 |
| P7.3 内容移到新 Facet | 目标 Facet 的联系和授权独立；旧 Realm Allow 不替代目标批准，不移走源 owner 的联系 |
| 复制 Store／旧备份恢复 | 本合同不新增同身份副本上线资格；原单写、禁止回滚与恢复限制保持 |

S2P2 已删除 writer 的任意 `AuthorizationBinding` 标签；writer 的授权身份依据是原 Store 的 RealmAttachment。
S2P3 继续使用这份唯一依据，不恢复旧标签、不以启动配置覆盖持久联系；地址变化不被误判为更换授权 owner。
交接过程中若必须维修联系，只能在原合同允许的可写阶段完成，不能借维修绕过 closed epoch。

旧 Store 缺少必需联系或其历史规则无法唯一重放时，在实现切片更新相应私有 FormatMarker，
明确拒绝旧格式；不靠运行时注入补字段，不加转换器、兼容读者或双规则重放。
是否需要更新 Realm Store 标记由实际 Realm 持久语义决定，不能因为 Core／Facet 新增联系就机械换标记。

## 7. 失败与隐私

| 已证明的结果 | 对外含义 |
|---|---|
| Realm 已认证，当前 caller／grant 不允许 | 按原边界 UNAUTHENTICATED／DENIED，不披露资源存在性 |
| 当前允许但旧相关 view 已变 | 原 INVALID_CONTEXT／GAP 等连续性失败，不能悄悄换新 view |
| 缺联系、错 Realm、错证书／pin、断连或无法核验 | 启动失败或 UNAVAILABLE，不当作确认拒权、空结果或 NOT_FOUND |
| 联系 CAS／OperationID 绑定冲突 | 原 owner 的相应冲突，不自动采用另一个地址 |
| 容量／deadline／取消 | 保留原有边界错误；未得到回执不推断业务是否提交 |

联系地址、Realm 内部修订、凭据和授权详情不进入普通业务错误。
受控诊断可定位“配置／信任／连接／授权／事实裁决”的失败层次，但不输出 secret、可兑现句柄或隐藏关系图。

## 8. 实现分工与验收

S2P2 只实现 local 联系闭环；S2P3 实现独立 Realm 服务及现有 Core／Facet/P6 消费链的网络组合。
两者使用同一持久联系语义，不先写一个可丢弃的本地模型再替换成另一个网络模型。
Realm 本身不导入 Core／Facet 内部包；连接适配不拥有 grants／事实；SDK 不替 owner 裁决。
持久联系进入既有 Core commit／Facet namespace 提交域，公开值／端口放对应 contract，
实际网络职责按 ENGINEERING 归位；本合同不预建目录、不启动 Realm 全量目录规范化。

| 最小反例／证据 | 所属门 |
|---|---|
| 正常 local 身份建立、授权、Core／Facet 读写；相同要求的内存／durable 结果一致 | S2P2 |
| 错 Realm、错 owner、缺／损坏记录、配置覆盖、请求自选 Realm 均不放行 | S2P2 |
| 重启保留联系；地址 CAS 并发、A→B→A、原 ID 重试／乱序、崩溃前后与响应丢失收敛 | S2P2 |
| 当前联系能授权时，新地址未上线也可提交；回执／重启保持新地址，实际连接未认证前不得发送凭据或服务，不回退旧地址 | S2P2；S2P3 增加 TLS 反例 |
| 旧地址离线时仅原 Realm 的合法维护者可经新地址维修；两条路径均无法核验或维修候选冒充时不改记录，明确拒权不能换路径找 Allow | S2P2；S2P3 增加 TLS 反例 |
| 纯地址变化不改事实 Revision／epoch；同一原 caller／view 经新联系重验连续后可续页／观察，在途旧结果未经重验不得提交或披露 | S2P2；S2P3 增加网络交错 |
| 地址变化叠加 Realm 重启、撤权／重授、过期时不能续接；临时断连不等于上下文永久失效，也不产生宽限期或离线 Allow | S2P2；S2P3 增加网络反例 |
| 同 owner handoff 保留 Realm，旧 writer 拒写；跨 owner 转移不导入联系／权限 | S2P2 |
| Realm 独立进程、独立 Store；正确 gRPC/TLS 目标；错误 pin 不发送凭据；错受众／错服务绑定拒绝 | S2P3 |
| caller／view／denial view 不作为本地字段上网；远程 handle 不跨 caller、服务 owner 或运行复用，单次当前核验结果不跨请求兑现 | S2P3 |
| 明确屏障验证撤权前后、撤销重授 ABA、Realm 重启；有限会话不能离线放行 | S2P3 |
| 慢／断连 Realm 不占住事实提交锁；重获锁时本地事实／联系／epoch 已变则不能直接提交或披露旧结果，只能按原规则重验或停止 | S2P3 |
| Core 与 Facet 各用独立 Realm 的真实服务链，另验显式共享 Realm；P6 原调用者接力与双方独立授权不倒退 | S2P3 |
| 中途断连、取消、已提交响应丢失、重连与原结果恢复；权限未复核不披露旧回执 | S2P3 |

这是有限矩阵，不要求穷举全部拓扑／故障组合或重开 P6 的无场景环境补测。
进程边界验收采用真实 gRPC/TLS 与独立 Store；同机证据不宣称真实多机／生产能力。
每个代码切片仍须对最终集合运行完整 `go run ./cmd/verify`，分别记录实现、验证、用户接受及提交。
S2P1 只做合同、引用与工程一致性检查，不能以文档通过代证运行行为。
