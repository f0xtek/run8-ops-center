package web

import (
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
)

// OpenBrowser opens rawURL in the user's default browser. It returns once the
// opener process has started; it does not wait for the browser. The caller decides
// whether to call it (Config.NoBrowser).
func OpenBrowser(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("open browser: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("open browser: refusing non-http URL %q", rawURL)
	}
	name, args := browserCommand(runtime.GOOS, rawURL)
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open browser with %s: %w", name, err)
	}
	go func() { _ = cmd.Wait() }() // reap the opener
	return nil
}

// browserCommand returns the opener command line for an operating system.
func browserCommand(goos, rawURL string) (string, []string) {
	switch goos {
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", rawURL}
	case "darwin":
		return "open", []string{rawURL}
	default:
		return "xdg-open", []string{rawURL}
	}
}
