package osc

import "testing"

func TestIsSSHSession(t *testing.T) {
	t.Run("no SSH vars", func(t *testing.T) {
		t.Setenv("SSH_CONNECTION", "")
		t.Setenv("SSH_CLIENT", "")
		t.Setenv("SSH_TTY", "")

		if IsSSHSession() {
			t.Error("expected false when no SSH vars are set")
		}
	})

	t.Run("SSH_CONNECTION set", func(t *testing.T) {
		t.Setenv("SSH_CONNECTION", "")
		t.Setenv("SSH_CLIENT", "")
		t.Setenv("SSH_TTY", "")

		t.Setenv("SSH_CONNECTION", "192.168.1.10 54321 192.168.1.1 22")

		if !IsSSHSession() {
			t.Error("expected true when SSH_CONNECTION is set")
		}
	})

	t.Run("SSH_CLIENT set", func(t *testing.T) {
		t.Setenv("SSH_CONNECTION", "")
		t.Setenv("SSH_CLIENT", "")
		t.Setenv("SSH_TTY", "")

		t.Setenv("SSH_CLIENT", "192.168.1.10 54321 22")

		if !IsSSHSession() {
			t.Error("expected true when SSH_CLIENT is set")
		}
	})

	t.Run("SSH_TTY set", func(t *testing.T) {
		t.Setenv("SSH_CONNECTION", "")
		t.Setenv("SSH_CLIENT", "")
		t.Setenv("SSH_TTY", "")

		t.Setenv("SSH_TTY", "/dev/pts/0")

		if !IsSSHSession() {
			t.Error("expected true when SSH_TTY is set")
		}
	})

	t.Run("all three set", func(t *testing.T) {
		t.Setenv("SSH_CONNECTION", "")
		t.Setenv("SSH_CLIENT", "")
		t.Setenv("SSH_TTY", "")

		t.Setenv("SSH_CONNECTION", "10.0.0.5 12345 10.0.0.1 22")
		t.Setenv("SSH_CLIENT", "10.0.0.5 12345 22")
		t.Setenv("SSH_TTY", "/dev/pts/1")

		if !IsSSHSession() {
			t.Error("expected true when all SSH vars are set")
		}
	})

	t.Run("SSH_CONNECTION empty and no others", func(t *testing.T) {
		t.Setenv("SSH_CONNECTION", "")
		t.Setenv("SSH_CLIENT", "")
		t.Setenv("SSH_TTY", "")

		if IsSSHSession() {
			t.Error("expected false when SSH_CONNECTION is empty and no other vars set")
		}
	})

	t.Run("localhost SSH connection", func(t *testing.T) {
		t.Setenv("SSH_CONNECTION", "")
		t.Setenv("SSH_CLIENT", "")
		t.Setenv("SSH_TTY", "")

		t.Setenv("SSH_CONNECTION", "127.0.0.1 12345 127.0.0.1 22")

		if !IsSSHSession() {
			t.Error("expected true for localhost SSH (we detect presence, even if false positive)")
		}
	})
}
