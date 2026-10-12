// UI state machine: state -> render(). No framework.
(function () {
  const t = window.i18n;
  const api = window.api;
  const $ = (id) => document.getElementById(id);
  const MODES = ["video_audio", "video_only", "audio_only"];

  const state = {
    startup: { status: "loading", message: t.startup.checking, done: 0, total: 0, error: "" },
    ytdlpVersion: "",
    warnings: [],
    checkingUpdates: false,
    url: "",
    video: { status: "empty", info: null, error: null }, // empty | loading | ok | error
    mode: "video_audio",
    outputDir: "",
    job: { status: "idle", progress: null, result: null, error: null }, // idle | running | done | error | cancelled
  };

  // ------------------------------------------------------------ formatting
  function fmtDuration(sec) {
    sec = Math.max(0, Math.round(sec || 0));
    const h = Math.floor(sec / 3600);
    const m = Math.floor((sec % 3600) / 60);
    const s = sec % 60;
    const p = (n) => String(n).padStart(2, "0");
    return `${p(h)}:${p(m)}:${p(s)}`;
  }

  function fmtPercent(p) {
    return `${Math.round(p)} %`;
  }

  function baseName(path) {
    return String(path || "").split(/[\\/]/).pop();
  }

  function setHidden(el, hidden) {
    el.hidden = !!hidden;
  }

  // ---------------------------------------------------------------- render
  function renderStartup() {
    const s = state.startup;
    const overlay = $("startup");
    setHidden(overlay, s.status === "done");
    $("startup-title").textContent = s.status === "error" ? t.startup.errorTitle : t.appName;
    setHidden($("startup-spinner"), s.status !== "loading");
    $("startup-line").textContent = s.status === "loading" ? s.message : "";
    const showBar = s.status === "loading" && s.total > 0;
    setHidden($("startup-bar"), !showBar);
    if (showBar) $("startup-bar").firstElementChild.style.width = Math.min(100, (s.done / s.total) * 100) + "%";
    setHidden($("startup-error"), s.status !== "error");
    $("startup-error-msg").textContent = s.error;
    $("app").hidden = s.status !== "done";
    if (s.status === "done") $("app").removeAttribute("aria-hidden");
    else $("app").setAttribute("aria-hidden", "true");
  }

  function renderCard() {
    const v = state.video;
    const card = $("card");
    card.dataset.state = v.status;
    const msg = $("card-text");
    const img = $("thumb");
    if (v.status === "ok") {
      const i = v.info;
      msg.textContent = "";
      if (img.getAttribute("src") !== i.thumbnail) img.src = i.thumbnail || "";
      $("card-title").textContent = i.title;
      $("card-channel").textContent = i.uploader || "";
      const meta = [];
      meta.push(i.isLive ? t.card.live : fmtDuration(i.duration));
      if (i.maxHeight) meta.push(t.card.maxQuality(i.maxHeight));
      $("card-meta").textContent = meta.join(" · ");
    } else {
      img.removeAttribute("src");
      $("card-title").textContent = "";
      $("card-channel").textContent = "";
      $("card-meta").textContent = "";
      msg.textContent =
        v.status === "loading" ? t.card.loading : v.status === "error" ? v.error.message : t.card.empty;
    }
  }

  function renderModes() {
    const box = $("modes");
    if (!box.children.length) {
      MODES.forEach((m) => {
        const lab = document.createElement("label");
        lab.className = "seg";
        const input = document.createElement("input");
        input.type = "radio";
        input.name = "mode";
        input.value = m;
        input.addEventListener("change", () => setMode(m));
        const body = document.createElement("span");
        body.className = "seg-body";
        const ic = document.createElement("span");
        ic.className = "seg-icon";
        ic.setAttribute("aria-hidden", "true");
        ic.textContent = t.mode[m].icon;
        const tx = document.createElement("span");
        tx.className = "seg-text";
        const a = document.createElement("span");
        a.textContent = t.mode[m].label;
        const b = document.createElement("small");
        b.textContent = t.mode[m].ext;
        tx.append(a, b);
        body.append(ic, tx);
        lab.append(input, body);
        box.append(lab);
      });
    }
    const running = state.job.status === "running";
    box.querySelectorAll("input").forEach((i) => {
      i.checked = i.value === state.mode;
      i.disabled = running;
    });
  }

  function renderStatus() {
    const j = state.job;
    const el = $("status");
    el.dataset.state = j.status;
    setHidden(el, j.status === "idle");
    if (j.status === "idle") return;

    const icon = $("status-icon");
    const title = $("status-title");
    const sub = $("status-sub");
    const bar = $("progress");
    setHidden(bar, j.status !== "running");
    setHidden($("result-actions"), j.status !== "done");
    setHidden($("detail"), !(j.status === "error" && j.error && j.error.detail));

    if (j.status === "running") {
      const p = j.progress;
      icon.textContent = "";
      title.textContent = p && p.phase ? p.phase : t.status.starting;
      const parts = [];
      if (p) {
        parts.push(fmtPercent(p.percent || 0));
        if (p.speed) parts.push(p.speed);
        if (p.eta) parts.push(t.status.remaining(p.eta));
      }
      sub.textContent = parts.join(" · ");
      const pct = p ? Math.min(100, p.percent || 0) : 0;
      bar.firstElementChild.style.width = pct + "%";
      bar.setAttribute("aria-valuenow", String(Math.round(pct)));
      bar.classList.toggle("indeterminate", !p);
    } else if (j.status === "done") {
      icon.textContent = "✓";
      title.textContent = t.status.successTitle;
      sub.textContent = baseName(j.result.filePath);
      sub.title = j.result.filePath;
    } else if (j.status === "error") {
      icon.textContent = "✕";
      title.textContent = t.status.errorTitle;
      sub.textContent = j.error.message;
      $("detail-text").textContent = j.error.detail || "";
    } else if (j.status === "cancelled") {
      icon.textContent = "—";
      title.textContent = t.status.cancelled;
      sub.textContent = "";
    }
    if (j.status !== "done") sub.removeAttribute("title");
  }

  function render() {
    renderStartup();
    if (state.startup.status !== "done") return;

    const running = state.job.status === "running";
    const w = $("warnings");
    setHidden(w, !state.warnings.length);
    w.textContent = state.warnings.join(" · ");

    renderCard();
    renderModes();

    const out = $("outdir");
    out.placeholder = t.output.none;
    out.title = state.outputDir || "";
    middleTruncate(state.outputDir || "", out);
    $("url").disabled = running;
    $("paste").disabled = running;
    $("browse").disabled = running;

    $("download").disabled = running || state.video.status !== "ok" || !state.outputDir;
    setHidden($("cancel"), !running);
    setHidden($("download"), running);

    renderStatus();

    const ytdlp = state.ytdlpVersion ? t.footer.version(state.ytdlpVersion) : t.footer.versionUnknown;
    $("version").textContent = state.appVersion ? `v${state.appVersion} · ${ytdlp}` : ytdlp;
    const cu = $("check-updates");
    cu.textContent = state.checkingUpdates ? t.footer.checking : t.footer.checkUpdates;
    cu.disabled = running || state.checkingUpdates;
  }

  // The read-only input is truncated by CSS at the end; for the middle we rewrite its
  // visible value when it is too long (the full path stays in the title/tooltip).
  function middleTruncate(text, input) {
    const max = Math.max(20, Math.floor((input.clientWidth || 360) / 7.5));
    if (text.length <= max) {
      input.value = text;
      return;
    }
    const keep = max - 1;
    const head = Math.ceil(keep * 0.4);
    const tail = keep - head;
    input.value = text.slice(0, head) + "…" + text.slice(text.length - tail);
  }

  // --------------------------------------------------------------- actions
  let analyzeTimer = null;
  let analyzeSeq = 0;

  function scheduleAnalyze(immediate) {
    clearTimeout(analyzeTimer);
    const raw = $("url").value.trim();
    state.url = raw;
    if (!raw) {
      analyzeSeq++;
      state.video = { status: "empty", info: null, error: null };
      render();
      return;
    }
    analyzeTimer = setTimeout(analyze, immediate ? 0 : 400);
  }

  async function analyze() {
    const raw = state.url;
    if (!raw) return;
    const seq = ++analyzeSeq;
    state.video = { status: "loading", info: null, error: null };
    if (state.job.status !== "running") state.job = { status: "idle", progress: null, result: null, error: null };
    render();
    try {
      const info = await api.analyze(raw);
      if (seq !== analyzeSeq) return;
      state.video = { status: "ok", info, error: null };
    } catch (e) {
      if (seq !== analyzeSeq) return;
      state.video = { status: "error", info: null, error: api.toError(e) };
    }
    render();
  }

  async function paste() {
    try {
      const text = (await api.readClipboard()).trim();
      if (!text) return;
      $("url").value = text;
      scheduleAnalyze(true);
    } catch (_) { /* clipboard unavailable: user can paste manually */ }
  }

  function setMode(m) {
    state.mode = m;
    api.setMode(m).catch(() => {});
    render();
  }

  async function openShell(fn) {
    try {
      await fn(state.job.result.filePath);
    } catch (e) {
      state.warnings = [e.message + (e.detail ? " (" + e.detail + ")" : "")];
      render();
    }
  }

  async function browse() {
    try {
      const dir = await api.chooseOutputDir();
      if (dir) {
        state.outputDir = dir;
        render();
      }
    } catch (_) { /* dialog cancelled or failed */ }
  }

  async function startDownload() {
    if ($("download").disabled) return;
    state.job = { status: "running", progress: null, result: null, error: null };
    render();
    try {
      await api.download(state.video.info.webpageUrl || state.url, state.mode);
    } catch (e) {
      state.job = { status: "error", progress: null, result: null, error: api.toError(e) };
      render();
    }
  }

  async function cancel() {
    try { await api.cancel(); } catch (_) { /* job already finished */ }
  }

  async function checkUpdates() {
    state.checkingUpdates = true;
    render();
    try {
      await api.checkUpdates();
    } catch (_) {
      state.checkingUpdates = false;
      render();
    }
  }

  // --------------------------------------------------------------- events
  function bindEvents() {
    api.on("startup:status", (d) => {
      if (state.startup.status === "done") return; // update check: handled by footer state
      state.startup = { ...state.startup, status: "loading", message: d.message || t.startup.working, done: d.done || 0, total: d.total || 0 };
      render();
    });
    api.on("startup:error", (d) => {
      state.checkingUpdates = false;
      state.startup = { status: "error", message: "", done: 0, total: 0, error: (d && d.message) || t.errors.generic };
      render();
    });
    api.on("startup:done", (d) => {
      state.checkingUpdates = false;
      state.startup = { ...state.startup, status: "done" };
      state.ytdlpVersion = (d && d.ytdlpVersion) || state.ytdlpVersion;
      state.warnings = (d && d.warnings) || [];
      render();
    });
    api.on("download:progress", (d) => {
      state.job = { ...state.job, status: "running", progress: d };
      render();
    });
    api.on("download:done", (d) => {
      state.job = { status: "done", progress: null, result: d, error: null };
      render();
    });
    api.on("download:error", (d) => {
      state.job = { status: "error", progress: null, result: null, error: api.toError(d) };
      render();
    });
    api.on("download:cancelled", () => {
      state.job = { status: "cancelled", progress: null, result: null, error: null };
      render();
    });
  }

  function bindDom() {
    $("url-label").textContent = t.url.label;
    $("url").placeholder = t.url.placeholder;
    $("paste").textContent = t.url.paste;
    $("mode-legend").textContent = t.mode.legend;
    $("outdir-label").textContent = t.output.label;
    $("browse").textContent = t.output.browse;
    $("download").textContent = t.actions.download;
    $("cancel").textContent = t.actions.cancel;
    $("open-folder").textContent = t.status.openFolder;
    $("detail-summary").textContent = t.status.details;
    $("startup-retry").textContent = t.startup.retry;

    $("url").addEventListener("input", () => scheduleAnalyze(false));
    $("url").addEventListener("keydown", (e) => {
      if (e.key === "Enter") { e.preventDefault(); scheduleAnalyze(true); }
    });
    $("paste").addEventListener("click", paste);
    $("browse").addEventListener("click", browse);
    $("download").addEventListener("click", startDownload);
    $("cancel").addEventListener("click", cancel);
    $("open-folder").addEventListener("click", () => openShell(api.openFolder));
    $("check-updates").addEventListener("click", checkUpdates);
    $("startup-retry").addEventListener("click", boot);
    window.addEventListener("resize", () => { if (state.startup.status === "done") render(); });
  }

  async function boot() {
    state.startup = { status: "loading", message: t.startup.checking, done: 0, total: 0, error: "" };
    render();
    try {
      await api.bootstrap();
    } catch (e) {
      state.startup = { status: "error", message: "", done: 0, total: 0, error: api.toError(e).message };
      render();
    }
  }

  async function init() {
    bindDom();
    bindEvents();
    try {
      const cfg = await api.getConfig();
      if (cfg && cfg.outputDir) state.outputDir = cfg.outputDir;
      if (cfg && MODES.includes(cfg.lastMode)) state.mode = cfg.lastMode;
    } catch (_) { /* defaults */ }
    try { state.appVersion = await api.version(); } catch (_) { /* footer without version */ }
    if (api.mock) setupMockPanel();
    boot();
  }

  // ------------------------------------------------- mock state explorer
  // Lets every UI state be forced without going through the simulated flow.
  function setupMockPanel() {
    const panel = $("mock-panel");
    panel.hidden = false;
    const info = {
      id: "x", title: "Titre de démonstration", uploader: "Chaîne d'exemple", thumbnail: "",
      webpageUrl: "https://www.youtube.com/watch?v=x", duration: 3725, isLive: false, maxHeight: 2160, hasAudio: true,
    };
    const force = {
      "Démarrage": () => { state.startup = { status: "loading", message: "Téléchargement de ffmpeg… 42 %", done: 42, total: 100, error: "" }; },
      "Erreur démarrage": () => { state.startup = { status: "error", message: "", done: 0, total: 0, error: "Impossible de télécharger yt-dlp : pas de connexion et aucune version locale." }; },
      "Warning": () => { state.startup.status = "done"; state.warnings = ["Hors ligne : version locale de yt-dlp utilisée."]; },
      "Vide": () => { state.startup.status = "done"; state.video = { status: "empty", info: null, error: null }; state.job = { status: "idle" }; },
      "Analyse…": () => { state.startup.status = "done"; state.video = { status: "loading", info: null, error: null }; },
      "Vidéo OK": () => { state.startup.status = "done"; state.video = { status: "ok", info, error: null }; },
      "Vidéo erreur": () => { state.startup.status = "done"; state.video = { status: "error", info: null, error: { message: "Vidéo introuvable ou privée." } }; },
      "En cours": () => { state.startup.status = "done"; state.video = { status: "ok", info, error: null }; state.job = { status: "running", progress: { phase: "Téléchargement vidéo", percent: 42, speed: "3,2 Mo/s", eta: "00:35" }, result: null, error: null }; },
      "Succès": () => { state.startup.status = "done"; state.video = { status: "ok", info, error: null }; state.job = { status: "done", result: { filePath: "C:\\Videos\\Titre de démonstration [x].mp4" } }; },
      "Erreur DL": () => { state.startup.status = "done"; state.video = { status: "ok", info, error: null }; state.job = { status: "error", error: { message: "Le téléchargement a échoué.", detail: "ERROR: HTTP Error 403: Forbidden" } }; },
      "Annulé": () => { state.startup.status = "done"; state.video = { status: "ok", info, error: null }; state.job = { status: "cancelled" }; },
    };
    const label = document.createElement("span");
    label.textContent = "Mock :";
    panel.append(label);
    Object.entries(force).forEach(([name, fn]) => {
      const b = document.createElement("button");
      b.type = "button";
      b.textContent = name;
      b.addEventListener("click", () => { fn(); render(); });
      panel.append(b);
    });
    const hint = document.createElement("small");
    hint.textContent = "URL contenant « error », « live » ou « fail » = scénario correspondant.";
    panel.append(hint);
  }

  init();
})();
