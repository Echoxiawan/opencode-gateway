<div align="center">

# 🚪 OpenCode Gateway

**OpenCode Zen을 OpenAI / Anthropic / Codex 호환 API로 프록시하는 게이트웨이**

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](../../LICENSE)
[![Go 1.24+](https://img.shields.io/badge/Go-1.24+-00ADD8.svg)](https://go.dev/)
[![No CGO](https://img.shields.io/badge/CGO-not%20required-green.svg)](#-빠른-시작)

*Claude Code, Codex CLI, Cursor, Cline, Roo Code, Kilo Code, OpenAI SDK, LangChain, Continue 및 기타 모든 OpenAI / Anthropic 호환 도구에서 OpenCode Zen 모델을 사용할 수 있습니다*

[모델](#-지원-모델) • [기능](#-기능) • [빠른 시작](#-빠른-시작) • [구성](#%EF%B8%8F-구성) • [클라이언트 설정](#-클라이언트-설정) • [문제 해결](#-문제-해결)

[🇬🇧 English](../../README.md) • [🇨🇳 中文](../zh/README.md) • [🇯🇵 日本語](../ja/README.md) • 🇰🇷 한국어 • [🇪🇸 Español](../es/README.md) • [🇷🇺 Русский](../ru/README.md) • [🇧🇷 Português](../pt/README.md) • [🇮🇩 Indonesia](../id/README.md)

</div>

---

## 🤖 지원 모델

OpenCode Zen이 제공하는 모든 모델을 하나의 로컬 엔드포인트로 노출합니다 — 일반적으로 공식 OpenCode 클라이언트 내부에서만 동작하는 **무료 티어**까지 포함합니다.

**무료 모델** (API Key 불필요, 2026-09-29 기준):

| 모델 | 설명 |
|-------|-------|
| `mimo-v2.6-flash-free` | MiMo flash — 빠르고 에이전트 작업에 적합 |
| `mimo-v2.5-free` | 이전 MiMo 무료 릴리스 |
| `space-bunny-free` | 경량 범용 모델 |
| `longcat-2.5-preview-free` | 메이투안 멀티모달 추론 |
| `nemotron-3-ultra-free` | NVIDIA Nemotron |
| `nemotron-3.5-lightning-free` | NVIDIA Nemotron, 고속 티어 |
| `muse-spark-1.3-contributor-free` | Contributor 티어 |
| `muse-spark-1.2-contributor-free` | Contributor 티어 |
| `deepseek-v4-flash-free` | DeepSeek flash — *업스트림 가용성이 변동될 수 있습니다* |
| `ling-3.0-flash-fin-free` | Ling flash — *업스트림 가용성이 변동될 수 있습니다* |
| `jev-1.13-free` | TypeSafe System One |

**유료 모델** (Zen API Key 필요, 총 72개): Claude, GPT, Gemini, Grok, DeepSeek, GLM, Kimi, MiniMax, Qwen 및 Big Pickle 제품군.

```bash
# 언제나 이 목록이 기준입니다 — 무료 모델은 "free": true 로 표시됩니다
curl -s http://127.0.0.1:8787/v1/models
```

> ⚠️ **무료 모델의 가용성은 Zen이 결정하며 예고 없이 변경됩니다.** 어떤 `-free` 모델이 오늘은 동작하고 내일은 `Model is unavailable`을 반환할 수 있습니다 — 위 목록은 Zen이 **등록한** 모델일 뿐, **실제로 서비스된다는** 보장이 아닙니다. 모델을 바꾸거나 Zen API Key를 추가해 유료 티어를 사용하십시오.

---

## ✨ 기능

| 기능 | 설명 |
|---------|-------------|
| 🔌 **OpenAI 호환 API** | `POST /v1/chat/completions` — 스트리밍 및 비스트리밍 지원 |
| 🔌 **Anthropic 호환 API** | Claude Code용 네이티브 `POST /v1/messages` |
| 🔌 **OpenAI Responses API** | Codex CLI용 네이티브 `POST /v1/responses` |
| 📋 **모델 목록** | `GET /v1/models`, 무료/유료 표시 포함 |
| 🆓 **무료 티어 지원** | OpenCode 클라이언트 서명을 주입 — CLI 하위 프로세스 불필요 |
| 💳 **유료 티어 지원** | 여러 Zen Key를 라운드 로빈으로 직접 포워딩 |
| 📊 **사용량 추적** | SQLite에 요청별 토큰, 모델, 프로토콜, 비용 기록 |
| 📝 **파일 로깅** | `./logs/` 아래 일별 JSONL, 7일 보관 |
| 🖥️ **웹 콘솔** | 모델 카드, 30일 차트, 필터링 가능한 요청 로그, 구성 편집기 |
| 🔑 **키 관리** | 콘솔에서 게이트웨이 키 생성, 복사, 교체 |
| 📌 **트레이 아이콘(Windows)** | 알림 영역으로 최소화하고 오늘의 요청 수를 한눈에 표시 |
| 🪶 **단일 바이너리** | 순수 Go, CGO 없음, Node.js 없음, 데이터베이스 서버 불필요 |

---

## 🚀 빠른 시작

```bash
# 빌드
go build -o opencode-gateway .

# 실행 (127.0.0.1:8787에서 수신, 무료 모델은 즉시 사용 가능)
./opencode-gateway
```

첫 실행 시 게이트웨이는 작업 디렉터리에 `config.json`을 생성하고, 무작위로 생성된 자격 증명을 한 번 출력합니다:

```
已生成配置文件: config.json
  Web 控制台密码: xxxxxxxxxxxx
  网关 API Key  : sk-gw-xxxxxxxxxxxxxxxxxx
  (可随时编辑该文件修改；控制台地址 http://127.0.0.1:8787/admin)
```

`http://127.0.0.1:8787/admin`을 열고 해당 비밀번호로 로그인하십시오 — 콘솔에서 비밀번호, 게이트웨이 API Key, Zen Key를 변경할 수 있으며, 변경 사항은 즉시 `config.json`에 기록됩니다.

다른 방식을 선호하십니까? `config.json`을 직접 편집하거나, `-config /path/to/config.json`으로 다른 파일을 지정하거나, `OPENCODE_GATEWAY_NO_INIT=1`을 설정해 생성을 완전히 건너뛸 수 있습니다.

### Windows 트레이 아이콘

Windows에서 게이트웨이는 작업 표시줄 오른쪽 아래의 알림 영역에 아이콘을 표시합니다. 창의 최소화 버튼을 클릭하면 창이 작업 표시줄에서 사라지고 아이콘만 남으며, 게이트웨이는 계속 실행됩니다.

아이콘을 마우스 오른쪽 버튼으로 클릭하면 메뉴가 열립니다.

| 항목 | 기능 |
|------|------|
| 콘솔 열기… | `http://127.0.0.1:8787/admin`을 엽니다(아이콘을 한 번 왼쪽 클릭해도 동일) |
| *(회색 행)* | 오늘의 요청 수와 업스트림 상태(읽기 전용) |
| 콘솔 창 표시 / 숨기기 | 콘솔 창을 다시 표시하거나 숨깁니다 |
| 종료 | 게이트웨이를 정상적으로 종료합니다 |

아이콘의 점 색상은 업스트림 상태를 나타냅니다. 모델 카탈로그를 불러오는 동안에는 회색, 업스트림이 응답하면 녹색, 마지막 새로 고침에 실패하면 빨간색입니다.

| 옵션 | 효과 |
|------|------|
| `-hide-window` | 콘솔 창을 숨긴 상태로 시작하고 트레이 아이콘만 표시 |
| `-no-tray` | 아이콘을 설치하지 않고 일반 콘솔 앱으로 실행 |

Linux와 macOS 빌드는 빈 스텁을 사용하므로 해당 플랫폼에서는 이 옵션들이 아무런 영향을 주지 않습니다.

**요구 사항:** 빌드에는 Go 1.24+가 필요합니다. CGO는 필요하지 않습니다 — SQLite 드라이버가 순수 Go입니다.

---

## ⚙️ 구성

구성 소스는 정확히 **하나**입니다: 기본적으로 작업 디렉터리에 있는 `config.json`입니다. 웹 콘솔에서 편집할 수 있고, 직접 편집한 뒤 재시작해도 됩니다.

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

| 필드 | 설명 |
|-------|-------------|
| `listen` | 바인드 주소. 기본값 `127.0.0.1:8787` (루프백 전용) |
| `api_key` | 클라이언트가 이 게이트웨이에 접근할 때 사용하는 키. 비어 있으면 루프백 전용 접근 |
| `admin_password` | 웹 콘솔 비밀번호. 비어 있으면 `/admin` 비활성화 |
| `zen_keys` | 유료 모델용 Zen API Key. 여러 Key는 순환됩니다. 비어 있으면 무료 티어만 사용 |
| `upstream` | 업스트림 기본 URL. 기본값 `https://opencode.ai/zen` |
| `data_dir` | 사용량 데이터베이스 디렉터리 |
| `model_rules.model_blacklist` | `/v1/models`에서 숨기고 거부할 모델 ID |

환경 변수는 파일보다 우선합니다 (Docker / CI에 유용):

```bash
docker run -e OPENCODE_GATEWAY_API_KEY=sk-gw-... opencode-gateway
```

`OPENCODE_GATEWAY_LISTEN`, `_API_KEY`, `_ADMIN_PASSWORD`, `_UPSTREAM`, `_DATA_DIR` 및 `OPENCODE_ZEN_KEYS`를 지원합니다. 설정하지 않은 변수는 파일의 값을 그대로 유지합니다.

> `config.json`과 `data/`는 모두 `.gitignore`에 있습니다 — 시크릿과 로컬 사용 데이터를 담고 있습니다.

### 웹 콘솔에서 변경할 수 있는 것

**게이트웨이 API Key**(생성 버튼과 원클릭 복사 제공), **Zen 유료 Key**(유효성 검사 포함), **모델 블랙리스트**, **콘솔 비밀번호**. 변경 사항은 즉시 적용되고 `config.json`에 기록됩니다. 콘솔 비밀번호를 변경하면 기존 세션이 무효화됩니다.

### 로깅

요청은 `./logs/gateway-YYYY-MM-DD.log`에 JSONL로 기록되며, 이벤트당 한 줄이고 **7일**간 보관됩니다. 거부된 요청도 사유와 함께 기록됩니다. 로그는 디스크에만 남으므로 사용량은 콘솔의 **요청** 탭에서 확인합니다.

```bash
tail -f logs/gateway-$(date +%F).log   # 오늘 로그 따라가기
grep '"level":"error"' logs/*.log      # 실패만 보기
```

---

## 🔌 클라이언트 설정

### Claude Code

```bash
export ANTHROPIC_BASE_URL=http://127.0.0.1:8787
export ANTHROPIC_AUTH_TOKEN=sk-gw-your-secret
export ANTHROPIC_SMALL_FAST_MODEL=mimo-v2.6-flash-free
claude
```

무료 모델 이름은 Claude Code의 내장 목록에 없으므로, `--model`로 지정하면 클라이언트가 unrecognized model로 종료됩니다. 대신 `settings.json`의 `modelOverrides`로 매핑하십시오:

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

실측 검증 완료. Zen API Key를 구성하면 `claude-sonnet-5` 같은 네이티브 모델 이름을 매핑 없이 바로 사용할 수 있습니다.

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

`experimental_bearer_token`은 동작이 검증된 필드입니다. 일부 Codex 버전은 `env_keys` 하위 테이블을 대신 사용하므로, 사용 중인 버전의 문서를 확인하십시오.

Codex는 모델 이름이 내장 목록에 없기 때문에 `Model metadata ... not found. Defaulting to fallback metadata` 경고를 표시합니다. **경고일 뿐입니다** — Codex는 컨텍스트 창을 보수적으로 추정하며 모든 기능이 그대로 동작합니다. 경고를 없애려면 같은 파일에 실제 창 크기를 선언하십시오(`model_context_window = 200000`).

### OpenAI SDK / 호환 도구

```python
from openai import OpenAI

client = OpenAI(base_url="http://127.0.0.1:8787/v1", api_key="sk-gw-your-secret")
resp = client.chat.completions.create(
    model="mimo-v2.6-flash-free",
    messages=[{"role": "user", "content": "안녕하세요"}],
)
```

---

## 🧠 동작 원리

Zen 무료 티어(`Authorization: Bearer public`)는 요청이 실제 OpenCode 클라이언트에서 온 것인지 서버 측에서 검증합니다. 차분 테스트로 역설계한 규칙은 다음과 같습니다:

1. `tools`에 `bash`와 `read` 도구가 **모두** 포함되어야 합니다 (본문 내용은 무관)
2. `stream`이 `true`여야 합니다
3. `x-opencode-session`이 `ses_<12자리 hex><14자리 base62>`와 일치해야 합니다
4. 특정 `User-Agent`와 `x-opencode-client: cli`

게이트웨이는 이 서명을 주입해 Zen의 `/v1/chat/completions`를 직접 호출한 뒤, 이벤트 스트림을 클라이언트가 사용하는 프로토콜로 변환합니다. 비스트리밍 클라이언트는 집계된 응답을 받습니다. 유료 모델은 Zen 문서의 모델→엔드포인트 매핑에 따라 그대로 포워딩됩니다.

```
client (Claude Code / Codex / OpenAI SDK)
  │  /v1/messages | /v1/responses | /v1/chat/completions
  ▼
protocol 계층   세 가지 프로토콜 ⇄ 하나의 통합 중간 표현
  ▼
upstream 계층   zen: 무료 티어 서명 주입 / Bearer Key로 포워딩
  ▼
usage 계층      SQLite 집계 ─► 웹 콘솔
telemetry 계층  일별 JSONL 로그 (7일 보관)
```

---

## 📋 문제 해결

로그부터 확인하십시오 — 모든 요청에 모델, 상태, 사유가 담겨 있고 거부된 요청도 포함됩니다:

```bash
tail -20 logs/gateway-$(date +%F).log
```

콘솔의 **요청** 탭에서도 동일한 내용을 볼 수 있으며, 거부된 요청은 채널 열에 `rejected`로 표시됩니다.

| 증상 | 원인 및 해결 |
|---------|---------------|
| `402 paid model requires a configured zen API key` | 해당 모델 이름은 Zen에서 유료 모델입니다(`-free` 접미사 없음). 무료 모델을 사용하거나 Zen API Key를 추가하십시오 |
| `400 Model is unavailable` | 해당 무료 모델이 업스트림에서 일시적으로 중단된 상태입니다 — 모델을 교체하십시오 |
| `404 model "x" not found` | Zen 모델 ID가 아닙니다. `GET /v1/models`로 확인하십시오 |
| Claude Code가 `unrecognized model`과 함께 종료 | 무료 모델 이름이 내장 목록에 없습니다. `modelOverrides`로 매핑하십시오 |
| Codex가 `Model metadata ... not found` 경고 | 표시상의 문제일 뿐 — 컨텍스트 추정에만 영향을 줍니다 |
| 모델 카드에 "모델 카탈로그 로딩 중" 표시 | 방금 시작한 경우입니다. 모델 카탈로그는 몇 초 안에 채워집니다 |

---

## ⚠️ 제한 사항

- 무료 모델의 가용성은 Zen이 제어하며 예고 없이 변경됩니다
- embeddings, 이미지 생성, Responses의 검색/취소는 지원하지 않습니다
- 이미 시작된 스트리밍은 다른 경로로 재시도하지 않습니다

---

## 📄 라이선스

MIT
