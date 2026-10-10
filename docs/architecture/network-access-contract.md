# P6 网络接入与开放互联合同

**状态：P6 已按声明的实现与实测范围交付，用户决定停止当前后续环境补测。第 1–7 节定义基础网络接入，第 8.1–8.6 节定义并落实受限委托、身份信任、Facet／挂载控制及公共联邦协议。最终交付证据和未测边界见开发计划，不以合同文本代替测试。采用 gRPC＋Protobuf＋TLS；已有两机实测不代证未覆盖的环境条件。**

**Stage 2 successor notice：** [RealmAttachment 合同](realm-attachment-contract.md)已接替本文
第 3、8.1 节中“授权依赖仅本地装配、不实现 RealmAttachment”的切片限制：接收 owner 从原 Store
联系指定 Realm，local／network 明确配置且不能失败回退。P6 原调用者、Actor、签发者信任、
目标身份、有限会话及独立授权规则不变。该网络 Realm 增量的有限产品组合验证与 P6 两机证据
分别记录，不相互代证真实多机 Realm 或生产验收。

本文落实[主线 D6–D11](ucupa-concept-mainline-baseline.zh-CN.md)的真实通信边界，
沿用[P5 公共行为](public-access-and-observation.md)、[挂载控制与查询](core-mount-contract.md)
和[Facet 真相闭环](facet-local-truth-and-core-lifecycle.md)的 owner、提交与权限规则。
阶段顺序及交付状态只见[开发计划](development-plan.md)；本文定义网络增量，不另建路由或授权模型。

## 1. 环境与证明范围

首个可实施切片是：独立调用进程通过 gRPC／HTTP/2／TLS 访问持久 Core 的 P5 公共操作。
Core 服务与调用方不共享指针、CallerContext、Store、锁或测试内存；双方只交换公开字节。
本机使用独立监听端口，后续换成其他主机地址时复用同一协议与程序，不增加另一套远端 API。
网络方法与结构化消息使用 Protobuf，不提供 JSON/HTTP、REST gateway、gRPC-Web 或协议 fallback。
选择 gRPC 是复用成熟 RPC、生成客户端和连接管理能力，不宣称已证明性能比另一方案更高。

完整 P6 还包含 Facet 双向网络链、挂载服务控制消息、公共嵌套联邦及独立实现互操作；
这些不能由首个 Core endpoint 代证，第 8 节列出其必须补齐的具体边界。
同机多进程是可执行的验证环境之一，不缩减 P6 的目标，也不成为空间模型限制。

| 验证环境 | 可以证明 | 不可以代证 |
|---|---|---|
| gRPC handler／TLS 定向测试 | 消息、防御性输入、身份与错误映射 | 独立进程、持久化恢复或跨机器 |
| 同机独立进程＋TCP/TLS＋独立 Store | 实际序列化、认证、进程隔离、断连、退出和恢复 | 真实主机网络、防火墙、机器时钟差异 |
| 两个真实主机上的相同程序 | 声明环境的地址可达性、证书部署、独立重启及故障表现 | 任意网络、规模、高可用 |
| 独立实现共同通过 corpus | 声明公共语义的互操作 | 仅两个相同 Go 进程或独立客户端就等于两个实现 |

没有跨机器环境时，不伪造通过记录；完成本地网络切片后可以继续其余可本地验证的 P6 工作。
真实跨机器验收须单独记录；部分实测不等于任意运行环境通过。当前 P6 工作停止在已记录
证据处，未测场景不作为无条件的建设队列；具体进度与后续按需边界见开发计划。

## 2. NextHop 与目标认证

### 2.1 一个明确的网络地址 profile

`NextHop` 的网络实现同时扩展代码、共同值定义及相关 schema，增加明确的网络地址形态，
而不是在 transport 里把一个 local 名称偷偷映射成远端 URL。

- 网络 NextHop 固定为 `https://host:port`：端口显式，范围 1–65535，无前导零；无路径、查询、fragment、userinfo。
  这里的 `https` 表示 TLS 保护的 HTTP/2 服务入口，本合同唯一应用协议为原生 gRPC，不表示提供 JSON/HTTP API。
  适配器解析并校验这一地址后构造固定 host:port 的 gRPC target；不能将原始 URI 直接交给默认 resolver 猜含义。
- host 使用小写 ASCII DNS 名、规范 IPv4 或方括号中的规范 IPv6；不接受百分号编码、zone、空白或尾点。
  精确规范校验与正反例随共同 schema／值测试落实，不通过自动清洗输入改变持久联系。
- `local:` 继续表示实际支持的进程内适配，不是旧协议兼容；网络端拒绝 local 输入，不能回退到本地连接表。
- 地址始终来自入口或精确关系中指定的联系；DNS 只解析这个 host，不按 FactAuthorityRef 查地址。
- 客户端固定连接该 host:port；禁用环境代理与 resolver 提供的 service config，不接入 xDS、SRV 服务发现、
  自定义 Ref resolver、备用服务或由对端下发的负载均衡/重试策略。同一 DNS host 的 IP 解析不是新增 UCUPA 路由。
  禁止从 HTTP 重定向、gRPC metadata 或错误消息学习新联系；没有 h2/TLS 就失败，不降级明文或 JSON。
  地址更换仍由原关系 owner 经授权/CAS/OperationID 提交，网络库不能替它更新。

### 2.2 地址、密钥与事实身份分别核对

首个 profile 采用 gRPC transport credentials、TLS 1.3、ALPN `h2`、标准证书链及 hostname 验证，并附加受信任的
`expected FactAuthorityRef → server SPKI SHA-256` 绑定。该绑定仅说明“哪个密钥可以代表预期目标”，
不含地址，不是 Ref 路由表。首次信任由调用方受控配置提供，不从 SpaceReference/trustHint 自动信任。
测试使用临时测试 CA 和每个 owner 独立的服务密钥；不依赖全网 CA、在线证书颁发平台或 K8s。

客户端在发送凭据、会话或业务正文前完成 TLS 和预期 owner 的密钥绑定验证；仅看响应中的 Ref 不够。
每个 RPC 另带唯一的 `ucupa-target` metadata 值，接收方与自己已打开 Store 的真实 AuthorityRef 精确比较；
缺失/重复/非法 metadata 拒绝，不能用任意 `:authority` 覆盖目标身份、证书 hostname 或 owner。
宿主启动时配置声明的 owner 与 Store 不符就拒绝监听，不自动创建/替换 Store 来凑齐身份。
不同事实 owner 不共用本 profile 的服务密钥；进程换机器不改变 AuthorityRef，改密钥须显式更新信任材料。

TLS 由服务进程终止；首个 profile 不支持代理代报身份或根据 `X-Forwarded-*` 建立信任。
不使用 `InsecureSkipVerify`，不把证书过期当作暂时放行；连接重建仍核对身份。
信任配置本次运行固定，改变它需要重新装配，不能靠保留旧连接绕过撤销。
同一密钥/Store 的恶意可写克隆不在本切片的安全承诺内，唯一 writer 仍受原 owner 规则约束。

## 3. 请求身份与有限会话

网络认证复用 Realm 已有的 `CredentialEvidence` 和 `EstablishCaller`，不把
`TrustLocalCaller(客户端自报 Principal)` 当网络认证，不把本地 CallerContext／view 直接序列化。
授权依赖仍在接收方本地明确装配；不实现 Stage 2 RealmAttachment 或跨 Realm 自动信任。

每次 EstablishCaller 都生成新的 establishment，故不能每个业务请求重建一次再假装旧 handle 仍有效。
采用接收服务运行内的有限会话，维持同一次已认证 CallerContext：

1. `SessionService.EstablishSession` 是 unary RPC，接收 typed message：`audience:string`、
   `credential_id:bytes`、`secret:bytes`。ID/secret 分别精确为 16/32 bytes，沿用原 Realm 非零与 audience 校验。
   不接受 Principal、Realm 地址或客户端自报授权。请求消息最大 4 KiB，只经已验证的 TLS 发送。
2. 宿主适配将证据交给已装配 Realm；校验成功后生成随机 32-byte 的不透明 session，
   返回 typed response 的 `session:bytes` 与 `expires_in_seconds:uint64`；后者必须为正。
   不返回 secret、CallerContext、Proof、view 或私有 Realm 状态。
3. 会话只保存随机 token 的 SHA-256 索引、原 CallerContext 和本次运行的绝对到期依据，不保存凭据 secret。
   期限从创建起固定；请求、轮询及重试不自动延长，不提供 refresh 或跨重启恢复。
