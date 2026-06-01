package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestAssetArchiveName(t *testing.T) {
	cases := []struct {
		version, goos, goarch, want string
	}{
		{"v0.2.2", "darwin", "arm64", "yggdrasil_0.2.2_darwin_arm64.tar.gz"},
		{"0.2.2", "linux", "amd64", "yggdrasil_0.2.2_linux_amd64.tar.gz"},
		{"0.2.2", "windows", "amd64", "yggdrasil_0.2.2_windows_amd64.zip"},
	}
	for _, c := range cases {
		if got := assetArchiveName(c.version, c.goos, c.goarch); got != c.want {
			t.Errorf("assetArchiveName(%q,%q,%q) = %q, want %q", c.version, c.goos, c.goarch, got, c.want)
		}
	}
}

func makeTarGz(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExtractBinaryFromTarGz(t *testing.T) {
	want := []byte("\x7fELF-fake-binary")
	archive := makeTarGz(t, map[string][]byte{
		"README.md": []byte("hello\n"),
		"yggdrasil": want,
	})
	got, err := extractBinaryFromTarGz(archive, "yggdrasil")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("extracted wrong bytes: %q", got)
	}
}

func TestExtractBinaryFromTarGz_NotFound(t *testing.T) {
	archive := makeTarGz(t, map[string][]byte{"README.md": []byte("hi\n")})
	if _, err := extractBinaryFromTarGz(archive, "yggdrasil"); err == nil {
		t.Error("expected error when the binary is absent from the archive")
	}
}

func TestVerifyChecksum(t *testing.T) {
	data := []byte("the archive bytes")
	sum := sha256.Sum256(data)
	name := "yggdrasil_0.2.2_darwin_arm64.tar.gz"
	checksums := []byte(
		"deadbeef  some_other_file.tar.gz\n" +
			hex.EncodeToString(sum[:]) + "  " + name + "\n")

	if err := verifyChecksum(name, data, checksums); err != nil {
		t.Errorf("expected checksum to match: %v", err)
	}
	if err := verifyChecksum(name, []byte("tampered"), checksums); err == nil {
		t.Error("expected mismatch error for tampered data")
	}
	if err := verifyChecksum("missing.tar.gz", data, checksums); err == nil {
		t.Error("expected error when the file has no checksum line")
	}
}
