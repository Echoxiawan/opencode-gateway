<div align="center">

# 🚪 OpenCode Gateway

**Gateway proxy untuk OpenCode Zen — API yang kompatibel dengan OpenAI / Anthropic / Codex**

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](../../LICENSE)
[![Go 1.24+](https://img.shields.io/badge/Go-1.24+-00ADD8.svg)](https://go.dev/)
[![No CGO](https://img.shields.io/badge/CGO-not%20required-green.svg)](#-mulai-cepat)

*Gunakan model OpenCode Zen dari Claude Code, Codex CLI, Cursor, Cline, Roo Code, Kilo Code, OpenAI SDK, LangChain, Continue, dan alat lain apa pun yang kompatibel dengan OpenAI atau Anthropic*

[Model](#-model-yang-didukung) • [Fitur](#-fitur) • [Mulai Cepat](#-mulai-cepat) • [Konfigurasi](#%EF%B8%8F-konfigurasi) • [Menyiapkan Klien](#-menyiapkan-klien) • [Pemecahan Masalah](#-pemecahan-masalah)

[🇬🇧 English](../../README.md) • [🇨🇳 中文](../zh/README.md) • [🇯🇵 日本語](../ja/README.md) • [🇰🇷 한국어](../ko/README.md) • [🇪🇸 Español](../es/README.md) • [🇷🇺 Русский](../ru/README.md) • [🇧🇷 Português](../pt/README.md) • 🇮🇩 Indonesia

</div>

---

## 🤖 Model yang Didukung

Satu endpoint lokal yang menyediakan semua model yang ditawarkan OpenCode Zen — termasuk **tier gratis**, yang biasanya hanya bisa dipakai dari dalam klien OpenCode resmi.

**Model gratis** (tidak perlu API key, per 2026-09-29):

| Model | Catatan |
|-------|-------|
| `mimo-v2.6-flash-free` | MiMo flash — cepat, cocok untuk tugas agentik |
| `mimo-v2.5-free` | Rilis gratis MiMo sebelumnya |
| `space-bunny-free` | Model umum yang ringan |
| `longcat-2.5-preview-free` | Penalaran multimodal Meituan |
| `nemotron-3-ultra-free` | NVIDIA Nemotron |
| `nemotron-3.5-lightning-free` | NVIDIA Nemotron, tier cepat |
| `muse-spark-1.3-contributor-free` | Tier Contributor |
| `muse-spark-1.2-contributor-free` | Tier Contributor |
| `deepseek-v4-flash-free` | DeepSeek flash — *ketersediaan di sisi hulu bisa berubah* |
| `ling-3.0-flash-fin-free` | Ling flash — *ketersediaan di sisi hulu bisa berubah* |
| `jev-1.13-free` | TypeSafe System One |

**Model berbayar** (butuh Zen API key, tersedia 72): keluarga Claude, GPT, Gemini, Grok, DeepSeek, GLM, Kimi, MiniMax, Qwen, dan Big Pickle.

```bash
# Selalu jadi daftar acuan — model gratis ditandai dengan "free": true
curl -s http://127.0.0.1:8787/v1/models
```

> ⚠️ **Ketersediaan model gratis ditentukan oleh Zen dan bisa berubah tanpa pemberitahuan.** Model `-free` bisa berfungsi hari ini lalu mengembalikan `Model is unavailable` besok — daftar di atas adalah yang **didaftarkan** Zen, bukan jaminan bahwa Zen benar-benar **melayaninya**. Ganti model, atau tambahkan Zen API key untuk memakai tier berbayar.

---

## ✨ Fitur

| Fitur | Deskripsi |
|---------|-------------|
| 🔌 **API kompatibel OpenAI** | `POST /v1/chat/completions` — streaming dan non-streaming |
| 🔌 **API kompatibel Anthropic** | `POST /v1/messages` native untuk Claude Code |
| 🔌 **OpenAI Responses API** | `POST /v1/responses` native untuk Codex CLI |
| 📋 **Daftar model** | `GET /v1/models` dengan penanda gratis/berbayar |
| 🆓 **Dukungan tier gratis** | Menyuntikkan signature klien OpenCode — tanpa perlu subprocess CLI |
| 💳 **Dukungan tier berbayar** | Penerusan langsung dengan round-robin di antara beberapa Zen key |
| 📊 **Pelacakan pemakaian** | SQLite mencatat token, model, protokol, dan biaya setiap permintaan |
| 📝 **Log berkas** | JSONL harian di `./logs/`, disimpan 7 hari |
| 🖥️ **Konsol web** | Kartu model, grafik 30 hari, log permintaan yang bisa difilter, editor konfigurasi |
| 🔑 **Manajemen kunci** | Buat, salin, dan rotasi gateway key dari konsol |
| 🪶 **Satu binary** | Go murni, tanpa CGO, tanpa Node.js, tanpa server basis data |

---

## 🚀 Mulai Cepat

```bash
# Bangun
go build -o opencode-gateway .

# Jalankan (mendengarkan di 127.0.0.1:8787, model gratis langsung bisa dipakai)
./opencode-gateway
```

Pada eksekusi pertama, gateway membuat `config.json` di direktori kerja dan mencetak kredensial yang dihasilkan satu kali:

```
已生成配置文件: config.json
  Web 控制台密码: xxxxxxxxxxxx
  网关 API Key  : sk-gw-xxxxxxxxxxxxxxxxxx
  (可随时编辑该文件修改；控制台地址 http://127.0.0.1:8787/admin)
```

Buka `http://127.0.0.1:8787/admin` dan masuk dengan kata sandi tersebut — konsol bisa mengubah kata sandi, gateway API key, dan Zen key, lalu langsung menyimpan perubahannya kembali ke `config.json`.

Ingin cara lain? Sunting `config.json` secara manual, arahkan `-config /path/to/config.json` ke berkas lain, atau setel `OPENCODE_GATEWAY_NO_INIT=1` untuk melewati proses pembuatan berkas sama sekali.

**Persyaratan:** Go 1.24+ untuk build. Tanpa CGO — driver SQLite-nya Go murni.

---

## ⚙️ Konfigurasi

Sumber konfigurasi hanya ada **satu**: `config.json`, secara default di direktori kerja. Konsol web mengeditnya; Anda juga bisa mengeditnya manual lalu memulai ulang.

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

| Bidang | Deskripsi |
|-------|-------------|
| `listen` | Alamat bind. Default `127.0.0.1:8787` (hanya loopback) |
| `api_key` | Kunci yang dipakai klien untuk mengakses gateway ini. Kosong = hanya bisa diakses dari loopback |
| `admin_password` | Kata sandi konsol web. Kosong berarti `/admin` dinonaktifkan |
| `zen_keys` | Zen API key untuk model berbayar. Beberapa key akan dirotasi. Kosong = hanya tier gratis |
| `upstream` | Base URL hulu. Default `https://opencode.ai/zen` |
| `data_dir` | Direktori basis data pemakaian |
| `model_rules.model_blacklist` | ID model yang disembunyikan dari `/v1/models` dan ditolak |

Variabel lingkungan menimpa isi berkas (praktis untuk Docker / CI):

```bash
docker run -e OPENCODE_GATEWAY_API_KEY=sk-gw-... opencode-gateway
```

`OPENCODE_GATEWAY_LISTEN`, `_API_KEY`, `_ADMIN_PASSWORD`, `_UPSTREAM`, `_DATA_DIR`, dan `OPENCODE_ZEN_KEYS` didukung. Variabel yang tidak disetel tidak mengubah nilai di berkas.

> `config.json` dan `data/` keduanya ada di `.gitignore` — isinya kunci rahasia dan data pemakaian lokal Anda.

### Apa yang Bisa Diubah Lewat Konsol Web

**Gateway API key** (dengan tombol generate dan salin sekali klik), **Zen key berbayar** (dengan pemeriksaan validitas), **daftar hitam model**, dan **kata sandi konsol**. Perubahan langsung berlaku dan disimpan kembali ke `config.json`. Mengubah kata sandi konsol membuat sesi yang sudah ada tidak lagi berlaku.

### Log

Permintaan ditulis sebagai JSONL ke `./logs/gateway-YYYY-MM-DD.log`, satu baris per peristiwa, disimpan **7 hari**. Permintaan yang ditolak juga dicatat, lengkap dengan alasannya. Log hanya tersimpan di disk — pemakaian dilihat di tab **Requests** pada konsol.

```bash
tail -f logs/gateway-$(date +%F).log   # ikuti log hari ini
grep '"level":"error"' logs/*.log      # hanya kegagalan
```

---

## 🔌 Menyiapkan Klien

### Claude Code

```bash
export ANTHROPIC_BASE_URL=http://127.0.0.1:8787
export ANTHROPIC_AUTH_TOKEN=sk-gw-your-secret
export ANTHROPIC_SMALL_FAST_MODEL=mimo-v2.6-flash-free
claude
```

Nama model gratis tidak ada di katalog bawaan Claude Code, jadi memberikannya lewat `--model` membuat klien keluar karena model dianggap tidak dikenal. Petakan saja dengan `modelOverrides` di `settings.json`:

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

Sudah terverifikasi berfungsi. Dengan Zen API key yang sudah dikonfigurasi, Anda bisa langsung memakai nama native seperti `claude-sonnet-5` tanpa perlu pemetaan.

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

`experimental_bearer_token` adalah bidang yang terverifikasi berfungsi; sebagian versi Codex memakai sub-tabel `env_keys` — periksa dokumentasi versi yang Anda pakai.

Codex akan memperingatkan `Model metadata ... not found. Defaulting to fallback metadata` karena nama model tersebut tidak ada di katalog bawaannya. **Ini hanya peringatan** — Codex memperkirakan jendela konteks secara konservatif dan semuanya tetap berjalan normal. Untuk menghilangkannya, deklarasikan jendela konteks sebenarnya di berkas yang sama (`model_context_window = 200000`).

### OpenAI SDK / alat yang kompatibel

```python
from openai import OpenAI

client = OpenAI(base_url="http://127.0.0.1:8787/v1", api_key="sk-gw-your-secret")
resp = client.chat.completions.create(
    model="mimo-v2.6-flash-free",
    messages=[{"role": "user", "content": "Hello"}],
)
```

---

## 🧠 Cara Kerja

Tier gratis Zen (`Authorization: Bearer public`) memvalidasi di sisi server bahwa suatu permintaan benar-benar datang dari klien OpenCode asli. Aturan ini didapat lewat reverse engineering dengan pengujian diferensial:

1. `tools` harus memuat **keduanya**: tool `bash` dan `read` (isi bodynya tidak berpengaruh)
2. `stream` harus bernilai `true`
3. `x-opencode-session` harus cocok dengan `ses_<12 hex><14 base62>`
4. `User-Agent` tertentu dan `x-opencode-client: cli`

Gateway menyuntikkan signature tersebut lalu memanggil `/v1/chat/completions` milik Zen secara langsung, kemudian menerjemahkan aliran event ke protokol yang dipakai klien. Klien non-streaming menerima respons yang sudah diagregasi. Model berbayar diteruskan apa adanya mengikuti pemetaan model→endpoint di dokumentasi Zen.

```
client (Claude Code / Codex / OpenAI SDK)
  │  /v1/messages | /v1/responses | /v1/chat/completions
  ▼
lapisan protokol   tiga protokol ⇄ satu representasi perantara
  ▼
lapisan hulu       zen: suntikkan signature tier gratis / teruskan dengan Bearer key
  ▼
lapisan pemakaian  pencatatan SQLite ─► konsol web
lapisan telemetri  log JSONL harian (disimpan 7 hari)
```

---

## 📋 Pemecahan Masalah

Mulai dari log — setiap permintaan membawa model, status, dan alasannya, termasuk yang ditolak:

```bash
tail -20 logs/gateway-$(date +%F).log
```

Tab **Requests** di konsol menampilkan hal yang sama, dengan `rejected` di kolom channel.

| Gejala | Penyebab dan solusi |
|---------|---------------|
| `402 paid model requires a configured zen API key` | Nama model itu berbayar di Zen (tanpa sufiks `-free`). Pakai model gratis atau tambahkan Zen API key |
| `400 Model is unavailable` | Model gratis tersebut sedang tidak tersedia di sisi hulu — ganti model |
| `404 model "x" not found` | Bukan ID model Zen. Periksa lewat `GET /v1/models` |
| Claude Code keluar dengan `unrecognized model` | Nama model gratis tidak ada di katalognya; petakan dengan `modelOverrides` |
| Codex memperingatkan `Model metadata ... not found` | Hanya kosmetik — hanya memengaruhi estimasi konteks |
| Kartu model menampilkan "loading model catalog" | Baru saja dijalankan; katalog akan terisi dalam beberapa detik |

---

## ⚠️ Keterbatasan

- Ketersediaan model gratis dikendalikan Zen dan berubah tanpa pemberitahuan
- Tidak ada embeddings, pembuatan gambar, maupun retrieval/cancel pada Responses
- Stream yang sudah dimulai tidak dicoba ulang lewat jalur lain

---

## 📄 Lisensi

MIT
