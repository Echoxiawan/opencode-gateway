<div align="center">

# 🚪 OpenCode Gateway

**Gateway de proxy para o OpenCode Zen — API compatível com OpenAI / Anthropic / Codex**

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](../../LICENSE)
[![Go 1.24+](https://img.shields.io/badge/Go-1.24+-00ADD8.svg)](https://go.dev/)
[![No CGO](https://img.shields.io/badge/CGO-not%20required-green.svg)](#-início-rápido)

*Use os modelos do OpenCode Zen no Claude Code, Codex CLI, Cursor, Cline, Roo Code, Kilo Code, OpenAI SDK, LangChain, Continue e em qualquer outra ferramenta compatível com OpenAI ou Anthropic*

[Modelos](#-modelos-suportados) • [Recursos](#-recursos) • [Início Rápido](#-início-rápido) • [Configuração](#%EF%B8%8F-configuração) • [Configuração do Cliente](#-configuração-do-cliente) • [Solução de Problemas](#-solução-de-problemas)

[🇬🇧 English](../../README.md) • [🇨🇳 中文](../zh/README.md) • [🇯🇵 日本語](../ja/README.md) • [🇰🇷 한국어](../ko/README.md) • [🇪🇸 Español](../es/README.md) • [🇷🇺 Русский](../ru/README.md) • 🇧🇷 Português • [🇮🇩 Indonesia](../id/README.md)

</div>

---

## 🤖 Modelos Suportados

Um único endpoint local que expõe todos os modelos oferecidos pelo OpenCode Zen — incluindo o **nível gratuito**, que normalmente só funciona dentro do cliente oficial do OpenCode.

**Modelos gratuitos** (não exigem API key, em 2026-09-29):

| Modelo | Observações |
|-------|-------|
| `mimo-v2.6-flash-free` | MiMo flash — rápido, voltado a agentes |
| `mimo-v2.5-free` | Versão gratuita anterior do MiMo |
| `space-bunny-free` | Modelo generalista leve |
| `longcat-2.5-preview-free` | Raciocínio multimodal da Meituan |
| `nemotron-3-ultra-free` | NVIDIA Nemotron |
| `nemotron-3.5-lightning-free` | NVIDIA Nemotron, faixa rápida |
| `muse-spark-1.3-contributor-free` | Nível Contributor |
| `muse-spark-1.2-contributor-free` | Nível Contributor |
| `deepseek-v4-flash-free` | DeepSeek flash — *a disponibilidade no upstream varia* |
| `ling-3.0-flash-fin-free` | Ling flash — *a disponibilidade no upstream varia* |
| `jev-1.13-free` | TypeSafe System One |

**Modelos pagos** (exigem uma Zen API key, 72 disponíveis): as famílias Claude, GPT, Gemini, Grok, DeepSeek, GLM, Kimi, MiniMax, Qwen e Big Pickle.

```bash
# Sempre a lista oficial — modelos gratuitos são marcados com "free": true
curl -s http://127.0.0.1:8787/v1/models
```

> ⚠️ **A disponibilidade dos modelos gratuitos é decidida pela Zen e muda sem aviso.** Um modelo `-free` pode funcionar hoje e retornar `Model is unavailable` amanhã — a lista acima é o que a Zen *registra*, não uma garantia de que ela *serve*. Troque de modelo, ou adicione uma Zen API key para usar o nível pago.

---

## ✨ Recursos

| Recurso | Descrição |
|---------|-------------|
| 🔌 **API compatível com OpenAI** | `POST /v1/chat/completions` — com e sem streaming |
| 🔌 **API compatível com Anthropic** | `POST /v1/messages` nativo para o Claude Code |
| 🔌 **OpenAI Responses API** | `POST /v1/responses` nativo para o Codex CLI |
| 📋 **Lista de modelos** | `GET /v1/models` com marcadores de gratuito/pago |
| 🆓 **Suporte ao nível gratuito** | Injeta a assinatura do cliente OpenCode — sem subprocesso de CLI |
| 💳 **Suporte ao nível pago** | Encaminhamento direto com rodízio entre várias Zen keys |
| 📊 **Contabilização de uso** | O SQLite registra tokens, modelo, protocolo e custo por requisição |
| 📝 **Log em arquivo** | JSONL diário em `./logs/`, com retenção de 7 dias |
| 🖥️ **Console web** | Cards de modelos, gráfico de 30 dias, log de requisições filtrável e editor de configuração |
| 🔑 **Gerenciamento de chaves** | Gere, copie e rotacione a key do gateway pelo console |
| 🪶 **Binário único** | Go puro, sem CGO, sem Node.js, sem servidor de banco de dados |

---

## 🚀 Início Rápido

```bash
# Compilar
go build -o opencode-gateway .

# Executar (escuta em 127.0.0.1:8787, modelos gratuitos funcionam imediatamente)
./opencode-gateway
```

Na primeira execução, o gateway cria o `config.json` no diretório de trabalho e imprime as credenciais geradas uma única vez:

```
已生成配置文件: config.json
  Web 控制台密码: xxxxxxxxxxxx
  网关 API Key  : sk-gw-xxxxxxxxxxxxxxxxxx
  (可随时编辑该文件修改；控制台地址 http://127.0.0.1:8787/admin)
```

Abra `http://127.0.0.1:8787/admin` e faça login com essa senha — o console consegue alterar a senha, a API key do gateway e a Zen key, gravando as mudanças de volta no `config.json` na hora.

Prefere outra configuração? Edite o `config.json` à mão, aponte `-config /path/to/config.json` para outro arquivo, ou defina `OPENCODE_GATEWAY_NO_INIT=1` para pular completamente a geração.

**Requisitos:** Go 1.24+ para compilar. Sem CGO — o driver do SQLite é Go puro.

---

## ⚙️ Configuração

Existe exatamente **uma** fonte de configuração: o `config.json`, por padrão no diretório de trabalho. O console web o edita; você também pode editá-lo à mão e reiniciar.

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

| Campo | Descrição |
|-------|-------------|
| `listen` | Endereço de escuta. Padrão `127.0.0.1:8787` (somente loopback) |
| `api_key` | Key que os clientes usam para acessar este gateway. Vazio = acesso somente via loopback |
| `admin_password` | Senha do console web. Vazio desabilita o `/admin` |
| `zen_keys` | Zen API keys para os modelos pagos. Várias keys fazem rodízio. Vazio = somente nível gratuito |
| `upstream` | URL base do upstream. Padrão `https://opencode.ai/zen` |
| `data_dir` | Diretório do banco de uso |
| `model_rules.model_blacklist` | IDs de modelos a ocultar de `/v1/models` e a rejeitar |

Variáveis de ambiente sobrescrevem o arquivo (útil para Docker / CI):

```bash
docker run -e OPENCODE_GATEWAY_API_KEY=sk-gw-... opencode-gateway
```

`OPENCODE_GATEWAY_LISTEN`, `_API_KEY`, `_ADMIN_PASSWORD`, `_UPSTREAM`, `_DATA_DIR` e `OPENCODE_ZEN_KEYS` são suportadas. Variáveis não definidas deixam o valor do arquivo intacto.

> `config.json` e `data/` estão ambos no `.gitignore` — eles contêm segredos e dados de uso locais.

### O que o console web consegue alterar

**API key do gateway** (com botão de gerar e cópia em um clique), **Zen paid keys** (com verificação de validade), **blacklist de modelos** e a **senha do console**. As alterações passam a valer imediatamente e são gravadas de volta no `config.json`. Alterar a senha do console invalida as sessões existentes.

### Logs

As requisições são gravadas como JSONL em `./logs/gateway-YYYY-MM-DD.log`, uma linha por evento, mantidas por **7 dias**. Requisições rejeitadas também são registradas, com o motivo. Os logs ficam em disco — é na aba **Requests** do console que se consulta o uso.

```bash
tail -f logs/gateway-$(date +%F).log   # acompanhar o de hoje
grep '"level":"error"' logs/*.log      # apenas falhas
```

---

## 🔌 Configuração do Cliente

### Claude Code

```bash
export ANTHROPIC_BASE_URL=http://127.0.0.1:8787
export ANTHROPIC_AUTH_TOKEN=sk-gw-your-secret
export ANTHROPIC_SMALL_FAST_MODEL=mimo-v2.6-flash-free
claude
```

Os nomes de modelos gratuitos não estão no catálogo interno do Claude Code, então passá-los com `--model` faz o cliente encerrar acusando unrecognized model. Faça o mapeamento com `modelOverrides` no `settings.json`:

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

Verificado e funcionando. Com uma Zen API key configurada, você pode usar nomes nativos como `claude-sonnet-5` diretamente, sem precisar de mapeamento.

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

`experimental_bearer_token` é o campo verificado como funcional; algumas versões do Codex usam uma sub-tabela `env_keys` — consulte a documentação da sua versão.

O Codex vai avisar `Model metadata ... not found. Defaulting to fallback metadata` porque o nome do modelo não está no catálogo interno dele. **É só um aviso** — o Codex estima a janela de contexto de forma conservadora e tudo continua funcionando. Para silenciar o aviso, declare a janela real no mesmo arquivo (`model_context_window = 200000`).

### OpenAI SDK / ferramentas compatíveis

```python
from openai import OpenAI

client = OpenAI(base_url="http://127.0.0.1:8787/v1", api_key="sk-gw-your-secret")
resp = client.chat.completions.create(
    model="mimo-v2.6-flash-free",
    messages=[{"role": "user", "content": "Hello"}],
)
```

---

## 🧠 Como Funciona

O nível gratuito da Zen (`Authorization: Bearer public`) valida no servidor que a requisição vem do cliente OpenCode real. Regras descobertas por engenharia reversa com testes diferenciais:

1. `tools` precisa conter **tanto** uma ferramenta `bash` quanto uma `read` (o conteúdo não importa)
2. `stream` precisa ser `true`
3. `x-opencode-session` precisa corresponder a `ses_<12 hex><14 base62>`
4. Um `User-Agent` específico e `x-opencode-client: cli`

O gateway injeta essa assinatura e chama o `/v1/chat/completions` da Zen diretamente, depois converte o fluxo de eventos para o protocolo que o cliente fala. Clientes sem streaming recebem a resposta agregada. Modelos pagos são encaminhados ao pé da letra, conforme o mapeamento modelo→endpoint da documentação da Zen.

```
client (Claude Code / Codex / OpenAI SDK)
  │  /v1/messages | /v1/responses | /v1/chat/completions
  ▼
camada protocolo   três protocolos ⇄ uma representação intermediária
  ▼
camada upstream    zen: injeta a assinatura do tier gratuito / encaminha com Bearer key
  ▼
camada de uso      contabilidade em SQLite ─► console web
camada telemetria  logs JSONL diários (retenção de 7 dias)
```

---

## 📋 Solução de Problemas

Comece pelo log — toda requisição traz seu modelo, status e motivo, inclusive as rejeitadas:

```bash
tail -20 logs/gateway-$(date +%F).log
```

A aba **Requests** do console mostra o mesmo, com `rejected` na coluna de canal.

| Sintoma | Causa e solução |
|---------|---------------|
| `402 paid model requires a configured zen API key` | O nome do modelo é de um modelo pago na Zen (sem o sufixo `-free`). Use um modelo gratuito ou adicione uma Zen API key |
| `400 Model is unavailable` | Esse modelo gratuito está temporariamente fora do ar no upstream — troque de modelo |
| `404 model "x" not found` | Não é um ID de modelo da Zen. Confira com `GET /v1/models` |
| O Claude Code encerra com `unrecognized model` | Nomes de modelos gratuitos não estão no catálogo dele; mapeie com `modelOverrides` |
| O Codex avisa `Model metadata ... not found` | Apenas cosmético — afeta só a estimativa de contexto |
| Os cards de modelo mostram "loading model catalog" | Acabou de iniciar; o catálogo é preenchido em alguns segundos |

---

## ⚠️ Limitações

- A disponibilidade dos modelos gratuitos é controlada pela Zen e muda sem aviso
- Sem embeddings, geração de imagens, nem retrieval/cancel do Responses
- Um stream já iniciado não é repetido em outro caminho

---

## 📄 Licença

MIT
