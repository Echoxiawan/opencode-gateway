<div align="center">

# 🚪 OpenCode Gateway

**Proxy gateway for OpenCode Zen — OpenAI / Anthropic / Codex compatible API**

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go 1.24+](https://img.shields.io/badge/Go-1.24+-00ADD8.svg)](https://go.dev/)
[![No CGO](https://img.shields.io/badge/CGO-not%20required-green.svg)](#-quick-start)

*Use OpenCode Zen models from Claude Code, Codex CLI, Cursor, Cline, Roo Code, Kilo Code, OpenAI SDK, LangChain, Continue and any other OpenAI or Anthropic compatible tool*

[Models](#-supported-models) • [Features](#-features) • [Quick Start](#-quick-start) • [Configuration](#%EF%B8%8F-configuration) • [Client Setup](#-client-setup) • [Troubleshooting](#-troubleshooting)

🇬🇧 English • [🇨🇳 中文](docs/zh/README.md) • [🇯🇵 日本語](docs/ja/README.md) • [🇰🇷 한국어](docs/ko/README.md) • [🇪🇸 Español](docs/es/README.md) • [🇷🇺 Русский](docs/ru/README.md) • [🇧🇷 Português](docs/pt/README.md) • [🇮🇩 Indonesia](docs/id/README.md)

</div>

---

## 🤖 Supported Models

A single local endpoint exposing every model OpenCode Zen offers — including the **free tier**, which normally only works from inside the official OpenCode client.

**Free models** (no API key required, as of 2026-09-29):

| Model | Notes |
|-------|-------|
| `mimo-v2.6-flash-free` | MiMo flash — fast, agentic |
| `mimo-v2.5-free` | Previous MiMo free release |
| `space-bunny-free` | Lightweight general model |
| `longcat-2.5-preview-free` | Meituan multimodal reasoning |
| `nemotron-3-ultra-free` | NVIDIA Nemotron |
| `nemotron-3.5-lightning-free` | NVIDIA Nemotron, fast tier |
| `muse-spark-1.3-contributor-free` | Contributor tier |
| `muse-spark-1.2-contributor-free` | Contributor tier |
| `deepseek-v4-flash-free` | DeepSeek flash — *upstream availability varies* |
| `ling-3.0-flash-fin-free` | Ling flash — *upstream availability varies* |
| `jev-1.13-free` | TypeSafe System One |

**Paid models** (require a Zen API key, 72 available): the Claude, GPT, Gemini, Grok, DeepSeek, GLM, Kimi, MiniMax, Qwen and Big Pickle families.

```bash
# Always the authoritative list — free models are marked with "free": true
curl -s http://127.0.0.1:8787/v1/models
```

> ⚠️ **Free model availability is decided by Zen and changes without notice.** A `-free` model can work today and return `Model is unavailable` tomorrow — the list above is what Zen *registers*, not a guarantee it *serves*. Switch models, or add a Zen API key to use the paid tier.

---

## ✨ Features

| Feature | Description |
|---------|-------------|
| 🔌 **OpenAI-compatible API** | `POST /v1/chat/completions` — streaming and non-streaming |
| 🔌 **Anthropic-compatible API** | Native `POST /v1/messages` for Claude Code |
| 🔌 **OpenAI Responses API** | Native `POST /v1/responses` for the Codex CLI |
| 📋 **Model list** | `GET /v1/models` with free/paid markers |
| 🆓 **Free tier support** | Injects the OpenCode client signature — no CLI subprocess needed |
| 💳 **Paid tier support** | Direct forwarding with round-robin across multiple Zen keys |
| 📊 **Usage tracking** | SQLite records tokens, model, protocol and cost per request |
| 📝 **File logging** | Daily JSONL under `./logs/`, 7-day retention |
| 🖥️ **Web console** | Model cards, 30-day chart, filterable request log, config editor |
| 🔑 **Key management** | Generate, copy and rotate the gateway key from the console |
| 🪶 **Single binary** | Pure Go, no CGO, no Node.js, no database server |

---

## 🚀 Quick Start

```bash
# Build
go build -o opencode-gateway .

# Run (listens on 127.0.0.1:8787, free models work immediately)
./opencode-gateway
```

On first run the gateway creates `config.json` in the working directory and prints generated credentials once:

```
已生成配置文件: config.json
  Web 控制台密码: xxxxxxxxxxxx
  网关 API Key  : sk-gw-xxxxxxxxxxxxxxxxxx
  (可随时编辑该文件修改；控制台地址 http://127.0.0.1:8787/admin)
```

Open `http://127.0.0.1:8787/admin` and log in with that password — the console can change the password, the gateway API key and the Zen key, writing changes back to `config.json` immediately.

Prefer a different setup? Edit `config.json` by hand, point `-config /path/to/config.json` at another file, or set `OPENCODE_GATEWAY_NO_INIT=1` to skip generation entirely.

**Requirements:** Go 1.24+ to build. No CGO — the SQLite driver is pure Go.

---

## ⚙️ Configuration

There is exactly **one** config source: `config.json`, in the working directory by default. The web console edits it; you can also edit it by hand and restart.

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

| Field | Description |
|-------|-------------|
| `listen` | Bind address. Default `127.0.0.1:8787` (loopback only) |
| `api_key` | Key clients use to reach this gateway. Empty = loopback-only access |
| `admin_password` | Web console password. Empty disables `/admin` |
| `zen_keys` | Zen API keys for paid models. Multiple keys rotate. Empty = free tier only |
| `upstream` | Upstream base URL. Default `https://opencode.ai/zen` |
| `data_dir` | Usage database directory |
| `model_rules.model_blacklist` | Model IDs to hide from `/v1/models` and reject |

Environment variables override the file (handy for Docker / CI):

```bash
docker run -e OPENCODE_GATEWAY_API_KEY=sk-gw-... opencode-gateway
```

`OPENCODE_GATEWAY_LISTEN`, `_API_KEY`, `_ADMIN_PASSWORD`, `_UPSTREAM`, `_DATA_DIR`, and `OPENCODE_ZEN_KEYS` are supported. Variables that are not set leave the file value untouched.

> `config.json` and `data/` are both in `.gitignore` — they contain secrets and local usage data.

### What the web console can change

**Gateway API key** (with a generate button and one-click copy), **Zen paid keys** (with a validity check), **model blacklist**, and the **console password**. Changes apply immediately and are written back to `config.json`. Changing the console password invalidates existing sessions.

### Logging

Requests are written as JSONL to `./logs/gateway-YYYY-MM-DD.log`, one line per event, kept for **7 days**. Rejected requests are recorded too, with the reason. Logs stay on disk — the console's **Requests** tab is where usage is browsed.

```bash
tail -f logs/gateway-$(date +%F).log   # follow today
grep '"level":"error"' logs/*.log      # failures only
```

---

## 🔌 Client Setup

### Claude Code

```bash
export ANTHROPIC_BASE_URL=http://127.0.0.1:8787
export ANTHROPIC_AUTH_TOKEN=sk-gw-your-secret
export ANTHROPIC_SMALL_FAST_MODEL=mimo-v2.6-flash-free
claude
```

Free model names are not in Claude Code's built-in catalog, so passing one with `--model` makes the client exit as unrecognized. Map it with `modelOverrides` in `settings.json` instead:

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

Verified working. With a Zen API key configured you can use native names like `claude-sonnet-5` directly, no mapping needed.

### Codex CLI

`~/.codex/config.toml`:

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

`experimental_bearer_token` is the field verified to work; some Codex versions use an `env_keys` sub-table instead — check your version's docs.

Codex will warn `Model metadata ... not found. Defaulting to fallback metadata` because the model name is not in its built-in catalog. **It is only a warning** — Codex estimates the context window conservatively and everything still works. To silence it, declare the real window in the same file (`model_context_window = 200000`).

### OpenAI SDK / compatible tools

```python
from openai import OpenAI

client = OpenAI(base_url="http://127.0.0.1:8787/v1", api_key="sk-gw-your-secret")
resp = client.chat.completions.create(
    model="mimo-v2.6-flash-free",
    messages=[{"role": "user", "content": "Hello"}],
)
```

---

## 🧠 How It Works

The Zen free tier (`Authorization: Bearer public`) validates server-side that a request comes from the real OpenCode client. Reverse-engineered by differential testing:

1. `tools` must contain **both** a `bash` and a `read` tool (the bodies don't matter)
2. `stream` must be `true`
3. `x-opencode-session` must match `ses_<12 hex><14 base62>`
4. A specific `User-Agent` and `x-opencode-client: cli`

The gateway injects that signature and calls Zen's `/v1/chat/completions` directly, then translates the event stream into whichever protocol the client speaks. Non-streaming clients get the aggregated response. Paid models are forwarded verbatim according to the model→endpoint mapping in Zen's docs.

```
client (Claude Code / Codex / OpenAI SDK)
  │  /v1/messages | /v1/responses | /v1/chat/completions
  ▼
protocol layer   three protocols ⇄ one intermediate representation
  ▼
upstream layer   zen: inject free-tier signature / forward with Bearer key
  ▼
usage layer      SQLite accounting ─► web console
telemetry layer  daily JSONL logs (7-day retention)
```

---

## 📋 Troubleshooting

Start with the log — every request carries its model, status and reason, rejected ones included:

```bash
tail -20 logs/gateway-$(date +%F).log
```

The console's **Requests** tab shows the same, with `rejected` in the channel column.

| Symptom | Cause and fix |
|---------|---------------|
| `402 paid model requires a configured zen API key` | The model name is a paid one on Zen (no `-free` suffix). Use a free model or add a Zen API key |
| `400 Model is unavailable` | That free model is temporarily down upstream — switch models |
| `404 model "x" not found` | Not a Zen model ID. Check `GET /v1/models` |
| Claude Code exits with `unrecognized model` | Free model names aren't in its catalog; map with `modelOverrides` |
| Codex warns `Model metadata ... not found` | Cosmetic — affects context estimation only |
| Model cards say "loading model catalog" | Just started; the catalog fills in a few seconds |

---

## ⚠️ Limitations

- Free model availability is controlled by Zen and changes without notice
- No embeddings, image generation, or Responses retrieval/cancel
- A stream that has started is not retried on another path

---

## 📄 License

MIT
