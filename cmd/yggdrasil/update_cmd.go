package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/dakasa-yggdrasil/yggdrasil/internal/corecli"
)

const updateReleasesBase = "https://github.com/dakasa-yggdrasil/yggdrasil/releases"

// main runs the command, then drops a throttled "new release available"
// nudge on stderr. The nudge never changes the exit code.
func main() {
	dispatch()
	maybeNotifyUpdate()
}

// maybeNotifyUpdate checks GitHub for a newer release at most once per
// UpdateCheckInterval (caching the result in the config), then prints a
// one-line hint to stderr when the running binary is behind. Entirely
// best-effort: any error (offline, no config, dev build) is silent.
func maybeNotifyUpdate() {
	if len(os.Args) >= 2 && os.Args[1] == "update" {
		return // the update command does its own, louder reporting
	}
	cfg, _, err := corecli.Load()
	if err != nil {
		return
	}
	state := cfg.UpdateCheck
	if state == nil {
		state = &corecli.UpdateCheckState{}
	}
	latest := state.LatestVersion
	if corecli.ShouldCheckForUpdate(state.LastCheckedUnix, time.Now(), corecli.UpdateCheckInterval) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if tag, ferr := corecli.LatestReleaseTag(ctx, &http.Client{Timeout: 3 * time.Second}, ""); ferr == nil {
			latest = tag
			state.LatestVersion = tag
		}
		state.LastCheckedUnix = time.Now().Unix()
		cfg.UpdateCheck = state
		_ = corecli.Save(cfg) // best-effort cache write
	}
	if latest != "" && corecli.IsNewerVersion(version, latest) {
		fmt.Fprintf(os.Stderr, "\n→ A new yggdrasil release is available: %s → %s\n  Update with: yggdrasil update\n", version, latest)
	}
}

// runUpdate implements `yggdrasil update` — download the latest release's
// binary for this OS/arch, verify its checksum, and replace the running
// executable in place. `--check` only reports.
func runUpdate(args []string) error {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	checkOnly := fs.Bool("check", false, "only check whether a newer release exists; don't install")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	tag, err := corecli.LatestReleaseTag(ctx, nil, "")
	if err != nil {
		return fmt.Errorf("check latest release: %w", err)
	}

	switch {
	case version == "dev":
		fmt.Printf("running a development build; latest release is %s\n", tag)
	case !corecli.IsNewerVersion(version, tag):
		fmt.Printf("yggdrasil %s is already the latest (%s)\n", version, tag)
		return nil
	default:
		fmt.Printf("updating yggdrasil %s → %s\n", version, tag)
	}
	if *checkOnly {
		fmt.Println("run `yggdrasil update` to install it")
		return nil
	}

	name := assetArchiveName(tag, runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		return fmt.Errorf("self-update is not supported on windows yet — download %s from %s/latest", name, updateReleasesBase)
	}

	base := updateReleasesBase + "/download/" + tag + "/"
	fmt.Printf("downloading %s ...\n", name)
	data, err := httpGetBytes(ctx, base+name)
	if err != nil {
		return err
	}
	if sums, serr := httpGetBytes(ctx, base+"checksums.txt"); serr == nil {
		if verr := verifyChecksum(name, data, sums); verr != nil {
			return verr
		}
	} else {
		fmt.Println("warning: checksums.txt unavailable — skipping checksum verification")
	}
	bin, err := extractBinaryFromTarGz(data, "yggdrasil")
	if err != nil {
		return err
	}
	if err := replaceExecutable(bin); err != nil {
		return err
	}
	fmt.Printf("✓ updated to %s\n", tag)
	return nil
}

// assetArchiveName mirrors the goreleaser name_template:
// yggdrasil_<version>_<os>_<arch>.tar.gz (zip on windows). The leading
// 'v' on the tag is dropped to match the asset name.
func assetArchiveName(version, goos, goarch string) string {
	v := strings.TrimPrefix(version, "v")
	ext := "tar.gz"
	if goos == "windows" {
		ext = "zip"
	}
	return fmt.Sprintf("yggdrasil_%s_%s_%s.%s", v, goos, goarch, ext)
}

// extractBinaryFromTarGz returns the bytes of binaryName from a .tar.gz.
func extractBinaryFromTarGz(data []byte, binaryName string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if h.Typeflag == tar.TypeReg && filepath.Base(h.Name) == binaryName {
			return io.ReadAll(tr)
		}
	}
	return nil, fmt.Errorf("binary %q not found in archive", binaryName)
}

// verifyChecksum confirms sha256(data) matches the line for name in a
// goreleaser checksums.txt ("<hex>  <filename>").
func verifyChecksum(name string, data, checksums []byte) error {
	want := ""
	for _, line := range strings.Split(string(checksums), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && f[1] == name {
			want = f[0]
			break
		}
	}
	if want == "" {
		return fmt.Errorf("no checksum entry for %s", name)
	}
	got := sha256.Sum256(data)
	if !strings.EqualFold(hex.EncodeToString(got[:]), want) {
		return fmt.Errorf("checksum mismatch for %s (the download may be corrupt)", name)
	}
	return nil
}

func httpGetBytes(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// replaceExecutable atomically swaps the running binary for newBin by
// writing a sibling temp file and renaming over the current path (works
// on unix even while running). Needs write permission to the install dir.
func replaceExecutable(newBin []byte) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if resolved, lerr := filepath.EvalSymlinks(exe); lerr == nil {
		exe = resolved
	}
	dir := filepath.Dir(exe)
	tmp, err := os.CreateTemp(dir, ".yggdrasil-update-*")
	if err != nil {
		return fmt.Errorf("cannot write to %s: %w (try: sudo yggdrasil update)", dir, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(newBin); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return err
	}
	if err := os.Rename(tmpName, exe); err != nil {
		return fmt.Errorf("cannot replace %s: %w (try: sudo yggdrasil update)", exe, err)
	}
	return nil
}
