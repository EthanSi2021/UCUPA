# Core gRPC 服务

本程序是持久 Core 的网络宿主，不是新的事实模块。协议、认证与失败语义由
[P6 合同](../../docs/architecture/network-access-contract.md)定义；交付状态见开发计划。
目前开放 P5 的单 Core 公共操作、可选只读身份传递、挂载控制及 Facet 关系控制；
配置公共联邦后还支持沿挂载的多来源查询／分页、路径读取和直接列举，尚未完成整个 P6。
服务专用证据经明确认证的 typed 端口传递，不直接暴露本地 Go 对象或 Realm 管理操作。

## 启动

先通过原 Core／Realm owner API 显式初始化两份独立 Store、主体、凭据和授权，再启动服务。
服务只打开 Store；不会初始化新数据库、自动 seed 权限或替换不匹配的身份。
配置文件不包含调用方 secret。私钥文件、Store 和配置需要由部署方保护；不得提交生产秘密。

```powershell
go build -o ucupa-core.exe ./cmd/ucupa-core
./ucupa-core.exe -config C:/private/ucupa/core.json
```

下面仅为配置形状示例：路径、身份、地址必须换成实际已初始化的数据，数值不是生产默认值。
路径按进程工作目录解释，推荐使用绝对路径。监听地址与对外地址可以不同；对外地址不能是 `0.0.0.0`。

```json
{
  "listen": "127.0.0.1:8443",
  "next_hop": "https://127.0.0.1:8443",
  "expected_core": "00000000-0000-4000-8000-000000000001",
  "realm_id": "my-realm",
  "realm_next_hop": "local:my-realm",
  "core_store": "C:/private/ucupa/core.db",
  "realm_store": "C:/private/ucupa/realm.db",
  "certificate_file": "C:/private/ucupa/server.pem",
  "private_key_file": "C:/private/ucupa/server.key",
  "session_ttl": "30s",
  "rpc_deadline": "5s",
  "handshake_timeout": "1s",
  "connection_lifetime": "1m",
  "shutdown_timeout": "3s",
  "max_sessions": 16,
  "max_sessions_per_caller": 8,
  "max_connections": 16,
  "max_concurrent_rpcs": 8,
  "max_streams": 8,
  "max_metadata_bytes": 8192,
  "max_response_bytes": 8388608,
  "context_ttl": "1m",
  "observation_ttl": "1m"
}
```

- `realm_profile` 显式选择授权装配：省略或 `local` 走本地 Realm kernel，`network` 走钉住的
  Realm 网络服务（见下节）；两种 profile 的字段互斥，错配在监听前拒绝启动。
- `realm_id` 与 `realm_next_hop`（local profile）声明本宿主实际打开的授权 Realm 及其本地联系地址；
  Core Store 的持久 Realm 联系必须与两者精确一致，配置只能发现误装配，不能覆盖或补建联系。
- 网络时间／数量限制必须明确填写正值；会话 TTL 必须为整数秒，连接寿命不得小于 RPC 时限。
- `snapshot_retention` 不填即不保留历史快照；填入正时长才开启，无默认秒数。
- 可选的 `public_max_contexts`、`public_max_contexts_per_caller`、`public_max_groups_per_observation`、
  `public_max_bytes`、`public_max_concurrent_reads` 直接装配已有 P5 公共资源政策，不复制其计算规则。
  不填时沿用 P5 的数量默认值；与可选快照不同。读取也会保留有限的 receipt，不能只调大会话容量。
- `max_response_bytes` 同时限制内部规范响应和网络消息；配置过小会明确报资源不足，不截断数据。
  大结果需要合理的查询批量及接收限制，不能仅凭请求很小假定响应也小。
- Ctrl+C／SIGTERM 停止接入并有限等待，之后关闭 owner。连接寿命耗尽只关闭连接，未过期会话可以重新连接。
  重启后会话与公共读取上下文失效，已持久化事实和操作回执保留。

## 网络 Realm profile（S2P3b）

需要让本宿主经远端 Realm 网络服务（S2P3a 交付的 `cmd/ucupa-realm`）完成授权与身份时，
在原配置上显式选择 network profile：

