// Command preen-desktop hosts the shared web build and owns one supervised service.
package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"os"
	"runtime"
	"sync"

	"github.com/Clementine33/preen/desktop/supervisor"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:assets
var assets embed.FS

var version = "0.2.0-dev"

// Desktop exposes only the native host operations; all media operations use HTTP.
type Desktop struct {
	app   *application.App
	ready chan struct{}
	api   supervisor.API
	err   error
}

func (d *Desktop) ResolveApi(ctx context.Context) (supervisor.API, error) {
	select {
	case <-ctx.Done():
		return supervisor.API{}, ctx.Err()
	case <-d.ready:
		return d.api, d.err
	}
}

func (d *Desktop) PickFolder() (*string, error) {
	path, err := d.app.Dialog.OpenFile().CanChooseFiles(false).CanChooseDirectories(true).SetTitle("选择媒体库文件夹").PromptForSingleSelection()
	// Wails 3.0's Windows cancellation sentinel is in an internal package.
	if runtime.GOOS == "windows" && err != nil && err.Error() == "cancelled by user" {
		return nil, nil
	}
	if err != nil || path == "" {
		return nil, err
	}
	return &path, nil
}

func (d *Desktop) fail(err error) {
	dialog := d.app.Dialog.Error().SetTitle("Preen 无法继续运行").SetMessage(err.Error())
	button := dialog.AddButton("OK").OnClick(d.app.Quit)
	dialog.SetDefaultButton(button).SetCancelButton(button)
	dialog.Show()
}

func startService(ctx context.Context) (*supervisor.Process, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	origin := "wails://localhost"
	if runtime.GOOS == "windows" {
		origin = "http://wails.localhost"
	}
	plan, err := supervisor.Prepare(supervisor.Options{Executable: executable, GOOS: runtime.GOOS, Environ: os.Environ(), HomeDir: home, Origin: origin, Version: version})
	if err != nil {
		return nil, err
	}
	return supervisor.Start(ctx, plan)
}

func (d *Desktop) run(ctx context.Context) {
	process, err := startService(ctx)
	if err == nil {
		defer func() {
			if stopErr := process.Stop(); stopErr != nil {
				log.Print(stopErr)
			}
		}()
		d.api, err = process.ResolveAPI(ctx)
	}
	d.err = err
	close(d.ready)
	if err == nil {
		err = process.Wait()
	}
	if err != nil && ctx.Err() == nil {
		d.fail(fmt.Errorf("Preen 服务无法继续运行：%w", err))
	}
}

func main() {
	if !checkWebView2() {
		os.Exit(1)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	desktop := &Desktop{ready: make(chan struct{})}
	var window *application.WebviewWindow
	var windowMu sync.Mutex
	finished := make(chan struct{})
	var start sync.Once
	shutdown := func() {
		cancel()
		start.Do(func() { close(finished) })
		<-finished
	}
	app := application.New(application.Options{
		Name:     "Preen",
		Services: []application.Service{application.NewService(desktop)},
		Assets:   application.AssetOptions{Handler: application.BundledAssetFileServer(assets)},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "io.github.Clementine33.preen",
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				windowMu.Lock()
				defer windowMu.Unlock()
				if window != nil {
					window.Restore()
					window.Focus()
				}
			},
		},
		OnShutdown: shutdown,
	})
	// New acquires the single-instance lock before any service or data access.
	desktop.app = app
	windowMu.Lock()
	window = app.Window.NewWithOptions(application.WebviewWindowOptions{Title: "Preen", Width: 1280, Height: 800, MinWidth: 800, MinHeight: 600, URL: "/"})
	windowMu.Unlock()
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		start.Do(func() { go func() { defer close(finished); desktop.run(ctx) }() })
	})
	if err := app.Run(); err != nil {
		shutdown()
		log.Fatal(err)
	}
}
