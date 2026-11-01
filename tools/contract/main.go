// Command contract verifies the pinned real OpenCode runtime against a local fake model.
package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

const archiveURL = "https://registry.npmjs.org/@opencode/cli-linux-x64/-/cli-linux-x64-2.0.32.tgz"
const integrity = "Q50ofBQ3gdwjD6ZQhrVR8fVlm8D2xPMshy4dynY6l6b+qLdKgBSJ9WpoaDyrQX1YWmf2s7IN4RbXmoXXdJitDA=="

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		return fmt.Errorf("automatic contract setup requires Linux amd64; other hosts can set MSA_TEST_OPENCODE and run the test directly")
	}
	dir, err := os.MkdirTemp("", "minecraft-server-agent-contract-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Get(archiveURL)
	if err != nil {
		return fmt.Errorf("download pinned OpenCode: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("OpenCode package download returned HTTP %d", resp.StatusCode)
	}
	archive := filepath.Join(dir, "opencode.tgz")
	f, err := os.Create(archive)
	if err != nil {
		return err
	}
	hash := sha512.New()
	n, copyErr := io.Copy(io.MultiWriter(f, hash), io.LimitReader(resp.Body, 128*1024*1024+1))
	f.Close()
	if copyErr != nil {
		return copyErr
	}
	if n > 128*1024*1024 {
		return fmt.Errorf("OpenCode package exceeds download limit")
	}
	if base64.StdEncoding.EncodeToString(hash.Sum(nil)) != integrity {
		return fmt.Errorf("OpenCode package SHA-512 mismatch")
	}
	f, err = os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	binary := filepath.Join(dir, "opencode")
	found := false
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if header.Name != "package/bin/opencode" {
			continue
		}
		if header.Typeflag != tar.TypeReg || header.Size > 256*1024*1024 {
			return fmt.Errorf("unexpected OpenCode binary entry")
		}
		out, err := os.OpenFile(binary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
		if err != nil {
			return err
		}
		_, err = io.Copy(out, tr)
		out.Close()
		if err != nil {
			return err
		}
		found = true
		break
	}
	if !found {
		return fmt.Errorf("OpenCode binary missing from verified archive")
	}
	cmd := exec.Command("go", "test", "-race", "-count=1", "-timeout", "75s", "-run", "^TestPinnedRuntimeContract$", "-v", "./internal/opencode")
	cmd.Env = append(os.Environ(), "MSA_TEST_OPENCODE="+binary)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}