```json
{
  "realm_profile": "network",
  "realm_endpoints": [
    {
      "next_hop": "https://127.0.0.1:9443",
      "ca_file": "C:/private/ucupa/realm-ca.pem",
      "spki_sha256": "<Realm 服务公钥 SPKI SHA-256 的严格 base64url>",
      "service_certificate_file": "C:/private/ucupa/core-service.pem",
      "service_private_key_file": "C:/private/ucupa/core-service.key"
    }
  ]
}
```

这是配置片段，不是完整启动配置，数值仅为示例。`realm_store`／`realm_next_hop` 必须省略，
两种 profile 互斥。每项端点钉住 Realm 服务地址、TLS 根证书与 SPKI pin，并给出本 Core 在该
Realm 登记的 mTLS 服务身份；端点复用宿主 `rpc_deadline`／`handshake_timeout` 与消息上限，
重复或非法端点在监听前拒绝启动。

打开 Store 时先经当前持久联系在线首证，未通过前不对外服务；运行中的授权、复核、P6 源侧
身份与联系维护都经该连接在线完成，等待期间不持有事实锁，不可达一律失败关闭、不离线放行。
服务身份必须先在 Realm 侧被显式 admit，未接纳的身份得不到任何服务；配置不新增权限，
授权仍由 Realm 在线决定。验收见
[网络 Realm 联系进程测试](../../acceptance/network_realm_contact_process_test.go)与
[联系维护与锁证明](../../acceptance/network_realm_contact_maintenance_test.go)。

## 客户端

`ucupa-core/sdk/go` 的 `New(Config)` 接受明确 NextHop、预期 Core、可信 CA、预期服务密钥的 SPKI SHA-256，
以及调用／连接时限和接收上限。身份到密钥的绑定不包含地址，不能用于按 Ref 查找服务。
SDK 先完成 TLS 目标校验，再通过 `Establish(ctx, realmID, credentialID, secret)` 建立有限会话。
然后使用 `client.Core()` 返回的生成客户端调用 `Enter`、`ReadElement`、`QueryRegion` 等方法。
具体类型由 [core.proto](../../ucupa-core/api/core.proto) 和生成绑定提供，不另维护一套 SDK 方法副本。

调用者不能覆盖 SDK 的 metadata、codec、压缩、目标或重试选项；不通过环境代理连接，也不跟随重定向。
宿主只注册 Protobuf 编码，不注册任何压缩器或 JSON codec；没有 REST、gRPC-Web 或明文入口。
`Release` 释放会话，`Close` 关闭客户端连接，两者均不删除空间事实。

## 可选只读委托

需要让指定 Core 代调用方读取本目标时，在目标服务配置中加入 `read_delegation`；不填即关闭，无自动信任：

```json
{
  "read_delegation": {
    "client_ca_file": "C:/private/ucupa/actor-ca.pem",
    "client_spki_sha256": {
      "00000000-0000-4000-8000-000000000002": "<代办 Core 客户端公钥 SPKI SHA-256 的严格 base64url>"
    },
    "max_ttl": "10s",
    "max_grants": 64,
    "max_per_caller": 4
  }
}
```

这是配置片段，不是完整启动配置；数值为示例。时长必须是正整数秒，各数量为正，单主体上限不得超过全局上限。
代办证书需要可验证的客户端认证用途及相应私钥，目标同时检查 CA 链和上述身份到公钥的精确绑定。
此表不含地址，不是 Ref 路由；普通用户仍可只验证服务端 TLS 而不提供客户端证书。

1. 用户在发现可披露目标后，按自己的明确政策直接认证该目标，无须预登录全网 Core。
2. 通过 `client.Delegations().CreateReadDelegation` 指定 Reference、代办 Core、read/list/query 子集、观察开关和期限。
   目标以用户原权限建立限定入口，答复包含短期 token 和 handle，不包含用户 secret、session 或 CallerContext。
3. 代办方创建指向同一目标的 SDK client，配置 `Actor` 与 `ActorCertificate`，再 `UseReadDelegation(result)`。
   使用原 handle 调用生成的 `ReadElement`、`ListDirect`、`QueryRegion`；不能另 Enter、写入、恢复写结果或转授。
4. 用户可通过自己新的有效会话 `RevokeReadDelegation`。委托也会到期，且宿主重启后失效；重连不延长期限。

