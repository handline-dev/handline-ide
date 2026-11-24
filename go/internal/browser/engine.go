// Package browser is hl browser: one Chrome per agent, driven over the DevTools protocol. The engine is the machine's Chrome when it has
// one, otherwise Chrome for Testing, fetched once into the home by itself.
package browser

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// installed are the places a machine's own Chrome or Chromium lives, by system.
var installed = map[string][]string{
	"darwin": {
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"~/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
	},
	"linux": {"google-chrome", "google-chrome-stable", "chromium", "chromium-browser"},
}

// versionsURL lists Chrome for Testing's current builds and where to download each.
const versionsURL = "https://googlechromelabs.github.io/chrome-for-testing/last-known-good-versions-with-downloads.json"

// Engine is the Chrome program to run: the machine's own, else the one fetched into home (fetching it the first time).
func Engine(home string) (string, error) {
	if p := machineChrome(); p != "" {
		return p, nil
	}
	return fetched(home)
}

func machineChrome() string {
	user, _ := os.UserHomeDir()
	for _, p := range installed[runtime.GOOS] {
		if strings.HasPrefix(p, "~/") {
			p = filepath.Join(user, p[2:])
		}
		if !strings.Contains(p, "/") {
			if found, err := exec.LookPath(p); err == nil {
				return found
			}
			continue
		}
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}
	return ""
}

// platform is Chrome for Testing's name for this machine, and the program's path inside its zip.
func platform() (name, program string, err error) {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "darwin/arm64":
		name = "mac-arm64"
	case "darwin/amd64":
		name = "mac-x64"
	case "linux/amd64":
		return "linux64", "chrome-linux64/chrome", nil
	default:
		return "", "", fmt.Errorf("no Chrome for Testing build for %s/%s; install Chrome or Chromium", runtime.GOOS, runtime.GOARCH)
	}
	return name, "chrome-" + name + "/Google Chrome for Testing.app/Contents/MacOS/Google Chrome for Testing", nil
}

// engineDir holds the fetched engine: one folder per version, and current naming the one in use.
func engineDir(home string) string { return filepath.Join(home, "browser-engine") }

// fetched is the Chrome for Testing program under home, downloaded and unpacked, under a lock, the first time.
func fetched(home string) (string, error) {
	dir := engineDir(home)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	unlock, err := lockFile(filepath.Join(dir, "lock"))
	if err != nil {
		return "", err
	}
	defer unlock()
	name, program, err := platform()
	if err != nil {
		return "", err
	}
	if cur, err := os.ReadFile(filepath.Join(dir, "current")); err == nil {
		p := filepath.Join(dir, strings.TrimSpace(string(cur)), program)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	version, url, err := latest(name)
	if err != nil {
		return "", fmt.Errorf("fetching the browser: %w", err)
	}
	tmp, err := os.MkdirTemp(dir, ".fetch-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	if err := download(url, tmp); err != nil {
		return "", fmt.Errorf("fetching the browser from %s: %w", url, err)
	}
	final := filepath.Join(dir, version)
	_ = os.RemoveAll(final)
	if err := os.Rename(tmp, final); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "current"), []byte(version+"\n"), 0o644); err != nil {
		return "", err
	}
	return filepath.Join(final, program), nil
}

// latest is the current stable Chrome for Testing version for platform and its zip's address.
func latest(platform string) (version, url string, err error) {
	c := http.Client{Timeout: 30 * time.Second}
	resp, err := c.Get(versionsURL)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("%s: %s", versionsURL, resp.Status)
	}
	var list struct {
		Channels map[string]struct {
			Version   string `json:"version"`
			Downloads map[string][]struct {
				Platform string `json:"platform"`
				URL      string `json:"url"`
			} `json:"downloads"`
		} `json:"channels"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return "", "", err
	}
	stable := list.Channels["Stable"]
	for _, d := range stable.Downloads["chrome"] {
		if d.Platform == platform {
			return stable.Version, d.URL, nil
		}
	}
	return "", "", fmt.Errorf("no stable build for %s", platform)
}

// download unpacks the zip at url into dir.
func download(url, dir string) error {
	resp, err := (&http.Client{Timeout: 15 * time.Minute}).Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New(resp.Status)
	}
	f, err := os.CreateTemp(dir, "chrome-*.zip")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	n, err := io.Copy(f, resp.Body)
	if err != nil {
		return err
	}
	return unzip(f, n, dir)
}

// unzip writes the archive's files under dir, keeping their modes and symbolic links (an app bundle's frameworks are linked).
func unzip(r io.ReaderAt, size int64, dir string) error {
	z, err := zip.NewReader(r, size)
	if err != nil {
		return err
	}
	for _, f := range z.File {
		target := filepath.Join(dir, f.Name)
		if !strings.HasPrefix(target, filepath.Clean(dir)+string(filepath.Separator)) {
			return fmt.Errorf("%s leaves the archive", f.Name)
		}
		mode := f.Mode()
		if mode.IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		if mode&os.ModeSymlink != 0 {
			link, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return err
			}
			to := string(link)
			if filepath.IsAbs(to) || !strings.HasPrefix(filepath.Join(filepath.Dir(target), to), filepath.Clean(dir)+string(filepath.Separator)) {
				return fmt.Errorf("%s links out of the archive", f.Name)
			}
			if err := os.Symlink(to, target); err != nil {
				return err
			}
			continue
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm()|0o600)
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		rc.Close()
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// lockFile waits for the exclusive lock on path; the kernel lets it go if this process dies.
func lockFile(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() { f.Close() }, nil
}
