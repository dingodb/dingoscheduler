# Scheduler 测试辅助工具

`bootstrap/main.go` 供开发侧隔离校准使用，不是产品迁移或生产管理入口，也不属于外部验收镜像。它依赖本仓库的 Go module、内部 model 与已有 GORM/MySQL 依赖，不能脱离 Scheduler 仓库单独构建。

数据库模式只允许 `pipeline_*` 空 schema、固定 `mysql:3306` alias 和专用 pipeline 账户；这些字符串本身不证明环境安全，执行前仍需核验容器网络及下游真实归属。它会创建表，不能对共享、生产或已有业务数据运行。证书模式创建临时测试 CA/客户端/服务端材料并拒绝覆盖既有文件。不要为普通工具自测调用 bootstrap。

实际证书、账户和输出放本 testpipeline 下的 `.local/`、`.runs/` 或仓库外受控目录，禁止提交。Git 忽略与 Docker 构建排除分别维护；仓库根 `.dockerignore` 排除整个 `/testpipeline/`，包含 helper 源码及这些本地资产。没有修改 bootstrap 或业务 Dockerfile。

构建上下文必须是 Scheduler 仓库根目录。Makefile 使用 `docker build -f docker/Dockerfile-simple ... .`；如使用旧 `docker/Dockerfile`，也应从仓库根传入 `-f docker/Dockerfile` 和 `.`。其中 `COPY .. .` 仍按构建上下文处理，不能把父目录自动当作上下文。根排除规则适用于这两个 Dockerfile；没有 Dockerfile 专用 ignore 覆盖它。不要改用 ModelManager 父目录上下文并假定子仓库 `.dockerignore` 仍生效；其他上下文必须先另行核对排除规则。

这是静态构建边界修正，没有发现实际资产泄漏，也没有执行镜像构建或清理历史数据。外部验收工具位于同级 dingospeed 仓库的 testpipeline；其离线自测不需要运行本工具。
