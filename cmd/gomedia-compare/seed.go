package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// Seed media is the same file used by go-ffmpeg tests, pinned to a commit and verified by SHA-256.
const (
	goFFmpegSeedCommit = "eaaf6b064cf7143a719af6c50e222914b525e88b"
	seedFileName       = "Big_Buck_Bunny_1080_10s_2MB.mp4"
	seedSHA256         = "bad8caf78fbf9ca292db2f170f99ff6251782947c0bfa8bfd391b4da4a459163"
)

// seedURL returns the raw GitHub URL for the pinned seed object.
func seedURL() string {
	return fmt.Sprintf(
		"https://raw.githubusercontent.com/gtsteffaniak/go-ffmpeg/%s/test/data/%s",
		goFFmpegSeedCommit, seedFileName,
	)
}

// ensureSeed downloads the seed file when missing or when the checksum does not match.
func ensureSeed(destDir string) (string, error) {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(destDir, seedFileName)
	if ok, err := fileMatchesSHA256(path, seedSHA256); err != nil {
		return "", err
	} else if ok {
		return path, nil
	}
	if err := downloadSeed(path); err != nil {
		return "", err
	}
	if ok, err := fileMatchesSHA256(path, seedSHA256); err != nil {
		return "", err
	} else if !ok {
		return "", fmt.Errorf("seed checksum mismatch after download (expected %s)", seedSHA256)
	}
	return path, nil
}

// fileMatchesSHA256 reports whether path exists and matches wantHex.
func fileMatchesSHA256(path, wantHex string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}
	return hex.EncodeToString(h.Sum(nil)) == wantHex, nil
}

// downloadSeed fetches the pinned seed over HTTPS.
func downloadSeed(path string) error {
	resp, err := http.Get(seedURL())
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("seed download: HTTP %d", resp.StatusCode)
	}
	tmp := path + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, resp.Body); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}
