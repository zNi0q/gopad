package main

import (
	"embed"
	"fmt"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:            "GoPad",
		Width:            1200,
		Height:           800,
		MinWidth:         720,
		MinHeight:        480,
		Frameless:        true,
		AssetServer:      &assetserver.Options{Assets: assets},
		BackgroundColour: &options.RGBA{R: 15, G: 20, B: 25, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind:             []interface{}{app},
		// Gives the frontend real filesystem paths for dropped files/folders
		// (window.runtime.OnFileDrop), instead of the browser's sandboxed File API.
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop: true,
		},
		Linux: &linux.Options{
			ProgramName: "GoPad",
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "gopad: %v\n", err)
		os.Exit(1)
	}
}
