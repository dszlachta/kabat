package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"os"

	"github.com/dszlachta/kabat/internal/config"
	"github.com/dszlachta/kabat/internal/ipc"
	"github.com/dszlachta/kabat/internal/mcpserver"
	"github.com/dszlachta/kabat/internal/nativehost"
	"github.com/dszlachta/kabat/internal/report"
	"github.com/dszlachta/kabat/internal/wallets"
	"github.com/dszlachta/kabat/internal/walletservice"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

func init() {
	application.RegisterEvent[map[string]any]("pendingTransaction")
	application.RegisterEvent[map[string]any]("pendingConnection")
	application.RegisterEvent[[]walletservice.Account]("accounts")
	application.RegisterEvent[[]walletservice.StoredTx]("transactions")
	application.RegisterEvent[map[string]string]("abi")
	application.RegisterEvent[application.Void]("confirm-yes")
	application.RegisterEvent[application.Void]("confirm-no")
	application.RegisterEvent[string]("connect-allow")
	application.RegisterEvent[application.Void]("connect-deny")
	application.RegisterEvent[application.Void]("import-key")
	application.RegisterEvent[application.Void]("open-settings")
}

func main() {
	if nativehost.IsNativeLaunch(os.Args) {
		if err := nativehost.Run(ipc.DefaultSocketPath()); err != nil {
			log.Fatal(err)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "install-native-host" {
		if err := nativehost.Install(); err != nil {
			log.Fatal(err)
		}
		fmt.Println("Wrote native messaging host manifests for Chrome and Firefox.")
		return
	}
	runGUI()
}

func runGUI() {
	loadErr := wallets.LoadError()
	var svc *walletservice.WalletService
	var services []application.Service
	if loadErr == nil {
		svc = walletservice.New()
		services = []application.Service{application.NewService(svc)}
	}

	app := application.New(application.Options{
		Name:        "Kabat",
		Description: "Kabat Developer Wallet",
		Services:    services,
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})
	report.SetNotifier(func(err error) {
		app.Dialog.Error().SetTitle("Kabat").SetMessage(err.Error()).Show()
	})
	if loadErr != nil {
		report.Error(loadErr)
		return
	}

	svc.SetPickJSONFile(func() (string, error) {
		return app.Dialog.OpenFile().
			SetTitle("Load ABI").
			AddFilter("JSON", "*.json").
			PromptForSingleSelection()
	})
	svc.SetEmitter(func(event string, data any) {
		if event == "pendingTransaction" {
			showConfirmDialog(app, svc, data)
			return
		}
		if event == "pendingConnection" {
			showConnectDialog(app, svc, data)
			return
		}
		// Cancelled is a listener hook result, not a failure.
		_ = app.Event.Emit(event, data)
	})
	svc.StartWatch(context.Background())

	mcpSrv := mcpserver.New(svc)
	svc.SetOnMCP(mcpSrv.SetEnabled)
	if svc.Settings().MCP {
		if err := mcpSrv.Start(); err != nil {
			report.Error(err)
		}
	}

	addWalletMenu := app.ContextMenu.New()
	addWalletMenu.Add("Add random").OnClick(func(*application.Context) {
		go func() {
			if err := svc.AddRandomWallet(); err != nil {
				report.Error(err)
			}
		}()
	})
	addWalletMenu.Add("From private key").OnClick(func(*application.Context) {
		_ = app.Event.Emit("import-key")
	})
	app.ContextMenu.Add("add-wallet", addWalletMenu)

	settings := &settingsHost{}
	app.Event.On("open-settings", func(*application.CustomEvent) {
		settings.show(app)
	})

	if lis, err := ipc.ListenUnix(ipc.DefaultSocketPath()); err != nil {
		report.Error(err)
	} else {
		srv := ipc.NewServer(svc)
		go func() {
			if err := srv.Serve(lis); err != nil {
				report.Error(err)
			}
		}()
	}

	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:           "Kabat",
		Width:           minWindowWidth,
		Height:          minWindowHeight,
		MinWidth:        minWindowWidth,
		MinHeight:       minWindowHeight,
		InitialPosition: application.WindowCentered,
		Hidden:          true,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		URL: "/",
	})
	sizeStore := &windowSizeStore{path: config.DefaultPath()}
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		sizeStore.apply(app, win)
	})
	win.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
		sizeStore.apply(app, win)
	})
	win.OnWindowEvent(events.Common.WindowDidResize, func(*application.WindowEvent) {
		sizeStore.schedule(win)
	})
	win.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
		sizeStore.flush(win)
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
