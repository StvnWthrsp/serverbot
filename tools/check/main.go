// Command check runs the same checks locally and in any CI provider.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	if err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == ".cache" || d.Name() == "bin" || d.Name() == "dist") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		output, err := exec.Command("gofmt", "-l", path).CombinedOutput()
		if err != nil {
			return fmt.Errorf("gofmt: %w", err)
		}
		if len(output) > 0 {
			return fmt.Errorf("run gofmt on %s", path)
		}
		return nil
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	commands := [][]string{{"vet", "./..."}, {"test", "-race", "-count=1", "-coverprofile=coverage.out", "./..."}, {"build", "./..."}}
	for _, args := range commands {
		fmt.Println("go", strings.Join(args, " "))
		cmd := exec.Command("go", args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			os.Exit(1)
		}
	}
}
