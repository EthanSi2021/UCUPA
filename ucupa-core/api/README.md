# Core 本地公共 profile

本页描述仍实际使用的本地表示。P6 增加 [core.proto](core.proto) 的 typed gRPC 网络表示，
其唯一绑定由 `cmd/protogen` 生成；[网络宿主与 SDK](../../cmd/ucupa-core/README.md)不接受整包 JSON。
两种表示共用同一 owner 规则与操作历史，不是两个协议版本或旧 HTTP 兼容入口。

P7.2 的 `TransitionControlService` 目前提供源端 Begin／Inspect／Cancel／Recover，使用已认证的普通用户
会话，按具体步骤独立校验 export／commit／inspect；没有原 caller 字符串或用户可伪造的 peer 证据字段。
operation_id 是原始 bytes，Begin 的 expected 是原源 Element 的 `record-revision`，Cancel 的
expected 是原进度的 `transition-revision`。root 必须不带 final；slice 必须带完整最终 tuple。
INTENT 仅记录原输入，不代表目标预检、fence、安装或完整转移。该控制入口不是本地 JSON 命令；
本地调用使用 `instance.Transfers()` 的公开值合同，共用原提交与恢复结果。

`TransitionApprovalService` 为目标 Core 的运行期预检批准，不是持久 Accept。
原请求者在目标建立用户会话，提交 source、原始 operation bytes 与完整原 command；目标独立检查
`ucupa.core.transition.import`（目标 exact boundary），slice 还须最终 parent 的 `place-element` 权限。
批准绑定原 caller/view、源身份、原比较值、最终 tuple 与两个指定地址，有限 token 不向外披露 view。
Check 只允许明确配置且经证书认证的原 source 服务；普通用户会话、只读委托或裸 Ref 不能代替。
批准到期、原会话释放、重启或原授权变化后不能继续；撤权重授不复活旧 view。Check 成功也不代表
最终 placement 成立，更不是离线安装许可；后续 Accept 仍需原调用者有效授权与完整来源证据。

`TransitionSourceService` 提供源端只读 `ReadTransitionChunk`／`ReadTransitionDecision`。
请求只含原绑定及块位置／`max_bytes`，不接受调用者构造的 fence 或安装证明。
固定目标以双向 TLS 认证，源再核对自己的持久操作、目标与完整原输入；不使用用户会话代行。
块位置从零开始，单块上限 64 KiB、总清单上限 1 MiB；回应绑定原输入、源 final cut、摘要、
总长度和精确位置。接收侧必须收齐并校验摘要，不能把部分内容交给 owner 接纳。
决定回应保留原目标接纳 cut／revision 与源决定 cut／revision，不把传输成功称为安装完成。

`TransitionTargetService` 接纳／安装仅接收原绑定，首次接纳另带已申请的有限 `approval_token`。
目标按自己的指定源连接读取实际证据；不接收调用者上传的 fence、manifest 或决定。
首次接纳在拉取后与落盘前核对原目标 caller/view、批准、会话和期限；拉取期间撤销不会被锁阻塞。
缺少有效批准不能新增承诺，但固定源可恢复已持久接纳的精确原操作；安装不复活旧用户批准。
接纳回应保留原 fence 和 target cut/revision；安装回应保留原决定、安装与完成 cut/revision。
跨 owner comparison 仍属于绑定的 source，不把它改签成 target token；SDK 校验回包完整绑定。
派发写入后的 Store 错误返回 INDETERMINATE，拉取证据失败返回 UNAVAILABLE。
Recover 携带完整原绑定和目标批准 token；源 owner 先核对自己的原持久输入，完成状态也不允许
改绑定重试。普通会话贯穿本次有限推进，等待期间会话释放／到期会停止未提交步骤，不撤回 durable
义务。调用者不编排 fence／分块／接纳／安装，也不上传这些证明。每次继续仍检查状态披露权限；
未决定的导出／binding change 另验原用户权限，履行已决定的义务不要求复活旧批准。
源、目标 RPC 与公开 Recover 已接通受限 slice/root 路径；产品配置见宿主文档。carrier／外部边界
驱动、联系修复及完整独立进程故障矩阵仍待完成，不能据此称 P7.2 已交付。

