package main

import (
	"embed"
	"log"
	"path/filepath"
	"resocks5/internal/state"
	"resocks5/internal/storage"
	"resocks5/internal/utils"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

func main() {
	appConfigDir, err := utils.GetAppConfigDir()
	if err != nil {
		log.Fatalf("failed to get app config dir: %v", err)
	}

	dbPath := filepath.Join(appConfigDir, "settings.json")

	settingsStorage := storage.CreateJsonDB(dbPath, state.Settings{})

	err = settingsStorage.Read()
	if err != nil {
		log.Fatalf("failed to read settings: %v", err)
	}

	settingsState := state.NewSettingsState(settingsStorage.Get())

	settingsState.Subscribe(func(newValue *state.Settings) {
		settingsStorage.Set(newValue)
	})

	appService := NewAppService(settingsState, settingsStorage, appIcon)

	app := application.New(application.Options{
		Name:        "resocks5",
		Description: "A resocks5 application",
		Services: []application.Service{
			application.NewService(appService),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	systray := app.SystemTray.New()
	systray.SetIcon(appIcon)
	systray.SetLabel("resocks5")

	menu := app.NewMenu()
	menu.Add("Закрыть приложение").OnClick(func(ctx *application.Context) {
		app.Quit()
	})

	systray.SetMenu(menu)

	var mainWindow *application.WebviewWindow

	mainWindow = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: "resocks5",
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(27, 38, 54),
		URL:              "/",
		Frameless:        true,
		Hidden:           true,
		Width:            300,
		Height:           400,
	})

	appService.SetApp(app)

	go func() {
		if err := appService.AutoStartProxy(); err != nil {
			log.Printf("Failed to auto-start proxy: %v", err)
		}
	}()

	systray.AttachWindow(mainWindow)

	err = app.Run()

	if err != nil {
		log.Fatal(err)
	}
}
