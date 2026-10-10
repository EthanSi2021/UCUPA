# Facet gRPC 产品进程

`go run ./cmd/ucupa-facet -config <file>` 打开一份已经初始化的 Facet Store，
通过明确的 local／network profile 联系原 Realm；local 打开独立 Realm Store，network 不打开 Realm Store。
程序不创建事实、用户、凭据、授权或附着关系；身份与配置验证完成后才监听。
运行配置不是空间事实，不新增路由或 RealmAttachment。

配置格式（路径与身份必须换成实际值；所有运行期限与容量显式填写）：

```json
{
  "listen": "127.0.0.1:8444",
  "next_hop": "https://127.0.0.1:8444",
  "expected_facet": "00000000-0000-4000-8000-000000000001",
  "realm_id": "facet-realm",
  "realm_next_hop": "local:facet-realm",
  "facet_store": "facet.db",
  "realm_store": "facet-realm.db",
  "certificate_file": "facet.pem",
  "private_key_file": "facet.key",
  "session_ttl": "30s",
  "rpc_deadline": "5s",
  "handshake_timeout": "1s",
  "connection_lifetime": "1m",
  "shutdown_timeout": "2s",
  "max_sessions": 128,
  "max_sessions_per_caller": 8,
  "max_connections": 64,
  "max_concurrent_rpcs": 32,
  "max_streams": 32,
  "max_metadata_bytes": 8192,
  "max_response_bytes": 4194304,
  "context_ttl": "1m",
  "observation_ttl": "1m",
  "core_connections": [{
    "next_hop": "https://127.0.0.1:8443",
    "expected_core": "00000000-0000-4000-8000-000000000002",
    "ca_file": "core-ca.pem",
    "spki_sha256": "<32-byte SHA-256 as unpadded base64url>",
    "credential_file": "core-service-credential.json"
  }]
}
```

`realm_id` 与 `realm_next_hop` 声明本宿主实际打开的授权 Realm 及其本地联系地址；
Facet Store 的持久 Realm 联系必须与两者精确一致，配置只能发现误装配，不能覆盖或补建联系。

S2P3c 的 network profile 将 `realm_store`、`realm_next_hop` 删除，改为：

```json
{
  "realm_profile": "network",
  "realm_endpoints": [{
    "next_hop": "https://realm.example:8445",
    "ca_file": "realm-ca.pem",
    "spki_sha256": "<32-byte SHA-256 as unpadded base64url>",
    "service_certificate_file": "facet-realm-service.pem",
    "service_private_key_file": "facet-realm-service.key"
  }]
}
```

`realm_id` 仍是原 Store 中 RealmID 的断言。网络端点最多 256 项，每项固定地址、Realm TLS pin
与本 Facet 的 mTLS 服务证书；原持久地址必须有对应端点，Realm 必须显式准入该证书与本 Facet owner。
目标核对不等于准入；启动配置不能覆盖联系，local／network 不能混用或失败回退。
普通请求沿原 ctx 在线授权，Core 回查后重验原 view；最终事实边界不等待网络。
联系维护通过 `instance.OpenRealmMaintenance` 与已建立的 CallerContext 操作原独占 Store，
只有原地址不可用时才允许显式同 Realm 候选；拒权不能改问另一授权源。
可选的 `public_max_contexts`、`public_max_contexts_per_caller`、
`public_max_groups_per_observation`、`public_max_bytes`、`public_max_concurrent_reads`
映射到原 Facet PublicAccessPolicy；不改其语义。宿主配置文件最多 64 KiB；
Core 联系最多 256 项。超限拒绝启动，不截断。

固定服务凭据文件恰含三个字段：

```json
{"audience":"core-realm","credential_id":"<16 bytes as base64url>","secret":"<32 bytes as base64url>"}
```

这是目标 Core 的服务身份，不是内容用户的凭据。目标 Core 的启动配置须有对应的
`facet_services:[{"realm":"core-realm","principal":"facet-service","facet":"<Facet UUID>"}]`，
Realm 另外授予该主体具体元素上的附着读取、提案读取及所需附着观察权限。
绑定本身不授予权限。用户仍单独向 Facet 登录并由 Facet 的 Realm 授权内容操作。

