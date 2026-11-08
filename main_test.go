package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCheckNeatoInstalledUsesPath(t *testing.T) {
	dir := t.TempDir()
	name := "neato"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte("placeholder"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	if err := CheckNeatoInstalled(); err != nil {
		t.Fatalf("neato is available without an external which/where command: %v", err)
	}
	t.Setenv("PATH", t.TempDir())
	if err := CheckNeatoInstalled(); err == nil {
		t.Fatal("expected error when neato is absent")
	}
}
