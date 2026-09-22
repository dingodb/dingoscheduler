# 节点仓库观察

打开 `/node-health`，在节点卡片点击“查看仓库”。仓库区域展示选中实例的仓库总数、已记录容量以及仓库来源、namespace、名称、类型、commit、挂载状态和源仓库修改时间；挂载错误文本随记录展示。

当前页每 10 秒从数据库重新读取，刷新保留选中节点和页码。查询失败会清除旧表格并显示错误，可手动重试。每页 20 条，支持上一页/下一页。大表格在自身区域内滚动，小屏可横向滚动查看其余列。

## 数据口径

只读原有 `dingospeed` 和 `repository` 表，以节点 ID 查询 instance_id，再严格限定 `repository.instance_id`。不调用 Speed、不触发持久化或回源下载，不修改任何表结构或旧记录。

这里展示数据库中的仓库投影，不是磁盘扫描结果；未发布或未入库的缓存不在列表中，上传仓库沿用 main 投影。容量为 `used_storage` 的逻辑大小合计，不代表物理磁盘用量或去重后大小。节点断连时，仍能读取数据库记录，但这不证明当前磁盘内容完整。若在线/离线节点共用同一个 instance_id，现有表只能提供该实例的共同记录。

旧 HF `org=Qwen,repo=demo` 显示为 `namespace=huggingface,repo=Qwen/demo`；上传 `org=dingo-local/alice,repo=team/demo` 显示为 `namespace=alice,repo=team/demo`。无法按新身份规则解码的历史记录仍展示原始值，并标为“未识别身份”。

## 只读接口

`GET /api/v1/nodes/:id/repositories?after=0&limit=20`

- id：已注册节点的正整数 ID。
- after：非负 int64 仓库 ID 游标，默认 0。
- limit：1..100，默认 20。按仓库 ID 升序，用额外一行判断下一页。
- 返回：`nodeId`、`instanceId`、`items`、`total`、`usedStorage`、`nextAfter`、`observedAt`。
- 每条 item：`id`、`namespace`、`repo`、`datatype`、`identityValid`、`commit`、`usedStorage`、`mountStatus`、`errorMessage`、`lastModified`、`updatedAt`。
- 仓库 `id` 和 `nextAfter` 是十进制字符串，避免浏览器损失 int64 精度；`nextAfter="0"` 表示无下一页。
- 参数错误 400，节点不存在 404，数据库不可用 503。成功结果禁用 HTTP 缓存，数据库查询超时为 5 秒。

总数与列表分别读取，持续入库或删除期间可能出现短暂差异，下一次轮询会更新。查询严格使用绑定参数，数据库文本通过 textContent 渲染。

## 本地验证

全量 Go 测试与编译通过。隔离 MySQL + 正式 Scheduler + Chromium 验证脚本为工作空间 `integration/verify_node_repositories.py`，覆盖身份解码、大整数游标、跨节点隔离、分页、参数校验、数据库变更自动同步、空数据、浏览器错误恢复、文本转义与移动端布局。该脚本仅对专用本地夹具插入和删除测试行，不可用于生产库。

本次没有部署到 10.220.70.213，也没有向该环境写入数据。
