package main

import (
	"embed"
	"log"
	"log/slog"

	"github.com/cgamache/youtube-dl-gui/internal/config"
	"github.com/cgamache/youtube-dl-gui/internal/logging"
	"github.com/cgamache/youtube-dl-gui/internal/paths"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend
var assets embed.FS

// version is injected at build time: -ldflags "-X main.version=...".
var version = "dev"

func main() {
	p, err := paths.Resolve()
	if err != nil {
		log.Fatal(err)
	}
	closeLog, err := logging.Init(p.Logs)
	if err != nil {
		log.Fatal(err)
	}
	defer closeLog()

	cfg, err := config.Load(p.ConfigFile)
	if err != nil {
		log.Fatal(err)
	}
	slog.Info("startup", "version", version, "root", p.Root, "outputDir", cfg.OutputDir)

	app := NewApp(p, cfg, version)

	err = wails.Run(&options.App{
		Title:     "YouTube DL",
		Width:     640,
		Height:    560,
		MinWidth:  520,
		MinHeight: 420,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               "com.cgamache.youtube-dl-gui",
			OnSecondInstanceLaunch: app.secondInstance,
		},
		OnStartup:     app.startup,
		OnBeforeClose: app.beforeClose,
		OnShutdown:    app.shutdown,
		Bind:          []any{app},
	})
	if err != nil {
		log.Fatal(err)
	}
}
