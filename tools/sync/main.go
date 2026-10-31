// Command sync pushes the current branch to GitHub, then to the GitLab CI copy.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	branch, err := exec.Command("git", "symbolic-ref", "--quiet", "--short", "HEAD").Output()
	if err != nil {
		return fmt.Errorf("check out a named branch before syncing")
	}
	name := strings.TrimSpace(string(branch))
	if name == "main" || name == "master" {
		return fmt.Errorf("sync feature branches; update the default branch through review")
	}
	for _, remote := range []string{"origin", "gitlab"} {
		if err := exec.Command("git", "remote", "get-url", remote).Run(); err != nil {
			return fmt.Errorf("configure the %s remote before syncing", remote)
		}
	}
	for _, remote := range []string{"origin", "gitlab"} {
		fmt.Printf("Pushing %s to %s\n", name, remote)
		cmd := exec.Command("git", "push", "--set-upstream", remote, "HEAD")
		// Keep origin as the upstream even after the GitLab push.
		if remote == "gitlab" {
			cmd = exec.Command("git", "push", remote, "HEAD")
		}
		cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("push to %s failed; earlier pushes may have succeeded; resolve and rerun without force", remote)
		}
	}
	return nil
}
