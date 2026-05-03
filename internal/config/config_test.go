package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingReturnsDefault(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "does-not-exist.toml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	def := Default()
	if cfg != def {
		t.Errorf("got %+v, want %+v", cfg, def)
	}
}

func TestLoadFullFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	body := `theme = "default"
true_color = false
icons = false
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Theme != "default" {
		t.Errorf("Theme = %q", cfg.Theme)
	}
	if cfg.TrueColor == nil || *cfg.TrueColor != false {
		t.Errorf("TrueColor = %v", cfg.TrueColor)
	}
	if cfg.Icons != false {
		t.Errorf("Icons = %v", cfg.Icons)
	}
}

func TestLoadPartialKeepsDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	// Only override theme; icons should remain at its default (true).
	if err := os.WriteFile(path, []byte(`theme = "neon"`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Theme != "neon" {
		t.Errorf("Theme = %q", cfg.Theme)
	}
	if cfg.Icons != true {
		t.Errorf("Icons = %v, want default true", cfg.Icons)
	}
	if cfg.TrueColor != nil {
		t.Errorf("TrueColor should remain nil for autodetect, got %v", cfg.TrueColor)
	}
}

func TestLoadInvalidTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("not = valid = toml"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid TOML")
	}
}

func TestDefaultPathRespectsXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/custom/xdg")
	got := DefaultPath()
	want := filepath.Join("/custom/xdg", "tetra", "config.toml")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
