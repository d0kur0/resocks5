package main

import (
	"fmt"
	"resocks5/internal/consts"
	"resocks5/internal/proxy"
	"resocks5/internal/state"
	"resocks5/internal/storage"

	"github.com/gen2brain/beeep"
	"github.com/wailsapp/wails/v3/pkg/application"
)

type AppService struct {
	settingsState   *state.SettingsState
	settingsStorage *storage.JsonDb[state.Settings]
	proxyServer     *proxy.Server
	appIcon         []byte
	app             *application.App
}

func (a *AppService) SetApp(app *application.App) {
	a.app = app
}

func (a *AppService) emitProxyStarted() {
	if a.app != nil {
		a.app.Event.Emit("proxy-started")
	}
}

func (a *AppService) emitProxyStopped() {
	if a.app != nil {
		a.app.Event.Emit("proxy-stopped")
	}
}

func (a *AppService) SaveSettings(settings state.Settings) error {
	wasRunning := a.proxyServer.IsRunning()
	if wasRunning {
		if err := a.proxyServer.Stop(); err != nil {
			go beeep.Notify("Ошибка", fmt.Sprintf("Не удалось остановить прокси: %v", err), a.appIcon)
			return err
		}
		a.emitProxyStopped()
	}

	a.settingsState.Set(&settings)

	if wasRunning {
		if err := a.proxyServer.Start(&settings); err != nil {
			go beeep.Notify("Ошибка", fmt.Sprintf("Не удалось запустить прокси: %v", err), a.appIcon)
			return err
		}
		a.emitProxyStarted()
		go beeep.Notify("Успешно", "Настройки сохранены, прокси перезапущен", a.appIcon)
	} else {
		go beeep.Notify("Успешно", "Настройки сохранены", a.appIcon)
	}

	return nil
}

func (a *AppService) GetSettings() *state.Settings {
	return a.settingsState.Get()
}

func NewAppService(settingsState *state.SettingsState, settingsStorage *storage.JsonDb[state.Settings], appIcon []byte) *AppService {
	beeep.AppName = "resocks5"
	return &AppService{
		settingsState:   settingsState,
		settingsStorage: settingsStorage,
		proxyServer:     proxy.NewServer(),
		appIcon:         appIcon,
	}
}

func (a *AppService) GetLocalAddress() string {
	return fmt.Sprintf("%s:%d", consts.DefaultLocalHostname, consts.DefaultLocalPort)
}

func (a *AppService) StartProxy() error {
	settings := a.settingsState.Get()
	if settings == nil {
		err := fmt.Errorf("settings not loaded")
		go beeep.Notify("Ошибка", "Настройки не загружены", a.appIcon)
		return err
	}
	if settings.ServerAddress == "" || settings.ServerPort == 0 {
		err := fmt.Errorf("remote proxy settings not configured")
		go beeep.Notify("Ошибка", "Не настроен удаленный прокси сервер", a.appIcon)
		return err
	}
	if err := a.proxyServer.Start(settings); err != nil {
		go beeep.Notify("Ошибка", fmt.Sprintf("Не удалось запустить прокси: %v", err), a.appIcon)
		return err
	}

	settings.Enabled = true
	a.settingsState.Set(settings)

	a.emitProxyStarted()
	go beeep.Notify("Прокси запущен", fmt.Sprintf("Локальный прокси: %s", a.GetLocalAddress()), a.appIcon)
	return nil
}

func (a *AppService) StopProxy() error {
	if err := a.proxyServer.Stop(); err != nil {
		beeep.Notify("Ошибка", fmt.Sprintf("Не удалось остановить прокси: %v", err), a.appIcon)
		return err
	}

	settings := a.settingsState.Get()
	if settings != nil {
		settings.Enabled = false
		a.settingsState.Set(settings)
	}

	a.emitProxyStopped()
	go beeep.Notify("Прокси остановлен", "Локальный прокси сервер остановлен", a.appIcon)
	return nil
}

func (a *AppService) IsProxyRunning() bool {
	return a.proxyServer.IsRunning()
}

func (a *AppService) AutoStartProxy() error {
	settings := a.settingsState.Get()
	if settings == nil || !settings.Enabled {
		return nil
	}

	if settings.ServerAddress == "" || settings.ServerPort == 0 {
		return nil
	}

	err := a.proxyServer.Start(settings)
	if err == nil {
		go a.emitProxyStarted()
	}
	return err
}
