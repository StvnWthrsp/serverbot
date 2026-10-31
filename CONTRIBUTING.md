# Contributing

Use a feature branch and submit a pull request. Run `go run ./tools/check` before submitting. Tests need local loopback sockets; never point them at a real Minecraft server.

Keep Go packages independent of CI providers. Prefer the standard library unless a dependency has a clear benefit. Test behavior and security boundaries when changing authorization, protocols or operations. New mutations need verification and recovery designs.

Never submit credentials, worlds, player lists, private logs or full agent histories. Use synthetic fixtures. Follow SECURITY.md for private vulnerability reports.