4. 后续 RPC 使用唯一的 `authorization` metadata：`Bearer <session 的严格无 padding base64url>`。
   session 解码长度必须为 32 bytes；只有 metadata 使用该文本表达，不将整个 Protobuf 消息 JSON 化。
   该 bearer 只在此服务有效，不向下一跳转发；gRPC channel 已连通不等于 caller 已认证。
   owner 仍每次执行原 AuthorizeView／ValidateView，因而撤销凭据或授权不能等到 session 到期才生效。
   认证状态无法确认时不放行；错误不披露凭据、主体或资源是否存在。
5. `SessionService.ReleaseSession` 使用同一 bearer 和空请求释放本地会话；形状合法但未知/已释放 token
   返回相同的空成功结果，不形成 token 存在性接口；不增加匿名业务调用的豁免。
   身份相关的 owner 派生资源按原 TTL 回收，不删除真相或回执；不要求 transport 枚举私有句柄。
6. 失联后 session 仍有效可重新连接继续原读取链；session 过期或宿主/Realm 重启后重新认证、Enter、Read。
   不把新会话套进旧句柄；观察旧依据失效需要新 prepared Read，不能从最新状态偷偷续接。

首个 profile 的 session 是受保护的 bearer，不承诺被窃取后的防重放或设备绑定。
它只减少反复传送长效凭据，不替代最终授权；秘密只从受控输入读取，不进入 CLI 参数、示例提交、日志或异常正文。
不提供 cookie、浏览器自动登录或开放 CORS，避免凭据被自动附加到跨站请求。

## 4. RPC、消息与失败语义

### 4.1 明确方法，不提供 JSON 隧道

首个 Core 网络 service 名为 `ucupa.core.CoreService`，会话 service 为 `ucupa.network.SessionService`；
后者由每个事实服务本地实现，不是独立认证服务或新的 Realm。所有首轮方法均为 unary：

| RPC | 对应已接受的公共操作 |
|---|---|
| Enter / Close | `enter` / `close` |
| ReadElement / ListDirect / QueryRegion | `read` / `list` / `query` |
| BeginObserve / PollChanges / ReleaseObserve | `begin-observe` / `poll` / `release-observe` |
| CreateAnchor / CreateEntity | `create-anchor` / `create-entity` |
| UpdateAnchor / UpdateEntity | `update-anchor` / `update-entity` |
| DeleteAnchor / DeleteEntity / RecoverOutcome | `delete-anchor` / `delete-entity` / `recover-outcome` |

每个方法有独立 request/response message，字段继承 P5 对应操作的必需性、范围和语义，不新增泛化 CRUD。
不得提供 `Call(bytes json)`、`google.protobuf.Struct/Any` 或自定义 JSON codec 作为公共主入口。
Facet 内容与服务控制方法在其切片按同一规则定义，不借当前 `.proto` 提前暴露未经授权的控制接口。
不因 gRPC 支持 streaming 就自动将 Observe 改成推送；首轮仍保留 prepared Read→Begin→Poll、cursor 和 GAP。
将来若需要流式传输，须证明背压、重连、逐批授权及原有缺口规则，stream 本身不能代证连续性。

### 4.2 Protobuf 表达与唯一语义

`.proto` 使用 proto3 和显式 presence；它是网络字段/编号的唯一源，不手写多套网络绑定。
共享网络值与 session 定义归 `shared/api/`，Core/Facet 方法归各自 `api/`；依赖方向为 owner proto → shared proto。
具体字段编号、所有消息与正反例在代码切片生成前落实，不能用本节的方法表代替完整可编译协议。

| 语义 | 网络表示与校验 |
|---|---|
| UUID、RealmID、PrincipalID、NextHop | 明确 string 字段，沿用各自已接受校验，不把名字/地址互相转换 |
| OperationID、摘要、token value、session | bytes，保留原长度与角色约束；token owner/kind 仍明确，不使用私有序号 |
| 查询 UpDepth / 路径 up | 显式存在的 sint64 / uint64；仅 UpDepth 可为 -1，不改变零/缺省/上界规则 |
| Pose、Range、查询球 | 结构化 message、有限 double；保留原数值、范围与相交规则，不采用 float32 降精度 |
| 必填/可选字段、互斥结果 | message/optional/oneof 表达 presence；必填缺失拒绝，不能用 Protobuf 零值自动补业务输入 |
| Facet TypeRef 和 exact payload | TypeRef 按固定 32-byte 不透明值传输，Payload bytes 原样传输；Facet 不解析类型制品或校验内容格式，不转换成 Struct 或重编码内容 |

数值零按原 JCS 语义处理，不能让 Protobuf 的负零位模式产生另一项业务请求；NaN/Infinity 拒绝。
protobuf 解码后递归拒绝未知字段、未定义枚举、必需 oneof 缺失及角色/长度错误，不以忽略未知内容保留历史兼容。
不强制“重编码字节必须等于输入”，也不把 deterministic protobuf serialization 当规范化。
Protobuf 字段顺序/编码差异在解码为同一合法值时不产生不同业务请求。

**原 OperationID 绑定、Facet digest 与公共 Revision 语义不变。** 服务端把已校验 typed request
唯一映射到 P5 的规范语义输入；首个实现可通过确定性字段映射构造 canonical JSON 调用现有 PublicCall，
这一操作 envelope 的 JSON 只在服务端内部存在，不作为整包 gRPC payload 或另一个网络 endpoint。
Facet 内容本身的 exact JSON bytes 仍按上表原样承载；这不是用 JSON 表达整个 RPC。适配只表达消息，不裁决业务。
可选字段的省略与显式值按 P5 精确保留；RecoverOutcome 使用 typed 原命令（不含过期 handle），
通过同一映射产生原规范请求，不把原始 Protobuf bytes、gRPC metadata 或新的 session 纳入操作身份。
response 同样转成 typed value；原切面/角色/权限校验不因绑定代码生成而省略。

现有本地 PublicCall 与 JCS schema 继续描述实际支持的本地表示，不是待部署的旧 HTTP 服务。
proto 不复制领域规则；跨表示 conformance 证明等值输入得到相同裁决/回执，维护一份语义而非两个版本。
若编码/解码成本需要优化，应以实测为据收敛共同值层，不能为宣称二进制性能而重写提交规则。

### 4.3 领域结果与 gRPC status 分离

每个业务 response 明确 `oneof { result, failure }`，必须恰好有一个分支；failure 的枚举精确保留现有
PublicError 含义。有效 owner 结果使用 gRPC `OK`，不把网络错误推断成持久回执。

| 结果来源 | gRPC 表示 | 语义 |
|---|---|---|
| owner 成功 | OK + typed result | 必须完整接收、校验结果和最终 status；headers 不是提交证明 |
| owner 明确 PublicError | OK + typed failure | 保留 DENIED/NOT_FOUND/CONFLICT/GAP/INDETERMINATE 等，不压成通用 INTERNAL |
| 缺失/非法/过期会话或认证未通过 | UNAUTHENTICATED | 不表示资源存在或不存在，不替代 owner 的权限拒绝 |
| 消息字段/metadata 形状不符合 | INVALID_ARGUMENT | 无 owner 调用；不输出内部错误文本 |
| 消息/metadata/连接或请求准入超限 | RESOURCE_EXHAUSTED | 不替换成空事实或新查询模式 |
| 预期目标不符、授权依赖不可用、服务关闭 | UNAVAILABLE | 不转移目标，不声称 NOT_FOUND |
| RPC 超时/取消/无法解释的内部失败 | DEADLINE_EXCEEDED / CANCELLED / INTERNAL 等 | 不证明 owner 未提交，见下节恢复规则 |
| 未提供的 service/method | UNIMPLEMENTED | 仅表示 RPC 不存在，不等于 Element NOT_FOUND，不回退 JSON/HTTP |

不提供 JSON 错误 envelope 或 HTTP 状态到领域状态的猜测；未知/缺失/非法终态一律不当作成功。
禁止缓存业务响应、自动解压未允许的消息及在 status details/trailers 中输出 secret、私有状态或隐藏图。
身份与授权拦截器不替 owner 决定空间/类型合法性，领域拒绝仍在原边界发生。

### 4.4 deadline、重试与恢复

客户端必须区分“完整 owner 回执”与“没有取得结果”。TLS/EOF/截断/超时或 gRPC 错误不等于提交拒绝。
一旦可能派发到 owner，写操作的本地恢复状态为结果未确定；这不是伪造 owner 的 INDETERMINATE 回执。
只有能够证明发送前失败才可标为未派发。业务恢复沿用原 OperationID、actor、规范请求和前置条件；
会话丢失后重新认证再调用 RecoverOutcome。不换 ID、不更新 CAS 后暗中重写，不以观察消息代替回执。

