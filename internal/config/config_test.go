package config

import "testing"

func TestLoadDefaultAddress(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/pandora")
	t.Setenv("HTTP_ADDR", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseURL != "postgres://localhost/pandora" || cfg.HTTPAddress != "127.0.0.1:18080" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestLoadEnvironment(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/custom")
	t.Setenv("HTTP_ADDR", "0.0.0.0:9000")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseURL != "postgres://localhost/custom" || cfg.HTTPAddress != "0.0.0.0:9000" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("HTTP_ADDR", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for missing DATABASE_URL")
	}
}
