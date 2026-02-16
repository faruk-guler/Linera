package main

import (
	"embed"
	"io/fs"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var frontendAssets embed.FS

//go:embed assets/* icon/*
var serverAssets embed.FS

func main() {
	// Create an instance of the app structure
	app := NewApp()
	app.Assets = serverAssets

	// Create a sub-FS for the frontend assets
	frontendFS, err := fs.Sub(frontendAssets, "frontend/dist")
	if err != nil {
		log.Fatalf("Failed to create frontend sub-FS: %v", err)
	}

	// Create application with options
	err = wails.Run(&options.App{
		Title:         "Linera Share (Wails)",
		Width:         450,
		Height:        480,
		DisableResize: true,
		AssetServer: &assetserver.Options{
			Assets: frontendFS,
		},
		BackgroundColour: &options.RGBA{R: 240, G: 240, B: 240, A: 1},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		log.Fatal("Error:", err.Error())
	}
}
