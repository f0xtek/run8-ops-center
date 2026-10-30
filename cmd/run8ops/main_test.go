package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testdata = "../../testdata"

// copyFixture copies a testdata file into dir under dstName.
func copyFixture(t *testing.T, dir, src, dstName string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(testdata, src))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, dstName), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// saveFolders builds a save folder holding WorldSave (a world save) and 29K (a
// train save), an empty auto-save folder and a data folder.
func saveFolders(t *testing.T) (saves, autos, data string) {
	t.Helper()
	root := t.TempDir()
	saves = filepath.Join(root, "Trains")
	autos = filepath.Join(root, "AutoSaves")
	data = filepath.Join(root, "data")
	for _, d := range []string{saves, autos} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{"WorldSave.xml", "WorldSave.info", "29K.xml", "29K.info"} {
		copyFixture(t, saves, f, f)
	}
	return saves, autos, data
}

// runArgs runs the command and returns its exit code and output.
func runArgs(t *testing.T, opt options, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errb bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	code = run(ctx, args, &out, &errb, opt)
	return code, out.String(), errb.String()
}

func TestSaveFlagIsRequiredAndErrorListsWorldSaves(t *testing.T) {
	saves, autos, data := saveFolders(t)
	code, stdout, stderr := runArgs(t, options{}, "--save-dir", saves, "--autosave-dir", autos, "--data", data, "--no-browser")
	if code == 0 {
		t.Fatal("exit code 0 without --save")
	}
	if !strings.Contains(stderr, "--save is required") || !strings.Contains(stderr, "WorldSave") {
		t.Fatalf("stderr does not explain the flag and list the world saves:\n%s", stderr)
	}
	if strings.Contains(stderr, "29K") {
		t.Fatalf("a Train save is listed as a world save:\n%s", stderr)
	}
	if strings.Contains(stdout, "open:") {
		t.Fatalf("the UI started without --save:\n%s", stdout)
	}
}

func TestUnknownSaveNameExitsBeforeServing(t *testing.T) {
	saves, autos, data := saveFolders(t)
	code, stdout, stderr := runArgs(t, options{}, "--save", "Nope", "--save-dir", saves, "--autosave-dir", autos, "--data", data, "--no-browser")
	if code == 0 {
		t.Fatal("exit code 0 for an unknown save")
	}
	if !strings.Contains(stderr, "Nope") || !strings.Contains(stderr, "WorldSave") {
		t.Fatalf("stderr does not name the bad name and list the world saves:\n%s", stderr)
	}
	if strings.Contains(stdout, "open:") {
		t.Fatalf("the UI started for an unknown save:\n%s", stdout)
	}
}

func TestTrainSaveNameIsRejectedAsNotAWorldSave(t *testing.T) {
	saves, autos, data := saveFolders(t)
	code, stdout, stderr := runArgs(t, options{}, "--save", "29K", "--save-dir", saves, "--autosave-dir", autos, "--data", data, "--no-browser")
	if code == 0 {
		t.Fatal("exit code 0 for a Train save")
	}
	if !strings.Contains(strings.ToLower(stderr), "not a world save") || !strings.Contains(stderr, "WorldSave") {
		t.Fatalf("stderr does not say 29K is not a world save and list the world saves:\n%s", stderr)
	}
	if strings.Contains(stdout, "open:") {
		t.Fatalf("the UI started for a Train save:\n%s", stdout)
	}
}

func TestMissingSaveFolderExitsWithoutFallback(t *testing.T) {
	_, autos, data := saveFolders(t)
	missing := filepath.Join(t.TempDir(), "NoSuchTrains")
	code, stdout, stderr := runArgs(t, options{}, "--save", "WorldSave", "--save-dir", missing, "--autosave-dir", autos, "--data", data, "--no-browser")
	if code == 0 {
		t.Fatal("exit code 0 with a missing save folder")
	}
	if !strings.Contains(stderr, missing) {
		t.Fatalf("stderr does not name the missing folder:\n%s", stderr)
	}
	if strings.Contains(stdout, "open:") {
		t.Fatalf("the UI started without a save folder:\n%s", stdout)
	}
}

