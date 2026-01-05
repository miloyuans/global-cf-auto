# global-cf-auto

全球 Cloudflare 自动化工具（global-cf-auto）

一个用于从CF读取域名并检测到期时间，以及批量管理 Cloudflare Zone、导出 DNS、以及通过 Telegram 接收/触发操作的轻量级工具。适用于需要集中管理多个 Cloudflare 账号并通过机器人快速执行常用运维任务的场景。

**主要功能**

- 批量读取并监控 Cloudflare 域名（支持过期提醒与自动删除）。
- 通过 Telegram Bot 提供交互式命令：查询 DNS、添加域名、查看 NS、设置解析、删除域名、导出 CSV。
- 支持将 DNS 导出为 CSV（按账号/Zone/记录分行）。
- 内置 Cloudflare API 抽象（`cfclient`），方便替换为测试假实现。

**仓库结构（精简）**

- `main.go`：程序入口，初始化组件并启动监听。
- `config/`：配置加载与结构定义，读取 `config.yaml`。
- `cfclient/`：Cloudflare 客户端抽象与实现，提供 `Client` 接口。
- `internal/app/`：核心业务逻辑（通知、收集器、检查器等）。
- `telegram/`：Telegram 相关的 Sender、命令处理与导出逻辑。
- `callback/`：Telegram 回调处理（按钮交互）。
- `domain/`：域名仓库与管理辅助。
- `scheduler/`：调度逻辑（定期任务触发）。
- `tools/`：工具函数与小脚本。

**主要文件**

- 配置示例：`config.yaml`
- Telegram 命令处理：`telegram/commands.go` 和分割的命令文件（`dns_command.go`, `getns_command.go`, `setdns_command.go`, `status_command.go`, `delete_command.go`, `csv_command.go`）
- Cloudflare 抽象：`cfclient/client.go`

**配置（快速开始）**

1. 复制并编辑 `config.yaml`，至少设置 Telegram Bot Token 与 Cloudflare 账号：

```yaml
# 示例（需按实际填写）
telegram:
	token: "<BOT_TOKEN>"
	chat_id: 123456789

cloudflare_accounts:
	- label: "acc1"
		api_token: "<CF_API_TOKEN>"
```

2. 可选：将 `aws.txt` / `expiring_domains.txt` 用作导入或记录。

**运行**

构建并运行：

```bash
go build ./...
./global-cf-auto
```

程序会初始化 Cloudflare 客户端、Telegram Sender，并在配置的群组/私聊中监听命令与回调。

**Telegram 命令（机器人支持）**

- `/dns <domain.com>`：列出域名的 DNS 记录。
- `/getns <domain.com>`：查询域名是否存在，若不存在则尝试创建 zone 并返回 NS。
- `/status <domain.com>`：查看 Zone 状态（是否 paused）并显示操作人。
- `/delete <domain.com>`：触发删除确认，会发送带按钮的确认消息。
- `/setdns <domain> <type> <name> <content> [proxied] [ttl]`：创建或更新解析记录。
- `/csv <label|all>`：导出指定账号或全部账号的 DNS 为 CSV 并发送文件。
- `/originssl domain.com *`：生成源站15年的ssl证书,host 为domain.com 和  *.domain.com
**开发与测试**

- 运行所有测试：

```bash
go test ./...
```

- 本项目对 Cloudflare 操作使用 `cfclient.Client` 接口，测试中常用 fake 实现（见 `internal/app/notifier_test.go`）。

**扩展建议**

- 将 Telegram 发送器的实现抽离为可插拔模块（便于本地/远程部署）。
- 在 `csv` 导出中支持更多字段和过滤（按类型、TTL、是否代理）。
- 为长运行命令添加进度反馈与限流控制。
- 将查询的到期时间缓存起来，到期前不用再次查询，提高效率
- 将无法查询到的域名统一报出来
