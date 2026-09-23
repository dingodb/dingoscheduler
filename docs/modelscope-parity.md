# ModelScope 远端仓库管理补齐

2026-09-23。范围为 Scheduler 的远端下载管理；不修改上传库存、仓库范本或节点文件，不迁移数据库。

## 行为

- 未指定版本的仓库发现按来源取默认分支：HF 为 `main`，ModelScope 为 `master`。
- 缓存任务完成后使用任务保存的 commit 获取元数据；返回 commit 不一致或清单缺失时拒绝登记。
- 完整性核验逐一匹配节点、来源、仓库类型、仓库、路径、内容摘要和大小，不再以文件总数代替清单。缺少内容身份的 HF 元数据通过固定 commit 的递归 tree 补齐；Speed 负责上游分页。不会通过文件 GET 下载内容。
- 下载完成与仓库登记分别表达：登记失败保留任务完成状态，`errorMsg` 标明 `repository registration pending`，RPC 返回错误，进入 Speed 既有持久化通知重试。成功重试清除错误。
- 同一节点仓库更新原记录并替换标签，保留数据库 ID、挂载状态与节点关系；重复回报不重复插入。旧任务迟到回报不会覆盖编号更新的已完成任务。
- 不再因为 Scheduler 存在同仓库历史任务而拦截创建；由 Speed 根据当前 commit 判断运行中、已缓存或创建新任务，Scheduler 透传结果。

## 凭据

```yaml
scheduler:
    modelscopeToken: "" # 私有仓库需要配置所属 ModelScope 账号的凭据
```

ModelScope 的创建、恢复、仓库登记及默认挂载使用该配置。空值表示匿名请求，不回退到 HF token。HF 保持原有 `hf_token` 数据库选择方式。挂载请求显式提供的 `token` 仍优先使用。停止和实时进度查询不发送上游凭据。

配置修改需在后续部署或服务重启时生效；本次没有修改运行环境或配置真实凭据。

## 查询兼容

仓库列表 `/api/v1/repositories` 和任务列表 `/api/v1/cacheJob/list` 增加可选参数 `namespace=modelscope` 或 `namespace=huggingface`，也支持具体上传 namespace。过滤发生在数据库分页和计数之前；省略参数保持混合列表行为。

普通仓库列表/详情与任务列表新增统一身份字段：

```json
{
  "namespace": "modelscope",
  "fullRepo": "owner/demo",
  "repositoryId": "modelscope/owner/demo"
}
```

`repositoryId` 是带 namespace 的业务标识，仍需结合 `repoType/datatype` 区分类型；数据库数字 `id` 不变。为兼容已有调用方，不改旧 `org/repo/orgRepo` 含义。任务列表分页改为标准 OFFSET/LIMIT，并使用 ID 作为相同创建时间的排序补充。

`POST /api/persistRepo` 可在明确指定仓库时提供 `commit`，用于人工重试固定版本登记。没有指定 commit 的待登记仓库优先使用最新已完成任务的 commit，没有完成任务时才按来源默认分支发现。旧 `offVerify` 显式选项继续保留；任务完成通知不再开启它。

## 验证

新增 DAO 与 service 测试使用临时 SQLite 数据库及本机临时 HTTP 端口，覆盖：

- ModelScope 默认 `master`，逐路径校验而非数量比较。
- 元数据服务失败后补报，任务完成事实及登记错误回读。
- commit 不匹配、旧内容冒充新内容时拒绝登记。
- 重试幂等、新版本原位更新、旧通知不回退新版本。
- HF/ModelScope 凭据隔离及 ModelScope 匿名访问。
- 两来源同名仓库的筛选、分页、计数及统一身份字段。
- 已存在任务仍转交 Speed 决定复用，并透传 disposition。
- HF 固定版本 tree 的 LFS 内容身份解析。

使用只读源码挂载、禁用外网的 Linux Go 1.24 容器执行 `go test ./...`。本机默认 Go 1.26 与 sonic 不兼容；Windows Go 1.23.1 还受既有 Linux `syscall.Stat_t` 代码限制，因此未改项目依赖或平台代码来绕过。

本次验证为真实 DAO/HTTP 调用及隔离数据库测试，上游和 Speed 响应使用受控 fixture；未连接真实 ModelScope 私有仓库，未进行真实多节点下载或浏览器验收。

## 尚未扩展的展示信息

当前 Speed 的 ModelScope 元数据适配只提供仓库版本、文件清单和容量，未提供点赞、下载数、标签、任务分类及组织头像。本次不把缺失信息伪造成上游统计，也不根据 HF 标签体系猜测 ModelScope 分类；这些展示字段仍需确认实际上游接口及映射后单独补充。