反向联系只使用关系指定地址，并与已配置客户端的地址、Core 身份及 TLS pin 同时匹配。
未知地址、错身份、Core 离线或授权不足不会回退本地对象，也不会被解释为不存在。
服务调用按需读取本地凭据文件；同一有效会话不重复登录，到期或更换 CredentialID 后建立新会话，
不自动重发业务操作。新会话不能延续旧 preparation/cursor，因此旧观察按原规则 GAP。
目标提前重启时，旧会话可能先失败；明确拒绝的旧会话被清除，下一次显式调用才重新登录，
不无条件重试失败请求，也不把旧观察切换成新会话。

附着关系现在也可由网络建立：Core 用户 Propose → Facet 用户通过 `Relationships().AcceptAttachment`
提交公开 proposal、指定 Core 地址及操作 ID → Core 用户 Activate。Facet 首次接纳时用上述固定
服务身份向 Core 核对当前提案；激活时 Core 反向读取 Facet 的接纳证据。

反向服务读取还需在本 Facet 配置 `core_services`，例如：

```json
{"core_services":[{"realm":"facet-realm","principal":"core-service","core":"00000000-0000-4000-8000-000000000002"}]}
```

该绑定限制固定服务所能核对的 Core，不代替 Realm 的 `ucupa.facet.element.acceptance-read` 授权。
Core 对应配置 `facet_connections`，见 [Core 宿主说明](../ucupa-core/README.md)。
`Relationships().InspectAcceptance`／`UpdateCoreContact` 使用公开联系 token，Core 离线也可修复其地址。
关系写和重试仍检查原主体、原命令与 CAS；普通内容接口不接收私有服务证据。

### 单写交接

P7.1 的 Facet `writer` 配置、32-byte 私有安装文件、原操作 ID 的标准 base64、
先关闭旧进程再独占原 Store 的步骤，与 [Core 单写交接](../ucupa-core/README.md#单写交接)相同。
这里交接的是本 Facet 的原始 Store 与其 writer 能力，不是 Core 的 Store 或授权；
授权身份由本 Store 的持久 Realm 联系约束。初始 Store 的交接开关由可信初始化持久化，
本宿主不自动启用交接或修复旧格式。
SDK 使用 `Writer().InspectWriter/HandoffWriter`，Realm 分别授予本 Facet AuthorityScope 上的
`ucupa.facet.writer.inspect`／`ucupa.facet.writer.handoff`，内容权限不能替代。
原内容、接纳、联系、签名材料与原结果保留；旧会话和回查 Core 中的在途读取仍须停止披露。
单写控制不自动接管、不复制库，也不改变 Realm 授权与联系。现行 namespace replacement 与
远端 Realm profile 分别复用原转移和 S2P3 网络机制，不增加第二套 writer 或授权内核。

### 验证

验证独立产品进程：

```powershell
go test ./acceptance -run TestFacetAndCoreIndependentProducts -count=1 -v
go test ./acceptance -run TestFacetRelationshipIndependentProducts -count=1 -v
go test ./acceptance -run TestWriterFacetIndependentProducts -count=1 -v
go test ./acceptance -run 'TestFacet(Relationship|Lifecycle|Replacement)NetworkRealmProducts' -count=1 -v
```

前两项分别验证预置附着内容链，以及从空附着开始的网络建立、读写、双向离线联系修复、
双方重启和解除后拒绝读取；第三项验证原始 Facet Store 的单写交接、指定安装与原结果保留。
测试使用两个独立 Realm／Store 和实际 TCP/TLS，关闭初始化对象后启动两个产品程序；
不等于真实跨机器、自动联邦或独立第二实现互操作验证。全仓交付仍用 `go run ./cmd/verify`。
新增三项在同机启动独立 Realm／Facet 产品，以真实 gRPC/TLS 验证关系内容、恢复处置及
独立源／目标 Realm 的内容转移；产品进程不持有 Realm 内核对象。