关闭策略重试、hedging 和 WaitForReady；不接受 resolver 下发的 service config，也不配置本地自动业务重试。
grpc-go 的 `WithDisableRetry` 不关闭所有透明重试：库确定未发出或未被服务端应用处理的 RPC 仍可能重发。
仅接受这种未进入应用处理的透明重试，并以故障测试证明不重复进入提交路径；不能声称该开关等于完全零重试。
所有尝试/重连受同一调用 deadline 约束，不能每次重新给一份完整超时。无法证明该边界的运行库不通过验收。
已经进入应用的操作，只由调用方按原 owner 合同显式精确重试/恢复；gRPC 的 retry commit 概念不等于事实提交。
取消只停止等待，不回滚已提交事实。Poll 丢响应不推进客户端 cursor，不新增消息总线或永久历史。

## 5. 配置、生命周期与有限资源

服务入口只打开已明确初始化的 durable owner/Realm Store；初始化与服务启动分开，
缺文件/身份不符/Store 损坏均拒绝启动，不在正常重启时重新 seed 权限或生成另一个身份。
一次进程只托管本切片声明的 owner；独立服务不共享可写 Store，不以数据库路径作客户端地址。

网络配置包含：监听地址、对外 NextHop、预期 owner、证书/私钥文件、Realm 装配、会话及传输资源限制。
客户端受控配置持有目标信任材料和调用凭据；地址仍由具体入口/关系提供，不能由信任表反推。
监听地址和广告地址分开，`0.0.0.0` 不是可发布的 NextHop；本机验证仅监听 loopback，不自动开放防火墙。

- `SessionTTL` 必填正整数秒，无默认时长；TLS/HTTP2 建连、读取未认证消息、单 RPC 及 shutdown 都有显式正超时。
  客户端每次 RPC 设置 deadline，服务端以配置的最大执行时长收紧它；调用者不带 deadline 也不能无限执行。
- 首个测试配置选择具体有限值，但它们是测试输入，不变成生产默认或用户已接受的性能指标。
- 网络总连接、HTTP/2 并发 streams、全服务执行/认证、metadata 大小、session 总数及每主体数量均显式有界。
  不能只限制连接数而允许一个 channel 无限开 RPC；也不能只在解码后的 interceptor 才限制内存。
  以连接/stream/message 接收上限约束解码前占用，再执行应用准入；慢握手、慢消息和无效认证均有拒绝与回收门。
- 首个业务请求的 Protobuf 消息上限为 4 MiB；转换后的 P5 规范输入也受原 4 MiB 及结构上限约束，
  不能用紧凑 wire 绕过原内核限额。消息复杂度/递归深度/重复项另作显式检查。
  双方显式设置 send/receive/metadata 上限，响应上限由最大合法业务结果反例证明，拒绝压缩消息。
- owner 的 PublicAccessPolicy、SnapshotRetention 仍独立配置；网络开启不暗中开启快照、观察或默认保留时长。
- 到期使用服务端单调经过时间判定；返回剩余时长只是提示，不依赖客户端时钟进行授权或切面组合。
- 正常关闭先停止接入，有限等待已准入请求，再关闭 owner 和 Realm；强制退出后按原 Store 规则恢复。
  不因为停机强迫丢掉已确认提交，不把独立 Store 合成一次事务。

## 6. 实现归位与替换边界

| 位置 | 本轮允许的职责与依赖 |
|---|---|
| `shared/values` | 最小 NextHop/共同业务值；仅标准库，不导入 Protobuf/gRPC 或网络状态 |
| `shared/api`、`ucupa-core/api` | 共同消息/会话及 Core 的唯一 proto 源和公共 conformance；不复制字段/领域规则 |
| `shared/bindings/go/protobuf`、`ucupa-core/bindings/go/protobuf` | 仅由对应 proto 生成的消息与 stub；共同绑定不导入 owner 绑定；不手写业务 |
| `ucupa-core/transport/grpc` | 服务端 RPC、会话、消息映射；消费生成绑定/Core 公共操作/认证端口，不访问 Store、不导入 Realm |
| `ucupa-facet/api`、`ucupa-facet/bindings/go/protobuf` | Facet 内容协议源与唯一生成绑定；公共 AttachmentKey 复用共同消息，私有控制证据不进入内容结果 |
| `ucupa-facet/transport/grpc` | typed 内容消息适配到原 Facet PublicCall；已认证内容 caller 与固定 Core 回查服务 caller 分开 |
| `internal/protovalue` | 两侧 transport 共用的私有字段表示转换；不依赖 owner、不提供认证、不复制业务裁决；内核、SDK、Composition 不导入 |
| `internal/grpcbound`、`internal/grpcdial` | 实际双消费者共用的连接准入／截止与指定目标 TLS 建连；前者仅 transport、后者仅 SDK 导入；不含 owner 状态或路由 |
| `ucupa-facet/sdk/go` | Facet 生成 stub 的目标、TLS、有限会话、deadline 与结果检查；不导入 owner 或 Core SDK |
| `ucupa-core/sdk/go` | 实际客户端所需的目标/TLS/会话/deadline 装配；复用生成 stub，不复制已生成方法，不导入 Core 实现 |
| `composition` | 将 Realm credential establishment 接给消费方端口；不负责授权裁决或保存会话/事实 |
| `cmd/ucupa-core` | 配置、打开 durable 实例、组装、监听和关闭；不复制领域规则 |
| `acceptance` | 真实子进程、独立 Store、TLS 和故障验收；fixture 不冒充产品 host |
| `cmd/verify` | 精确允许实际新增包，验证 transport/SDK 的单向依赖；继续禁止任意公开子包 |

会话归服务端 transport 的运行内认证适配，不进入 Core 空间状态或 Realm 授权真相。
仅供该适配的代码在其包内按职责拆文件；不预建通用 gateway、host 库或空目录。
采用 gRPC 后 bindings 是有实际协议源的必要生成物，生成代码不因此成为新产品模块。
Facet 的同类位置在它开始网络切片时实际建立；共同机制只在两个真实消费者成立后评估提取，
不为目录对称复制协议或先造一套服务框架。

实现时固定 protoc、protoc-gen-go、protoc-gen-go-grpc 和 Go runtime 依赖的具体版本与生成命令；
用临时目录重生成并比较，纳入唯一 verify 入口，缺工具不能跳过生成检查。开发时需要生成工具，服务运行时不需要 protoc。
共享绑定是工程规范规定的窄生成物例外，不能据此把 shared/values 改为依赖网络库；检查器只开放精确产物路径与依赖。
proto package/service 不加 UCUPA 版本后缀；不增加 REST/JSON 转码或兼容 reader。采用外部工具的版本锁定不等于产品多版本设计。

实施直接修订 NextHop 校验、共同 schema 与受影响测试，保留真正使用的 local adapter。
不添加 `/v1`、旧格式 reader、URL→local 名转换或反向兼容 Store；若实际需要改变持久编码，
必须先在对应 owner 合同点明不变量和 marker，不借 transport 改写历史数据。
本合同授权明确的网络增量，不提前把 P5/P3 本地 current 合同改成“已支持网络”。

## 7. 首个网络切片的验收反例

以下是验收反例清单；实际实现、运行过的测试与待补证据见开发计划的 P6 交付记录，不能把本表整体当作通过记录：

| 反例/场景 | 必须证明 |
|---|---|
| 本机 caller 与 Core 独立进程 | Enter → 创建未知 UUID → Discover → 同依据 Read → prepared Observe → 更新/删除 → Poll；只走公开字节 |
| 未知 CA、错误 hostname、错误 pin、错 Ref/错 Store | 不发送凭据给未认证目标；不接受错误 owner，不降级/跟随重定向 |
| 明文/h2c、非 h2、环境代理、resolver 注入 service config、地址中凭据或路径 | 不降级/另找目标，不泄露认证数据，不恢复 Ref 路由 |
| 超大/截断消息、缺终态、UNIMPLEMENTED/UNKNOWN、伪成功 response | 不把库/协议状态当作事实成功或 NOT_FOUND |
| 字段重排的等值 Protobuf、unknown field/enum、缺必填/oneof、NaN | 前者产生同一规范操作；其余按形状/数值规则拒绝，不以原始 wire bytes 区分操作 |
| 同原命令经本地 JCS 与 gRPC 两种表示重试 | 同 actor/OperationID/语义得到原回执，改变真实字段才冲突，不另建提交历史 |
| 伪造 Principal/CallerContext、错误 audience/secret、凭据撤销 | 不建立错误身份，不披露资源存在性；有效会话也不能绕过撤销 |
| 已授权读取后撤权再重授 | 旧 view/continuation/观察按原合同失效，不重新认证后偷接历史 |
| 会话有效期间 TCP 断连再连 | 保留同一 caller establishment，满足原条件才可续页/轮询 |
| session 到期、主动释放、宿主或 Realm 重启 | 旧 session 不恢复；重新建立身份/依据，不把快照过期伪装成无记录 |
| 提交已落盘但响应被切断，再终止进程 | reopen 后用同 actor 原 OperationID 恢复已确认结果；不二次修改 |
| 发送前失败、REFUSED_STREAM/GOAWAY、handler 之后断连、重连 | 区分安全透明重试与已处理调用；后者不自动重发，原 deadline 不重置 |
| 返回请求取消与 owner commit 并发 | 允许结果未确定，不伪造 rollback；恢复可给出真实持久结果 |
| 会话/连接/streams 满、慢消息、巨型 metadata、反复错误登录、关闭时仍有请求 | 有界内存、连接和时间，无无限等待；完整验证含 race |
| 隐藏/闭包外更新、无权调用 | 不增加公开细节或按私有提交频率生成公共事件；status/trailers 不泄露证据 |
| 外部消费者与生成检查 | 仅持入口、信任、凭据和公共 proto/stub，不读 Store；重新生成无差异 |
| 请求大小、吞吐/延迟与保留量测量 | 记录方法、负载和实测，不因采用 gRPC 就宣称更快；不以性能优化弱化正确性 |

