# Realm 网络服务产品进程

`go run ./cmd/ucupa-realm -init <bootstrap.json> -store <path>` 执行一次显式可信引导，
按引导文件创建新的持久 Realm Store；`go run ./cmd/ucupa-realm -config <file>` 打开
一份已经初始化的 Realm Store 并作为固定网络服务运行。程序不自动 seed 主体、不授予
权限、不把缺失 Store 当成重开成功；`-init` 与 `-config` 互斥。初始化输出只报告
RealmID，不输出任何凭据 secret。

引导文件最多 64 KiB，主体、绑定、凭据各最多 256 项，未知字段拒绝；每种 scope 只接受
本类的精确字段（facet 为 32 字节小写 hex，credential id／secret 分别为 16／32 字节小写 hex）：

```json
{
  "realm_id": "realm-a",
  "principals": ["admin", "user"],
  "bindings": [
    {"subject": {"realm": "realm-a", "principal": "admin"},
     "permission": "ucupa.realm.binding.put",
     "scope": {"kind": "realm", "realm": "realm-a"}},
    {"subject": {"realm": "realm-a", "principal": "user"},
     "permission": "ucupa.test.read",
     "scope": {"kind": "core_element", "authority": "<FactAuthorityRef UUID>",
               "element": "<ElementID UUID>"}}
  ],
  "credentials": [
    {"subject": "user", "audience": "realm-a",
     "id": "<16 bytes as lowercase hex>", "secret": "<32 bytes as lowercase hex>"}
  ]
}
```

scope kind 取值：`core_element`／`facet_element`／`anchor_closure`（authority＋element）、
`facet_key`（authority＋element＋facet）、`realm`、`authority`。绑定 subject 的 realm 必须
等于 realm_id，且主体已注册；凭据 audience 必须等于 realm_id。这是引导文件，不是远程
权限管理接口——服务运行后修改绑定仍走原 Realm 管理 API。

运行配置（路径与身份必须换成实际值；所有运行期限与容量显式填写，无零值默认）：

```json
{
  "realm_store": "realm.db",
  "realm_id": "realm-a",
  "listen": "127.0.0.1:8445",
  "next_hop": "https://127.0.0.1:8445",
  "certificate_file": "realm.pem",
  "private_key_file": "realm.key",
  "client_roots_file": "client-roots.pem",
  "admitted_services": [
    {"spki_sha256": "<32-byte SPKI SHA-256 as lowercase hex>",
     "owner": "<FactAuthorityRef UUID>"}
  ],
  "handle_ttl": "10s",
  "challenge_ttl": "10s",
  "rpc_deadline": "5s",
  "handshake_timeout": "1s",
  "connection_lifetime": "30s",
  "shutdown_timeout": "2s",
  "max_handles": 128,
  "max_handles_per_caller": 16,
  "max_challenges": 64,
  "max_challenges_per_owner": 16,
  "max_requirements": 16,
  "max_connections": 64,
  "max_concurrent_rpcs": 32,
  "max_response_bytes": 1048576,
  "max_streams": 32,
  "max_metadata_bytes": 8192
}
```

`realm_id` 必须与实际打开 Store 的身份一致，不一致拒绝启动。`handle_ttl`／
`challenge_ttl` 必须为整秒，`connection_lifetime` 不得小于 `rpc_deadline`。
`admitted_services` 把每个服务客户端证书 SPKI 绑定到唯一事实 owner：mTLS 接入资格
与用户对事实的权限分别判断，未获绑定的服务无法建立身份或授权。客户端根证书为
单个显式 PEM 束（最多 256 KiB），空或不肯解析的信任集直接拒绝。

`foreign` 为可选段：缺失时 foreign 身份建立端口保持 fail-closed，
`EstablishForeignCaller` 一律返回不可用；出现则必须完整，否则拒绝启动。每个受信
issuer 声明签名公钥（PEM ed25519）、钉住的 issuer Core 连接（`next_hop`＋
`expected_core`＋`ca_file`＋`spki_sha256` 小写 hex）与本服务在该 Core 侧的
delegation 身份（`verifier`）：

```json
{
  "foreign": {
    "max_proof_ttl": "30s",
    "issuers": {
      "identity-home": {
        "signing_public_key_file": "issuer-signing.pub",
        "next_hop": "https://127.0.0.1:8446",
        "expected_core": "<issuer Core FactAuthorityRef UUID>",
        "ca_file": "issuer-core-ca.pem",
        "spki_sha256": "<issuer Core service SPKI SHA-256 as lowercase hex>",
        "verifier": "<this service's delegation FactAuthorityRef UUID>"
      }
    }
  }
}
```

在线活性回查复用网络访问合同的既有 P6 proof 检查：receiver 用自己的服务证书
（须兼含 clientAuth EKU）连 issuer Core，issuer 侧 read_delegation 钉住该证书 SPKI
并映射到 `verifier` 身份。issuer 明确拒绝 proof 报告为未认证，issuer 不可达或
核对无法完成报告为不可用——没有本地或离线放行，proof 与 challenge 寿命也不延长。
最多 256 个受信 issuer；跨产品进程证据见
`acceptance/realm_foreign_process_test.go`。

`EstablishForeignCaller.actor` 是 P6 原签名绑定的接力 Core，由接收 Core 核验服务身份后传入；
不能将它与接收服务 owner 混为一谈。Realm 按该 actor 核对签名；接收 owner 仍来自 mTLS 的
`admitted_services`，challenge 与结果句柄仍绑定接收 owner。缺 actor 或与签名不符即拒绝，
不支持旧消息回退或仅按自报主体建立身份。

服务端句柄是有限运行内引用：绑定 RealmID、本次运行、接收 owner 与原 caller，
不跨 owner／caller／Realm／运行复用，不持久化；Realm 重启后旧句柄全部失效。
每次操作都在线核验，句柄不是可离线兑现的 Allow。

SDK 侧目标核验是发送凭据的先决条件：`CheckTarget` 通过前，客户端不发送用户
凭据或受保护授权正文；核验失败、未核验或连接重建后的旧通过结果都不放行。

### 验证

验证独立产品进程的启停、身份保持与旧运行句柄失效：

```powershell
go test ./cmd/ucupa-realm -count=1 -v
```

测试构建真实产品二进制并经实际 TCP/TLS 走 SDK 全链：引导、重复引导拒绝、
CheckTarget、身份建立、view 核验、强杀重启后 RealmID 保持、旧 caller／view 失效、
新身份正常工作，以及未配置 foreign 时 `EstablishForeignCaller` 保持 fail-closed。

跨产品进程的 foreign 身份链（一个 ucupa-core issuer 与一个 ucupa-realm receiver）：

```powershell
go test ./acceptance -run TestRealmProductForeignIdentityAcrossProcesses -count=1 -v
```

loopback 测试不等于真实跨机器或生产部署验证；全仓交付仍用
`go run ./cmd/verify`。
