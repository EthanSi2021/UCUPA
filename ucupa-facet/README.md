# UCUPA Facet

Facet 保存 Element 的类型化扩展内容，不保存空间事实，也不决定谁拥有 Element。
Stage 1 · P4 的完整本地闭环已实现、通过完整验证，并按用户授权完成交付验收；
证据与提交记录见[开发计划](../docs/architecture/development-plan.md)，current 范围见架构权威清单。

实现依据：[Core／Facet 本地合同](../docs/architecture/facet-local-truth-and-core-lifecycle.md)。
工程组织遵守仓根 [ENGINEERING.md](../ENGINEERING.md)。

本地生命周期入口位于 [`instance`](instance/instance.go)：使用 `instance.New`、
`CreateDurable`、`OpenDurable` 获取句柄，通过 `handle.Namespace` 直接调用内容／联系操作，
通过 `handle.Close()` 关闭。实例只装配和管理生命周期，不转发一整套业务方法。
共享 Go 值、结果、错误及必要注入端口位于 [`contract`](contract/namespace.go)，
私有 token 表示仍由 [`internal/token`](internal/token/token.go) 唯一定义。
正式内容、接纳与提交／恢复归 [`internal/namespace`](internal/namespace/owner.go)，
有限上下文与观察资源归 [`internal/access`](internal/access/resources.go)。根目录不留旧入口、alias 或测试。
结构归位保留 P6 的比较、前置条件、类型和存储含义；当前重构的冻结验证与交付状态统一见开发计划。

## 访问链与公开操作

Core 提议附着 → Facet 接纳 → Core 激活。接纳本身不使内容可用。
Core 的 ACTIVE 附着返回 Facet 身份、指定地址和精确关系键；调用方按地址连接。
Facet 则按自身保存的反向地址回查原 Core，确认当前仍为 PRESENT＋matching ACTIVE。

- `AcceptAttachment`：首次接纳原子保存不可变 acceptance 和初始 Core 联系。
- `ReadAcceptance`：返回原 acceptance、当前反向联系和 Facet cut；精确不存在有独立证明。
- `UpdateCoreNextHop`：仅本地授权与联系 CAS，不要求 Core 在线。
- `Get`／`ListTypes`：单次 F0 校验，返回内容／完整类型集合及各 owner 的实际证据。
- `Put`／`Delete`：类型化条件写入、独立提交和精确操作重放。

Get／Put／Delete 使用 `FacetKeyScope(self, Element, TypeRef)`；
接纳、接纳读取、联系更新与 ListTypes 使用 `FacetElementScope(self, Element)`。
namespace 不是 key 权限的通配符，服务调用者不自动继承终端调用者的权限。

## 类型与一致性

P7.3a 起 `FacetTypeRef` 是任意 32-byte 值，不解释为地址、名称或版本。
Facet 不获取、解析类型制品，也不执行 schema 校验；Put 按 `(Element, TypeRef)`
保存有界、逐字节不变的不透明 Payload。payload 上限 1 MiB，
每个 Element 最多 256 个当前类型；ListTypes 按原始字节完整排序。

每个 fresh 内容操作都回查 Core。Get／ListTypes 先捕获 Facet 的 F0、接纳与联系，
锁外读取 Core，再确认 Facet cut 未变；否则返回 `ErrSnapshotChanged`，不拼接新旧结果。
Put／Delete 已取得合法 Core 证据后，允许与随后发生的 detach／delete 交错提交；
结果报告实际 Core cut，之后的 fresh read 则隐藏失效内容。不伪造跨 Store 原子性。

缺少接纳、指定地址不可达、身份不符，均不能冒充元素不存在。
元素不存在、NONE、DETACHED、非当前 owner、关系变化、key 不存在、快照变化和无权各自区分。
不提供物理 orphan Reconcile、自动重新附着、裸 Ref 查找、备用地址或内容迁移。

## 状态、提交与恢复

New 新建稳定 Facet 身份；CreateDurable 独占新建，OpenDurable 恢复原身份。
三个构造入口均要求明确给出 Realm 连接装配、CoreSource 与可信初始化的
RealmID＋明确的 local／network 地址：创建时先核验装配到达该 Realm 再绑定身份，重开只从
原 Store 恢复联系并核对；没有裸 Authorizer 注入，配置也不能覆盖或补建联系。
PublicAccessPolicy 不填即不启用公共 profile，不设默认 TTL，普通本地操作不因此关闭。

