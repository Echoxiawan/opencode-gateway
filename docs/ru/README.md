<div align="center">

# 🚪 OpenCode Gateway

**Прокси-шлюз к OpenCode Zen — API, совместимый с OpenAI / Anthropic / Codex**

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](../../LICENSE)
[![Go 1.24+](https://img.shields.io/badge/Go-1.24+-00ADD8.svg)](https://go.dev/)
[![No CGO](https://img.shields.io/badge/CGO-not%20required-green.svg)](#-быстрый-старт)

*Используйте модели OpenCode Zen из Claude Code, Codex CLI, Cursor, Cline, Roo Code, Kilo Code, OpenAI SDK, LangChain, Continue и любого другого инструмента, совместимого с OpenAI или Anthropic*

[Модели](#-поддерживаемые-модели) • [Возможности](#-возможности) • [Быстрый старт](#-быстрый-старт) • [Конфигурация](#%EF%B8%8F-конфигурация) • [Настройка клиентов](#-настройка-клиентов) • [Решение проблем](#-решение-проблем)

[🇬🇧 English](../../README.md) • [🇨🇳 中文](../zh/README.md) • [🇯🇵 日本語](../ja/README.md) • [🇰🇷 한국어](../ko/README.md) • [🇪🇸 Español](../es/README.md) • 🇷🇺 Русский • [🇧🇷 Português](../pt/README.md) • [🇮🇩 Indonesia](../id/README.md)

</div>

---

## 🤖 Поддерживаемые модели

Одна локальная точка входа, отдающая все модели OpenCode Zen — включая **бесплатный уровень**, который обычно работает только внутри официального клиента OpenCode.

**Бесплатные модели** (API-ключ не нужен, по состоянию на 2026-09-29):

| Модель | Примечания |
|-------|-------|
| `mimo-v2.6-flash-free` | MiMo flash — быстрая, для агентных задач |
| `mimo-v2.5-free` | Предыдущий бесплатный релиз MiMo |
| `space-bunny-free` | Лёгкая универсальная модель |
| `longcat-2.5-preview-free` | Мультимодальные рассуждения Meituan |
| `nemotron-3-ultra-free` | NVIDIA Nemotron |
| `nemotron-3.5-lightning-free` | NVIDIA Nemotron, быстрый уровень |
| `muse-spark-1.3-contributor-free` | Уровень Contributor |
| `muse-spark-1.2-contributor-free` | Уровень Contributor |
| `deepseek-v4-flash-free` | DeepSeek flash — *доступность на стороне апстрима меняется* |
| `ling-3.0-flash-fin-free` | Ling flash — *доступность на стороне апстрима меняется* |
| `jev-1.13-free` | TypeSafe System One |

**Платные модели** (нужен Zen API Key, доступно 72): семейства Claude, GPT, Gemini, Grok, DeepSeek, GLM, Kimi, MiniMax, Qwen и Big Pickle.

```bash
# Всегда актуальный список — бесплатные модели помечены "free": true
curl -s http://127.0.0.1:8787/v1/models
```

> ⚠️ **Доступность бесплатных моделей определяет Zen, и она меняется без предупреждения.** Модель с суффиксом `-free` может работать сегодня, а завтра вернуть `Model is unavailable` — таблица выше показывает, что Zen *регистрирует*, а не гарантию, что он это *обслуживает*. Переключитесь на другую модель или добавьте Zen API Key, чтобы использовать платный уровень.

---

## ✨ Возможности

| Возможность | Описание |
|---------|-------------|
| 🔌 **API, совместимый с OpenAI** | `POST /v1/chat/completions` — потоковый и обычный режимы |
| 🔌 **API, совместимый с Anthropic** | Нативный `POST /v1/messages` для Claude Code |
| 🔌 **OpenAI Responses API** | Нативный `POST /v1/responses` для Codex CLI |
| 📋 **Список моделей** | `GET /v1/models` с пометками «бесплатная»/«платная» |
| 🆓 **Поддержка бесплатного уровня** | Подставляет подпись клиента OpenCode — без запуска CLI-подпроцесса |
| 💳 **Поддержка платного уровня** | Прямая пересылка с круговым перебором нескольких Zen Key |
| 📊 **Учёт расхода** | SQLite фиксирует tokens, модель, протокол и стоимость по каждому запросу |
| 📝 **Логи в файлах** | Ежедневный JSONL в `./logs/`, хранение 7 дней |
| 🖥️ **Веб-консоль** | Карточки моделей, график за 30 дней, журнал запросов с фильтрами, редактор конфигурации |
| 🔑 **Управление ключами** | Генерация, копирование и ротация ключа шлюза прямо из консоли |
| 🪶 **Один бинарник** | Чистый Go, без CGO, без Node.js, без сервера БД |

---

## 🚀 Быстрый старт

```bash
# Сборка
go build -o opencode-gateway .

# Запуск (слушает 127.0.0.1:8787, бесплатные модели работают сразу)
./opencode-gateway
```

При первом запуске шлюз создаёт `config.json` в рабочем каталоге и один раз печатает сгенерированные учётные данные:

```
已生成配置文件: config.json
  Web 控制台密码: xxxxxxxxxxxx
  网关 API Key  : sk-gw-xxxxxxxxxxxxxxxxxx
  (可随时编辑该文件修改；控制台地址 http://127.0.0.1:8787/admin)
```

Откройте `http://127.0.0.1:8787/admin` и войдите с этим паролем — из консоли можно сменить пароль, API-ключ шлюза и Zen Key, и изменения сразу записываются обратно в `config.json`.

Хотите настроить иначе? Отредактируйте `config.json` вручную, укажите другой файл через `-config /path/to/config.json` или задайте `OPENCODE_GATEWAY_NO_INIT=1`, чтобы полностью пропустить генерацию.

**Требования:** для сборки нужен Go 1.24+. CGO не требуется — драйвер SQLite написан на чистом Go.

---

## ⚙️ Конфигурация

Источник конфигурации ровно **один**: `config.json`, по умолчанию в рабочем каталоге. Веб-консоль редактирует его; можно также править файл вручную и перезапускать шлюз.

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

| Поле | Описание |
|-------|-------------|
| `listen` | Адрес прослушивания. По умолчанию `127.0.0.1:8787` (только локально) |
| `api_key` | Ключ, с которым клиенты обращаются к шлюзу. Пусто = доступ только с локальной машины |
| `admin_password` | Пароль веб-консоли. Пусто — раздел `/admin` отключён |
| `zen_keys` | Zen API Key для платных моделей. Несколько ключей перебираются по кругу. Пусто = только бесплатный уровень |
| `upstream` | Базовый URL апстрима. По умолчанию `https://opencode.ai/zen` |
| `data_dir` | Каталог базы учёта расхода |
| `model_rules.model_blacklist` | ID моделей, которые нужно скрыть из `/v1/models` и отклонять |

Переменные окружения перекрывают файл (удобно для Docker / CI):

```bash
docker run -e OPENCODE_GATEWAY_API_KEY=sk-gw-... opencode-gateway
```

Поддерживаются `OPENCODE_GATEWAY_LISTEN`, `_API_KEY`, `_ADMIN_PASSWORD`, `_UPSTREAM`, `_DATA_DIR` и `OPENCODE_ZEN_KEYS`. Незаданные переменные не трогают значение из файла.

> `config.json` и `data/` перечислены в `.gitignore` — в них секреты и локальные данные об использовании.

### Что можно менять в веб-консоли

**API-ключ шлюза** (с кнопкой генерации и копированием в один клик), **платные Zen Key** (с проверкой валидности), **чёрный список моделей** и **пароль консоли**. Изменения применяются сразу и записываются обратно в `config.json`. Смена пароля консоли делает активные сессии недействительными.

### Логирование

Запросы пишутся в формате JSONL в `./logs/gateway-YYYY-MM-DD.log`, одна строка на событие, хранение **7 дней**. Отклонённые запросы тоже фиксируются, вместе с причиной. Логи остаются на диске — расход смотрят на вкладке **Requests** в консоли.

```bash
tail -f logs/gateway-$(date +%F).log   # следить за сегодняшним
grep '"level":"error"' logs/*.log      # только ошибки
```

---

## 🔌 Настройка клиентов

### Claude Code

```bash
export ANTHROPIC_BASE_URL=http://127.0.0.1:8787
export ANTHROPIC_AUTH_TOKEN=sk-gw-your-secret
export ANTHROPIC_SMALL_FAST_MODEL=mimo-v2.6-flash-free
claude
```

Имён бесплатных моделей нет во встроенном каталоге Claude Code, поэтому передача такого имени через `--model` заставляет клиент завершиться с ошибкой «unrecognized model». Вместо этого сопоставьте модель через `modelOverrides` в `settings.json`:

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

Проверено на практике. С настроенным Zen API Key можно использовать нативные имена вроде `claude-sonnet-5` напрямую, без сопоставления.

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

Именно `experimental_bearer_token` — проверенное рабочее поле; в некоторых версиях Codex вместо него используется подтаблица `env_keys` — сверьтесь с документацией вашей версии.

Codex предупредит `Model metadata ... not found. Defaulting to fallback metadata`, потому что имени модели нет в его встроенном каталоге. **Это всего лишь предупреждение** — Codex оценивает контекстное окно по консервативному значению, и всё продолжает работать. Чтобы убрать предупреждение, объявите реальное окно в том же файле (`model_context_window = 200000`).

### OpenAI SDK / совместимые инструменты

```python
from openai import OpenAI

client = OpenAI(base_url="http://127.0.0.1:8787/v1", api_key="sk-gw-your-secret")
resp = client.chat.completions.create(
    model="mimo-v2.6-flash-free",
    messages=[{"role": "user", "content": "Hello"}],
)
```

---

## 🧠 Как это работает

Бесплатный уровень Zen (`Authorization: Bearer public`) проверяет на стороне сервера, что запрос пришёл от настоящего клиента OpenCode. Правила восстановлены методом дифференциального тестирования:

1. `tools` должен содержать **одновременно** инструменты `bash` и `read` (их содержимое не важно)
2. `stream` должен быть `true`
3. `x-opencode-session` должен соответствовать `ses_<12 hex><14 base62>`
4. Определённые `User-Agent` и `x-opencode-client: cli`

Шлюз подставляет эту подпись и обращается напрямую к `/v1/chat/completions` в Zen, а затем переводит поток событий в тот протокол, на котором говорит клиент. Клиенты без потокового режима получают агрегированный ответ. Платные модели пересылаются как есть, согласно сопоставлению «модель → эндпоинт» из документации Zen.

```
client (Claude Code / Codex / OpenAI SDK)
  │  /v1/messages | /v1/responses | /v1/chat/completions
  ▼
слой протоколов   три протокола ⇄ одно промежуточное представление
  ▼
слой upstream     zen: подпись бесплатного тарифа / пересылка с Bearer key
  ▼
слой статистики   учёт в SQLite ─► веб-консоль
слой телеметрии   ежедневные JSONL-логи (хранение 7 дней)
```

---

## 📋 Решение проблем

Начните с лога — каждый запрос несёт свою модель, статус и причину, включая отклонённые:

```bash
tail -20 logs/gateway-$(date +%F).log
```

Вкладка **Requests** в консоли показывает то же самое, а в колонке канала у отклонённых запросов стоит `rejected`.

| Симптом | Причина и решение |
|---------|---------------|
| `402 paid model requires a configured zen API key` | Имя модели в Zen относится к платным (нет суффикса `-free`). Возьмите бесплатную модель или добавьте Zen API Key |
| `400 Model is unavailable` | Эта бесплатная модель временно недоступна на апстриме — переключитесь на другую |
| `404 model "x" not found` | Это не ID модели Zen. Проверьте `GET /v1/models` |
| Claude Code завершается с `unrecognized model` | Имени бесплатной модели нет в его каталоге; сопоставьте через `modelOverrides` |
| Codex предупреждает `Model metadata ... not found` | Косметика — влияет только на оценку контекста |
| Карточки моделей показывают «loading model catalog» | Шлюз только что запущен; каталог заполнится через несколько секунд |

---

## ⚠️ Ограничения

- Доступность бесплатных моделей контролирует Zen, и она меняется без предупреждения
- Нет embeddings, генерации изображений, а также retrieval/cancel в Responses
- Уже начавшийся поток не повторяется по другому пути

---

## 📄 Лицензия

MIT