每次读取仍检查用户在目标的当前权限及凭据；不是用代办 Core 的服务权限读取用户内容。
不同委托有独立的运行内 caller，句柄／观察不能混用；允许观察也不能绕过 P5 的撤权重授 GAP。
原用户释放会话不等于撤销委托，委托仍以原始会话到期为最晚期限；需提前终止时显式撤销。
创建答复丢失只可能留下有限期名额，不意味着空间写入，也不由 SDK 自动重发创建。

这条链提供目标签发、代办使用及撤销；**单独启用它不等于启用 Core 自动跨挂载接力**，自动公共联邦还需下文的显式身份交换与 `federation` 配置。
目标与原用户的身份／信任政策尚不存在时明确失败，不自动学习或扩大信任。

## 显式身份签发与交换

需要此能力时，在原配置上增加以下字段；省略即关闭，不自动生成密钥或信任。
两侧还需显式 `read_delegation` 服务 peer CA/pin：签发方允许目标 Core 回查，目标允许入口 Core 交换和使用委托。

```json
{
  "identity_issuer": {
    "signing_key_file": "identity-private.pem",
    "max_ttl": "30s", "max_proofs": 100, "max_per_caller": 10
  }
}
```

密钥为单一 PKCS#8 Ed25519 私钥 PEM，与 TLS 身份分开管理；此片段不是完整启动配置，数值仅为示例。
目标的配置形态为：

```json
{
  "external_identity": {
    "max_ttl": "30s",
    "issuers": {
      "issuer-realm": {
        "signing_public_key_file": "issuer-public.pem",
        "next_hop": "https://issuer.example:443",
        "expected_core": "<明确的签发 Core UUID>",
        "ca_file": "issuer-ca.pem",
        "spki_sha256": "<严格 base64url SHA-256>"
      }
    }
  }
}
```

公钥为 PKIX Ed25519 PUBLIC KEY PEM。目标使用自己的配置 TLS 证书作为回查客户端身份证明，
证书必须适合 client authentication；地址、issuer、公钥和 Core pin 都不是从请求或挂载自动学来的。
回查使用已有明确 RPC／连接时间与消息上限，不另设隐式默认值。

调用顺序：入口服务 `Delegations().BeginIdentityChallenge` → 用户在签发方
`IdentityProofs().IssueIdentityProof` → 入口向目标 `Delegations().ExchangeIdentity` →
新 actor client `UseReadDelegation`。用户不需要获得隐藏目标的地址；该交换供自动联邦接线使用，单独配置交换不启用联邦查询。
只验证签名不能放行：目标每次使用还会回查签发方原凭据状态，再由自己的 Realm 授权。

## 结果与恢复规则

### 可选公共联邦入口

入口的 `federation` 配置直接复用指定地址、预期 Core 和 TLS 信任材料，不使用身份到地址的目录：

```json
{"federation":{"ttl":"20s","max_parts":64,"peers":[{"next_hop":"https://core-b.example:443","expected_core":"00000000-0000-4000-8000-000000000002","ca_file":"peer-ca.pem","spki_sha256":"<base64url-encoded-32-byte-pin>"}]}}
```

这是可选配置片段，数值仅为示例。省略整个字段时不装配自动公共联邦；显式配置必须有 1–256 个 peer，
TTL 为正整数秒，`max_parts` 为正数，限制此入口所有调用者并发保留的远端来源租约。
重复地址、本 Core 自身、无效网络地址／身份、缺失 CA／pin／客户端密钥均拒绝启动。
peer 是对关系中**指定地址**的信任配置，不决定空间归属，也不意味着启动时连接或扫描这些 Core。
入口协调多级读取时，需要明确配置实际接触的各来源地址；不会让中间 Core 凭自己的权限替原请求者继续转授。

入口还必须配置 `identity_issuer`。各来源需要 `external_identity` 接受原签发 Realm，
并用 `read_delegation` 的服务 CA／peer pin 验证入口身份；签发方也须允许来源回查原凭据。
这些信任配置不授予空间权限，目标 Realm 仍独立检查原请求者的实际操作。
来源租约受配置 TTL、原会话和来源实际期限共同限制，后续翻页不会自动续租或重新捕获状态。