完整验证继续使用 `go run ./cmd/verify`。网络子进程测试纳入同一入口，保留 P1–P5 全仓回归；
编译、handler 或 loopback TCP 单测通过不能代替子进程门。

## 8. P6 完整网络链的边界

以下为完整网络链的验收要求；具体协议见第 8.1–8.6 节，实际测试证据见开发计划，不能仅凭基础 transport 宣称满足：

1. **Facet 正反访问链：** 内容 caller 与固定 Facet 服务身份使用不同、目标限定的会话；
   Core 附着/接纳/联系与 Facet CoreControlSource 需要明确公共服务消息，保留 F0 capture/fresh-check/recheck，
   私有 Core view/cursor 不交给消费者。必须先补 wire/权限映射，再移除该切片对进程内反向适配的依赖。
2. **挂载控制：** ApplyMount、Prepare/Finish、决定读取与恢复分别有明确调用角色和接收方授权。
   不把 LocalCallers 或本地 PrincipalRef 序列化成可信证明；A 持久占槽前必须已具备合同要求的 B caller 证据。
3. **公共联邦调用者：** B 必须独立建立原调用者在 B 的身份/授权。不能转发 A 的会话、长效 secret、
   自报 Principal 或借用 A 服务权限替代原调用者；固定服务控制身份只服务其已授权的控制端口。
   目标限定的身份传递证据、失效和恢复是单独安全边界，尚未由本切片的 bearer 会话解决。
4. **嵌套 source graph：** 从双 Core 扩展到 A→B→C 和兄弟分支，验证外部父、环拒绝、匹配接纳/联系切面、
   保守剪枝、独立根边界、同一来源的去重与 provenance。UpDepth 仍只选择语义范围，不换成网络跳数。
5. **多来源分页/观察与隐私：** 不能直接把 P3 私有 ConnectedCut/完整内部图变成公共格式。
   需具体实现 D7/D10/D11 的独立 caller/view、可披露来源、opaque closure 与 resume；
   新来源先建 read basis/cursor，旧 continuation 不补插来源；不可证明则 PARTIAL/UNAVAILABLE/GAP。
   请求时限/工作限额独立于 UpDepth，不另建泛化预算模块，不以资源不足静默缩小搜索范围。
6. **互操作与真实主机：** 独立客户端只证明消费协议；完整开放门还需两个不共享内部 Go 类型、
   Store 或 gateway 代码的实现通过公共 corpus。真实主机门另外记录网络/证书/时间/进程恢复证据。

基础网络接入与完整网络链分别验收，不把有限场景的验证写成任意规模、任意环境下的“普适联邦”。
P7 writer/owner 转移、P8 生产运行验收、Stage 2 Realm 联系、K8s 和 Console 均不进入本次实现范围。

### 8.1 已接受的受限委托与按需接入

**用户已接受受限委托方向并明确授权继续实现；实现进度仍见开发计划。** 首个网络会话只允许调用方直接访问一个目标，
不能凭它让入口 A 在 B 上取得原调用方权限。现行本地 CallerContext 带 Realm 运行内证明，
也不能通过序列化变成跨服务身份；固定 A 服务身份不能取代用户的 B 权限。

采用显式、目标限定的短期委托，不采用让应用自己聚合所有目标结果的替代架构。
证据绑定 B、原调用者、受托 A、允许的锚范围／操作和期限；A 不取得用户长效 secret，也不能自行扩权或转授给 C。
这不要求预先列出或登录所有目标：可披露目标在沿关系发现后，由调用方客户端在其明确授权政策下按需认证并取得委托。
没有相应身份或信任依据就明确失败／未覆盖，不能凭 A 的会话跳过 B 的独立身份建立。
本轮先落实目标签发与使用的闭环；隐藏来源的安全身份传递、公开来源图及自动接力仍须第 8 节的联邦协议，
不能把“让客户端直接联系隐藏 B”当成隐私方案，也不能宣称首个委托接口已经完成透明联邦。

#### 目标 Core 的最小只读委托协议

- 新增 typed `ReadDelegationService` 的创建／撤销方法。只有直接认证到目标的用户会话能创建委托；
  委托不能创建另一份委托。明确输入目标 Reference、受托 Core 身份、read/list/query 子集、是否允许观察及正整数期限。
  目标按原 owner Enter 检查所申请入口；后续每次操作仍由 Core／Realm 根据原调用者检查当前权限。
- 接收方启动时显式开启此能力，提供受信任的代办 Core 身份到客户端证书 SPKI 的绑定、客户端 CA、
  最大时长和全局／每主体数量限额。绑定不含服务地址，不是 Ref 路由，不从请求自行学习信任。
  普通用户连接仍可不带客户端证书；使用委托的连接必须完成 TLS 客户端证书认证，并匹配委托绑定的代办方。
- 原用户经过本地 Realm 验证后派生一个新的、本次委托专用的 CallerContext，保留原 CredentialID 与主体；
  它不是把代办 Core 的权限当作用户权限，也不能由请求自报 Principal 建立。新 context 使不同委托的句柄不可混用，
  凭据轮换／撤销和 Realm 重启仍由原有效性检查阻断。该 local context 不上网、不持久化。
- 目标保留有限的运行内委托记录：随机 32-byte token 的 SHA-256 索引、原用户、专用 context、受托身份、
  唯一入口 handle、允许的操作及绝对到期时间。到期不得晚于原用户会话到期，也不能由调用或重连延长。
  对外只返回 token、目标／受托身份、handle 和剩余期限；不返回 CallerContext、授权 view 或凭据。
  剩余秒数向上取整，仅供调用方估计，目标执行精确绝对期限；owner handle／快照还受各自原有期限约束，不能由委托续期。
  释放原用户会话不撤销已经发出的委托，其期限仍不晚于原会话的原始期限；要提前终止使用显式撤销。
  创建答复丢失可能留下到期前占用名额的运行内委托，不影响空间事实，也不自动重发创建或将其视为已收到。
- 代办调用使用 `authorization: Delegation <严格 base64url token>`。read/list/query 只能使用这份委托的 handle；
  PrepareObserve/Begin/Poll/Release 只有明确允许观察时可用，且 token 仍由原 owner 绑定同一专用 caller。
  不开放 Enter、写入、RecoverOutcome、身份管理或新委托。此限制只作用于代办链，不删除用户直接调用的写能力。
- 每次调用前后检查委托仍有效；普通 owner 当前授权、prepared Read、ABA、快照与 GAP 规则不变。
  原用户可以从自己新建立的有效会话撤销自己发出的委托；token 形状合法但未知／已撤销时返回同样成功，不枚举存在性。
  删除委托后原有句柄不能恢复使用，尽力关闭其 owner 入口；未及时释放的派生资源仍受原 owner TTL／容量限制。
  token 不跨宿主重启恢复；重新申请生成不同 caller 和 token，不能续接旧观察。地址变化也不能改变目标或借用旧信任。

Facet 固定服务身份的控制消息仍需按第 8 节第 1 条保持其披露边界，不因接受用户委托而改用服务权限读取用户内容。
本补充不提前实现 Stage 2 RealmAttachment，不新增中心认证／路由服务。

### 8.2 显式身份签发信任（已交付）

用户已接受：目标可显式信任指定身份签发方，验证入口转交的短期身份证明，仍由目标独立授权。
这不是让入口自行声明用户身份，不建立全网总 Realm，也不因挂载学习信任。

最小实现先归 Realm 的身份签发／验证能力，由 Composition 接给网络端口；不让 Core 导入 Realm。
密钥、信任与数量／期限限制都是显式启动配置，不写入 Element，不改变 Realm 授权绑定或 Store 格式。