func TestDataDirInsideRunInstallExitsBeforeCreatingAnything(t *testing.T) {
	saves, autos, _ := saveFolders(t)
	install := filepath.Join(t.TempDir(), "Run8 Train Simulator V3")
	if err := os.MkdirAll(filepath.Join(install, "Content"), 0o755); err != nil {
		t.Fatal(err)
	}
	before := tree(t, install)
	for _, data := range []string{
		filepath.Join(install, "OpsData"),
		filepath.Join(install, "Content", "OpsData"),
		install,
	} {
		code, stdout, stderr := runArgs(t, options{ExtraProtected: []string{install}},
			"--save", "WorldSave", "--save-dir", saves, "--autosave-dir", autos, "--data", data, "--no-browser")
		if code == 0 {
			t.Fatalf("--data %s: exit code 0", data)
		}
		// The error quotes paths with %q, which doubles backslashes on Windows.
		if !strings.Contains(stderr, fmt.Sprintf("%q", install)) {
			t.Fatalf("--data %s: error does not name the protected directory:\n%s", data, stderr)
		}
		if strings.Contains(stdout, "open:") {
			t.Fatalf("--data %s: the UI started", data)
		}
		if after := tree(t, install); after != before {
			t.Fatalf("--data %s changed the install directory:\nbefore:\n%s\nafter:\n%s", data, before, after)
		}
	}
}

// tree lists every path under dir, for before/after comparison.
func tree(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		fmt.Fprintf(&b, "%s %d\n", p, info.Size())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// freePort returns a TCP port that is free right now.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func TestRunServesTheNamedWorldAndStopsCleanly(t *testing.T) {
	saves, autos, data := saveFolders(t)
	saveBefore := tree(t, saves)

	ready := make(chan string, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var out, errb bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- run(ctx, []string{
			"--save", "worldsave", "--save-dir", saves, "--autosave-dir", autos, "--data", data,
			"--port", fmt.Sprint(freePort(t)), "--no-browser",
		}, &out, &errb, options{ListenReady: func(u string) { ready <- u }})
	}()

	var url string
	select {
	case url = <-ready:
	case code := <-done:
		t.Fatalf("run exited early with %d:\n%s", code, errb.String())
	case <-time.After(15 * time.Second):
		t.Fatal("server never started listening")
	}

	// The reference save holds 458 freight cars; the dashboard shows the count
	// once the first snapshot is ingested.
	if !pageEventuallyContains(t, url, "458", 15*time.Second) {
		t.Fatalf("dashboard never showed the 458 cars of the named save:\n%s", fetch(t, url))
	}
	for _, path := range []string{"orders", "yards", "learn", "setup", "activity"} {
		if code, _ := get(t, url+path); code != http.StatusOK {
			t.Errorf("GET /%s = %d", path, code)
		}
	}

	cancel()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("exit code %d after shutdown:\n%s", code, errb.String())
		}
	case <-time.After(15 * time.Second):
		t.Fatal("run did not stop after the context was cancelled")
	}
	if after := tree(t, saves); after != saveBefore {
		t.Fatalf("the save folder changed while running:\nbefore:\n%s\nafter:\n%s", saveBefore, after)
	}
	if _, err := os.Stat(filepath.Join(data, "profiles", "ags-south-p1", "profile.yaml")); err != nil {
		t.Errorf("bundled profile not installed in the data folder: %v", err)
	}
}

func get(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func fetch(t *testing.T, url string) string {
	_, body := get(t, url)
	return body
}

func pageEventuallyContains(t *testing.T, url, want string, within time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if strings.Contains(fetch(t, url), want) {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

func TestVersionFlagPrintsVersionWithoutTouchingAnything(t *testing.T) {
	data := filepath.Join(t.TempDir(), "data")
	code, stdout, stderr := runArgs(t, options{}, "--version", "--data", data)
	if code != 0 {
		t.Fatalf("exit code %d, stderr:\n%s", code, stderr)
	}
	if stdout != "run8ops dev\n" {
		t.Fatalf("stdout = %q, want %q", stdout, "run8ops dev\n")
	}
	if _, err := os.Stat(data); !os.IsNotExist(err) {
		t.Fatalf("--version created the data folder (stat error: %v)", err)
	}
}
