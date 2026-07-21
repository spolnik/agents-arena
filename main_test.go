package main

import (
	"path/filepath"
	"testing"
)

func TestDefaultAddressUsesPlatformPort(t *testing.T) {
	t.Setenv("PORT", "24680")
	if got := defaultAddress(); got != ":24680" {
		t.Fatalf("defaultAddress() = %q", got)
	}
}

func TestDefaultDatabasePath(t *testing.T) {
	t.Run("explicit override", func(t *testing.T) {
		t.Setenv("ARENA_DATABASE_PATH", "/custom/arena.db")
		t.Setenv("RAILWAY_VOLUME_MOUNT_PATH", "/data")
		if got := defaultDatabasePath(); got != "/custom/arena.db" {
			t.Fatalf("defaultDatabasePath() = %q", got)
		}
	})
	t.Run("Railway volume", func(t *testing.T) {
		t.Setenv("ARENA_DATABASE_PATH", "")
		t.Setenv("RAILWAY_VOLUME_MOUNT_PATH", "/data")
		if got := defaultDatabasePath(); got != filepath.Join("/data", "arena.db") {
			t.Fatalf("defaultDatabasePath() = %q", got)
		}
	})
}