owner 公共 Read 已能用原 receipt 作 `basis`，在原封闭来源快照上独立检查 Read 权限。
网络链通过来源一次性挑战取得独立 Read 请求证明，绑定原查询依据、目标元素和原请求者，
不能把 Query 委托直接改作 Read，也不刷新原快照或授权。Query／List 已支持多来源 prepared Observe，
各来源独立保留提交顺序，调用方使用入口的聚合游标。精确 Read preparation 也支持实际路径与目标的观察，
不观察无关元素；已有历史 receipt 的 Read 不能追溯补开 preparation。完整来源证据披露仍在收口。
完整性、混合快照政策与来源资源回收遵循[P6 合同第 8.5 节](../../docs/architecture/network-access-contract.md#85-公共联邦的来源投影与组合已交付)。

### 挂载批准交换

可选 `mount_approvals` 需要明确 `max_ttl`、`max_approvals`、`max_per_caller`，例如
`{"max_ttl":"30s","max_approvals":64,"max_per_caller":8}`。省略则关闭；不隐式启用。
连接身份使用显式 `read_delegation` 中的 CA／peer pin，但批准有独立容量和操作范围，不能用只读 grant 换取。
用户直接认证到目标后调用 `MountApprovals().CreateMountApproval`，指定唯一命令绑定及期限；
受托 Core 用配置 mTLS 身份调用 `CheckMountApproval`，目标重新验证原请求者凭据和授权 view。
原用户可 `RevokeMountApproval`；重启后旧批准无效。Check 返回主体只用于归因，不可作为网络身份证明再次转发。
`MountPeer()` 提供 mTLS 服务专用 PrepareMount／FinishMount／ReadMountDecision。
PrepareMount 同时携带精确请求与批准，核对原主体、命令 commitment 和原授权 view；
FinishMount／ReadMountDecision 用已确认关系的服务恢复权限，不依赖用户批准继续有效。
这些 peer 消息不向普通消费者开放，不能用 Bearer 或只读 grant 代替服务身份。
普通用户用 `MountControl().InspectMount` 取得公开比较 token，`DescribeMount` 取得需要的 B 精确批准 binding，
再 `ApplyMount` 提交原命令。服务通过可选 `mount_connections` 装配正式连接；每项明确
`next_hop`、`expected_core`、`ca_file`、`spki_sha256`，例如：

```json
{"mount_connections":[{"next_hop":"https://core-b.example:443","expected_core":"00000000-0000-4000-8000-000000000002","ca_file":"peer-ca.pem","spki_sha256":"<base64url-encoded-32-byte-pin>"}]}
```

不填则不开放普通挂载控制；显式空数组非法。连接使用本宿主证书作 mTLS 身份，不保管用户批准，
只按关系指定地址选取并核对 owner，不按 Ref 搜索或回退。每个目标服务仍需明确登记来源 peer 的证书信任。
启动后作一次受 RPC deadline 限制的恢复遍历：已持久决定可 Finish／ReadDecision；未决定写入不得用裸主体继续。
对端尚未可达时保留未完成操作和写门，既不宣称恢复成功也不自动 Abort；原调用方可重新认证后按原命令精确重试。
退出先取消并等待该遍历，再关闭服务、连接与 Store。

`MountContact().InspectMountContact`／`RepairMountContact` 用公开 contact token 维护指定方向的地址。
父侧需 MountControl，承载侧需 MountAccept，读联系需 MountRead。只修改本侧联系记录，不访问旧地址，
也不被正在等待的范围操作卡住。新地址必须能由明确配置的连接使用；不自动发现、重定向或按 Ref 查找。
修复地址后用原挂载命令精确重试，不能通过改地址变更事实归属或绕过权限。

验收见[实际 TLS 与权限反例](../../acceptance/network_mount_approval_test.go)和
[独立产品进程重启](../../acceptance/network_mount_approval_process_test.go)。
另有 [挂载 peer 的真实 TLS Prepare／决定读取／Finish](../../acceptance/network_mount_peer_test.go)：
父侧保留本地控制驱动，明确不声称整个请求从网络应用入口进入。
另有[普通控制入口黑盒](../../acceptance/network_mount_public_test.go)和
[双独立产品进程及强制重启](../../acceptance/network_mount_product_test.go)，后者关闭测试进程中的 owner 后才启动宿主。

### Facet 关系与双向联系

普通用户通过 `FacetControl().InspectFacet` 读取提案／联系比较 token，再通过 `ApplyFacet` 提交
propose、cancel、activate、detach 或 contact。首次接纳由用户向 Facet 的 Relationships 接口发起，
Facet 用固定服务回查本 Core 的当前提案；本 Core 的 `facet_services` 明确绑定该服务主体与 Facet。
固定服务仍需原 proposal-read／attachment-read 等独立权限，不能凭绑定获得用户权限。

激活时需要本 Core 向 Facet 读取接纳证据，配置例如：

```json
{"facet_connections":[{"next_hop":"https://facet.example:443","expected_facet":"00000000-0000-4000-8000-000000000003","ca_file":"facet-ca.pem","spki_sha256":"<base64url-encoded-32-byte-pin>","credential_file":"facet-service-credential.json"}]}
```

该文件恰含 `audience`、`credential_id`（16-byte base64url）、`secret`（32-byte base64url），
表示本 Core 在目标 Facet Realm 中的固定服务凭据，不是原用户的秘密。
目标 Facet 配置相应 `core_services` 并独立授予 acceptance-read，见 [Facet 宿主说明](../ucupa-facet/README.md)。
省略连接即不装配；显式空数组、重复地址、无效身份或信任材料拒绝启动。最多 256 项，
CA 文件最多 1 MiB、凭据文件最多 4 KiB；只按关系指定地址选取，不按 Ref 查找或回退。
对端离线时 contact 命令仍可修复本侧地址；换地址不换 proposal、接纳或事实归属。

[空关系开始的独立产品测试](../../acceptance/network_facet_relationship_product_test.go)覆盖网络建立、
内容读写、双向离线地址修复、双方重启和解除后拒绝读取。

### 单写交接

P7.1 在本机独立产品进程中完成验证；仅支持同一原始 Core Store 的显式顺序交接，
不是复制库或失联自动接管。`writer` 配置只含 `handoff` 开关与安装字段；交接能力的
启用状态在可信初始化时随原始 Store 持久化，之后每次打开必须精确一致，不能事后切换。
授权身份由原始 Store 的持久 Realm 联系约束，不再有自由格式的装配标签；
旧配置写 `authorization_binding` 会被严格解码器直接拒绝。启用交接的普通启动例如：

```json
{"writer":{"handoff":true}}
```

`handoff` 不是授权证明，只声明本次运行启用交接能力；与原 Store 记录不一致即拒绝启动。
不填 `writer` 时只能打开交接能力关闭的 Store，不能因此启用交接；公共准入／会话仍须按前文显式配置。
原调用者正常认证后，可用 SDK `Writer().InspectWriter` 查询 epoch／原决定，
用 `Writer().HandoffWriter` 提交原操作 ID、expected epoch 和目标安装秘密的 SHA-256 承诺。
Realm 必须分别授予本 Core exact AuthorityScope 上的 `ucupa.core.writer.inspect`／`ucupa.core.writer.handoff`；
普通空间权限或 read-delegation 不够。秘密不进入 RPC、日志或普通响应。

使用 `values.NewWriterInstallation(operation)` 生成独立随机安装资格，把其 32 个原始秘密字节
通过受控方式保管并只交给指定新实例。源只获得 Commitment。源 fence 成功后先停止旧产品进程，
待其释放原 Store，再以指定资格启动新实例，例如原操作 ID 为 `handoff-1`：

```json
{"writer":{"handoff":true,"install_operation":"aGFuZG9mZi0x","install_secret_file":"writer-installation.bin"}}
```

`install_operation` 是标准 JSON base64 的 1..128 原始操作字节，不是 UTF-8 限定名称；
`install_secret_file` 恰为 32 字节二进制秘密，不是 hex／base64 文本，两个字段必须一起给出。
新实例持独占锁重新核对同一 Store、owner、交接开关、操作与秘密后安装新 epoch，再对外监听。
普通 Open 不能接管待安装记录；错误开关、错误秘密、旧代次安装资格明确失败。
已安装代次的精确重启不再次推进 epoch；后续普通重启可去掉安装字段，`handoff` 保持原值。
响应丢失时重新认证并查询原 ID／精确重试，不能换目标、换 ID 或把超时当作未提交。
旧会话、快照、观察和在途读取不能借交接继续披露；新运行保留原身份、事实与原业务操作结果。

验证见 [Writer 产品进程测试](../../acceptance/writer_product_test.go)。新 private Store 格式没有旧 reader 或转换器，
也没有已复制 Store 的全局防双写能力；部署必须满足上述原始 Store 前提。

### Core 转移源控制（P7.2 增量）

宿主把实际 `owner.Transfers()` 装配到 `TransitionControlService`；SDK 通过
`client.Transitions()` 调用 BeginTransition／InspectTransition／CancelTransition。
这三个动作只管理源端原意图，尚不能驱动目标接纳、数据搬迁与安装，不代表完整 P7.2 已交付。

先用真实凭据建立用户会话；export 和 inspect 分别针对源 Core 的 exact Element boundary 授权，
普通读写、只读委托及固定服务身份不能代替它。转移 OperationID 不得复用普通写入的 ID。
Begin 保存原 expected、目标、最终 placement 和指定地址，不立即关闭源内容；
取消只在 fence 前有效。精确重试保持原 ID／原输入／原 expected，重启后需重新认证原调用者。
写入返回 INDETERMINATE 或网络断连时，不能认为未提交或自动换 ID；认证后按原 key Inspect，
或精确重试原 Begin／Cancel。SDK 校验返回归属而不编排业务。独立源进程与重启测试见
[转移源控制验收](../../acceptance/transition_product_test.go)；这不是完整多方转移测试。

### 目标转移预检批准（P7.2 实施中）

目标转移批准单独启用：配置 `transition_approvals`，显式给出 `max_ttl`（正整数秒时长）、
`max_approvals` 与 `max_per_caller`；缺省关闭，不沿用 Mount 批准额度或权限。
服务证书信任复用 `read_delegation` 中显式配置的 peers，但不会因此得到读取或转移权限。
SDK 的 `TransitionApprovals()` 只提供生成客户端：Create／Revoke 使用原用户会话，Check 使用
固定 source 的证书身份。批准只覆盖完整绑定的原转移，期限不长于原用户会话，重启后重新认证、
精确申请；旧 token 不持久化，也不是已接纳内容的存续依据。撤销运行期批准不取消已持久义务。
产品进程反例见[目标批准与重启](../../acceptance/transition_approval_process_test.go)。
当前仍没有完成源冻结到目标安装的整个公开推进链，不能据此宣称 P7.2 已交付。

### 受限源证据读取（P7.2 实施中）

宿主将同一 `owner.Transfers()` 装配到 `TransitionSourceService`；SDK 通过
`TransitionSource()` 使用生成的只读块／决定 RPC。只有 `read_delegation.peers` 中配置、
证书认证通过且与原操作 target 完全一致的服务可读；普通用户 bearer 不适用。
读取持久义务不需要复活已经到期的运行期批准，也不能借此发起新的导出或安装。
单块 64 KiB、总清单 1 MiB 上限之外，仍受宿主并发、响应大小和 RPC 时限约束。
接收适配器位于 `transport/grpc/transitionconn`，复用指定地址的 pinned SDK client，
要求调用方装配显式总拉取期限与并发限制；没有独立地址目录或 Ref 解析。
真实 TLS／源重开场景见[源证据网络读取测试](../../ucupa-core/tests/transfer/network_source_test.go)。
这不是完整网络转移交付，受限目标接纳、安装和公开恢复的产品装配见下文。

### 受限目标接纳与安装（P7.2 实施中）

Go 宿主可显式配置同一 owner 的 `TransitionTarget` 与 `TransitionConnections`，
并保留既有服务证书信任；缺一项或配置为另一 owner 时拒绝启动。SDK 的 `TransitionTarget()`
提供生成方法，`transitionconn.NewTargetConnection` 绑定同一指定地址、原 command 和原批准。
同一组明确地址连接用于向源读取依据和向目标推进；不保留旧单向装配名或另一套地址索引。

接纳先通过指定源 RPC 收齐不可变证据，再串行核对原批准与会话并调用实际 owner；准入锁内
只有本次预取结果和有界提交，没有网络等待，也不创建持久缓存或新的转移协调器。
批准与 grant 撤销影响首次接纳，不抹除已经落盘的义务；精确重试从原 Store 返回原结果。
双向 TLS、批准变化、并发、响应丢失及目标重开见
[目标网络路径测试](../../ucupa-core/tests/transfer/network_target_test.go)。其中包含本地恢复调用及
公开 Recover 等待期间释放源会话的反例，不代替 carrier／外部挂载组合。

### 指定连接与公开恢复（P7.2 受限源控制路径）

启动文件中的可选 `transition_connections` 显式提供 `peers`、`max_concurrent_pulls` 和
`pull_deadline`；不填不启用该网络推进能力。每个 peer 提供 `next_hop`、`expected_core`、
`ca_file`、`spki_sha256`，与现有 `read_delegation` 服务证书信任中的同一 peer/pin 一致。
peer 数和并发拉取数均为 1–1024，期限必须是明确正时长；重复地址、自身、错误身份／pin、
缺信任或非法限制在监听前拒绝。宿主负责关闭 SDK client，无自动发现、换地址或后台转移循环。

原用户先在目标取得 `TransitionApproval`，再向源的 `Transitions().RecoverTransition` 提交
原 `binding` 与 `approval_token`。源仍使用当前用户会话和原授权，服务证书只承担跨 Core 身份；
目标的原授权独立校验。请求按原操作推进一次，不在 SDK 内自动重试或重建批准。
源会话的释放、过期、宿主停止和请求取消都限制未提交步骤；已经落盘的义务不会因此删除。
两个真实产品进程的恢复、安装、重启及原结果一致性见
[转移产品测试](../../acceptance/transition_product_test.go)。carrier／外部边界与联系修复仍未完成。

### 父侧 carrier 控制（P7.2 受限增量）

同一 `TransitionControlService` 与 `client.Transitions()` 另提供 `BeginCarrierTransition`、
`InspectCarrierTransition`、`CancelCarrierTransition`，不增加服务配置开关。
连接目标是 parent Core，不是旧 source carrier；使用在 parent 建立的普通用户会话。
Begin／Cancel 需 parent + mount 的 `transition.commit`，Inspect 独立要求 `transition.inspect`。
这些入口不会代用户取得源／目标批准，也不会把预留成功当成源已关闭或目标已安装。
原挂载比较值可由既有 `MountControl().InspectMount` 获取，该挂载接口仍按自己的连接配置启用。

许可派发 fence 之前，父侧预留／取消不要求源或目标在线；已许可后不能凭失联直接取消。
Begin 精确重试返回原 reservation，Inspect 返回当前状态，Cancel 使用原 reservation 比较值。
重启后须重新认证；原始操作 bytes 和持久结果保留。取消更新挂载比较修订，但不变更空间
元素、承载方或地址；新操作不能重用旧比较值。
验证见[独立父侧产品重启测试](../../acceptance/carrier_product_test.go)。三方 carrier 的完整
认证恢复驱动仍待接续，不能以这三个端口代替完整 P7.2 交付。

### Carrier 父侧证据读取（P7.2 受限增量）

宿主将实际 `Transfers()` 装配给 `TransitionParentService`；复用既有服务 peer 信任配置，
未配置时读取明确不可用，不新增认证体系。源与目标使用自己的固定 TLS 证书，沿原指定 parent
地址调用 `client.TransitionParent().ReadCarrierPlan/ReadCarrierDecision`。
这两项是履行原持久操作的只读证据，不是用户权限授予，也不推进状态；原绑定不匹配就拒绝。
`transitionconn.Connections` 复用已有地址集合、并发与 deadline 限制；不自动选路或更换目标。
真实三方 Store + TLS 回查、父侧重启与错误证书／身份的验证见
[父侧网络测试](../../ucupa-core/tests/transfer/network_carrier_parent_test.go)。
这些测试只把父侧回查接到网络，源／目标控制仍为可信本地调用；不能据此宣称三方产品流程完成。

### 普通读写结果

结果要同时检查 RPC error 和 response 的 `result/failure`。完整接收的领域 failure 是 owner 的答复，
网络超时／断连不是提交失败的证据。可能已经派发的写入保持“结果未确定”，
按原 OperationID、原请求和原 CAS 调用 `RecoverOutcome`；不得换 ID 或更新 CAS 后自动重写。
同依据的 Read 必须使用原 query/list 的 handle 和 receipt，不能换成另一个 Enter 的 handle。

真实调用例见 [独立进程验收](../../acceptance/network_process_test.go)及
[空间读写与观察链](../../acceptance/network_core_test.go)。测试 CA、凭据和 Store 均临时生成，不是部署材料。
