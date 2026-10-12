// Thin wrapper around the Wails bindings and runtime events.
// With ?mock=1 the backend is simulated so the UI can be iterated in a browser.
(function () {
  const mock = new URLSearchParams(location.search).get("mock") === "1";
  const t = window.i18n;

  // Normalise any Go error (string, Error, {message, detail}) to {message, detail}.
  function toError(e) {
    if (e && typeof e === "object" && e.message && !(e instanceof Error)) {
      return { message: String(e.message), detail: e.detail ? String(e.detail) : "" };
    }
    const raw = e instanceof Error ? e.message : typeof e === "string" ? e : "";
    if (raw.startsWith("{")) {
      try {
        const o = JSON.parse(raw);
        if (o.message) return { message: o.message, detail: o.detail || "" };
      } catch (_) { /* not JSON */ }
    }
    return { message: raw || t.errors.generic, detail: "" };
  }

  // ---------------------------------------------------------------- real API
  function bound(name) {
    const app = window.go && window.go.main && window.go.main.App;
    return app && typeof app[name] === "function" ? app[name].bind(app) : null;
  }

  async function call(name, ...args) {
    const fn = bound(name);
    if (!fn) throw { message: t.errors.unavailable, detail: `binding ${name} missing` };
    try {
      return await fn(...args);
    } catch (e) {
      throw toError(e);
    }
  }

  function onEvent(name, cb) {
    if (window.runtime && window.runtime.EventsOn) {
      return window.runtime.EventsOn(name, cb);
    }
    return () => {};
  }

  const real = {
    mock: false,
    bootstrap: () => (bound("Bootstrap") ? call("Bootstrap") : Promise.resolve()),
    getConfig: async () => (bound("GetConfig") ? call("GetConfig") : {}),
    version: async () => (bound("Version") ? call("Version") : ""),
    analyze: (url) => call(bound("Analyze") ? "Analyze" : "AnalyzeURL", url),
    chooseOutputDir: () => call("ChooseOutputDir"),
    setMode: (m) => (bound("SetMode") ? call("SetMode", m) : Promise.resolve()),
    download: (url, mode) => call("Download", url, mode),
    cancel: () => call("Cancel"),
    openFolder: (p) => call("OpenFolder", p),
    checkUpdates: () => call("CheckUpdates"),
    readClipboard: async () => {
      try {
        return await navigator.clipboard.readText();
      } catch (_) {
        if (window.runtime && window.runtime.ClipboardGetText) return window.runtime.ClipboardGetText();
        throw toError("clipboard");
      }
    },
    on: onEvent,
    toError,
  };

  // --------------------------------------------------------------- mock API
  const listeners = {};
  function emit(name, data) {
    (listeners[name] || []).forEach((cb) => cb(data));
  }

  const sampleInfo = {
    id: "dQw4w9WgXcQ",
    title: "Un titre de vidéo volontairement assez long pour vérifier la troncature sur plusieurs lignes dans la carte",
    uploader: "Chaîne d'exemple",
    thumbnail: "data:image/svg+xml;utf8," + encodeURIComponent(
      '<svg xmlns="http://www.w3.org/2000/svg" width="320" height="180"><defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#6366f1"/><stop offset="1" stop-color="#ec4899"/></linearGradient></defs><rect width="320" height="180" fill="url(#g)"/><polygon points="130,60 130,120 190,90" fill="#fff" opacity=".85"/></svg>'),
    webpageUrl: "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
    duration: 3725,
    isLive: false,
    maxHeight: 2160,
    hasAudio: true,
  };

  let mockTimer = null;
  let mockCfg = { outputDir: "C:\\Users\\Exemple\\Videos\\Téléchargements\\YouTube", lastMode: "video_audio" };

  function stopJob() {
    clearInterval(mockTimer);
    mockTimer = null;
  }

  // Scenario is chosen from the URL text so every state is reachable by typing.
  const fake = {
    mock: true,
    bootstrap: async () => {
      const steps = [
        ["Vérification de yt-dlp…", 0, 0],
        ["Téléchargement de ffmpeg… 42 %", 42, 100],
        ["Installation de deno…", 100, 100],
      ];
      let i = 0;
      const tick = () => {
        if (i < steps.length) {
          const [message, done, total] = steps[i++];
          emit("startup:status", { step: "mock", message, done, total });
          setTimeout(tick, 600);
        } else {
          emit("startup:done", { ytdlpVersion: "2026.10.07", ffmpegOk: true, denoOk: true, warnings: [] });
        }
      };
      setTimeout(tick, 200);
    },
    getConfig: async () => ({ ...mockCfg }),
    version: async () => "dev",
    analyze: (url) =>
      new Promise((resolve, reject) => {
        setTimeout(() => {
          if (/error/i.test(url)) reject({ message: "Vidéo introuvable ou privée.", detail: "ERROR: [youtube] xxx: Video unavailable" });
          else if (/live/i.test(url)) resolve({ ...sampleInfo, isLive: true, title: "Direct en cours" });
          else resolve({ ...sampleInfo });
        }, 1200);
      }),
    chooseOutputDir: async () => {
      mockCfg.outputDir = "D:\\Vidéos\\Nouveau dossier";
      return mockCfg.outputDir;
    },
    setMode: async (m) => { mockCfg.lastMode = m; },
    download: async (url, mode) => {
      stopJob();
      let pct = 0;
      const fail = /fail/i.test(url);
      mockTimer = setInterval(() => {
        pct += 2;
        let phase = pct < 70 ? "Téléchargement vidéo" : pct < 90 ? "Téléchargement audio" : "Fusion";
        if (mode === "audio_only") phase = pct < 90 ? "Téléchargement" : "Conversion MP3";
        emit("download:progress", {
          phase, percent: Math.min(pct, 100), speed: "3,2 Mo/s", eta: "00:" + String(Math.max(0, 50 - Math.floor(pct / 2))).padStart(2, "0"),
        });
        if (fail && pct >= 40) {
          stopJob();
          emit("download:error", { message: "Le téléchargement a été interrompu.", detail: "ERROR: unable to download video data: HTTP Error 403: Forbidden\n(simulé)" });
        } else if (pct >= 100) {
          stopJob();
          const ext = mode === "audio_only" ? "mp3" : "mp4";
          emit("download:done", { filePath: `${mockCfg.outputDir}\\Un titre de vidéo [dQw4w9WgXcQ].${ext}` });
        }
      }, 150);
    },
    cancel: async () => {
      stopJob();
      emit("download:cancelled", {});
    },
    openFolder: async () => {},
    checkUpdates: async () => {
      emit("startup:status", { step: "mock", message: "Vérification de yt-dlp…", done: 0, total: 0 });
      setTimeout(() => emit("startup:done", { ytdlpVersion: "2026.10.07", ffmpegOk: true, denoOk: true, warnings: [] }), 800);
    },
    readClipboard: async () => "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
    on: (name, cb) => {
      (listeners[name] = listeners[name] || []).push(cb);
      return () => { listeners[name] = listeners[name].filter((c) => c !== cb); };
    },
    toError,
    emit,
  };

  window.api = mock ? fake : real;
})();