父侧 `c.Transfers()` 及同一 `TransitionControlService` 提供 `BeginCarrierTransition`、`InspectCarrierTransition`、
`CancelCarrierTransition`。它们使用 [CarrierCommand／CarrierStatus](../contract/carrier.go)，
明确区分 parent（控制 owner）与 source（旧承载方）；不会把挂载锚移到目标。
Begin／Cancel 独立要求 parent + mount 范围的 `transition.commit`，Inspect 要求
`transition.inspect`，均复核实际 caller/view。普通挂载许可或 source export 不代替父侧授权。
Begin 固定原比较值、三方身份和指定联系，精确重试返回原 reservation；Inspect 返回当前父侧
进度。`carrier-revision` 只是该操作的有限比较值，不是源 fence、目标批准或安装证据。
取消使用原 reservation 比较；一旦父侧已允许派发 fence，不能凭网络无响应执行本地取消。
这三个方法已接入 gRPC 和 `client.Transitions()`，复用原用户会话与请求取消寿命，不新增服务、
配置开关、客户端恢复编排或私有状态。请求不接受自报 caller、fence 或目标接纳证明。
网络回包把 parent 与 source 明确分开；SDK 核对 parent、原始操作 bytes、mount、比较值种类
及进度，Begin 保留原 RESERVED，Cancel 返回 CANCELLED，Inspect 返回当前父侧状态。
取消后挂载比较修订更新，不能用旧 CAS 发起新操作；空间元素、承载方与联系不因此改变。
Store 写入不确定仍返回 INDETERMINATE，读取 Store 失败为 UNAVAILABLE，失去会话不披露结果。
上述控制 RPC 只负责父侧预留、检查及允许范围内的取消；完整三方网络恢复驱动仍待接续。

`TransitionParentService` 提供 `ReadCarrierPlan`／`ReadCarrierDecision`，SDK 入口为
`client.TransitionParent()`。只允许原绑定中的 source 或 target 以实际 TLS 证书回查 parent；
客户端填写的身份不是证明，普通用户会话或自报完成证据不能代替服务身份。
请求只带完整原绑定，parent 从自己的原操作读取 reservation／permit／取消及唯一 binding，
并在披露前再次核对同一实际参与方；没有决定时明确不可用，不把 reservation 当成决定。
回应保留原 OperationID 与 actor 的 bytes、三方 cut／revision、挂载比较与指定地址；
单个绑定上限 4 KiB、证据上限 16 KiB，接收端核对完整绑定和各 owner 的有限证据。
Store 重启后按原持久结果读取，不制造新批准，不使用 Ref 路由或缓存代替不可达的 parent。
这些 codec 只转换有限值，不保存事实、锁或 Store；三方状态仍由原 owner 独立裁决。
实际三方本地流程中的父侧回查已用真实 TLS 验证，源／目标命令尚非全网络调用；
完整公开驱动与独立三进程故障矩阵仍属于 P7.2 未完工作。

P6 的聚合 Read／List／Query 返回 `closure`：JSON 为规范 base64url digest，Protobuf 为 32 bytes。
它绑定原来源组合，续页不变；本地非聚合调用不返回此字段。该标记不是权限或离线完整性证明，
更不能把覆盖缺口变为 COMPLETE。聚合结果同时返回 `sourceGraph`，形状由
[来源证据 schema](read-source-evidence.schema.json)定义：披露授权来源的身份、cut／授权视图承诺、
父子挂载证据以及 Query 扫描覆盖承诺；不含地址、私有 basis、凭据或隐藏来源占位。
Query 节点的 `parents` 给出 scope 及命中路径上可披露父锚的 `parent`／`adjacency` 承诺，
绑定完整直接邻接页链，不给出子元素名单、页数或内部 continuation；固定查询的各页保持同一集合。
它是认证入口对原来源组合的声明，不是消费者可自行重算的几何证明；交付边界见网络合同第 8.5 节。

