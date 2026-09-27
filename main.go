// Seaglass is a Windows game launcher that finds every game on the PC
// on its own, store installs and unofficial copies alike.
package main

import (
	"embed"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/ApolloF/Seaglass/internal/achievements"
	"github.com/ApolloF/Seaglass/internal/app"
	"github.com/ApolloF/Seaglass/internal/logx"
	"github.com/ApolloF/Seaglass/internal/meta"
	"github.com/ApolloF/Seaglass/internal/platform"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

// version is set at build time (-ldflags "-X main.version=v0.1.0").
var version = "dev"

// uniqueID keeps Seaglass to one instance per Windows session.
const uniqueID = "nl.apollof.seaglass"

// oldUniqueID is WaterLauncher's, from before the rename.
const oldUniqueID = "nl.apollof.waterlauncher"

func main() {
	args := app.ParseArgs(os.Args[1:])
	dev := args.Dev.Any() && app.DevAllowed(version)
	if dev && args.Dev.DataDir != "" {
		platform.UseAppDir(args.Dev.DataDir)
	}
	if args.Updated {
		// Started by an update: the old version may still be closing
		// (WaterLauncher, when it's the one that updated).
		platform.WaitInstanceGone(uniqueID, 15*time.Second)
		platform.WaitInstanceGone(oldUniqueID, 15*time.Second)
	}
	var moved []string
	if !(dev && args.Dev.DataDir != "") && !platform.InstanceRunning(uniqueID) {
		// WaterLauncher's library and settings.
		moved = platform.MoveOldData(platform.InstanceRunning(oldUniqueID))
	}
	if args.Diagnostics {
		// Works while Seaglass runs, or when its interface won't open.
		core, err := app.NewCore(version)
		if err != nil {
			log.Fatal(err)
		}
		if p, err := app.WriteDiagnostics(core); err == nil {
			_ = platform.OpenFile(p)
		}
		return
	}
	if platform.InstanceRunning(uniqueID) {
		// Hand the arguments to the running Seaglass; Wails passes
		// them on and exits, before the library is even opened.
		application.New(application.Options{
			Name:           "Seaglass",
			Assets:         application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
			SingleInstance: &application.SingleInstanceOptions{UniqueID: uniqueID},
		})
		os.Exit(0) // it exited just now: nothing to hand over
	}
	if args.Quit {
		return // nothing runs, nothing to close (the installer asks)
	}
	// Only the instance that runs keeps a crash log (a second one exited above).
	app.CaptureCrashes()
	if args.Play == 0 && !args.Updated && app.ApplyPendingUpdate(version, args.Tray) {
		return // the new version takes over
	}

	core, err := app.NewCore(version)
	if err != nil {
		logx.Printf("start: %v", err)
		log.Fatal(err)
	}
	core.Frozen = dev && args.Dev.DataDir != ""
	logx.Printf("Seaglass %s starting", version)
	for _, m := range moved {
		logx.Printf("rename: %s", m)
	}
	shell := app.NewShell(core)
	launcher := app.NewLaunchService(core)

	var browserArgs []string
	if dev && args.Dev.CDPPort > 0 {
		browserArgs = append(browserArgs, "--remote-debugging-port="+strconv.Itoa(args.Dev.CDPPort))
	}
	wa := application.New(application.Options{
		Name:        "Seaglass",
		Description: "Game launcher that finds every game on your PC",
		Services: []application.Service{
			application.NewService(app.NewLibraryService(core)),
			application.NewService(launcher),
			application.NewService(app.NewSavesService(core)),
			application.NewService(app.NewAchievementsService(core)),
			application.NewService(app.NewAccountsService(core)),
			application.NewService(app.NewSettingsService(core)),
			application.NewService(app.NewPadService(core)),
			application.NewService(app.NewUpdateService(core)),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
			Middleware: func(next http.Handler) http.Handler {
				art := meta.ArtHandler(platform.CacheDir("art"))
				icons := meta.ImageHandler(achievements.IconPrefix, platform.CacheDir("achievements", "icons"))
				return art(icons(next))
			},
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: uniqueID,
			OnSecondInstanceLaunch: func(d application.SecondInstanceData) {
				a := app.ParseArgs(d.Args)
				switch {
				case a.Quit: // the installer or uninstaller needs Seaglass closed
					logx.Printf("asked to quit")
					application.Get().Quit()
				case a.Play != 0:
					launcher.PlayFromArgs(d.Args)
				case a.Tray, a.Updated: // already running
				default:
					shell.OpenMain()
				}
			},
		},
		Windows: application.WindowsOptions{
			WebviewUserDataPath: platform.CacheDir("webview"),
			// The interface closes while a game runs; Seaglass keeps
			// going in the tray. Closing the window yourself still quits.
			DisableQuitOnLastWindowClosed: true,
			AdditionalBrowserArgs:         browserArgs,
		},
	})
	if dev {
		app.StartDev(core, args.Dev)
	}
	switch {
	case args.Play != 0:
		// "--play <id>" starts a game straight away, without the interface.
		shell.StartHidden()
		wa.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
			go launcher.PlayFromArgs(os.Args[1:])
		})
	case args.Tray:
		// "--tray": started with Windows; only the tray icon until opened.
		shell.StartHidden()
	default:
		shell.Start()
	}

	if err := wa.Run(); err != nil {
		logx.Printf("run: %v", err)
		log.Fatal(err)
	}
}
