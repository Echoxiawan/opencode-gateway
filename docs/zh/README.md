<div align="center">

# 🚪 OpenCode Gateway

**把 OpenCode Zen 代理为 OpenAI / Anthropic / Codex 兼容 API 的网关**

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](../../LICENSE)
[![Go 1.24+](https://img.shields.io/badge/Go-1.24+-00ADD8.svg)](https://go.dev/)
[![No CGO](https://img.shields.io/badge/CGO-not%20required-green.svg)](#-快速开始)

*用 Claude Code、Codex CLI、Cursor、Cline、Roo Code、Kilo Code、OpenAI SDK、LangChain、Continue 以及任何 OpenAI / Anthropic 兼容工具调用 OpenCode Zen 的模型*

[模型](#-支持的模型) • [功能](#-功能) • [快速开始](#-快速开始) • [配置](#%EF%B8%8F-配置) • [客户端接入](#-客户端接入) • [故障排查](#-故障排查)

[🇬🇧 English](../../README.md) • 🇨🇳 中文 • [🇯🇵 日本語](../ja/README.md) • [🇰🇷 한국어](../ko/README.md) • [🇪🇸 Español](../es/README.md) • [🇷🇺 Русский](../ru/README.md) • [🇧🇷 Português](../pt/README.md) • [🇮🇩 Indonesia](../id/README.md)

</div>

---

## 🤖 支持的模型

一个本地端点，暴露 OpenCode Zen 的全部模型 —— 包括**免费层**，而免费层通常只能在官方 OpenCode 客户端内部使用。

**免费模型**（无需 API Key，截至 2026-09-29）：

| 模型 | 说明 |
|-------|-------|
| `mimo-v2.6-flash-free` | MiMo flash —— 快，适合智能体任务 |
| `mimo-v2.5-free` | 上一版 MiMo 免费模型 |
| `space-bunny-free` | 轻量通用模型 |
| `longcat-2.5-preview-free` | 美团多模态推理 |
| `nemotron-3-ultra-free` | NVIDIA Nemotron |
| `nemotron-3.5-lightning-free` | NVIDIA Nemotron 快速档 |
| `muse-spark-1.3-contributor-free` | Contributor 档 |
| `muse-spark-1.2-contributor-free` | Contributor 档 |
| `deepseek-v4-flash-free` | DeepSeek flash —— *上游可用性会变动* |
| `ling-3.0-flash-fin-free` | Ling flash —— *上游可用性会变动* |
| `jev-1.13-free` | TypeSafe System One |

**付费模型**（需要 Zen API Key，共 72 个）：Claude、GPT、Gemini、Grok、DeepSeek、GLM、Kimi、MiniMax、Qwen 以及 Big Pickle 系列。

```bash
# 永远以这里为准 —— 免费模型带 "free": true 标记
curl -s http://127.0.0.1:8787/v1/models
```

> ⚠️ **免费模型的可用性由 Zen 决定，随时可能变化。** 某个 `-free` 模型可能今天能用、明天返回 `Model is unavailable` —— 上表列出的是 Zen **登记**的模型，不代表它**实际提供服务**。换一个模型，或配置 Zen API Key 走付费通道。

---

## ✨ 功能

| 功能 | 说明 |
|---------|-------------|
| 🔌 **OpenAI 兼容 API** | `POST /v1/chat/completions` —— 支持流式与非流式 |
| 🔌 **Anthropic 兼容 API** | 原生 `POST /v1/messages`，供 Claude Code 使用 |
| 🔌 **OpenAI Responses API** | 原生 `POST /v1/responses`，供 Codex CLI 使用 |
| 📋 **模型列表** | `GET /v1/models`，带免费/付费标记 |
| 🆓 **免费层支持** | 自动注入 OpenCode 客户端签名 —— 无需 CLI 子进程 |
| 💳 **付费层支持** | 直连转发，多个 Zen Key 轮换 |
| 📊 **用量统计** | SQLite 记录每次请求的 tokens、模型、协议与费用 |
| 📝 **文件日志** | 按天写 JSONL 到 `./logs/`，保留 7 天 |
| 🖥️ **Web 控制台** | 模型卡片、30 天图表、可筛选的请求明细、配置编辑 |
| 🔑 **密钥管理** | 在控制台生成、复制、轮换网关 Key |
| 🪶 **单一二进制** | 纯 Go，无 CGO、无 Node.js、无需数据库服务 |

---

## 🚀 快速开始

```bash
# 构建
go build -o opencode-gateway .

# 运行（监听 127.0.0.1:8787，免费模型开箱可用）
./opencode-gateway
```

首次运行会在工作目录生成 `config.json`，并打印一次随机生成的凭据：

```
已生成配置文件: config.json
  Web 控制台密码: xxxxxxxxxxxx
  网关 API Key  : sk-gw-xxxxxxxxxxxxxxxxxx
  (可随时编辑该文件修改；控制台地址 http://127.0.0.1:8787/admin)
```

打开 `http://127.0.0.1:8787/admin` 用该密码登录 —— 控制台可以改密码、网关 API Key 和 Zen Key，改动会立即写回 `config.json`。

想换个方式？直接手改 `config.json`，或用 `-config /path/to/config.json` 指定其他位置，或设 `OPENCODE_GATEWAY_NO_INIT=1` 完全跳过生成。

**环境要求：** 构建需要 Go 1.24+。不需要 CGO —— SQLite 驱动是纯 Go 实现。

---

## ⚙️ 配置

配置只有**一个**来源：`config.json`，默认在工作目录。Web 控制台可以编辑它，你也可以手改后重启。

```json
{
  "listen": "127.0.0.1:8787",
  "api_key": "sk-gw-...",
  "zen_keys": [],
  "admin_password": "...",
  "upstream": "https://opencode.ai/zen",
  "data_dir": "data",
  "timeout": { "request_seconds": 300, "refresh_seconds": 300 },
  "model_rules": { "allow_paid": true, "model_blacklist": [] }
}
```

| 字段 | 说明 |
|-------|-------------|
| `listen` | 监听地址。默认 `127.0.0.1:8787`（仅本机） |
| `api_key` | 客户端访问本网关使用的密钥。留空 = 仅限本机访问 |
| `admin_password` | Web 控制台密码。留空则禁用 `/admin` |
| `zen_keys` | 付费模型的 Zen API Key。多个 Key 会轮换。留空只用免费层 |
| `upstream` | 上游地址。默认 `https://opencode.ai/zen` |
| `data_dir` | 用量数据库目录 |
| `model_rules.model_blacklist` | 要从 `/v1/models` 隐藏并拒绝的模型 ID |

环境变量可以覆盖文件（适合 Docker / CI）：

```bash
docker run -e OPENCODE_GATEWAY_API_KEY=sk-gw-... opencode-gateway
```

支持 `OPENCODE_GATEWAY_LISTEN`、`_API_KEY`、`_ADMIN_PASSWORD`、`_UPSTREAM`、`_DATA_DIR` 和 `OPENCODE_ZEN_KEYS`。未设置的环境变量不会影响文件里的值。

> `config.json` 和 `data/` 都在 `.gitignore` 里 —— 它们含密钥和本地使用数据。

### Web 控制台能改什么

**网关 API Key**（带生成按钮和一键复制）、**Zen 付费 Key**（带有效性验证）、**模型黑名单**、**控制台密码**。改动立即生效并写回 `config.json`。修改控制台密码会使已登录会话失效。

### 日志

请求以 JSONL 写入 `./logs/gateway-YYYY-MM-DD.log`，每行一个事件，保留 **7 天**。被拒绝的请求也会记录，并带上原因。日志只落盘，用量请在控制台的「请求明细」页查看。

```bash
tail -f logs/gateway-$(date +%F).log   # 跟踪今天的日志
grep '"level":"error"' logs/*.log      # 只看失败
```

---

## 🔌 客户端接入

### Claude Code

```bash
export ANTHROPIC_BASE_URL=http://127.0.0.1:8787
export ANTHROPIC_AUTH_TOKEN=sk-gw-your-secret
export ANTHROPIC_SMALL_FAST_MODEL=mimo-v2.6-flash-free
claude
```

免费模型名不在 Claude Code 的内置目录里，直接用 `--model` 指定会让客户端以 unrecognized model 退出。改用 `settings.json` 里的 `modelOverrides` 映射：

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "http://127.0.0.1:8787",
    "ANTHROPIC_AUTH_TOKEN": "sk-gw-your-secret",
    "ANTHROPIC_MODEL": "mimo-v2.6-flash-free",
    "ANTHROPIC_SMALL_FAST_MODEL": "mimo-v2.6-flash-free"
  },
  "modelOverrides": {
    "mimo-v2.6-flash-free": { "behavesAs": "claude-sonnet-4" }
  }
}
```

已实测可用。配置 Zen API Key 后可直接使用 `claude-sonnet-5` 等原生模型名，无需映射。

### Codex CLI

`~/.codex/config.toml`：

```toml
model_provider = "opencode-gateway"
model = "mimo-v2.6-flash-free"

[model_providers.opencode-gateway]
name = "opencode-gateway"
base_url = "http://127.0.0.1:8787/v1"
wire_api = "responses"
requires_openai_auth = false
experimental_bearer_token = "sk-gw-your-secret"
```

`experimental_bearer_token` 是实测可用的字段；部分 Codex 版本改用 `env_keys` 子表 —— 以你所用版本的文档为准。

Codex 会警告 `Model metadata ... not found. Defaulting to fallback metadata`，因为模型名不在它的内置目录里。**这只是警告** —— Codex 会用保守值估算上下文窗口，功能不受影响。想消除它，可在同一文件里声明真实窗口（`model_context_window = 200000`）。

### OpenAI SDK / 兼容工具

```python
from openai import OpenAI

client = OpenAI(base_url="http://127.0.0.1:8787/v1", api_key="sk-gw-your-secret")
resp = client.chat.completions.create(
    model="mimo-v2.6-flash-free",
    messages=[{"role": "user", "content": "你好"}],
)
```

---

## 🧠 实现原理

Zen 免费层（`Authorization: Bearer public`）会在服务端校验请求是否来自真实的 OpenCode 客户端。通过差分实验逆向出的规则：

1. `tools` 必须**同时**包含 `bash` 和 `read` 工具（内容无关）
2. `stream` 必须为 `true`
3. `x-opencode-session` 必须匹配 `ses_<12 位 hex><14 位 base62>`
4. 特定的 `User-Agent` 与 `x-opencode-client: cli`

网关注入该签名后直连 Zen 的 `/v1/chat/completions`，再把事件流转译成客户端使用的协议。非流式客户端会收到聚合后的完整响应。付费模型按 Zen 文档里的模型→端点映射原样转发。

```
client (Claude Code / Codex / OpenAI SDK)
  │  /v1/messages | /v1/responses | /v1/chat/completions
  ▼
protocol 层   三种协议 ⇄ 一份统一中间表示
  ▼
upstream 层   zen：注入免费层签名 / 用 Bearer Key 转发
  ▼
usage 层      SQLite 记账 ─► Web 控制台
telemetry 层  按天 JSONL 日志（保留 7 天）
```

---

## 📋 故障排查

先看日志 —— 每条请求都带模型、状态和原因，被拒的也在内：

```bash
tail -20 logs/gateway-$(date +%F).log
```

控制台的「请求明细」页能看到同样的内容，被拒请求的通道列显示 `rejected`。

| 现象 | 原因与处理 |
|---------|---------------|
| `402 paid model requires a configured zen API key` | 模型名在 Zen 属于付费模型（没有 `-free` 后缀）。换免费模型，或配置 Zen API Key |
| `400 Model is unavailable` | 该免费模型上游暂时不可用 —— 换一个模型 |
| `404 model "x" not found` | 不是 Zen 的模型 ID。用 `GET /v1/models` 查 |
| Claude Code 退出并报 `unrecognized model` | 免费模型名不在它的目录里；用 `modelOverrides` 映射 |
| Codex 警告 `Model metadata ... not found` | 仅提示性质 —— 只影响上下文估算 |
| 模型卡片显示「正在加载模型目录」 | 刚启动；模型目录几秒内自动填充 |

---

## ⚠️ 限制

- 免费模型的可用性由 Zen 控制，随时可能变化
- 不支持 embeddings、图像生成，以及 Responses 的检索/取消
- 已开始的流式请求不会换路径重试

---

## 📄 许可证

MIT
