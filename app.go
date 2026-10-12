package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/cgamache/youtube-dl-gui/internal/config"
	"github.com/cgamache/youtube-dl-gui/internal/deps"
	"github.com/cgamache/youtube-dl-gui/internal/github"
	"github.com/cgamache/youtube-dl-gui/internal/paths"
	"github.com/cgamache/youtube-dl-gui/internal/procutil"
	"github.com/cgamache/youtube-dl-gui/internal/ytdlp"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// shutdownWait bounds how long shutdown waits for a cancelled job to clean up.
const shutdownWait = 5 * time.Second

// UIError is an error serialised as JSON ({message, detail}) so the frontend
// can show a readable message and a separate technical detail.
type UIError struct {
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

func (e *UIError) Error() string {
	b, err := json.Marshal(e)
	if err != nil {
		return e.Message
	}
	return string(b)
}

// uiError converts err into a *UIError. Messages already written for the user
// (ytdlp errors) are kept as the message; the wrapped chain goes in the detail.
func uiError(err error) *UIError {
	var ue *UIError
	if errors.As(err, &ue) {
		return ue
	}
	return &UIError{Message: err.Error()}
}

// App holds the state exposed to the frontend through Wails bindings.
type App struct {
	ctx    context.Context
	root   context.Context // cancelled at shutdown
	stop   context.CancelFunc
	paths  paths.Paths
	deps   *deps.Manager
	runner ytdlp.Runner

	mu            sync.Mutex
	cfg           config.Config
	cancel        context.CancelFunc // running download, nil otherwise
	jobDone       chan struct{}      // closed when the running download goroutine ends
	cancelAnalyze context.CancelFunc // running analysis, nil otherwise
	analyzeGen    int                // id of the latest analysis
	ensuring      bool               // deps.Ensure in progress
	version       string             // application version (build-time)
	startWarnings []string           // warnings detected before the UI is ready
}

// NewApp creates a new App.
func NewApp(p paths.Paths, cfg config.Config, version string) *App {
	root, stop := context.WithCancel(context.Background())
	var warnings []string
	if dir, warn := config.UsableOutputDir(cfg.OutputDir); warn != "" {
		slog.Warn("output dir unusable, falling back", "was", cfg.OutputDir, "now", dir)
		cfg.OutputDir = dir
		warnings = append(warnings, warn)
	}
	return &App{
		root: root, stop: stop,
		paths: p, cfg: cfg,
		version: version, startWarnings: warnings,
		deps: deps.NewManager(p, &github.Client{}),
		runner: ytdlp.Runner{
			YtDlp:     p.Tool("yt-dlp"),
			FFmpegDir: p.Bin,
			Deno:      p.Tool("deno"),
		},
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// shutdown cancels everything still running and waits briefly for cleanup.
func (a *App) shutdown(_ context.Context) {
	a.stop()
	a.mu.Lock()
	done := a.jobDone
	a.mu.Unlock()
	if done != nil {
		select {
		case <-done:
		case <-time.After(shutdownWait):
			slog.Warn("shutdown: download did not stop in time")
		}
	}
}

// beforeClose asks for confirmation when a download is running.
// Returning true prevents the window from closing.
func (a *App) beforeClose(ctx context.Context) bool {
	a.mu.Lock()
	running := a.cancel != nil
	a.mu.Unlock()
	if !running {
		return false
	}
	choice, err := runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
		Type:          runtime.QuestionDialog,
		Title:         "Téléchargement en cours",
		Message:       "Un téléchargement est en cours. Quitter l'annuler et supprimera les fichiers partiels.\n\nQuitter quand même ?",
		Buttons:       []string{"Quitter", "Continuer"},
		DefaultButton: "Continuer",
		CancelButton:  "Continuer",
	})
	if err != nil {
		slog.Warn("close confirmation dialog", "err", err)
		return false
	}
	return choice != "Quitter" && choice != "Yes"
}

func (a *App) emit(event string, data any) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, event, data)
	}
}

// ---------------------------------------------------------------- startup

