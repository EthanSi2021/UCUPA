# Facet 本地公共 profile

入口为 `facet.Namespace.PublicCall(ctx, caller, canonicalJSON)`，其中 `facet` 由
`ucupa-facet/instance` 创建或打开；caller 独立建立，不从请求字段推断。
先在 Core 的 ACTIVE attachment 取得 key 与 NextHop，按该指定地址连接预期 Facet。
公共 `access` 恰为 `{key:{core,facet,element,proposal},nextHop}`，proposal 是 Core 发行的不透明 token。
不会把 owner 私有 AcceptanceKey 的字节编码直接发给客户端，也不按裸 Ref 解析地址。

- [请求](request.schema.json)、[响应](response.schema.json)、[类型](types.schema.json)。
- [正反例](conformance.json)。
- [公共合同](../../docs/architecture/public-access-and-observation.md)定义全部时序、编码和失败语义。

`get`、`list-types` 可设置 `prepareObserve:true`；Facet 在 F0 准备本地队列，再由固定服务身份
在 Core 获取 C0 控制准备，最后重验 F0/联系/授权才交付。服务端控制 token 永不转交客户端。
`begin-observe`/`poll` 同时核验两侧连续性；Core detach/delete/联系变化即使没有 Facet 写入也会终止旧组合。
`release-observe` 最佳努力释放对端准备，失联时由其独立期限回收，不持有 Facet 锁等待网络。

`put` 的 expected 为 `{absent:true}` 或 `{revision:token}`；`delete` 直接提供 expected token。
`recover-outcome` 提供 access、OperationID 和原规范请求；只读自己的回执，不重新执行写入，
也不新增 Core 在线或外部类型前置。

## P6 类型化网络表示（实现中）

`facet.proto` 定义 Get、ListTypes、Put、Delete、RecoverOutcome、BeginObserve、
PollChanges、ReleaseObserve 八个内容 RPC。公共 AttachmentKey 与 Core 返回的附着引用共用
`shared/api/network.proto` 中的同一消息，不包含本地 AttachmentRevision／CoreCutEvidence 编码。
由 `go run ./cmd/protogen` 使用仓内固定工具版本生成 Go 绑定；完整验证重新生成并比较。

`transport/grpc` 当前实现类型化消息到原 PublicCall 的适配与防御性响应检查；调用身份由 host
的已认证会话边界提供，实际授权、接纳、精确重试、F0 与 Core 回查仍由原内核负责。
`transport/grpc.New` 已提供有界 TLS 网络服务与短期会话，`sdk/go` 提供指定目标、证书绑定、
会话和严格结果检查；通过真实 TCP/TLS 测试，不将 handler 当作完整网络证据。
独立 `cmd/ucupa-facet` host、固定服务身份回查 Core 及附着关系网络控制写链均已实现。
从空关系建立、双向联系修复到重启与解除的产品进程证据见 `acceptance/network_facet_relationship_product_test.go`。
配置和限制见[宿主说明](../../cmd/ucupa-facet/README.md)。
协议语料往返测试及真实 owner 适配测试分别在 `transport/grpc/representation_test.go` 和
`acceptance/network_facet_adapter_test.go`；后者不冒充 TLS／独立进程验证。
真实 TLS、会话容量、凭据撤销、目标错误与 transport 重建测试见 `acceptance/network_facet_test.go`；
它的 Core 回查仍是本地固定服务适配，不冒充跨进程反向连接。
新增 `acceptance/network_facet_reverse_test.go` 验证真实网络反向检查与观察，
`acceptance/network_facet_process_test.go` 验证两个独立产品进程、不同 Realm、持久恢复及服务会话续建。

## 类型与不透明内容

TypeRef 是任意 32-byte 值（公开编码 43 字符），不解释为地址、名称或版本。
P7.3a 起 Facet 不获取、解析或校验类型制品：Put 按 `(Element, TypeRef)` 保存有界、
逐字节不变的不透明 Payload，不做 schema 校验；payload 上限 1 MiB，每个 Element 最多
256 个当前类型。内容与回执使用同一事务；Get／ListTypes 只返回当前 key 的内容与类型集合。

## 配置与证明

New/CreateDurable/OpenDurable 均显式接收 Config。PublicAccessPolicy 不填则公共面关闭；
两项 TTL 均无默认秒数，其他有限默认额度见合同。每个上下文 2 KiB、每组通知 512 逻辑字节；
执行额外预留 `8×请求字节 + 4 MiB`，包含解码与最大内容返回批次，不只计算 payload。
不同 Facet 的上下文和 Core 服务依赖均独立计费、独立到期，不合成全局 cut。

完整可运行链见 `go run ./cmd/ucupa`；[公共黑盒](../../acceptance/public_facet_test.go)、
[并发/重启反例](../../acceptance/public_facet_failure_test.go)
分别证明访问行为、服务/客户端隔离和容量边界；单独通过 schema 不代表通过业务或安全门。
