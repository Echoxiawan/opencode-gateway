// Copyright (c) 2026 Echoxiawan
// SPDX-License-Identifier: MIT
// https://github.com/Echoxiawan/opencode-gateway

// opencode-gateway admin console
//
// Data flow: fetch*() loads into `state` and then calls the matching
// render*(). Renderers are idempotent, read only from `state`, and skip work
// when their panel is hidden — a hidden panel has zero layout width, which
// would otherwise draw the canvas at an invalid size and blank the chart.
// Switching tabs re-renders from cache immediately, then refreshes in the
// background, so a panel is never empty after a switch.
(function () {
  "use strict";

  var token = localStorage.getItem("gw_token") || "";
  var gwBase = window.location.origin;
  var currentCtab = "claude";
  var currentAPIKey = "";

  // The gateway host may sit in a different timezone than whoever is looking
  // at the console. Every admin call carries the browser's offset so "today"
  // and the daily chart follow the viewer's calendar, not the server's.
  // JS returns minutes west of UTC, the API wants minutes east.
  var TZ = -new Date().getTimezoneOffset();

  var PROTO_LABEL = { chat: "OpenAI Chat", anthropic: "Anthropic", responses: "Responses" };
  var CHANNEL_LABEL = { free: "免费", paid: "付费", rejected: "被拒" };

  // Cached server state.
  var state = {
    overview: null, // /api/overview
    usage: null,    // /api/usage
    requests: null, // /api/requests
    config: null,   // /api/config
  };

  function $(id) { return document.getElementById(id); }
  function show(id) { $(id).classList.remove("hidden"); }
  function hide(id) { $(id).classList.add("hidden"); }

  function isTabActive(name) {
    var panel = $("tab-" + name);
    return panel && !panel.classList.contains("hidden");
  }

  // ---------- login ----------
  function tryLogin() {
    var pass = $("login-pass").value;
    fetch(gwBase + "/api/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ password: pass }),
    })
      .then(function (r) { return r.json().then(function (d) { return { ok: r.ok, d: d }; }); })
      .then(function (res) {
        if (!res.ok) {
          $("login-err").textContent = res.d.error || "登录失败";
          return;
        }
        token = res.d.token;
        localStorage.setItem("gw_token", token);
        enterMain();
      })
      .catch(function (e) {
        $("login-err").textContent = "登录失败: " + e.message;
      });
  }

  function enterMain() {
    hide("login-view");
    show("main-view");
    // Prefetch everything so tab switches render instantly from cache.
    refreshAll();
    if (!refreshTimer) refreshTimer = setInterval(refreshAll, 15000);
    // Browsers throttle timers in background tabs, so a console left open in
    // another tab can sit on stale numbers for minutes. Refresh as soon as it
    // is looked at again, but not more often than every few seconds. Bound once
    // — enterMain runs again on every login, and re-adding a listener per login
    // would stack up duplicates.
    if (!wakeBound) {
      wakeBound = true;
      document.addEventListener("visibilitychange", function () {
        if (!document.hidden) refreshIfStale(5000);
      });
      window.addEventListener("focus", function () { refreshIfStale(5000); });
    }
  }

  var lastRefreshAt = 0;
  var refreshTimer = null;
  var wakeBound = false;
  function refreshIfStale(minAgeMS) {
    if (Date.now() - lastRefreshAt < minAgeMS) return;
    refreshAll();
  }

  function authHeaders() {
    return { "Authorization": "Bearer " + token, "Content-Type": "application/json" };
  }

  function api(path) {
    return fetch(gwBase + path, { headers: authHeaders() }).then(function (r) {
      if (r.status === 401) {
        token = "";
        localStorage.removeItem("gw_token");
        state = { overview: null, usage: null, requests: null, config: null };
        show("login-view");
        hide("main-view");
        // Stop polling while logged out: otherwise every timer tick hammers
        // the admin API with 401s until someone logs back in.
        if (refreshTimer) { clearInterval(refreshTimer); refreshTimer = null; }
        throw new Error("unauthorized");
      }
      if (!r.ok) throw new Error("HTTP " + r.status);
      return r.json();
    });
  }

  // ---------- tabs ----------
  function switchTab(name) {
    document.querySelectorAll(".tab-btn").forEach(function (b) {
      b.classList.toggle("active", b.dataset.tab === name);
    });
    document.querySelectorAll(".tab-panel").forEach(function (p) {
      p.classList.add("hidden");
    });
    show("tab-" + name);

    // Render from cache first (instant), then refresh in the background.
    switch (name) {
      case "models":
        renderModelsTab();
        bg(fetchOverview());
        bg(fetchUsage());
        break;
      case "requests":
        renderRequestFilter();
        renderRequestTable();
        bg(fetchRequests());
        break;
      case "config":
        renderConfigForm();
        bg(fetchConfig());
        break;
      case "connect":
        renderSnippet();
        bg(fetchConfig());
        break;
    }
  }

  document.querySelectorAll(".tab-btn").forEach(function (btn) {
    btn.addEventListener("click", function () { switchTab(btn.dataset.tab); });
  });

  document.querySelectorAll(".ctab-btn").forEach(function (btn) {
    btn.addEventListener("click", function () {
      document.querySelectorAll(".ctab-btn").forEach(function (b) { b.classList.remove("active"); });
      btn.classList.add("active");
      currentCtab = btn.dataset.ctab;
      renderSnippet();
    });
  });

  // ---------- helpers ----------
  function fmtNum(n) {
    if (n >= 1e9) return (n / 1e9).toFixed(2) + "B";
    if (n >= 1e6) return (n / 1e6).toFixed(2) + "M";
    if (n >= 1e3) return (n / 1e3).toFixed(1) + "K";
    return String(n || 0);
  }

  function esc(s) {
    return String(s == null ? "" : s).replace(/[&<>"']/g, function (c) {
      return { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c];
    });
  }

  function pad2(n) { return (n < 10 ? "0" : "") + n; }

  // Request times arrive as epoch milliseconds; render them in the browser's
  // timezone rather than slicing the server's UTC string.
  function fmtLocalTime(ms, fallback) {
    if (!ms) return fallback ? String(fallback).replace("T", " ").slice(0, 19) : "";
    var d = new Date(ms);
    return d.getFullYear() + "-" + pad2(d.getMonth() + 1) + "-" + pad2(d.getDate()) +
      " " + pad2(d.getHours()) + ":" + pad2(d.getMinutes()) + ":" + pad2(d.getSeconds());
  }

  function fmtClock(ms) {
    var d = new Date(ms);
    return pad2(d.getHours()) + ":" + pad2(d.getMinutes()) + ":" + pad2(d.getSeconds());
  }

  // Local-midnight boundaries in epoch ms, used for the time-range filter.
  function dayStart(daysAgo) {
    var d = new Date();
    d.setHours(0, 0, 0, 0);
    d.setDate(d.getDate() - daysAgo);
    return d.getTime();
  }

  function timeRange() {
    var v = $("req-time-filter").value;
    if (v === "today") return { since: dayStart(0), until: dayStart(-1) };
    if (v === "7d") return { since: dayStart(6), until: dayStart(-1) };
    if (v === "30d") return { since: dayStart(29), until: dayStart(-1) };
    return null; // all
  }

  // "Failed to fetch" is what the browser says when the gateway is down or
  // unreachable; the operator cares about the cause, not the exception text.
  function reasonOf(e) {
    var m = (e && e.message) || "";
    if (/Failed to fetch|NetworkError|load failed/i.test(m)) return "无法连接网关";
    return m || "未知错误";
  }

  function showError(msg) {
    var b = $("error-banner");
    b.textContent = msg;
    b.classList.remove("hidden");
  }
  function clearError() { $("error-banner").classList.add("hidden"); }

  function statCard(label, value, sub) {
    return '<div class="stat-card"><div class="label">' + esc(label) + '</div>' +
      '<div class="value">' + esc(value) + '</div>' +
      (sub ? '<div class="sub">' + esc(sub) + "</div>" : "") + "</div>";
  }

  // ---------- models tab ----------
  // The model catalog loads asynchronously at startup; poll a few times so a
  // freshly started gateway fills in without waiting for the 15s cycle.
  var catalogRetries = 0;
  var catalogTimer = null;
  function scheduleCatalogRetry() {
    if (catalogTimer || catalogRetries >= 6) return;
    catalogRetries++;
    catalogTimer = setTimeout(function () {
      catalogTimer = null;
      bg(fetchOverview());
    }, 1500);
  }
  function renderModelsTab() {
    if (!isTabActive("models")) return; // hidden panel has zero width
    if (state.overview) {
      renderSummary(state.overview);
      renderModels(state.overview.models || [], state.overview.loading);
    }
    if (state.usage) renderChart(state.usage.series);
  }

  function renderSummary(overview) {
    var models = overview.models || [];
    var freeCount = models.filter(function (m) { return m.free; }).length;
    var paidCount = models.length - freeCount;
    var totalIn = 0, totalOut = 0, totalCost = 0, totalReq = 0;
    models.forEach(function (m) {
      if (!m.usage) return;
      totalIn += m.usage.input_tokens || 0;
      totalOut += m.usage.output_tokens || 0;
      totalReq += m.usage.requests || 0;
      totalCost += m.usage.cost_usd || 0;
    });
    $("summary-row").innerHTML =
      statCard("模型总数", models.length, freeCount + " 免费 · " + paidCount + " 付费") +
      statCard("累计请求", totalReq, "") +
      statCard("输入 tokens", fmtNum(totalIn), "") +
      statCard("输出 tokens", fmtNum(totalOut), "") +
      statCard("估算费用", "$" + totalCost.toFixed(4), "按 models.dev 定价估算");
  }

  function renderModels(models, loading) {
    // Most-used first, ties broken by model id. The tie-breaker is what keeps
    // the grid from reshuffling on every refresh: without it, the order of
    // equally-unused models followed whatever the server happened to send.
    var sorted = models.slice().sort(function (a, b) {
      var ua = a.usage ? a.usage.input_tokens + a.usage.output_tokens : 0;
      var ub = b.usage ? b.usage.input_tokens + b.usage.output_tokens : 0;
      if (ub !== ua) return ub - ua;
      return a.id < b.id ? -1 : a.id > b.id ? 1 : 0;
    });
    if (sorted.length === 0) {
      $("model-grid").innerHTML = '<div class="hint">' +
        (loading ? "正在加载模型目录…" : "暂无可用模型（上游不可用或已被黑名单隐藏）") + "</div>";
      if (loading) scheduleCatalogRetry();
      return;
    }
    $("model-grid").innerHTML = sorted.map(function (m) {
      var u = m.usage || {};
      var badge = m.free ? '<span class="badge free">免费</span>' : '<span class="badge paid">付费</span>';
      var costLine = !m.free && (m.cost_input_per_m || m.cost_output_per_m)
        ? '<div class="cost">$' + m.cost_input_per_m + ' / $' + m.cost_output_per_m + " 每 1M tokens</div>" : "";
      return '<div class="model-card">' +
        '<div class="top"><span class="name">' + esc(m.id) + "</span>" + badge + "</div>" +
        '<div class="stats">' +
        '<div class="stat"><div class="k">请求</div><div class="v">' + (u.requests || 0) + "</div></div>" +
        '<div class="stat"><div class="k">今日输入</div><div class="v">' + fmtNum(u.today_input_tokens || 0) + "</div></div>" +
        '<div class="stat"><div class="k">今日输出</div><div class="v">' + fmtNum(u.today_output_tokens || 0) + "</div></div>" +
        '<div class="stat"><div class="k">累计费用</div><div class="v">$' + ((u.cost_usd || 0).toFixed(4)) + "</div></div>" +
        "</div>" + costLine + "</div>";
    }).join("");
  }

  function renderChart(series) {
    var canvas = $("usage-chart");
    if (!canvas) return;
    var W = canvas.parentElement.clientWidth - 32;
    var H = 180;
    // The panel is hidden (display:none) so layout width is 0 — drawing now
    // would size the canvas to an invalid value and leave it blank. Skip;
    // switchTab renders again once the panel is visible.
    if (W <= 0) return;

    var ctx = canvas.getContext("2d");
    var dpr = window.devicePixelRatio || 1;
    canvas.width = Math.round(W * dpr);
    canvas.height = Math.round(H * dpr);
    canvas.style.width = W + "px";
    canvas.style.height = H + "px";
    // setTransform (not scale) so repeated renders don't compound the ratio.
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, W, H);

    if (!series || series.length === 0) {
      ctx.fillStyle = "#8b93a3";
      ctx.font = "13px sans-serif";
      ctx.fillText("暂无数据", 12, 24);
      return;
    }

    var maxVal = Math.max(1, Math.max.apply(null, series.map(function (d) {
      return (d.input_tokens || 0) + (d.output_tokens || 0);
    })));
    var pad = { l: 44, r: 10, t: 10, b: 26 };
    var plotW = W - pad.l - pad.r;
    var plotH = H - pad.t - pad.b;
    var n = series.length;

    ctx.strokeStyle = "#2a3140";
    ctx.fillStyle = "#8b93a3";
    ctx.font = "10px sans-serif";
    ctx.textAlign = "right";
    for (var g = 0; g <= 3; g++) {
      var gy = pad.t + plotH * g / 3;
      ctx.beginPath();
      ctx.moveTo(pad.l, gy);
      ctx.lineTo(W - pad.r, gy);
      ctx.stroke();
      ctx.fillText(fmtNum(Math.round(maxVal * (3 - g) / 3)), pad.l - 6, gy + 3);
    }

    ctx.textAlign = "center";
    var step = Math.max(1, Math.ceil(n / 14));
    for (var i = 0; i < n; i++) {
      if (i % step !== 0 && i !== n - 1) continue;
      var cx = pad.l + plotW * (i + 0.5) / n;
      var d = series[i];
      ctx.fillStyle = "#8b93a3";
      ctx.fillText(String(d.date || "").slice(5), cx, H - 8);
      var total = (d.input_tokens || 0) + (d.output_tokens || 0);
      if (total === 0) continue;
      var barH = plotH * total / maxVal;
      var bw = Math.min(plotW / n * 0.7, 28);
      ctx.fillStyle = "#4f8cff";
      ctx.fillRect(cx - bw / 2, pad.t + plotH - barH, bw, barH);
    }
    ctx.textAlign = "left";
  }

  // ---------- requests tab ----------
  function renderRequestFilter() {
    if (!state.requests) return;
    var sel = $("req-model-filter");
    var current = sel.value;
    var seen = {};
    // Records rejected before the model was parsed have no model; skip them
    // rather than adding a blank option to the dropdown.
    state.requests.forEach(function (r) { if (r.model) seen[r.model] = true; });
    var ids = Object.keys(seen).sort();
    sel.innerHTML = '<option value="">全部模型</option>' +
      ids.map(function (id) { return '<option value="' + esc(id) + '">' + esc(id) + "</option>"; }).join("");
    if (current && seen[current]) sel.value = current;
  }

  function renderRequestTable() {
    if (!isTabActive("requests")) return;
    var rows = state.requests || [];
    var modelFilter = $("req-model-filter").value;
    var protoFilter = $("req-proto-filter").value;
    rows = rows.filter(function (r) {
      if (modelFilter && r.model !== modelFilter) return false;
      if (protoFilter && r.protocol !== protoFilter) return false;
      return true;
    });

    var sumIn = 0, sumOut = 0, sumReason = 0, sumCache = 0, sumCost = 0, ok = 0;
    rows.forEach(function (r) {
      sumIn += r.input_tokens || 0;
      sumOut += r.output_tokens || 0;
      sumReason += r.reasoning_tokens || 0;
      sumCache += r.cache_read_tokens || 0;
      sumCost += r.cost_usd || 0;
      if (r.status >= 200 && r.status < 300) ok++;
    });
    $("req-summary").textContent = rows.length === 0
      ? "无记录"
      : rows.length + " 条（成功 " + ok + "）· 输入 " + fmtNum(sumIn) +
        " · 输出 " + fmtNum(sumOut) + " · 推理 " + fmtNum(sumReason) +
        " · 缓存 " + fmtNum(sumCache) + " · 费用 $" + sumCost.toFixed(6);

    var tbody = document.querySelector("#req-table tbody");
    if (rows.length === 0) {
      tbody.innerHTML = '<tr><td colspan="12" class="hint">暂无记录</td></tr>';
      return;
    }
    tbody.innerHTML = rows.map(function (r) {
      var statusCls = r.status >= 200 && r.status < 300 ? "ok" : "bad";
      var id = esc(r.id || "");
      // Session is the client's session hint when it sends one, otherwise the
      // gateway falls back to the request id — worth showing, but not as a
      // second identical line.
      var sess = r.session && r.session !== r.id ? esc(r.session) : "—";
      var proto = PROTO_LABEL[r.protocol] || r.protocol || "";
      var chan = CHANNEL_LABEL[r.channel] || r.channel || "";
      return "<tr>" +
        "<td>" + esc(fmtLocalTime(r.time_ms, r.time)) + "</td>" +
        '<td class="mono">' + esc(r.model || "—") + "</td>" +
        '<td class="cell-stack">' + sess +
        '<div class="sub-id mono" title="' + id + '">' + id + "</div></td>" +
        "<td>" + esc(proto) + "</td>" +
        '<td class="' + (r.channel === "rejected" ? "bad" : "") + '">' + esc(chan) + "</td>" +
        '<td class="' + statusCls + '">' + r.status + "</td>" +
        "<td>" + fmtNum(r.input_tokens) + "</td>" +
        "<td>" + fmtNum(r.output_tokens) + "</td>" +
        "<td>" + fmtNum(r.reasoning_tokens) + "</td>" +
        "<td>" + fmtNum(r.cache_read_tokens) + "</td>" +
        "<td>" + r.duration_ms + "ms</td>" +
        "<td>$" + (r.cost_usd || 0).toFixed(6) + "</td>" +
        "</tr>";
    }).join("");
  }

  // ---------- config tab ----------
  function renderConfigForm() {
    if (!isTabActive("config")) return;
    var cfg = state.config;
    if (!cfg) return;
    // Don't clobber what the user is currently editing.
    if (document.activeElement !== $("cfg-apikey")) $("cfg-apikey").value = cfg.api_key || "";
    if (document.activeElement !== $("cfg-keys")) $("cfg-keys").value = (cfg.zen_keys || []).join("\n");
    if (document.activeElement !== $("cfg-blacklist")) {
      $("cfg-blacklist").value = (cfg.model_blacklist || []).join("\n");
    }
    currentAPIKey = cfg.api_key || "";
    $("server-info").textContent =
      "上游: " + cfg.upstream +
      " · 监听: " + cfg.listen +
      " · 配置来源: config.json（环境变量可覆盖）";
  }

  function saveConfig() {
    var keys = $("cfg-keys").value.split("\n").map(function (s) { return s.trim(); }).filter(Boolean);
    var blacklist = $("cfg-blacklist").value.split("\n").map(function (s) { return s.trim(); }).filter(Boolean);
    var payload = {
      api_key: $("cfg-apikey").value.trim(),
      zen_keys: keys,
      model_blacklist: blacklist,
    };
    var newPass = $("cfg-adminpass").value.trim();
    if (newPass) payload.admin_password = newPass;

    var el = $("save-result");
    el.textContent = "保存中…";
    fetch(gwBase + "/api/config", {
      method: "POST",
      headers: authHeaders(),
      body: JSON.stringify(payload),
    }).then(function (r) { return r.json(); }).then(function (d) {
      if (!d.ok) { el.textContent = d.error || "保存失败"; return; }
      el.textContent = newPass ? "已保存 ✓（密码已更新，请重新登录）" : "已保存 ✓";
      $("cfg-adminpass").value = "";
      // Refresh cached config and any view that shows the key.
      bg(fetchConfig());
      bg(fetchOverview()); // blacklist changes the model list
      renderSnippet();
      setTimeout(function () { el.textContent = ""; }, 4000);
    }).catch(function (e) { el.textContent = "保存失败: " + e.message; });
  }

  function genAPIKey() {
    var chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789";
    var buf = new Uint8Array(18);
    crypto.getRandomValues(buf);
    var s = "sk-gw-";
    for (var i = 0; i < buf.length; i++) s += chars[buf[i] % chars.length];
    $("cfg-apikey").value = s;
  }

  function verifyKey() {
    var el = $("verify-result");
    el.textContent = "验证中…";
    fetch(gwBase + "/api/keys/verify", {
      method: "POST",
      headers: authHeaders(),
      body: JSON.stringify({}),
    }).then(function (r) { return r.json(); }).then(function (d) {
      if (d.valid) {
        el.textContent = "✓ Key 有效";
      } else if (d.status === 401) {
        el.textContent = "✗ Key 无效（401）：" + (d.detail || "");
      } else {
        el.textContent = "状态 " + d.status + "：" + (d.detail || "");
      }
    }).catch(function (e) { el.textContent = "验证失败: " + e.message; });
  }

  // ---------- connect tab ----------
  var SNIPPETS = {
    claude: function (key) {
      return [
        "# Claude Code 接入 opencode-gateway",
        "export ANTHROPIC_BASE_URL=" + gwBase,
        "export ANTHROPIC_AUTH_TOKEN=" + (key || "<你的网关APIKey>"),
        "",
        "# 免费模型不在 Claude Code 本地目录中，需用 modelOverrides 映射",
        "# （详见 README「客户端接入」一节）",
      ].join("\n");
    },
    codex: function (key) {
      return [
        "# Codex CLI 接入 opencode-gateway",
        "# ~/.codex/config.toml",
        "[model_providers.opencode-gateway]",
        'name = "opencode-gateway"',
        'base_url = "' + gwBase + '/v1"',
        'wire_api = "responses"',
        "",
        "[model_providers.opencode-gateway.env_keys]",
        'OPENAI_API_KEY = "' + (key || "<你的网关APIKey>") + '"',
        "",
        "# 使用: codex --profile opencode-gateway",
      ].join("\n");
    },
    openai: function (key) {
      return [
        "# OpenAI SDK / 兼容客户端",
        "from openai import OpenAI",
        "",
        "client = OpenAI(",
        '    base_url="' + gwBase + '/v1",',
        '    api_key="' + (key || "<你的网关APIKey>") + '",',
        ")",
        "",
        "resp = client.chat.completions.create(",
        '    model="mimo-v2.6-flash-free",',
        '    messages=[{"role": "user", "content": "你好"}],',
        ")",
      ].join("\n");
    },
  };

  function renderSnippet() {
    if (!isTabActive("connect")) return;
    var key = (state.config && state.config.api_key) || currentAPIKey || "";
    $("connect-snippet").textContent = SNIPPETS[currentCtab](key);
    $("gw-key-display").textContent = key || "（未设置，仅本机可访问）";
  }

  // ---------- fetchers ----------
  // Fetchers reject on failure so refreshAll can report it; the places that
  // fire one off in the background wrap it in bg() to swallow the rejection.
  function bg(p) { p.catch(function () {}); return p; }

  function fetchOverview() {
    return api("/api/overview?tz=" + TZ).then(function (d) {
      state.overview = d;
      // The status dot reflects upstream health regardless of which tab is
      // open, so update it here rather than inside the models renderer.
      var cls = "status-dot";
      if ((d.models || []).length > 0) cls += " ok";
      else if (!d.loading) cls += " bad"; // still loading → neutral
      $("status-dot").className = cls;
      renderModelsTab();
    }).catch(function (e) {
      if (e.message !== "unauthorized") $("status-dot").className = "status-dot bad";
      throw e;
    });
  }

  function fetchUsage() {
    return api("/api/usage?days=30&tz=" + TZ).then(function (d) {
      state.usage = d;
      renderModelsTab();
    });
  }

  function fetchRequests() {
    // The time range is applied server-side: the console only holds the most
    // recent page of records, so filtering client-side would silently drop
    // rows that fall inside the range.
    var q = "/api/requests?limit=500&tz=" + TZ;
    var range = timeRange();
    if (range) q += "&since_ms=" + range.since + "&until_ms=" + range.until;
    return api(q).then(function (d) {
      state.requests = d.requests || [];
      renderRequestFilter();
      renderRequestTable();
    });
  }

  function fetchConfig() {
    return api("/api/config").then(function (c) {
      state.config = c;
      renderConfigForm();
      renderSnippet();
    });
  }

  // refreshAll reloads the cached data. Renderers decide whether their panel
  // is visible, so background refreshes never blank a hidden chart. Any
  // failure is surfaced: a console quietly showing stale numbers is exactly
  // the failure mode that is impossible to notice from the outside.
  function refreshAll() {
    // While the login view is up there is nothing to refresh, and firing the
    // admin calls anyway would answer every focus / visibility event with
    // another round of 401s.
    if (!token) return Promise.resolve();
    lastRefreshAt = Date.now();
    var jobs = [fetchOverview(), fetchUsage()];
    if (isTabActive("requests")) jobs.push(fetchRequests());
    // Config is tiny and feeds both the config form and the connect snippets;
    // fetch it once on the first pass so switching tabs never shows a
    // placeholder, then only refresh it while its tabs are open.
    if (!state.config || isTabActive("config") || isTabActive("connect")) jobs.push(fetchConfig());

    return Promise.all(jobs).then(function () {
      clearError();
      $("refresh-note").textContent = "最后更新 " + fmtClock(Date.now());
    }).catch(function (e) {
      if (e && e.message === "unauthorized") return; // login view is showing
      showError("刷新失败：" + reasonOf(e) + "（下方数据可能已过期）");
    });
  }

  // ---------- clipboard ----------
  function copyToClipboard(text, btn) {
    var label = btn.dataset.label || btn.textContent;
    if (!text) {
      btn.textContent = "无内容";
      setTimeout(function () { btn.textContent = label; }, 1200);
      return;
    }
    navigator.clipboard.writeText(text).then(function () {
      btn.textContent = "已复制 ✓";
      setTimeout(function () { btn.textContent = label; }, 1500);
    }).catch(function () {
      // Clipboard API needs a secure context; fall back to manual copy.
      btn.textContent = "请手动复制";
      setTimeout(function () { btn.textContent = label; }, 1500);
    });
  }

  function copySnippet() {
    copyToClipboard($("connect-snippet").textContent, $("copy-btn"));
  }
  function copyAPIKey() {
    copyToClipboard($("cfg-apikey").value.trim(), $("copy-apikey-btn"));
  }
  function copyGWKey() {
    var key = (state.config && state.config.api_key) || currentAPIKey || "";
    copyToClipboard(key, $("copy-gwkey-btn"));
  }

  // ---------- events ----------
  $("login-btn").addEventListener("click", tryLogin);
  $("login-pass").addEventListener("keydown", function (e) {
    if (e.key === "Enter") tryLogin();
  });
  $("save-cfg-btn").addEventListener("click", saveConfig);
  $("gen-apikey-btn").addEventListener("click", genAPIKey);
  $("clear-apikey-btn").addEventListener("click", function () { $("cfg-apikey").value = ""; });
  $("copy-apikey-btn").addEventListener("click", copyAPIKey);
  $("copy-gwkey-btn").addEventListener("click", copyGWKey);
  $("verify-key-btn").addEventListener("click", verifyKey);
  $("copy-btn").addEventListener("click", copySnippet);
  $("req-refresh-btn").addEventListener("click", function () { bg(refreshAll()); });
  $("refresh-btn").addEventListener("click", function () { bg(refreshAll()); });
  $("req-model-filter").addEventListener("change", renderRequestTable);
  $("req-proto-filter").addEventListener("change", renderRequestTable);
  // Time range changes what the server returns, so it refetches.
  $("req-time-filter").addEventListener("change", function () { bg(fetchRequests()); });

  // Redraw the chart on resize (debounced); it is sized in device pixels.
  var resizeTimer = null;
  window.addEventListener("resize", function () {
    if (resizeTimer) clearTimeout(resizeTimer);
    resizeTimer = setTimeout(function () {
      if (isTabActive("models") && state.usage) renderChart(state.usage.series);
    }, 150);
  });

  // ---------- boot ----------
  if (token) {
    // Optimistically enter; a 401 from any call bounces back to login.
    enterMain();
  } else {
    show("login-view");
  }
})();