- 签发方仅从当前有效、凭据认证的本地 caller 签发；可信装配 caller、外来证明或既有委托不能转签。
  证明只含签发 Realm、主体、受托 Core、目标生成的随机 challenge、签发／到期时刻及随机证明标识，
  不含 secret、CallerContext、权限 view、目标地址或隐藏来源图。签名使用固定 Ed25519 与域隔离的规范字节。
- challenge 必须由目标有限保存，并绑定目标、受托者、请求范围与动作；客户端只中继随机 challenge，
  不需要知道隐藏目标地址。签发方对未知 challenge 签名本身不授予目标权限。
- 目标仅接受本地显式配置的“签发 Realm → 公钥”；签发命名空间必须精确匹配，不允许任意主体映射、
  请求携带公钥或将外来签发方映射为本地管理员。签名、受托者、challenge 与有限期限都必须匹配。
- 签发方有限保存证明对应的原 caller；验证活性时重新校验原凭据，撤销／轮换、Realm 或签发进程重启均使证明失效。
  目标使用前后还须通过明确配置并认证的签发方连接回查活性；失败不得仅凭未到期签名离线放行。
  此回查证明签发方读取时的身份有效性，不承诺两个独立 owner 的全局原子撤销时刻。
- 完成目标身份建立后仍检查目标当前授权与原 ABA 规则；签名不包含可直接兑现的权限。
  challenge 交换、目标 caller、受托连接密钥及查询状态必须绑定，不能从另一入口重放或续接旧观察。
- 本条实现记录须分别区分 Realm 签发／校验、typed 网络交换、自动联邦接力；前者通过不代表整链闭环。
  外部身份网络会话尚未落实前，不将裸主体交给 `TrustLocalCaller` 当作已完成网络认证。

具体网络交换采用：

1. 受托 Core 以配置匹配的 mTLS 身份调用目标 `BeginIdentityChallenge`，指定 Reference、只读动作、观察开关和期限。
   目标有限保存这份请求并生成随机 32-byte challenge；只将 challenge 和剩余时间交给用户中继，不披露目标地址。
   挑战全局／单受托者容量分别使用 `max_grants`／`max_per_caller`，与已建立委托名额分开计数，均有硬上限。
2. 用户通过自己在签发 Core 的有效会话调用 `IssueIdentityProof`。签发方只签当前凭据关联主体，
   期限不晚于原会话到期；返回的 `payload/signature` 是需保持原字节的签名材料，不是任意 JSON 业务 RPC。
   payload 为域隔离 Ed25519 签名覆盖的固定七字段规范 JSON，拒绝额外字段／非规范编码；算法不可由请求切换。
3. 受托者携证明向同一目标 `ExchangeIdentity`；目标消费该一次性挑战、验证签发命名空间、公钥、受托者、challenge 与期限，
   再通过配置固定的 issuer 地址／预期 Core／CA／pin，以自身 mTLS 身份调用 `CheckIdentityProof`。
   服务身份的 peer binding 复用当前显式 `read_delegation` 连接信任，签发功能未配置时不可调用。
4. 只有两项验证均通过，Composition 才建立新的 target-local caller。主体命名空间保持原样，不做外来主体到本地主体的映射；
   同一逻辑 Realm 的显式受信任签发方也可使用，本地主体是否存在及实际权限仍由目标 Realm 检查。
   Core 原 Enter 验证所选范围／动作，成功后返回同一 `ReadDelegation` 格式；权限不足不会因签名有效而放行。
5. 每次使用这类委托前后均重新检查签名期限及签发方活性。确认无效终止该委托；回查不可达返回 UNAVAILABLE，
   不伪造撤销，也不离线放行。网络恢复后仍需重新检查；签发方或目标重启不恢复旧证明／挑战／委托。
   交换失败或答复丢失需新建挑战，不能重放旧挑战取得新 caller。新增身份通路不允许写入、转签或再委托。

以上提供显式中继的身份交换原语；自动发现来源、对用户隐藏来源图、递归查询／分页／观察仍须完成第 8 节的公共联邦实现。
不能把交换原语本身描述为已完成入口自动发现、自动取得全图身份或无条件信任后继 Core。

### 8.3 Facet 固定服务回查端口

Core 的 `AttachmentControlService` 是服务端口，不是消费者的空间读取接口。
启动配置显式绑定已认证 Realm 主体与 Facet 身份；缺省关闭。它使用独立登录取得的目标 Core 会话，
不接受只读委托，不从请求主体建立身份。绑定只限制可请求的 Facet，Core 原附着读取／观察授权仍独立执行。
调用前后的凭据与授权有效性仍以 owner 的认证读取与 prepared 检查为准。

`ReadControl` 接受精确接纳关系及是否准备观察，返回最小附着状态、Core cut、公开 proposal 对照值与服务专用 preparation；
`CheckControl` 校验同会话、同关系的 preparation。后续服务观察使用原 typed Begin/Poll/Release 方法，
不会将服务 preparation/cursor 放进 Facet 的消费者响应；不同会话不能借用它们。
网络失效不解释为附着不存在，响应的身份、元素、revision 与 key 必须一致。
`ReadProposalControl` 使用独立提案读取权限和当前 caller 校验，返回 pending 的最小投影；
不支持 prepare，不把未经认证的 PrincipalRef 当作网络调用者。

关系写入使用 Core 的 `InspectFacetControl`／`ApplyFacetControl` 认证 owner 入口：
消费者只提供 propose、cancel、activate、detach 或 contact 的封闭命令和公开比较 token，
不提供私有 revision、cut、接纳证明或主体字段。提案控制读取使用独立的 proposal-read 权限，
不会借普通元素读取披露 pending；比较 token 与现有公开附着投影保持同一发行域。
原调用者与原 authorization view 必须在读取回执或提交前核验，Activate 的固定服务往返后
再次核验同一 view；撤权后重授不能刷新旧请求。提交后披露前撤权时不返回成功内容，
已经持久的决定不回滚，原主体以后重新认证可用原 ID、原命令取得精确结果。
公共输入通过 owner 当前状态或原操作日志解析，地址维护不调用不可达的旧服务。
此入口复用唯一附着转换与持久日志，不改变空间事实和存储编码；
对应 `FacetControlService` 的 InspectFacet／ApplyFacet 使用同一 owner 裁决。

首次接纳通过 Facet 的 `FacetRelationshipService`：调用者提交公开 Core／Element／proposal、
指定 Core 地址和 OperationID，Facet 使用固定服务身份调用 Core 的 `ResolveProposalControl`。
该端口将公开 proposal 与当前、同一 Facet 的 pending 提案精确对照，不要求消费者提供私有 proposal key，
不通过扫描历史或猜测最新提案完成接纳。取消或换提案后旧 token 不能取得新关系。
Facet 保存接纳命令的原公开对照，精确重试及本侧 Core 地址维护不依赖 Core 在线。
原 Facet 调用者／view 在外部等待前后和唯一提交锁内核验；服务身份不代替接纳用户授权。

激活时 Core 使用固定服务身份调用 Facet 的 `FacetAcceptanceService.ReadAcceptance`，
核对完整关系、Facet cut 和 accepted-at，不把断连解释为未接纳。
Facet 配置 `core_services` 将固定 Realm 主体限制到预期 Core，仍独立要求接纳读取权限。
Core 的可选 `facet_connections` 与 Facet 的 `core_connections` 只从关系指定地址选择精确连接，
每项包含预期 owner、CA、pin 与固定服务凭据文件；不从 Ref 反查地址。
两端地址维护只有原关系的联系 CAS 发生变化，不创建新提案、接纳或内容事实。

Facet host 按需从本地明确配置的文件取得固定服务凭据，SDK 在原会话有效期间复用它；
达到保守的本地到期时刻、CredentialID 改变或旧会话被明确拒绝后，下一次显式调用才重新建立会话，
不自动重发失败业务请求。拒绝只清除本次使用的旧会话，不能清除并发建立的新会话。
原 preparation/cursor 仍绑定原 caller，新登录不能续接；目标提前重启时当前失败请求仍返回不可用。
连接配置必须与关系地址及预期 Core 一致；不通过查 Ref 或临时本地对象修复失败。

服务消息中的 revision 明确定义为非零 32-byte incarnation 与非零 uint64 sequence；
关系另带 Core、Facet、Element，附着 revision 的元素由该 Element 确定。Core cut 使用独立消息，不能互换。
这只是已授权服务间的精确证据编码，不是消费者公共 revision，也不是序列化本地 Go 对象或全局时间。
未知字段、畸形证据和未知状态一律拒绝，不以零值拼出成功。

### 8.4 挂载写入批准（已交付）

