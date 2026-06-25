package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	initAppLogger()
	defer func() {
		if value := recover(); value != nil {
			logPanic("main", value)
			panic(value)
		}
	}()
	// Create an instance of the app structure
	app := NewApp()

	// Create application with options
	err := wails.Run(&options.App{
		Title:     "OriginBlueprint",
		Width:     1500,
		Height:    900,
		MinWidth:  1000,
		MinHeight: 650,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 24, G: 24, B: 24, A: 1},
		OnStartup:        app.startup,
		OnBeforeClose:    app.beforeClose,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		logError("wails.Run", err)
		println("Error:", err.Error())
	}
}