// Bootstrap is called by the frontend once it listens to events. It installs
// or updates the external tools in the background.
func (a *App) Bootstrap() {
	a.runEnsure(false)
}

// CheckUpdates forces a new check of every tool. Refused while a download runs.
func (a *App) CheckUpdates() error {
	a.mu.Lock()
	busy := a.cancel != nil
	a.mu.Unlock()
	if busy {
		return &UIError{Message: "Impossible de vérifier les mises à jour pendant un téléchargement."}
	}
	if !a.runEnsure(true) {
		return &UIError{Message: "Une vérification est déjà en cours."}
	}
	return nil
}

// runEnsure starts deps.Ensure in a goroutine; it reports false if one is
// already running. force ignores the per-tool check interval.
func (a *App) runEnsure(force bool) bool {
	a.mu.Lock()
	if a.ensuring {
		a.mu.Unlock()
		return false
	}
	a.ensuring = true
	a.mu.Unlock()

	go func() {
		defer func() {
			a.mu.Lock()
			a.ensuring = false
			a.mu.Unlock()
		}()
		a.ensureDeps(force)
	}()
	return true
}

func (a *App) ensureDeps(force bool) {
	mgr := a.deps
	if force {
		forced := *mgr
		forced.Tools = append([]deps.Tool(nil), mgr.Tools...)
		for i := range forced.Tools {
			forced.Tools[i].CheckEvery = 0
		}
		mgr = &forced
	}
	st, err := mgr.Ensure(a.root, func(p deps.Progress) {
		slog.Info("deps", "tool", p.Tool, "step", p.Step, "done", p.Done, "total", p.Total)
		a.emit("startup:status", map[string]any{
			"step": p.Tool + ":" + p.Step, "message": p.Message, "done": p.Done, "total": p.Total,
		})
	})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		slog.Error("deps ensure failed", "err", err)
		a.emit("startup:error", map[string]any{"message": err.Error()})
		return
	}
	for _, w := range st.Warnings {
		slog.Warn("deps", "warning", w)
	}
	warnings := append(append([]string(nil), a.startWarnings...), st.Warnings...)
	a.emit("startup:done", map[string]any{
		"ytdlpVersion": st.Versions["yt-dlp"],
		"ffmpegOk":     st.Versions["ffmpeg"] != "",
		"denoOk":       st.Versions["deno"] != "",
		"warnings":     warnings,
	})
}

// Version returns the application version injected at build time.
func (a *App) Version() string {
	return a.version
}

// ----------------------------------------------------------------- config

// GetConfig returns the saved preferences.
func (a *App) GetConfig() config.Config {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg
}

// saveConfig persists cfg; callers hold a.mu.
func (a *App) saveConfig() {
	if err := config.Save(a.paths.ConfigFile, a.cfg); err != nil {
		slog.Warn("save config", "err", err)
	}
}

// ChooseOutputDir opens a native folder picker and saves the choice.
// It returns the current directory unchanged if the dialog is dismissed.
func (a *App) ChooseOutputDir() (string, error) {
	a.mu.Lock()
	current := a.cfg.OutputDir
	a.mu.Unlock()

	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title:            "Choisir le dossier de sortie",
		DefaultDirectory: current,
	})
	if err != nil {
		return "", &UIError{Message: "Impossible d'ouvrir le sélecteur de dossier.", Detail: err.Error()}
	}
	if dir == "" {
		return current, nil
	}
	a.mu.Lock()
	a.cfg.OutputDir = dir
	a.saveConfig()
	a.mu.Unlock()
	return dir, nil
}

// SetMode remembers the last used mode.
func (a *App) SetMode(mode string) {
	if !ytdlp.Mode(mode).Valid() {
		return
	}
	a.mu.Lock()
	a.cfg.LastMode = mode
	a.saveConfig()
	a.mu.Unlock()
}

// ---------------------------------------------------------------- analyze

