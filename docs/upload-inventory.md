# 上传库存（第一阶段）

本功能只记录各 DingoSpeed 节点已经生效且可读取的本地上传文件，不参与远端仓库下载，也不提供权威 revision、集群发布事务或自动同步。

## 持久化边界

Scheduler 使用三个独立表：

- `upload_inventory_file`：由 `namespace + repo_type + repo + path + sha256` 唯一确定的文件；`size` 用于描述和一致性校验。
- `upload_inventory_holding`：文件与 Speed `instance_id` 的多对多持有关系。
- `upload_inventory_state`：每个节点最后接收的 epoch、单调序号、完整性、确认时间和错误信息。

表结构见 `upload-inventory-schema.sql`。Scheduler 启动时也会以 GORM `AutoMigrate` 创建这些新增表。原有 `repository`、`model_file_record`、`model_file_process` 等远端业务表不会被上传库存的写入、删除、恢复或对账修改。

## 收敛机制

DingoSpeed 每 30 秒以及本地上传、发布、删除、回收或恢复发生变化后扫描一次有效本地 revision，并将完整快照原子写入 `<repos>/.upload-inventory/snapshot.json`。快照 epoch 和 sequence 持久化，因此服务重启后仍能继续单调报告；本地操作已经成功但进程在通知前退出时，下一次启动扫描也会补报。

Speed 向 Scheduler 的 `IngestRepository` RPC 只发送一次性读取令牌。Scheduler 从已注册节点的内部 HTTP 端点读取对应完整快照。读取或校验失败不会改变现有持有关系；扫描不完整时只记录失败状态，也不会把未出现的文件解释为删除。只有更新的完整快照才能替换该节点的持有集合。重复、旧序号或旧 epoch 快照会被忽略。

节点失去心跳只影响查询结果中的 `nodeAvailable`，不会删除最后确认的持有关系。只有完整快照确认缺失，或本地明确操作后产生的完整快照，才移除该节点持有关系；某文件不再有任何节点持有后才删除全局文件记录。

## 查询接口

- `GET /api/v1/upload-inventory/repositories`
- `GET /api/v1/upload-inventory/files`，可选 `namespace`、`repoType`、`repo`、`instanceId`
- `GET /api/v1/upload-inventory/nodes/:instanceId/files`（响应的 `state` 即使文件列表为空也会给出节点最后库存确认状态）

文件结果同时包含仓库身份、相对路径、SHA256、大小、节点、最后确认时间、节点当前可用性，以及该节点最近库存是否完整确认。Speed 的 `/api/upload-inventory` 是 Scheduler 使用的令牌保护内部端点，不是面向用户的查询接口。

## 配置和启动

沿用现有 Scheduler 数据库配置和 Speed 注册/心跳配置，无需新增配置项。先启动 Scheduler，再按原方式启动 Speed；Scheduler 暂时不可用不影响 Speed 的本地上传、发布、删除、回收或恢复。连接恢复后，周期完整快照会自动对账。
