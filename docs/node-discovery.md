# 节点自动发现接口

2026-09-22：Register 请求新增 managementUrl（字段 5）与 downloadUrl（字段 6）。
可选 HTTP(S) URL 随注册保存至附加表 node_endpoints，以 Scheduler 数字节点 ID 为主键。
服务启动自动创建该表，不修改模型文件、远端库存或官方版本。旧客户端不发送字段仍可注册。

`GET /api/v1/nodes/health?endpoints=true&after=0&limit=200` 在原健康视图上附加
managementUrl/downloadUrl，继续使用 nextAfter 游标。端点尚未上报的旧节点返回空字符串。
communication 表示注册通信有效性；onlineMode 表示节点运行模式，不能当成在线状态。
端点存储或健康查询失败返回 503，不返回假成功的空列表。

ModelFleet 使用唯一 Scheduler 的 HTTP 地址发现节点，用 gRPC 地址配置 Speed。
只读发现无需管理令牌，不返回任何节点凭据。
