<div align="center">

# 🚪 OpenCode Gateway

**Gateway proxy para OpenCode Zen — API compatible con OpenAI / Anthropic / Codex**

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](../../LICENSE)
[![Go 1.24+](https://img.shields.io/badge/Go-1.24+-00ADD8.svg)](https://go.dev/)
[![No CGO](https://img.shields.io/badge/CGO-not%20required-green.svg)](#-inicio-rápido)

*Usa los modelos de OpenCode Zen desde Claude Code, Codex CLI, Cursor, Cline, Roo Code, Kilo Code, OpenAI SDK, LangChain, Continue y cualquier otra herramienta compatible con OpenAI o Anthropic*

[Modelos](#-modelos-compatibles) • [Características](#-características) • [Inicio rápido](#-inicio-rápido) • [Configuración](#%EF%B8%8F-configuración) • [Conexión de clientes](#-conexión-de-clientes) • [Solución de problemas](#-solución-de-problemas)

[🇬🇧 English](../../README.md) • [🇨🇳 中文](../zh/README.md) • [🇯🇵 日本語](../ja/README.md) • [🇰🇷 한국어](../ko/README.md) • 🇪🇸 Español • [🇷🇺 Русский](../ru/README.md) • [🇧🇷 Português](../pt/README.md) • [🇮🇩 Indonesia](../id/README.md)

</div>

---

## 🤖 Modelos compatibles

Un único endpoint local que expone todos los modelos que ofrece OpenCode Zen — incluido el **nivel gratuito**, que normalmente solo funciona desde dentro del cliente oficial de OpenCode.

**Modelos gratuitos** (sin API Key, a fecha de 2026-09-29):

| Modelo | Notas |
|-------|-------|
| `mimo-v2.6-flash-free` | MiMo flash — rápido, orientado a agentes |
| `mimo-v2.5-free` | Versión gratuita anterior de MiMo |
| `space-bunny-free` | Modelo general ligero |
| `longcat-2.5-preview-free` | Razonamiento multimodal de Meituan |
| `nemotron-3-ultra-free` | NVIDIA Nemotron |
| `nemotron-3.5-lightning-free` | NVIDIA Nemotron, gama rápida |
| `muse-spark-1.3-contributor-free` | Nivel Contributor |
| `muse-spark-1.2-contributor-free` | Nivel Contributor |
| `deepseek-v4-flash-free` | DeepSeek flash — *la disponibilidad upstream varía* |
| `ling-3.0-flash-fin-free` | Ling flash — *la disponibilidad upstream varía* |
| `jev-1.13-free` | TypeSafe System One |

**Modelos de pago** (requieren una Zen API Key; hay 72 disponibles): las familias Claude, GPT, Gemini, Grok, DeepSeek, GLM, Kimi, MiniMax, Qwen y Big Pickle.

```bash
# Siempre la lista oficial — los modelos gratuitos se marcan con "free": true
curl -s http://127.0.0.1:8787/v1/models
```

> ⚠️ **La disponibilidad de los modelos gratuitos la decide Zen y cambia sin previo aviso.** Un modelo `-free` puede funcionar hoy y devolver `Model is unavailable` mañana — la lista de arriba es lo que Zen *registra*, no una garantía de que lo *sirva*. Cambia de modelo o añade una Zen API Key para usar el nivel de pago.

---

## ✨ Características

| Función | Descripción |
|---------|-------------|
| 🔌 **API compatible con OpenAI** | `POST /v1/chat/completions` — con streaming y sin streaming |
| 🔌 **API compatible con Anthropic** | `POST /v1/messages` nativo para Claude Code |
| 🔌 **OpenAI Responses API** | `POST /v1/responses` nativo para el Codex CLI |
| 📋 **Lista de modelos** | `GET /v1/models` con marcas de gratuito y de pago |
| 🆓 **Soporte del nivel gratuito** | Inyecta la firma del cliente de OpenCode — sin necesidad de un subproceso del CLI |
| 💳 **Soporte del nivel de pago** | Reenvío directo con round-robin entre varias Zen Keys |
| 📊 **Seguimiento de uso** | SQLite registra tokens, modelo, protocolo y coste de cada petición |
| 📝 **Logs en archivo** | JSONL diario en `./logs/`, con 7 días de retención |
| 🖥️ **Consola web** | Tarjetas de modelos, gráfico de 30 días, registro de peticiones filtrable, editor de configuración |
| 🔑 **Gestión de claves** | Genera, copia y rota la clave del gateway desde la consola |
| 📌 **Icono de bandeja (Windows)** | Se minimiza al área de notificación y muestra de un vistazo las peticiones de hoy |
| 🪶 **Binario único** | Go puro, sin CGO, sin Node.js, sin servidor de base de datos |

---

## 🚀 Inicio rápido

```bash
# Compilar
go build -o opencode-gateway .

# Ejecutar (escucha en 127.0.0.1:8787, los modelos gratuitos funcionan al instante)
./opencode-gateway
```

En el primer arranque el gateway crea `config.json` en el directorio de trabajo e imprime una sola vez las credenciales generadas:

```
已生成配置文件: config.json
  Web 控制台密码: xxxxxxxxxxxx
  网关 API Key  : sk-gw-xxxxxxxxxxxxxxxxxx
  (可随时编辑该文件修改；控制台地址 http://127.0.0.1:8787/admin)
```

Abre `http://127.0.0.1:8787/admin` e inicia sesión con esa contraseña — la consola puede cambiar la contraseña, la clave API del gateway y la Zen Key, y los cambios se escriben de inmediato en `config.json`.

¿Prefieres otra forma de montarlo? Edita `config.json` a mano, apunta `-config /path/to/config.json` a otro archivo, o define `OPENCODE_GATEWAY_NO_INIT=1` para omitir por completo la generación.

### Icono de bandeja de Windows

En Windows, el gateway coloca un icono en el área de notificación, en la esquina inferior derecha de la barra de tareas. Al hacer clic en el botón de minimizar, la ventana desaparece de la barra de tareas y queda representada por el icono, mientras el gateway sigue funcionando.

Haz clic derecho en el icono para abrir el menú:

| Elemento | Función |
|------|------|
| Abrir consola… | Abre `http://127.0.0.1:8787/admin` (un clic izquierdo también lo hace) |
| *(línea atenuada)* | Número de peticiones de hoy y estado del upstream — solo lectura |
| Mostrar / ocultar ventana de la consola | Vuelve a mostrar u oculta la ventana de la consola |
| Salir | Apaga el gateway correctamente |

El color del punto del icono indica el estado del upstream: gris mientras se carga el catálogo de modelos, verde cuando el upstream responde y rojo si la última actualización falló.

| Opción | Efecto |
|------|--------|
| `-hide-window` | Inicia con la ventana de la consola oculta; solo queda el icono de bandeja |
| `-no-tray` | No instala el icono y funciona como una aplicación de consola normal |

Las compilaciones para Linux y macOS usan un stub vacío, por lo que estas opciones no tienen efecto en esas plataformas.

**Requisitos:** Go 1.24+ para compilar. Sin CGO — el driver de SQLite es Go puro.

---

## ⚙️ Configuración

Existe exactamente **una** fuente de configuración: `config.json`, por defecto en el directorio de trabajo. La consola web lo edita; también puedes editarlo a mano y reiniciar.

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

| Campo | Descripción |
|-------|-------------|
| `listen` | Dirección de escucha. Por defecto `127.0.0.1:8787` (solo loopback) |
| `api_key` | Clave que usan los clientes para llegar a este gateway. Vacío = acceso solo por loopback |
| `admin_password` | Contraseña de la consola web. Vacío desactiva `/admin` |
| `zen_keys` | Zen API Keys para los modelos de pago. Con varias claves se van rotando. Vacío = solo nivel gratuito |
| `upstream` | URL base del upstream. Por defecto `https://opencode.ai/zen` |
| `data_dir` | Directorio de la base de datos de uso |
| `model_rules.model_blacklist` | IDs de modelo que se ocultan de `/v1/models` y se rechazan |

Las variables de entorno sobrescriben el archivo (cómodo para Docker / CI):

```bash
docker run -e OPENCODE_GATEWAY_API_KEY=sk-gw-... opencode-gateway
```

Se admiten `OPENCODE_GATEWAY_LISTEN`, `_API_KEY`, `_ADMIN_PASSWORD`, `_UPSTREAM`, `_DATA_DIR` y `OPENCODE_ZEN_KEYS`. Las variables que no estén definidas dejan intacto el valor del archivo.

> `config.json` y `data/` están ambos en `.gitignore` — contienen secretos y datos de uso locales.

### Qué puede cambiar la consola web

**La clave API del gateway** (con botón de generación y copia en un clic), **las Zen Keys de pago** (con comprobación de validez), **la lista negra de modelos** y **la contraseña de la consola**. Los cambios se aplican al instante y se escriben de vuelta en `config.json`. Cambiar la contraseña de la consola invalida las sesiones existentes.

### Registro de logs

Las peticiones se escriben como JSONL en `./logs/gateway-YYYY-MM-DD.log`, una línea por evento, con una retención de **7 días**. Las peticiones rechazadas también se registran, junto con el motivo. Los logs se quedan en disco: el uso se consulta en la pestaña **Solicitudes** del panel.

```bash
tail -f logs/gateway-$(date +%F).log   # seguir el de hoy
grep '"level":"error"' logs/*.log      # solo los fallos
```

---

## 🔌 Conexión de clientes

### Claude Code

```bash
export ANTHROPIC_BASE_URL=http://127.0.0.1:8787
export ANTHROPIC_AUTH_TOKEN=sk-gw-your-secret
export ANTHROPIC_SMALL_FAST_MODEL=mimo-v2.6-flash-free
claude
```

Los nombres de los modelos gratuitos no están en el catálogo integrado de Claude Code, así que si pasas uno con `--model` el cliente se cierra con un error de modelo no reconocido. En su lugar, mapéalo con `modelOverrides` en `settings.json`:

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

Verificado y funcionando. Con una Zen API Key configurada puedes usar directamente nombres nativos como `claude-sonnet-5`, sin necesidad de mapeo.

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

`experimental_bearer_token` es el campo que está verificado que funciona; algunas versiones de Codex usan en su lugar una subtabla `env_keys` — consulta la documentación de tu versión.

Codex avisará con `Model metadata ... not found. Defaulting to fallback metadata` porque el nombre del modelo no está en su catálogo integrado. **Solo es una advertencia** — Codex estima la ventana de contexto de forma conservadora y todo sigue funcionando. Para silenciarla, declara la ventana real en el mismo archivo (`model_context_window = 200000`).

### OpenAI SDK / herramientas compatibles

```python
from openai import OpenAI

client = OpenAI(base_url="http://127.0.0.1:8787/v1", api_key="sk-gw-your-secret")
resp = client.chat.completions.create(
    model="mimo-v2.6-flash-free",
    messages=[{"role": "user", "content": "Hello"}],
)
```

---

## 🧠 Cómo funciona

El nivel gratuito de Zen (`Authorization: Bearer public`) valida del lado del servidor que la petición provenga del cliente real de OpenCode. Reglas obtenidas por ingeniería inversa mediante pruebas diferenciales:

1. `tools` debe contener **tanto** una herramienta `bash` como una `read` (el contenido da igual)
2. `stream` debe ser `true`
3. `x-opencode-session` debe coincidir con `ses_<12 hex><14 base62>`
4. Un `User-Agent` concreto y `x-opencode-client: cli`

El gateway inyecta esa firma y llama directamente a `/v1/chat/completions` de Zen, y después traduce el flujo de eventos al protocolo que hable el cliente. Los clientes sin streaming reciben la respuesta agregada. Los modelos de pago se reenvían tal cual, siguiendo el mapeo modelo→endpoint de la documentación de Zen.

```
client (Claude Code / Codex / OpenAI SDK)
  │  /v1/messages | /v1/responses | /v1/chat/completions
  ▼
capa protocolo    tres protocolos ⇄ una representación intermedia
  ▼
capa upstream     zen: inyecta la firma del tier gratuito / reenvía con Bearer key
  ▼
capa de uso       contabilidad SQLite ─► consola web
capa telemetría   logs JSONL diarios (retención de 7 días)
```

---

## 📋 Solución de problemas

Empieza por el log — cada petición lleva su modelo, su estado y su motivo, incluidas las rechazadas:

```bash
tail -20 logs/gateway-$(date +%F).log
```

La pestaña **Solicitudes** de la consola muestra lo mismo, con `rejected` en la columna de canal.

| Síntoma | Causa y solución |
|---------|---------------|
| `402 paid model requires a configured zen API key` | El nombre del modelo es de pago en Zen (no tiene sufijo `-free`). Usa un modelo gratuito o añade una Zen API Key |
| `400 Model is unavailable` | Ese modelo gratuito está temporalmente caído en el upstream — cambia de modelo |
| `404 model "x" not found` | No es un ID de modelo de Zen. Consulta `GET /v1/models` |
| Claude Code se cierra con `unrecognized model` | Los nombres de modelos gratuitos no están en su catálogo; mapéalos con `modelOverrides` |
| Codex avisa con `Model metadata ... not found` | Es solo estético — únicamente afecta a la estimación del contexto |
| Las tarjetas de modelo dicen "cargando catálogo de modelos" | Acaba de arrancar; el catálogo se rellena en unos segundos |

---

## ⚠️ Limitaciones

- La disponibilidad de los modelos gratuitos la controla Zen y cambia sin previo aviso
- No hay embeddings, ni generación de imágenes, ni retrieval/cancel de Responses
- Un stream ya iniciado no se reintenta por otra ruta

---

## 📄 Licencia

MIT