一个 owner 使用一把状态锁、一份状态、一条提交历史和一个 OperationID 命名空间。
普通授权与 Core 回查在锁外；Facet 不获取类型制品。授权 view 取自本 owner 的当前 Realm 联系，
network profile 在锁外取得／重验原 view，最终比较、ctx 和联系代次检查与 owner 提交／披露
在同一锁序列中，不持有事实锁等待 Realm。local profile 保留有界内核核验，端口不能反向调用 owner。
一次性 view 最后使用后释放，观察 view 跟随原有限上下文寿命；不另建句柄表或离线许可。
本地 Principal 操作仅供可信 local profile；网络调用使用已建立的 CallerContext，不把主体名称当身份。
内存与 bbolt 使用同一个确定性裁决和历史校验。本片验证与验收状态见开发计划。
接纳、Core 联系、持久 Realm 联系、内容、tombstones、cut、公开 Revision 密钥与操作回执共同持久化。
当前私有格式只识别 `ucupa.facet.local@4d8e1a6f3c9b7250e6a2f8d5c1b7e3a9`，不识别旧格式。

Store 保存一份 canonical 私有 document：身份／scope、当前事实投影和可重放事件。
打开时先只读校验，再以禁止创建的方式打开并复核身份；提交前后和 reopen 使用同一重放规则。
无不明格式转换、补生成身份、第二 reader 或损坏修复。历史当前不压缩；
这是可验证的本地实现，不代表已完成大规模存储或生产性能优化。

OperationID 为 1..128 字节，绑定调用者、owner、精确关系、操作、输入和条件。
精确重试先重新授权，再返回原回执，不再调用 Core，不回写旧地址。
明确提交前失败为 unavailable；响应丢失为 indeterminate，重试读取已保存结果。
调用者 ID、操作 ID、类型和 payload 均保持精确字节，不经 JSON 文本的 UTF-8 替换。

[公共保存／恢复反例](internal/namespace/public_store_recovery_test.go)分别检查保存前失败、发布后响应丢失，
以及磁盘已提交但保存结果不确定时的真实加载与恢复安装。最后一种只使用包内故障接缝，
不新增公开配置；事实 cut 改变时旧观察返回 GAP，未改变 cut 的回执恢复不误关观察，
精确重试不重复提交。完整验证状态见开发计划。

## 同原始 Store 的单写交接

P7.1 已实现并通过完整验证，交付／验收状态见开发计划；只落实
[P7 第 6 节](../docs/architecture/owner-transition-contract.md#6-同-owner-writer-handoff)的同域 profile。
通过 `handle.Namespace.InspectWriter/HandoffWriter` 调用真实 namespace owner，SDK 用 `Writer()`。
控制使用已启用的 PublicAccessPolicy 计量及可信授权 view，独立检查
`AuthorityScope(FacetRef)` 上的 `ucupa.facet.writer.inspect` 或 `ucupa.facet.writer.handoff`，不继承内容或 Element 权限。

`instance.Config.Writer` 与 Core 使用相同的受控启动值，但日志、锁与最终提交仍由本 Facet 持有。
源在原 Store 原子固定 final cut 并 fence；在途 Get 回查 Core 后也不能披露旧内容。
指定实例独占原 Store 并核对安装资格，内容、接纳、联系、原结果及 scope／密钥不因换运行而重写。
普通 Open 不抢占待安装记录，普通重启不推进 epoch，旧运行期句柄与会话不复活。
不增加第二个 namespace、不复制 Store，也不提前实施 P7.3 的 Payload／关系修订。
启动字段见 [Facet 宿主](../cmd/ucupa-facet/README.md#单写交接)。

## 验证范围

- 内存／持久化生命周期、CAS、无变化、独立类型、输入输出复制、重放、授权顺序。
- 类型边界、raw-byte 排序、完整 256 类型、不透明 payload 容量与逐字节不变性。
- 接纳、取消交错、反向联系、旧回执不恢复地址、F0 与并发写／改地址的屏障测试。
- 断连、错误身份、无接纳、所有 Core 状态分类与失效内容隐藏。
- 事务前故障／响应丢失、进程退出、reopen、格式／事实／历史损坏的只读拒绝。
- 真实 Composition 的一 Core→多 Facet、一 Facet←多 Core、双向地址维修及重启。

predecessor 的 existence／Reconcile 测试已按新合同接替：元素删除后的隐藏仍验证，
主动物理清理不再是产品行为；原自由类型／二桶编码样例由精确类型及单一 owner 历史损坏测试替换。
CAS、独立授权、字节复制、容量、并发、持久化、进程退出和 Store 隔离覆盖继续保留。
唯一完整交付入口为 `go run ./cmd/verify`；定向测试或 checks-only 不冒充完整通过。