本地 ApplyMount 的 LocalCallers 不能序列化成网络身份；第 8.1／8.2 节已接受的委托明确只读，
不能扩大成挂载写权限。用户已明确接受保留并验证原请求者身份、目标独立授权的方向并授权继续实现。

当原挂载合同要求承载侧 B 调用者证明时，由调用方直接向 B 认证，并使用 B 的既有 Realm 权限，
申请一份绑定目标 B、来源 A、OperationID、精确关系与命令的短期批准。A 仍独立检查自己的原调用者权限，
占用控制槽前验证 B 的批准；不能转授、替换命令、从服务身份推导用户权限，或将其用于内容／空间一般写入。
只读委托保持不变。原合同不要求 B 新批准的操作不能被此提案顺手增加前置。

实施时须完成一次性使用、过期、答复丢失、持久 prepare 后恢复、撤权与服务身份的具体协议及反例；
不将批准超时解释为已撤销关系，不让一次短期凭据过期阻止已持久决定的安全关闭。
方向接受不代表网络实现或验证已经完成。短期批准是验证原请求的手段，不另造授权模型。

承载侧 owner 的认证准备入口为 `PrepareMountControl`：服务适配从已认证连接建立 source，
从目标认证边界取得原请求者 CallerContext，不能从消息中的 ParentActor／CarrierActor 建立 caller。
CarrierActor 必须与 caller.Subject 精确一致；目标按原 MountAccept 权限与精确锚目标取得 view，
在 owner 锁内、读取既有 prepare 回执或执行唯一持久 transition 之前重新检查凭据与 view。
授权拒绝、ABA、取消或容量不足均不占用承载侧控制槽、不改变事实。
精确重试复用已有持久 prepare，不重新关门或新增 cut；同 OperationID 不同请求仍为冲突。
此入口不签发批准、不验证网络 TLS，也不替代尚需实现的批准交换与服务连接适配。
已持久决定的 Finish／恢复不能因这个新入口而退回到重新 Prepare 或要求已过期的用户凭据。

批准交换采用独立 `MountApprovalService`，显式 `mount_approvals` 配置才启用；
服务 mTLS peer 信任复用已配置的连接身份，不复用任何只读 grant 或其权限。
`CreateMountApproval` 只接受直接用户会话，记录目标、受托 actor、Element、OperationID 和
精确命令的 32-byte commitment，保留原 caller 与 MountAccept authorization view，不把 view 返回到网络。
批准期限不得超过用户会话剩余期限；全局和每主体数量分别硬限制，答复丢失留下的批准按期限回收。
`CheckMountApproval` 只接受该 actor 的已认证 mTLS 连接与相同绑定，不接受用户 Bearer 或只读委托替代；
每次验证原凭据与原 view，撤权重授不能刷新旧批准。返回原 Realm/主体只是给 A 归因，不能在 A 变成可信 CallerContext。
`RevokeMountApproval` 由直接认证的原主体撤销，同主体重新登录可撤销旧批准，不泄露其他主体 token 是否存在。
过期、重启和撤销后的 token 不恢复；不可用与无权分开返回。

Check 是占槽前的校验，不消费批准；重复校验不产生写入。实际 Prepare 还必须匹配命令 commitment、
在承载 owner 提交边界验证捕获的 view，并绑定唯一持久请求；不能将 Check 成功缓存为以后无限写入资格。
`MountPeerService` 将 Prepare、Finish、ReadMountDecision 分为三个 typed RPC；服务身份来自明确配置的
mTLS key，主体字段只作原请求归因。Prepare 同时验证批准与命令，提交期间将本地批准撤销串行化，
使用批准到期截止，并在 owner 锁内重新验证原 view；不允许批准检查后的撤销被绕过。
Finish 和决定读取只复用已有关系／操作的恢复规则，不索取新的用户批准。
ServiceRevision／ServiceCoreCut 明确编码 incarnation 与 uint64 sequence，不复制 Store 编码，
也不把此证据作为普通消费者 token。解析只证明形状，不证明授权或事实发行。
Revoke 按原合同可不携带 CarrierActor，不因网络适配增加 B 用户批准前置。
当前三个 peer RPC 已接通；父侧认证应用入口、正式双向连接装配与进程恢复整链仍须完成，
不能把 peer 端口测试当作 A→B 挂载应用写闭环。

`MountCommandCommitment` 对规范 MountEstablish／MountUpdate 计算 SHA-256；输入为
`ucupa.mount-command` 加一个零字节，再接以下对象的 UCUPA 规范 JSON：parentCore、carrierCore、
operationId（base64url）、element、kind（establish/update）、carrierNextHop、parentNextHop、
expectedElement、expectedMount、final。revision 为 `[base64url incarnation, ElementUUID, 十进制 sequence 字符串]`，
不存在为 null；final 不存在为 null，否则为 parent、既有规范 pose、radius。缺省无效字段仍按原命令校验拒绝。
因此地址、比较依据、最终空间元组、操作标识和两侧身份都参与绑定，不用原始 Protobuf 字节作为语义依据。
`PrepareApprovedMountControl` 在 owner 锁内验证批准原 view，不重新取得新 view 修复 ABA，
再复用原 prepare transition 与持久精确重试。原本地认证 prepare 不是网络批准的回退路径。

父侧 `AuthorizeMountControl` 捕获原用户对本次命令所需权限的 view；`ApplyMountControl` 消费该 view，
通过调用绑定的 `MountControlTransport.CheckApproval` 向指定承载地址验证批准后，才进入原 reserve。
网络往返后在 reservation 锁内重检原 view、实际关系地址及其修订，不能用检查期间被替换的联系占槽。
Prepare 往返后，父侧在决定锁内重检原 view；确认撤权／ABA 可按原流程决定 Abort 并关闭准确请求，
授权依赖不可用不得被解释为拒绝并凭空决定 Abort。结果披露前仍检查原授权。
已决定操作的精确重试使用其保留的 B 主体归因，不要求再次签发 B 批准；只改位姿或撤销仍不增加 B 新批准要求。

`MountConnections` 仅按关系指定地址选取预先配置的 pinned SDK，`WithApproval` 产生一次调用的不可变绑定，
不安装进程级写授权缓存。无批准的连接仍可 Finish／ReadDecision，不能 Prepare。
带该网络端口的自主恢复不能凭持久 Principal 名称继续未决定写入；等待原用户重新认证并精确重试，
而已持久决定继续完成 Finish／Settle。不会为了恢复隐式创建 caller、刷新授权 view 或恢复过期批准。
父侧 authenticated owner 入口及正式网络适配已实现；普通调用者的 Inspect／Describe／Apply gRPC
入口、独立进程宿主装配及网络联系修复已接通；Prepare／Finish 前后崩溃和重启的专项证据见开发计划。

普通控制命令的比较值使用 owner MAC token：元素使用 `record-revision`，挂载使用独立域的
`mount-revision`。不把 ServiceRevision、incarnation、sequence、cut 或 peer admission key 暴露成公共控制响应。
`InspectMountControl` 的父侧投影要求原用户的 MountRead；`ResolveMountControl` 要求对应控制权限，
只将当前匹配的 token 或保留操作的精确原命令解析给可信适配层，不创建控制槽、不写事实，也不授予后续执行权限。
保留操作必须同时匹配原主体、操作 ID 和全部命令字段；换主体或改命令返回冲突。
Apply 再校验捕获的原 view；解析之后的事实变化仍由唯一提交路径的 CAS 裁决。
token 的 MAC 密钥随 owner 持久化，正常重开不改变比较值；无效／跨 owner／跨 token 用途不能替代比较。
该 owner 转换已实现并验证磁盘重开。MountControlService 的 Inspect 返回公开比较令牌；
Describe 只生成目标与精确批准 binding，不创建授权、控制槽或持久意图，也不冻结后续权限；
Apply 从本次原用户 session 重新捕获 view，经 B 批准检查后调用唯一 owner 提交路径。
服务只能装配无调用批准的 MountConnections，不能把用户批准挂进全局配置；只读 delegation 不可升级为控制调用。
SDK 校验响应与请求的元素、操作 ID、批准来源／目标及决定状态相符。网络结果不带内部 revision 或 cut。
真实 TLS 黑盒测试已从普通用户网络入口覆盖建立、范围修改、位姿修改、撤销及精确重试；
宿主 `mount_connections` 按指定地址装配有固定 TLS pin 的 peer SDK，不含用户批准。
启动后在监听已建立的情况下执行一次受 RPC deadline 限制的恢复遍历，退出取消并等待后才关闭依赖。
未决定网络操作不刷新原授权；已持久决定可收尾，暂时不可达仍保留待恢复状态。
双独立产品进程已验证网络建立、强制终止／重启后精确重试、撤销与撤销后的再次重启。
MountContactService 的 Inspect／Repair 支持两个联系方向，原用户分别使用 MountControl／MountAccept 修复，
读取仍要求 MountRead。比较 token 在 contact-revision 内使用独立 mount 域，并绑定关系、方向与 incarnation；
不能替换成 Facet 联系 token、相反方向或另一条挂载的版本。修复不等待 peer、不占控制槽，
因此断连或范围操作暂停时仍可执行。原主体／操作 ID／原比较与新地址必须精确匹配才能重试，
旧响应不会把当前地址重新覆盖回去。仍使用原地址变更持久转换，不改变空间事实或承载归属。
已验证原授权 ABA 不写入、暂停时双向修复、旧比较／改方向／换主体／改重试拒绝；
真实双进程测试覆盖对端停机时修改和恢复地址、反向联系及后续重启。
建立挂载的独立进程反例进一步覆盖 Prepare 前／持久化后、Finish 前／持久化后强制退出。
A 使用产品宿主，故障 B 使用生产 owner、Realm 与 gRPC 适配，只在上述边界注入进程退出；
恢复用正常产品宿主，不伪造 prepared／decision。pending 不因启动恢复自动决定，需原用户重新认证并取得
同一 commitment 的新批准；committed 保持决定并继续收尾，不索取新批准。
服务身份通道独立读取原持久决定，丢失响应不是未提交的证据。
这些案例不替代范围修改／撤销及授权变化的其他反例，也不代替 Facet 或多级查询的故障矩阵。

