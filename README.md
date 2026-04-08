# env-hub

轻量级环境配置工具站 — 通过 `curl` 一行命令完成 SSH 公钥安装、环境初始化等操作。

```bash
curl -fsSL https://env.moe/ssh | sh
```

## 功能

- **脚本分发**：通过 HTTP 路由返回可直接执行的 shell 脚本
- **SSH 公钥管理**：Web UI 增删公钥，`/ssh` 脚本自动安装
- **脚本在线编辑**：通过管理后台新增、编辑、删除脚本路由
- **Token 鉴权**：管理后台使用简单 Token 保护

## 快速开始

### Docker Compose（推荐）

```bash
cp .env.example .env
# 编辑 .env，设置 ADMIN_TOKEN
docker compose up -d
```

访问 `http://localhost:9800`，管理后台在 `http://localhost:9800/admin`。

### 使用预构建镜像

```bash
cp .env.example .env
# 编辑 .env，设置 ADMIN_TOKEN
docker compose -f docker-compose.prod.yml up -d
```

### 本地开发

```bash
export ADMIN_TOKEN=dev-token
export DATA_DIR=./data
go run .
```

## 环境变量

| 变量 | 必填 | 默认值 | 说明 |
|------|------|--------|------|
| `ADMIN_TOKEN` | 是 | - | 管理后台访问 Token |
| `LISTEN_ADDR` | 否 | `:9800` | 监听地址 |
| `DATA_DIR` | 否 | `./data` | SQLite 数据文件目录 |

## 路由

| 路径 | 说明 |
|------|------|
| `GET /` | 首页 |
| `GET /healthz` | 健康检查 |
| `GET /keys/main.pub` | SSH 公钥 |
| `GET /ssh` | SSH 公钥安装脚本 |
| `GET /admin` | 管理后台 |

脚本路由通过管理后台动态管理，访问时返回 `text/plain`，可直接 `curl | sh`。

## 部署建议

生产环境建议在前面挂一个反向代理（Caddy / Nginx）处理 HTTPS：

```
# Caddyfile 示例
env.moe {
    reverse_proxy localhost:9800
}
```
