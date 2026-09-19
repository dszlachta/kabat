package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"os"

	"github.com/dszlachta/kabat/internal/ipc"
	"github.com/dszlachta/kabat/internal/nativehost"
	"github.com/dszlachta/kabat/internal/report"
	"github.com/dszlachta/kabat/internal/wallets"
	"github.com/dszlachta/kabat/internal/walletservice"
	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

func init() {
	application.RegisterEvent[map[string]any]("pendingTransaction")
	application.RegisterEvent[[]walletservice.Account]("accounts")
	application.RegisterEvent[application.Void]("confirm-yes")
	application.RegisterEvent[application.Void]("confirm-no")
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
		Description: "Local Anvil development wallet",
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

	svc.SetEmitter(func(event string, data any) {
		if event == "pendingTransaction" {
			showConfirmDialog(app, svc, data)
			return
		}
		// Cancelled is a listener hook result, not a failure.
		_ = app.Event.Emit(event, data)
	})
	svc.StartWatch(context.Background())

	if lis, err := ipc.ListenUnix(ipc.DefaultSocketPath()); err != nil {
		report.Error(err)
	} else {
		srv := ipc.NewServer(svc)
		svc.SetOnSelected(func() { srv.AccountsChanged() })
		go func() {
			if err := srv.Serve(lis); err != nil {
				report.Error(err)
			}
		}()
	}

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "Kabat",
		Width:  1000,
		Height: 618,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		URL: "/",
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