// Analyze validates a pasted URL locally, then fetches the video metadata.
// A previous analysis still running is cancelled.
func (a *App) Analyze(raw string) (ytdlp.VideoInfo, error) {
	url, err := ytdlp.NormalizeURL(raw)
	if err != nil {
		return ytdlp.VideoInfo{}, &UIError{Message: err.Error()}
	}

	ctx, cancel := context.WithCancel(a.root)
	a.mu.Lock()
	if a.cancelAnalyze != nil {
		a.cancelAnalyze()
	}
	a.analyzeGen++
	gen := a.analyzeGen
	a.cancelAnalyze = cancel
	a.mu.Unlock()
	defer func() {
		cancel()
		a.mu.Lock()
		// Only clear our own registration, a newer call may have replaced it.
		if a.analyzeGen == gen {
			a.cancelAnalyze = nil
		}
		a.mu.Unlock()
	}()

	info, err := a.runner.Info(ctx, url)
	if err != nil {
		slog.Warn("analyze url", "url", url, "err", err)
		if errors.Is(err, context.Canceled) {
			return ytdlp.VideoInfo{}, &UIError{Message: "Analyse annulée."}
		}
		return ytdlp.VideoInfo{}, &UIError{Message: err.Error()}
	}
	return info, nil
}

// --------------------------------------------------------------- download

// Download starts a download and returns immediately. Progress and outcome
// are delivered through download:* events.
func (a *App) Download(rawURL, mode string) error {
	url, err := ytdlp.NormalizeURL(rawURL)
	if err != nil {
		return &UIError{Message: err.Error()}
	}
	m := ytdlp.Mode(mode)
	if !m.Valid() {
		return &UIError{Message: "Mode de téléchargement inconnu.", Detail: mode}
	}

	a.mu.Lock()
	if a.cancel != nil {
		a.mu.Unlock()
		return &UIError{Message: "Un téléchargement est déjà en cours."}
	}
	if a.ensuring {
		a.mu.Unlock()
		return &UIError{Message: "Les outils sont en cours de mise à jour, réessayez dans un instant."}
	}
	ctx, cancel := context.WithCancel(a.root)
	done := make(chan struct{})
	a.cancel, a.jobDone = cancel, done
	req := ytdlp.Request{URL: url, OutputDir: a.cfg.OutputDir, Mode: m}
	a.cfg.LastMode = mode
	a.saveConfig()
	a.mu.Unlock()

	go func() {
		defer close(done)
		path, err := a.runner.Download(ctx, req, func(ev ytdlp.Event) {
			a.emit("download:progress", ev)
		})
		cancel()
		a.mu.Lock()
		a.cancel = nil
		a.mu.Unlock()

		switch {
		case errors.Is(err, ytdlp.ErrCancelled):
			a.emit("download:cancelled", map[string]any{})
		case err != nil:
			slog.Error("download failed", "url", url, "err", err)
			a.emit("download:error", uiError(err))
		default:
			slog.Info("download done", "file", path)
			a.emit("download:done", map[string]any{"filePath": path})
		}
	}()
	return nil
}

// Cancel stops the running download, if any.
func (a *App) Cancel() {
	a.mu.Lock()
	cancel := a.cancel
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// ---------------------------------------------------------------- shell

// OpenFolder opens the file explorer with filePath selected.
func (a *App) OpenFolder(filePath string) error {
	slog.Info("open folder", "path", filePath)
	var err error
	if _, serr := os.Stat(filePath); serr != nil {
		// File gone: fall back to its folder.
		err = procutil.OpenDir(filepath.Dir(filePath))
	} else {
		err = procutil.ShowInFolder(filePath)
	}
	if err != nil {
		slog.Error("open folder failed", "path", filePath, "err", err)
		return &UIError{Message: "Impossible d'ouvrir l'explorateur de fichiers.", Detail: err.Error()}
	}
	return nil
}

// secondInstance focuses the existing window when the app is launched again.
func (a *App) secondInstance(_ options.SecondInstanceData) {
	if a.ctx == nil {
		return
	}
	runtime.WindowUnminimise(a.ctx)
	runtime.Show(a.ctx)
}