网络 proto 还定义 `ReadDelegationService`：已认证用户可签发目标限定、客户端证书绑定、有限期的只读委托。
它是网络授权使用方式，不新增本地 JSON 命令或另一套空间裁决；具体限制见网络合同第 8.1 节。

调用入口是 `core.Access().PublicCall(ctx, caller, canonicalJSON)`，其中 `core` 由
`ucupa-core/instance` 创建或打开；也可使用 Composition 的指定地址适配。
`caller` 由 Realm 的 `EstablishCaller`（凭据）或显式可信本地 `TrustLocalCaller` 建立，
不放入 JSON。这不是 HTTP/TLS 服务；不得把本地信任当作远端认证。

- [请求 schema](request.schema.json)、[响应 schema](response.schema.json)、[结构类型](types.schema.json)。
- [共同编码](../../shared/api/values.schema.json)；[可独立消费的正反例](conformance.json)。
- [公共合同](../../docs/architecture/public-access-and-observation.md)拥有权限、时序与失败语义。

JSON 请求和响应使用 exact UTF-8 RFC 8785；拒绝重复键、非规范数字/字节编码及多余字段。
schema 检查形状，不能替代规范字节、整数范围、owner/caller/role、空间关系及授权检查。
schema 中允许的 JSON number 在实现中还须为有限 binary64；深度字符串受有符号/无符号 64-bit 边界限制。
`upDepth=-1` 是字符串 `"-1"`；`limit` 才是 JSON 小整数。示例的 token 只是形状样本，不可直接当运行凭据。

## 调用顺序

1. `enter`：指定 `{boundary:{core,anchor},nextHop}` 和实际 `intent`，获取 handle。
2. `query`/`list`：返回路径和 read receipt；`read` 可用该 receipt 作 `basis`，按同一依据读元素及附着。
3. 需要观察时在首次读取设置 `prepareObserve:true`，随后 `begin-observe`、`poll`。
   第一次 Begin 前的提交已进入原 preparation 队列；不能用普通 receipt 追补旧观察。
4. `create-*`、`update-*`、`delete-*` 使用 handle 下的 element、最终父锚/局部位姿/范围与条件。
   更新要求 update 和 place 同时成立；共享原 owner 的提交与 OperationID。
5. `recover-outcome` 提供原 reference、OperationID、去掉过期 handle 的原规范请求；只恢复原回执。
6. `release-observe` 释放观察；`close` 释放 handle 及依赖上下文，不删除事实。

公共 Revision 是持久 owner 密钥的 HMAC；不是私有序号或历史查询入口。
闭包查询遇相关 mount 返回 PARTIAL/缺口；本 profile 不声称完成跨 Core 公共联邦。

## 配置与资源

`Config.PublicAccessPolicy` 不填即关闭公共入口；ContextTTL、ObservationTTL 均需显式正时长，
没有默认秒数。快照保留独立配置，关闭快照仍可 Observe。其余默认额度见公共合同第 6.5 节。
当前内核按每个上下文 2 KiB 加保存条件字节、每组通知 512 逻辑字节核算；
每个执行额外预留 `8×请求字节 + 1 MiB`，覆盖解析结构和最大返回批次。
这是保守准入单位，不是精确 RSS 承诺；快照投影仍由 P3 独立额度管理。
全部准入在分配前检查，慢消费者溢出变 GAP，不阻塞普通事实写入。

`go run ./cmd/ucupa` 包含真实公共调用示例；跨 owner 正反例在
[acceptance](../../acceptance/public_facet_test.go)，schema 由独立 Draft 2020-12 测试消费者核验。
