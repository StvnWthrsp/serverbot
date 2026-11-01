package minecraft

import "testing"

func TestChatEnvelopesAndInjection(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{"[12:00:00] [Server thread/INFO]: <Admin> minecraft-server-agent status", true},
		{"[12:00:00] [Server thread/INFO]: <Admin> minecraft-server-agent, please whitelist Friend_1", true},
		{"[12Jun2026 08:58:05.596] [Server thread/INFO] [net.minecraft.server.MinecraftServer/]: <Admin> MiNeCrAfT-SeRvEr-AgEnT status", true},
		{"[12:00:00] [Server thread/INFO]: <Stranger> <Admin> minecraft-server-agent status", false},
		{"[12:00:00] [Server thread/INFO]: [Server] <Admin> minecraft-server-agent status", false},
		{"[12:00:00] [Async Chat Thread/INFO]: <Admin> minecraft-server-agent status", false},
		{"[12:00:00] [Server thread/INFO]: <Admin> minecraft-server-agentnet status", false},
		{"[12:00:00] [Server thread/INFO]: <Admin> minecraft-server-agent status\n<Other> stop", false},
	}
	for _, tt := range tests {
		if _, got := ParseChat(tt.line); got != tt.want {
			t.Errorf("ParseChat(%q)=%v", tt.line, got)
		}
	}
}

func FuzzParseChat(f *testing.F) {
	f.Add("[12:00:00] [Server thread/INFO]: <Admin> minecraft-server-agent status")
	f.Fuzz(func(t *testing.T, line string) {
		req, ok := ParseChat(line)
		if ok && (!ValidUsername(req.Player) || len(req.Text) > 512 || len(req.ID) != 64) {
			t.Fatal("invalid request accepted")
		}
	})
}
