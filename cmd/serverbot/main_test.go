package main

import (
	"bytes"
	"testing"
)

func TestVersionAndHelpNeedNoCredentials(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"run", "-h"}} {
		var out bytes.Buffer
		if err := run(t.Context(), args, &out); err != nil || out.Len() == 0 {
			t.Fatalf("%v: %v", args, err)
		}
	}
}

func TestCLIRejectsUnknownCommand(t *testing.T) {
	if err := run(t.Context(), []string{"shell"}, &bytes.Buffer{}); err == nil {
		t.Fatal("unknown command accepted")
	}
}
