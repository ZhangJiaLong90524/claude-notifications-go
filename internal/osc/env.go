package osc

import "os"

// IsSSHSession reports whether the current process is running inside an SSH session.
// Checks SSH_CONNECTION (OpenSSH, Dropbear, Bitvise, Tailscale SSH, Teleport),
// SSH_CLIENT (deprecated but still set by most servers),
// and SSH_TTY (set when SSH allocates a PTY).
//
// Known limitations:
//   - Mosh: leaves stale SSH_CONNECTION from bootstrap SSH session
//   - tmux/screen: may have stale values from original session
//   - VNC/RDP/web-terminals: not detected (no SSH vars)
func IsSSHSession() bool {
	return os.Getenv("SSH_CONNECTION") != "" ||
		os.Getenv("SSH_CLIENT") != "" ||
		os.Getenv("SSH_TTY") != ""
}
