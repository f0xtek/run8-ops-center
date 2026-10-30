// Command run8ops is the Run8 Operations Centre: it follows a Run8 world save
// and serves a local web page with the orders to run.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"run8-ops-center/internal/engine"
	"run8-ops-center/internal/profile"
	"run8-ops-center/internal/store"
	"run8-ops-center/internal/watch"
	"run8-ops-center/internal/web"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr, options{}))
}

// options lets tests change behaviour that is fixed in production.
type options struct {
	// ExtraProtected adds protected directories to the Run8 write guard. Tests
	// use it to stand in for the Windows install directory.
	ExtraProtected []string
	// ListenReady, when set, receives the URL once the server is listening.
	ListenReady func(url string)
}

const usage = `Usage: run8ops --save <name> [flags]

  --save <name>         world save to follow (required); the file name in the save folder, with or without .xml
  --save-dir <dir>      folder holding world saves (default: the route profile's Trains folder in the Run8 install)
  --autosave-dir <dir>  folder holding world auto-saves (default: the route profile's AutoSaves folder)
  --data <dir>          folder for profiles, track map, settings and order state (default: %APPDATA%\Run8OpsCenter)
  --port <n>            web server port for this run (default: the saved setting, 8080)
  --no-browser          do not open the browser on start
`

// run is main without the process-level pieces. It returns the exit code.
func run(ctx context.Context, args []string, stdout, stderr io.Writer, opt options) int {
	fs := flag.NewFlagSet("run8ops", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var (
		saveName    = fs.String("save", "", "")
		saveDir     = fs.String("save-dir", "", "")
		autoSaveDir = fs.String("autosave-dir", "", "")
		dataFlag    = fs.String("data", "", "")
		port        = fs.Int("port", 0, "")
		noBrowser   = fs.Bool("no-browser", false, "")
	)
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(stderr, "run8ops: %v\n\n%s", err, usage)
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "run8ops: unexpected argument %q\n\n%s", fs.Arg(0), usage)
		return 2
	}

	guard := store.NewGuard(opt.ExtraProtected...)

	// The data directory is checked against the Run8 install directory before
	// anything is created.
	dataDir, err := store.ResolveDataDir(*dataFlag, guard)
	if err != nil {
		fmt.Fprintf(stderr, "run8ops: %v\n", err)
		return 1
	}
	settings, err := store.LoadSettings(dataDir)
	if err != nil {
		fmt.Fprintf(stderr, "run8ops: %v\n", err)
		return 1
	}
	if settings.RunInstallDir != "" {
		if err := guard.SetInstallDir(settings.RunInstallDir); err != nil {
			fmt.Fprintf(stderr, "run8ops: saved Run8 install folder: %v\n", err)
			return 1
		}
	}

	// The profile supplies the default save folders. Installing it is
	// idempotent; the engine installs it too.
	profilesDir := filepath.Join(dataDir, "profiles")
	if err := profile.InstallBundled(guard, profilesDir); err != nil {
		fmt.Fprintf(stderr, "run8ops: installing the bundled profile: %v\n", err)
		return 1
	}
	profileID := settings.ProfileID
	if profileID == "" {
		profileID = "ags-south-p1"
	}
	prof, err := profile.Load(filepath.Join(profilesDir, profileID))
	if err != nil {
		fmt.Fprintf(stderr, "run8ops: loading profile %q: %v\n", profileID, err)
		return 1
	}
	if *saveDir == "" {
		*saveDir = prof.DefaultSaveDir
	}
	if *autoSaveDir == "" {
		*autoSaveDir = prof.DefaultAutoSaveDir
	}

	// --save is required. A missing, unknown or non-world name exits before
	// the UI starts, listing the world saves found.
	if strings.TrimSpace(*saveName) == "" {
		fmt.Fprintf(stderr, "run8ops: --save is required.\n%s\n", worldSaveList(*saveDir))
		return 2
	}
	if _, err := watch.Resolve(*saveDir, *saveName); err != nil {
		fmt.Fprintf(stderr, "run8ops: %v\n", err)
		return 1
	}

	eng, err := engine.New(engine.Config{
		DataDir:     dataDir,
		Guard:       guard,
		SaveName:    *saveName,
		SaveDir:     *saveDir,
		AutoSaveDir: *autoSaveDir,
		ProfileID:   profileID,
	})
	if err != nil {
		fmt.Fprintf(stderr, "run8ops: %v\n", err)
		return 1
	}
	w, err := watch.New(watch.Config{SaveDir: *saveDir, AutoSaveDir: *autoSaveDir, SaveName: *saveName})
	if err != nil {
		fmt.Fprintf(stderr, "run8ops: %v\n", err)
		return 1
	}

	webPort := settings.Port
	if *port != 0 {
		webPort = *port
	}
	srv, err := web.New(web.Config{
		Port:          webPort,
		AllInterfaces: settings.AllInterfaces,
		NoBrowser:     *noBrowser || settings.NoBrowser,
	}, web.NewBackendAdapter(eng))
	if err != nil {
		fmt.Fprintf(stderr, "run8ops: %v\n", err)
		return 1
	}
	web.RegisterDashboard(srv, eng)
	web.RegisterOrders(srv, eng)
	web.RegisterYards(srv, eng)
	web.RegisterLearn(srv, eng)
	web.RegisterSetup(srv, eng)
	web.RegisterActivity(srv, eng)

	ln, err := srv.Listen()
	if err != nil {
		fmt.Fprintf(stderr, "run8ops: %v\n", err)
		return 1
	}
	url := srv.URL(ln)
	fmt.Fprintf(stdout, "Run8 Operations Centre\n  world save:  %s\n  save folder: %s\n  auto-saves:  %s\n  data folder: %s\n  open:        %s\n",
		*saveName, *saveDir, *autoSaveDir, dataDir, url)
	if opt.ListenReady != nil {
		opt.ListenReady(url)
	}
	if !(*noBrowser || settings.NoBrowser) {
		if err := web.OpenBrowser(url); err != nil {
			fmt.Fprintf(stderr, "run8ops: could not open the browser: %v (open %s yourself)\n", err, url)
		}
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	errc := make(chan error, 3)
	go func() { errc <- eng.Run(ctx) }()
	go func() { errc <- w.Run(ctx, eng.Ingest) }()
	go func() { errc <- srv.Serve(ctx, ln) }()

	// The first goroutine to stop ends the others.
	err = <-errc
	cancel()
	for range 2 {
		<-errc
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintf(stderr, "run8ops: %v\n", err)
		return 1
	}
	return 0
}

// worldSaveList renders the world saves in dir for an error message.
func worldSaveList(dir string) string {
	names, err := watch.ListWorldSaves(dir)
	if err != nil {
		return fmt.Sprintf("Could not read the save folder %s: %v", dir, err)
	}
	if len(names) == 0 {
		return fmt.Sprintf("No world saves found in %s.", dir)
	}
	return fmt.Sprintf("World saves in %s:\n  %s", dir, strings.Join(names, "\n  "))
}
