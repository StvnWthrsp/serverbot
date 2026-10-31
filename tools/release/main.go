// Command release creates portable release archives for any CI provider.
package main

import (
	"archive/zip"
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	version := flag.String("version", "dev", "release version (dev or vMAJOR.MINOR.PATCH)")
	output := flag.String("output", "dist", "directory for release archives")
	flag.Parse()
	if !regexp.MustCompile(`^(dev|v[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.-]+)?)$`).MatchString(*version) {
		return fmt.Errorf("invalid version")
	}
	if err := os.MkdirAll(*output, 0755); err != nil {
		return err
	}
	checksums := strings.Builder{}
	for _, target := range []struct{ os, arch string }{{"linux", "amd64"}, {"linux", "arm64"}, {"windows", "amd64"}, {"windows", "arm64"}} {
		name := "serverbot"
		if target.os == "windows" {
			name += ".exe"
		}
		tmp, err := os.MkdirTemp("", "serverbot-release-")
		if err != nil {
			return err
		}
		binary := filepath.Join(tmp, name)
		cmd := exec.Command("go", "build", "-trimpath", "-ldflags", "-s -w -X main.version="+*version, "-o", binary, "./cmd/serverbot")
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+target.os, "GOARCH="+target.arch)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			os.RemoveAll(tmp)
			return err
		}
		archiveName := fmt.Sprintf("serverbot_%s_%s_%s.zip", *version, target.os, target.arch)
		archive := filepath.Join(*output, archiveName)
		err = pack(archive, map[string]string{name: binary, "README.md": "README.md", "LICENSE": "LICENSE", "config.example.json": "config.example.json", "config.opencode.example.json": "config.opencode.example.json", "docs/opencode.md": "docs/opencode.md", "examples/opencode/opencode.json": "examples/opencode/opencode.json"})
		os.RemoveAll(tmp)
		if err != nil {
			return err
		}
		f, err := os.Open(archive)
		if err != nil {
			return err
		}
		hash := sha256.New()
		_, copyErr := io.Copy(hash, f)
		f.Close()
		if copyErr != nil {
			return copyErr
		}
		fmt.Fprintf(&checksums, "%x  %s\n", hash.Sum(nil), archiveName)
		fmt.Println(archive)
	}
	return os.WriteFile(filepath.Join(*output, "SHA256SUMS"), []byte(checksums.String()), 0644)
}

func pack(path string, files map[string]string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	z := zip.NewWriter(f)
	names := []string{"serverbot", "serverbot.exe", "README.md", "LICENSE", "config.example.json", "config.opencode.example.json", "docs/opencode.md", "examples/opencode/opencode.json"}
	for _, name := range names {
		source, ok := files[name]
		if !ok {
			continue
		}
		h := &zip.FileHeader{Name: name, Method: zip.Deflate, Modified: time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)}
		h.SetMode(0644)
		if name == "serverbot" || name == "serverbot.exe" {
			h.SetMode(0755)
		}
		w, err := z.CreateHeader(h)
		if err != nil {
			z.Close()
			f.Close()
			return err
		}
		input, err := os.Open(source)
		if err != nil {
			z.Close()
			f.Close()
			return err
		}
		_, err = io.Copy(w, input)
		input.Close()
		if err != nil {
			z.Close()
			f.Close()
			return err
		}
	}
	if err := z.Close(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