### 8.5 公共联邦的来源投影与组合（已交付）

公共接入继续使用 Enter、Read、ListDirect、QueryRegion，不增加客户端遍历内部来源图的前置步骤。
入口 Core 是协调者；它按经过验证的挂载取得下一跳，使用第 8.2 节的原请求者证明，
让来源 Core 独立建立 caller/view。来源端只接受有期限、绑定目标／协调者／原 caller／请求摘要的读取依据。
Query、List、Read 分别检查自己的权限；内部路径投影不能将 Query 隐式升级成 Read、List 或 MountRead。

- 来源投影是 path／read／read-at／list／query 的封闭形状。路径只给必要的父边和 Pose；Query 只给命中摘要、
  必需的相交挂载与明确缺口，不发送整个子树的 Element/Facet 事实。Read 返回同 cut 的 Element 和 Attachment。
- Query 来源投影携带固定 32 bytes 的 `coverage`：来源在实际扫描时，按确定顺序记录本地 closure 的邻接、
  挂载剪枝／接续／未知判断和候选相交结果，再用私有随机 basis 作域隔离承诺，绑定 cut、原授权视图及精确投影条件。
  邻接通过与 ListDirect 共用的直接父成员筛选和 UUID 分页规则形成：每个父锚逐页记录直接子元素，
  包括空父锚的完整空页与末页 complete 标记；子锚未进入遍历 closure、查询根缺失或请求取消都不能形成证据。
  这是固定 cut 内的临时私有投影，不维护第二份索引；它不调用公共 List 接口、不升级 Query 权限，
  也不把私有邻接 transcript 直接输出给消费者。
  内部遍历摘要不直接出 Core，且没有第二份持久索引。证据不依赖本页大小或 continuation，
  因为来源每页仍执行同一固定 cut 上的完整本地候选检查；它不声称远端分支已闭合。
  `Gap` 非零仍是不完整，不能用非零 coverage 覆盖失败；非 Query 投影禁止附带该字段。
  协调者在规划时捕获原 coverage，后续来源扫描必须完全相同，并将其绑定进 aggregate closure。
  缺少实际 Query 扫描计划或零证据不能产生聚合查询 closure，单纯枚举已访问 owner 不足以证明查询闭包。
  SDK 拒绝 Query 缺失、全零或非 32 bytes 的证据。该字段同时接入公共 sourceGraph 的查询来源节点，
  但不是消费者可脱离认证来源独立核算的几何证明。
- 来源 read part 还绑定原 PrincipalRef、request-local cut stamp 和 authorization-view stamp。
  两个 stamp 由实际来源以随机私有 basis 加盐，并分别绑定 owner、boundary、normalized purpose 与原 cut／原授权建立；
  不直接公开存储 scope/serial、AuthorizationView、credential 或 Proof，也不使不同捕获的相同值可关联。
  协调者核对返回 subject 等于原 caller，两个 stamp 都必须是有限的非零固定长度值，无旧形状兼容分支。
  stamp 是认证来源返回的 opaque 证据，不是独立签名、访问凭据或额外授权；实际有效性仍由来源检查。
  当前该证据已通过来源 gRPC 链，并接入公共来源图与 aggregate closure。
  协调者现保留私有、有根的来源证据图：每个节点固定原 part，子节点同时保存父侧 outgoing 与自身 incoming。
  节点按已验证父边加入；封闭前后、续页与观察检查前后均核对原 subject、purpose、cut/view stamp、
  preparation 与方向独立的联系证据未替换。图不新增寻址能力，也不自动授权披露 owner／拓扑。
- `read-at` 是按既有依据执行的独立精确读取，不是 Query／List 的权限升级。来源另行取得并复核 Read view，
  在该依据内按元素保留首次成功的 Read view；后续使用不得通过重新授权绕过撤权重授。
  此关联受来源容量与原依据寿命限制，不扩大原查询授权或改变其观察准备。
  公共派生 receipt 只借用原封闭来源集合，不重新取得未捕获来源、不延长快照；沿派生链的 Read 授权继续校验。
  关闭或过期原依据同时使派生依据失效，派生读取不另占有或释放原组合。
  网络 Query／List grant 本身不允许 `read-at`；来源还须取得绑定这次精确 Read 的独立请求证明，
  不能以“同一个主体有 Read 权限”替代原委托的操作范围。来源签发一次性挑战，绑定原 grant、
  认证协调者、原 basis 和目标 Element；入口为原请求者取得新证明后提交。挑战及证明期限不能延长
  原会话、来源租约或快照。来源在执行前后验证身份与原授权；重放、替换目标或替换 grant 均拒绝。
- 协调者保留准确的逐段 Pose 配方，跨挂载不增加结构深度，也不先把累计投影舍入成一个浮点 Pose。
  UpDepth 从 Start 的已验证锚链选择 Scope，Basis 必须位于该 Scope，随后遍历相交后代。
- 来源首先捕获自己的 cut，协调者收集完必要来源后逐个确认捕获的 cut 仍 current。
  由于全部捕获先于全部复核，可证明这些切面曾共同存在；不是全局时钟或跨 owner 事务。
  混合保留配置在首页之前重新以 CurrentVerified 采集全部来源，不重用其中的 Retained 依据。
- 来源集合首页封闭，续页固定原来源、条件、范围及 UUID 排序。原来不可达或未接纳的分支不在续页补入。
  已定义范围的相关后代不可达只形成无标识／无数量的覆盖缺口，不把查询改成空结果。
  起点、Basis 或入口依据不能成立则不返回查询结果。已接纳来源的授权／联系失效不偷换 latest。
