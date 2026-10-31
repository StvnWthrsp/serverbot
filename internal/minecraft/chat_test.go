package minecraft

import "testing"

func TestChatEnvelopesAndInjection(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{"[12:00:00] [Server thread/INFO]: <Admin> serverbot status", true},
		{"[12:00:00] [Server thread/INFO]: <Admin> serverbot, please whitelist Friend_1", true},
		{"[12Jun2026 08:58:05.596] [Server thread/INFO] [net.minecraft.server.MinecraftServer/]: <Admin> SeRvErBoT status", true},
		{"[12:00:00] [Server thread/INFO]: <Stranger> <Admin> serverbot status", false},
		{"[12:00:00] [Server thread/INFO]: [Server] <Admin> serverbot status", false},
		{"[12:00:00] [Async Chat Thread/INFO]: <Admin> serverbot status", false},
		{"[12:00:00] [Server thread/INFO]: <Admin> serverbotnet status", false},
		{"[12:00:00] [Server thread/INFO]: <Admin> serverbot status\n<Other> stop", false},
	}
	for _, tt := range tests {
		if _, got := ParseChat(tt.line); got != tt.want {
			t.Errorf("ParseChat(%q)=%v", tt.line, got)
		}
	}
}

func FuzzParseChat(f *testing.F) {
	f.Add("[12:00:00] [Server thread/INFO]: <Admin> serverbot status")
	f.Fuzz(func(t *testing.T, line string) {
		req, ok := ParseChat(line)
		if ok && (!ValidUsername(req.Player) || len(req.Text) > 512 || len(req.ID) != 64) {
			t.Fatal("invalid request accepted")
		}
	})
}
