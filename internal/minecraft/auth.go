package minecraft

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var username = regexp.MustCompile(`^[A-Za-z0-9_]{3,16}$`)
var uuid = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func ValidUsername(s string) bool { return username.MatchString(s) }

type Authorizer struct{ Dir string }

func (a Authorizer) Authorize(player string) (string, error) {
	if !ValidUsername(player) {
		return "", errors.New("invalid player name")
	}
	props, err := os.Open(filepath.Join(a.Dir, "server.properties"))
	if err != nil {
		return "", errors.New("cannot read server authentication settings")
	}
	online := false
	scanner := bufio.NewScanner(props)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "online-mode=") {
			online = strings.TrimSpace(strings.TrimPrefix(line, "online-mode=")) == "true"
		}
	}
	err = scanner.Err()
	props.Close()
	if err != nil || !online {
		return "", errors.New("online-mode=true is required")
	}
	f, err := os.Open(filepath.Join(a.Dir, "ops.json"))
	if err != nil {
		return "", errors.New("cannot read OP permissions")
	}
	defer f.Close()
	var ops []struct {
		Name  string `json:"name"`
		UUID  string `json:"uuid"`
		Level int    `json:"level"`
	}
	d := json.NewDecoder(io.LimitReader(f, 1024*1024))
	if d.Decode(&ops) != nil {
		return "", errors.New("invalid OP permissions")
	}
	if d.Decode(new(any)) != io.EOF {
		return "", errors.New("invalid OP permissions")
	}
	for _, op := range ops {
		if strings.EqualFold(op.Name, player) && uuid.MatchString(op.UUID) && op.Level >= 1 && op.Level <= 4 {
			return op.UUID, nil
		}
	}
	return "", errors.New("player is not an OP")
}