- 聚合 receipt／continuation 由入口签发并绑定内部组合；已披露记录的 revision 仍标记实际事实 owner。
  SDK 只在明确的读结果记录中接受这种归属差异，不能将例外扩大到 mutation、handle、服务 grant 或续页 token。
  聚合 Read／List／Query 另返回固定 32 bytes 的 `closure`，本地非聚合结果不需要该字段。
  它以来源私有随机依据为密钥，对原请求、原 caller 建立、范围／实际深度、覆盖缺口与封闭来源图进行域隔离承诺；
  图节点绑定原 cut/view stamp、来源 basis 与双方挂载／联系，不公开图的内容或数量。
  续页保持相同 commitment；独立查询重新捕获来源，不使用可关联的全局摘要。
  沿历史 receipt 的 Read 保留原组合 closure，新增精确 Read 授权仍由派生 receipt 独立绑定和验证。
  closure 不授予权限、不提供脱离服务的离线证明，也不使 PARTIAL 或未知分支自动变成 COMPLETE。
  SDK 拒绝存在但空、全零或非 32 bytes 的标记；授权与来源检查仍在每次返回前执行。
  聚合标记已绑定隐藏分支的 owner-local 拒绝闭包。公共 `sourceGraph` 与 closure 必须同时存在或同时缺席：
  图只投影已授权来源，按父在子前的顺序给出 owner、boundary、原 PrincipalRef、cut／authorization stamp、readMode，
  非根节点给出父 owner／挂载锚及方向独立的 delegation／acceptance 承诺；Query 来源另给 queryScope／coverage。
  后两类承诺绑定实际扫描及双方关系，不能仅凭可见 owner 列表补造。隐藏来源不输出节点、数量或占位，
  私有地址、basis、grant 和凭据不出现在公共图中。页与派生 Read 保留原图，不重新捕获来源。
  SDK 拒绝重复 owner、非入口根、未出现的父节点、缺少匹配证据、查询缺少扫描证据，以及记录归属不在图中的响应。
  Query 来源节点另带 `parents`：按 UUID 顺序给出 scope 及全部命中路径上允许披露的父锚，每项为 parent 与 adjacency。
  adjacency 由来源按私有 basis、原 coverage、parent 和该父锚完整直接邻接页链生成域隔离承诺。
  内部完整页链包含空页、末页与 continuation，但子元素名单、页数和内部游标不直接披露。
  未命中分支的父锚不因遍历而进入公开集合；外部根／挂载边界的 scope 已是该查询的已披露上下文。
  集合不依赖本页匹配条数，Retained 后续写入不替换证据；来源续扫替换任一父锚证据会被拒绝。
  来源 SDK 与公共 SDK 还交叉检查命中路径：scope 必须在声明路径上，scope 以下每个父锚都有对应证据，
  Anchor 命中也须有自身邻接证据；重复路径、用 Entity 充当 scope 或遗漏证据均拒绝。
  公共 COMPLETE 结果的各 Query 来源不能缺少父锚闭包；PARTIAL 中未能形成切面运算的来源不得补造假证据。
  原路径中 scope 以上的前缀由其他来源负责，不把它强塞进当前来源的证明或权限范围。
  每来源最多 4096 项，采集前后均受 owner 临时内存与协调者配方容量约束，超限明确失败，不截断后声称完整。
  Query 证明不授权 List 请求：原始邻接页和未命中的剪枝边不借证据字段披露，仍绑定在 coverage／opaque closure 中。
  图是认证入口声明的来源组合，不是独立签名或离线几何证明。
  D10 的逐项对账见开发计划的收口记录：区分公开证据和不可披露的内部证据，字段校验不代替行为与故障验证。
- 来源观察复用捕获 cut 时已准备的流，通过原来源 grant 调用 Begin／Poll；不临时建立观察权限，
  不重新捕获 cut。来源将 basis、原 caller、认证协调者与 preparation／cursor 绑定后调用唯一观察队列，
  普通写入不要求旧 cut 仍 current；授权变化、关系失效、队列缺口及游标替换按原观察规则处理。
  结果中的来源游标与变化组是协调者内部证据，不能直接作为隐藏来源的公共输出。
- 联系装配按地址配置预期目标和 TLS 信任，不提供裸 Ref 解析。捕获来源数、配方长度、临时输出和
  持有资源均受上限约束，容量不足明确失败。关闭／过期先取消披露和在途请求，再有限释放来源；
  回收完成前资源仍计入容量，不能借关闭绕过限额，也不能为清理重试原业务操作。

当前已接通公共多来源查询／分页、结构路径精确读取及直接列举，owner 多级反例和真实 TLS 公共入口已运行。
历史聚合依据上的独立授权精确读已接入 owner 公共 Read，网络独立 Read 请求证明交换已接通并经过真实 TLS 验证。
Query／List 的多来源 prepared Observe 已接通公共 preparation→Begin→Poll→Release：
公共游标由入口持有，来源游标保持私有，各已授权来源的变化组仍带自身 owner 与 basis。
公共页大小不改变来源批次；重试可取同一批次的不同长度前缀，确认后的旧游标返回 GAP。
每次返回（包括缓存重试）重验原来源观察授权／关系，释放 preparation 同时回收关联 receipt 的来源租约。
尚未闭合的潜在分支不被包装成可完整观察的来源集合；准备失败，不降级成单来源成功。
普通写入后的观察、初始 cut 到 Begin 之间的变化、来源身份、撤权再授权和重复 Poll 已经过真实 TLS 公共 SDK 测试。
精确 Read 的多来源 preparation 也已接通：在原 cut 上绑定实际披露路径，中间来源只观察路径事实，
末端来源另外观察 exact Element／Attachment，不因无关元素写入披露变化频率。
路径兴趣在准备时计入有界容量；开始观察后不得扩大。补充绑定要求原 cut 仍 current，不能遗漏绑定前的变化。
沿既有历史 receipt 的 Read 不允许再开启 preparation，返回 INVALID，与本地合同一致：观察从当前读取开始，
不追溯补造未捕获的历史。opaque closure、公共来源图及三 Core 独立产品进程的查询／观察／断连恢复矩阵已接线并通过专项测试；
公共图已接入 Query 可披露父锚的完整邻接闭包承诺；主线 D10 的对账和证据入口见开发计划收口记录。
本条不声明整个 P6 已完成；最终证据须覆盖完整第 8 节与交付门。

### 8.6 隐藏分支的拒绝视图与静默观察

明确无发现权限与无法验证是不同结果。Realm 的受信任 `CaptureDenialView` 只为已验证 caller、
一条精确 requirement 的缺失生成 `AuthorizationDenialView`；类型及 MAC 域与 Allow 视图分离。
有授权时不生成拒绝依据，伪造／失效身份或非法 requirement 也不生成依据。
视图绑定原 caller establishment、requirement 与该 binding 的最后变更 epoch，
不依赖全 Realm revision；无关授权或空间写入不会使它变成隐藏变化计数器，授权新增及 grant/revoke ABA 使旧依据失效。

Core 的 `CaptureFederationDenial`／`CheckFederationDenial` 是受信任来源端口，
只用于已成立 incoming mount 上的 Query／List 本身无权限，不把缺少 Observe 权限当作隐藏可发现内容的理由，
也不把精确 Read 拒绝改成 NOT_FOUND。依据绑定实际 Core、协调者、caller、原请求、拒绝视图和原 incoming 联系／接纳状态，
寿命不超过 Core 的 ContextTTL。校验仍需原请求，不能跨查询借用；关系变更、期限结束或视图变化明确失效。
该值字段私有，不是公共消息、访问凭据或另一份持久真相；网络适配须将其放在有界认证上下文后，
每次使用前后继续校验原身份及在线证明。客户端不能接触拒绝视图或隐藏来源身份。

来源 `FederationReadPart` 以必填 `hidden` 标记区分正常投影与拒绝闭包，沿既有有限 grant 和原身份证明链返回，
不增设服务、凭据或解析器。隐藏 part 只保留拒绝依据与 incoming 关系，不捕获空间快照或事实队列；
普通投影和精确读取不能借该 part 取得事实。协调者将其纳入原封闭图和固定 closure，但不加入结果扫描计划。
只有拒绝依据持续有效时，Query／List 可排除该不可披露分支；连接失败、身份无效、关系失效仍不能证明排除。

若原请求准备观察，隐藏 part 的 Begin／Poll 只核验原拒绝视图与关系，返回固定私有游标与空事件集。
它不订阅空间写入，不刷新期限，也不因隐藏更新的数量或频率产生公共事件。SDK 与协调者均拒绝隐藏 part 返回事件。
拒绝视图变化（包括 grant/revoke ABA）使原组合失效，不能自动补入新可见来源；重新查询才能捕获新组合。
真实 TLS 公共 SDK 反例覆盖隐藏写入后续页保持闭包、空观察重试相同、授权 ABA 失效，
以及来源断连仍为 PARTIAL／UNRESOLVED_COVERAGE。此项不代替剩余公共来源证据披露与全 P6 验证门。

## 9. 工程参照与状态

机制参照：[gRPC 方法与生命周期](https://grpc.io/docs/what-is-grpc/core-concepts/)、
[认证](https://grpc.io/docs/guides/auth/)、[deadline](https://grpc.io/docs/guides/deadlines/)、
[status](https://grpc.io/docs/guides/status-codes/)、[重试](https://grpc.io/docs/guides/retry/)。
Go 的重试/解析配置以[grpc-go API](https://pkg.go.dev/google.golang.org/grpc#WithDisableRetry)与所选固定实现为准，
不以通用指南的“关闭重试”推断某个语言实现已关闭透明重试。
[Protobuf 非规范化编码说明](https://protobuf.dev/programming-guides/serialization-not-canonical/)约束原始 bytes 的用途；
TLS 校验采用 [Go crypto/tls](https://pkg.go.dev/crypto/tls)。实现前核对实际固定依赖和工具链，
不从文档站版本反推本仓已经升级。这些资料不替代 UCUPA 的 owner、恢复与权限证明。

用户已确认合同提交与 P6 实现授权；第 8 节增量须形成具体协议及验证，不以本次授权代证正确性。实现、故障验证、用户验收、提交、真实跨机器验收分别记录；
没有环境不妨碍开发，但也不能用设计推演或既有本地测试宣称网络实现通过。
