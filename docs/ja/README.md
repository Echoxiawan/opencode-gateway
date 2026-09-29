<div align="center">

# 🚪 OpenCode Gateway

**OpenCode Zen を OpenAI / Anthropic / Codex 互換 API としてプロキシするゲートウェイ**

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](../../LICENSE)
[![Go 1.24+](https://img.shields.io/badge/Go-1.24+-00ADD8.svg)](https://go.dev/)
[![No CGO](https://img.shields.io/badge/CGO-not%20required-green.svg)](#-クイックスタート)

*Claude Code、Codex CLI、Cursor、Cline、Roo Code、Kilo Code、OpenAI SDK、LangChain、Continue など、OpenAI / Anthropic 互換のあらゆるツールから OpenCode Zen のモデルを利用できます*

[モデル](#-サポートされているモデル) • [機能](#-機能) • [クイックスタート](#-クイックスタート) • [設定](#%EF%B8%8F-設定) • [クライアント設定](#-クライアント設定) • [トラブルシューティング](#-トラブルシューティング)

[🇬🇧 English](../../README.md) • [🇨🇳 中文](../zh/README.md) • 🇯🇵 日本語 • [🇰🇷 한국어](../ko/README.md) • [🇪🇸 Español](../es/README.md) • [🇷🇺 Русский](../ru/README.md) • [🇧🇷 Português](../pt/README.md) • [🇮🇩 Indonesia](../id/README.md)

</div>

---

## 🤖 サポートされているモデル

1 つのローカルエンドポイントから、OpenCode Zen が提供するすべてのモデルを利用できます —— 通常は公式 OpenCode クライアントの内部でしか使えない**無料枠**も含まれます。

**無料モデル**（API Key 不要、2026-09-29 時点）：

| モデル | 説明 |
|-------|-------|
| `mimo-v2.6-flash-free` | MiMo flash —— 高速でエージェント向け |
| `mimo-v2.5-free` | 前バージョンの MiMo 無料モデル |
| `space-bunny-free` | 軽量な汎用モデル |
| `longcat-2.5-preview-free` | 美団のマルチモーダル推論 |
| `nemotron-3-ultra-free` | NVIDIA Nemotron |
| `nemotron-3.5-lightning-free` | NVIDIA Nemotron の高速版 |
| `muse-spark-1.3-contributor-free` | Contributor 枠 |
| `muse-spark-1.2-contributor-free` | Contributor 枠 |
| `deepseek-v4-flash-free` | DeepSeek flash —— *上流の可用性は変動します* |
| `ling-3.0-flash-fin-free` | Ling flash —— *上流の可用性は変動します* |
| `jev-1.13-free` | TypeSafe System One |

**有料モデル**（Zen API Key が必要、全 72 個）：Claude、GPT、Gemini、Grok、DeepSeek、GLM、Kimi、MiniMax、Qwen、Big Pickle の各ファミリーです。

```bash
# 常にここが正 —— 無料モデルには "free": true が付いています
curl -s http://127.0.0.1:8787/v1/models
```

> ⚠️ **無料モデルの可用性は Zen が判断しており、予告なく変わります。** ある `-free` モデルが今日は使えても、明日には `Model is unavailable` を返すことがあります —— 上の表は Zen が**登録している**モデルの一覧であり、**実際に提供される**ことを保証するものではありません。別のモデルに切り替えるか、Zen API Key を設定して有料枠を利用してください。

---

## ✨ 機能

| 機能 | 説明 |
|---------|-------------|
| 🔌 **OpenAI 互換 API** | `POST /v1/chat/completions` —— ストリーミングと非ストリーミングに対応 |
| 🔌 **Anthropic 互換 API** | Claude Code 向けのネイティブ `POST /v1/messages` |
| 🔌 **OpenAI Responses API** | Codex CLI 向けのネイティブ `POST /v1/responses` |
| 📋 **モデル一覧** | `GET /v1/models`、無料/有料のマーク付き |
| 🆓 **無料枠対応** | OpenCode クライアントの署名を自動注入 —— CLI サブプロセスは不要 |
| 💳 **有料枠対応** | 複数の Zen Key をラウンドロビンして直接転送 |
| 📊 **利用状況の記録** | SQLite にリクエストごとのトークン数・モデル・プロトコル・コストを記録 |
| 📝 **ファイルログ** | `./logs/` 配下に日次 JSONL を出力、7 日間保持 |
| 🖥️ **Web コンソール** | モデルカード、30 日間チャート、絞り込み可能なリクエストログ、設定エディタ |
| 🔑 **キー管理** | コンソールからゲートウェイキーを生成・コピー・ローテーション |
| 🪶 **単一バイナリ** | ピュア Go、CGO 不要、Node.js 不要、データベースサーバー不要 |

---

## 🚀 クイックスタート

```bash
# ビルド
go build -o opencode-gateway .

# 実行（127.0.0.1:8787 で待ち受け、無料モデルはそのまま使えます）
./opencode-gateway
```

初回起動時、ゲートウェイは作業ディレクトリに `config.json` を生成し、生成した認証情報を一度だけ表示します：

```
已生成配置文件: config.json
  Web 控制台密码: xxxxxxxxxxxx
  网关 API Key  : sk-gw-xxxxxxxxxxxxxxxxxx
  (可随时编辑该文件修改；控制台地址 http://127.0.0.1:8787/admin)
```

`http://127.0.0.1:8787/admin` を開き、そのパスワードでログインしてください —— コンソールからパスワード、ゲートウェイ API Key、Zen Key を変更でき、変更は即座に `config.json` へ書き戻されます。

別の構成にしたい場合は、`config.json` を直接手で編集するか、`-config /path/to/config.json` で別のファイルを指定するか、`OPENCODE_GATEWAY_NO_INIT=1` を設定して生成自体をスキップしてください。

**動作要件：** ビルドには Go 1.24+ が必要です。CGO は不要 —— SQLite ドライバはピュア Go 実装です。

---

## ⚙️ 設定

設定のソースは**ただ 1 つ**、`config.json` だけです。既定では作業ディレクトリに置かれます。Web コンソールから編集でき、手で編集して再起動することもできます。

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

| フィールド | 説明 |
|-------|-------------|
| `listen` | バインドアドレス。既定は `127.0.0.1:8787`（ループバックのみ） |
| `api_key` | クライアントが本ゲートウェイにアクセスするためのキー。空 = ループバックからのみアクセス可 |
| `admin_password` | Web コンソールのパスワード。空にすると `/admin` が無効になります |
| `zen_keys` | 有料モデル用の Zen API Key。複数指定するとローテーションされます。空 = 無料枠のみ |
| `upstream` | 上流のベース URL。既定は `https://opencode.ai/zen` |
| `data_dir` | 利用状況データベースのディレクトリ |
| `model_rules.model_blacklist` | `/v1/models` から隠し、リクエストを拒否するモデル ID |

環境変数はファイルの設定を上書きします（Docker / CI で便利）：

```bash
docker run -e OPENCODE_GATEWAY_API_KEY=sk-gw-... opencode-gateway
```

`OPENCODE_GATEWAY_LISTEN`、`_API_KEY`、`_ADMIN_PASSWORD`、`_UPSTREAM`、`_DATA_DIR`、`OPENCODE_ZEN_KEYS` に対応しています。設定していない環境変数は、ファイルの値をそのまま使います。

> `config.json` と `data/` はどちらも `.gitignore` に含まれています —— 秘密情報とローカルの利用データが入るためです。

### Web コンソールで変更できること

**ゲートウェイ API Key**（生成ボタンとワンクリックコピー付き）、**Zen の有料 Key**（有効性チェック付き）、**モデルブラックリスト**、**コンソールパスワード**。変更は即座に反映され、`config.json` に書き戻されます。コンソールパスワードを変更すると、既存のセッションは無効になります。

### ログ

リクエストは `./logs/gateway-YYYY-MM-DD.log` に JSONL として書き込まれ、1 行 1 イベントで **7 日間**保持されます。拒否されたリクエストも理由付きで記録されます。ログはディスクに残るだけなので、利用状況はコンソールの**リクエスト**タブで確認します。

```bash
tail -f logs/gateway-$(date +%F).log   # 今日のログを追う
grep '"level":"error"' logs/*.log      # 失敗だけを抽出
```

---

## 🔌 クライアント設定

### Claude Code

```bash
export ANTHROPIC_BASE_URL=http://127.0.0.1:8787
export ANTHROPIC_AUTH_TOKEN=sk-gw-your-secret
export ANTHROPIC_SMALL_FAST_MODEL=mimo-v2.6-flash-free
claude
```

無料モデル名は Claude Code の組み込みカタログに存在しないため、`--model` で指定するとクライアントが unrecognized model として終了します。代わりに `settings.json` の `modelOverrides` でマッピングしてください：

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

動作確認済みです。Zen API Key を設定すれば、`claude-sonnet-5` のようなネイティブ名をそのまま使え、マッピングは不要です。

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

`experimental_bearer_token` は動作確認済みのフィールドです。Codex のバージョンによっては `env_keys` サブテーブルを使うものもあります —— お使いのバージョンのドキュメントを確認してください。

Codex はモデル名が組み込みカタログにないため、`Model metadata ... not found. Defaulting to fallback metadata` という警告を出します。**これは警告にすぎません** —— Codex はコンテキストウィンドウを保守的に見積もるだけで、すべてそのまま動作します。警告を消したい場合は、同じファイルで実際のウィンドウを宣言してください（`model_context_window = 200000`）。

### OpenAI SDK / 互換ツール

```python
from openai import OpenAI

client = OpenAI(base_url="http://127.0.0.1:8787/v1", api_key="sk-gw-your-secret")
resp = client.chat.completions.create(
    model="mimo-v2.6-flash-free",
    messages=[{"role": "user", "content": "Hello"}],
)
```

---

## 🧠 仕組み

Zen の無料枠（`Authorization: Bearer public`）は、リクエストが本物の OpenCode クライアントから送られたものかをサーバー側で検証します。差分テストによってリバースエンジニアリングしたルールは次のとおりです：

1. `tools` に `bash` と `read` の**両方**が含まれていること（中身は問いません）
2. `stream` が `true` であること
3. `x-opencode-session` が `ses_<12 桁 hex><14 桁 base62>` に一致すること
4. 特定の `User-Agent` と `x-opencode-client: cli`

ゲートウェイはこの署名を注入して Zen の `/v1/chat/completions` を直接呼び出し、そのイベントストリームをクライアントが話すプロトコルへ変換します。非ストリーミングのクライアントには集約済みのレスポンスを返します。有料モデルは、Zen のドキュメントにあるモデル→エンドポイントのマッピングに従ってそのまま転送されます。

```
client (Claude Code / Codex / OpenAI SDK)
  │  /v1/messages | /v1/responses | /v1/chat/completions
  ▼
protocol レイヤ   3 つのプロトコル ⇄ 1 つの中間表現
  ▼
upstream レイヤ   zen：無料枠の署名を注入 / Bearer Key で転送
  ▼
usage レイヤ      SQLite で記録 ─► Web コンソール
telemetry レイヤ  日次 JSONL ログ（7 日間保持）
```

---

## 📋 トラブルシューティング

まずログを見てください —— すべてのリクエストがモデル・ステータス・理由を伴って記録され、拒否されたものも含まれます：

```bash
tail -20 logs/gateway-$(date +%F).log
```

コンソールの**リクエスト**タブでも同じ内容を確認できます。拒否されたリクエストは channel 列に `rejected` と表示されます。

| 症状 | 原因と対処 |
|---------|---------------|
| `402 paid model requires a configured zen API key` | そのモデル名は Zen では有料モデルです（`-free` サフィックスがありません）。無料モデルに切り替えるか、Zen API Key を設定してください |
| `400 Model is unavailable` | その無料モデルが上流で一時的に停止しています —— 別のモデルに切り替えてください |
| `404 model "x" not found` | Zen のモデル ID ではありません。`GET /v1/models` を確認してください |
| Claude Code が `unrecognized model` で終了する | 無料モデル名が組み込みカタログにありません。`modelOverrides` でマッピングしてください |
| Codex が `Model metadata ... not found` を警告する | 表示上の問題のみ —— 影響するのはコンテキストの見積もりだけです |
| モデルカードに「モデルカタログを読み込み中」と表示される | 起動直後の状態です。カタログは数秒で自動的に埋まります |

---

## ⚠️ 制限事項

- 無料モデルの可用性は Zen が管理しており、予告なく変わります
- embeddings、画像生成、Responses の retrieval/cancel には対応していません
- 一度開始したストリームは、別の経路でリトライされません

---

## 📄 ライセンス

MIT
