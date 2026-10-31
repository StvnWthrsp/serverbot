package minecraft

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"
)

// Only the server-thread chat envelope is accepted. Log files remain a trusted local input.
var chat = regexp.MustCompile(`^\[[^\]\r\n]+\] \[Server thread/INFO\](?: \[[^\]\r\n]+\])?: <([A-Za-z0-9_]{3,16})> ([^\r\n]*)$`)
var trigger = regexp.MustCompile(`(?i)^serverbot(?:\s|$)`)

type Request struct{ ID, Player, Text string }

func ParseChat(line string) (Request, bool) {
	m := chat.FindStringSubmatch(line)
	if m == nil || !trigger.MatchString(strings.TrimSpace(m[2])) || len(m[2]) > 512 {
		return Request{}, false
	}
	return Request{ID: fmt.Sprintf("%x", sha256.Sum256([]byte(line))), Player: m[1], Text: strings.TrimSpace(m[2])}, true
}
