# BENZHI_README

基于 Go 实现的TapeMaster Gate Web 项目，一款后端服务，已完整实现磁带数字化质量放行浏览器工作台，覆盖建档前检、采集与规则质检、人工发现、替代版本整改、批准冻结、不可变凭据签发验证和完整审计时间线。

## 项目说明
- 项目：benzhi-project-af64d1d2-d334-4d6b-9432-958b66f017fd
- 项目用途：已完整实现磁带数字化质量放行浏览器工作台，覆盖建档前检、采集与规则质检、人工发现、替代版本整改、批准冻结、不可变凭据签发验证和完整审计时间线。
- Go 工具链：`golang:1.22`
- 前端工具链：原生 HTML、CSS 和 JavaScript，由 Go 服务直接提供

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...
cd /app && GOTOOLCHAIN=local go run ./cmd/server -selfcheck -addr=127.0.0.1:19081
cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh
./build_benzhi_docker.sh benzhi-project-af64d1d2-d334-4d6b-9432-958b66f017fd-amd64 linux/amd64
./build_benzhi_docker.sh benzhi-project-af64d1d2-d334-4d6b-9432-958b66f017fd-arm64 linux/arm64
docker run -it benzhi-project-af64d1d2-d334-4d6b-9432-958b66f017fd-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/server -selfcheck -addr=127.0.0.1:19081`
