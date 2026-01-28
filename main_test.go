package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCleanFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"app-secrets.enc.yaml", "app"},
		{"myapp.sops.yaml", "myapp"},
		{"config-secrets.yaml", "config"},
		{"/path/to/app-secrets.enc.yaml", "app"},
		{"service.yaml", "service"},
		{"plainfile", "plainfile"},
		{"multi-part-name-secrets.yaml", "multi-part-name"},
		{"app.prod.sops.yaml", "app"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := cleanFilename(tt.input)
			if result != tt.expected {
				t.Errorf("cleanFilename(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCounterpartFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"app-secrets.enc.yaml", "app.yaml"},
		{"myapp.sops.yaml", "myapp.yaml"},
		{"/path/to/config-secrets.yaml", "/path/to/config.yaml"},
		{"service.yaml", "service.yaml"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := counterpartFilename(tt.input)
			if result != tt.expected {
				t.Errorf("counterpartFilename(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestUpdateCounterpartFile(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	t.Run("updates flat keys with vault refs", func(t *testing.T) {
		path := filepath.Join(tmpDir, "flat.yaml")
		initial := []byte("password: placeholder\ndb_url: placeholder\n")
		os.WriteFile(path, initial, 0644)

		updated, err := updateCounterpartFile(path, "secret/myapp", []string{"password", "db_url"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !updated {
			t.Fatal("expected updated=true")
		}

		fileContent, _ := os.ReadFile(path)
		expected := "password: ref+vault://secret/myapp/password#value\ndb_url: ref+vault://secret/myapp/db_url#value\n"
		if string(fileContent) != expected {
			t.Errorf("unexpected output:\ngot:\n%s\nexpected:\n%s", string(fileContent), expected)
		}
	})

	t.Run("updates nested keys", func(t *testing.T) {
		path := filepath.Join(tmpDir, "nested.yaml")
		initial := []byte("admin:\n  oauth2:\n    clientID: placeholder\n")
		os.WriteFile(path, initial, 0644)

		updated, err := updateCounterpartFile(path, "secret/myapp", []string{"admin.oauth2.clientID"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !updated {
			t.Fatal("expected updated=true")
		}

		fileContent, _ := os.ReadFile(path)
		expected := "admin:\n  oauth2:\n    clientID: ref+vault://secret/myapp/admin.oauth2.clientID#value\n"
		if string(fileContent) != expected {
			t.Errorf("unexpected output:\ngot:\n%s\nexpected:\n%s", string(fileContent), expected)
		}
	})

	t.Run("creates nested structure when no flat keys exist", func(t *testing.T) {
		path := filepath.Join(tmpDir, "partial.yaml")
		initial := []byte("existing: value\n")
		os.WriteFile(path, initial, 0644)

		updated, err := updateCounterpartFile(path, "secret/myapp", []string{"new.key"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !updated {
			t.Fatal("expected updated=true")
		}

		fileContent, _ := os.ReadFile(path)
		// No flat keys at root, so creates nested structure
		expected := "existing: value\nnew:\n  key: ref+vault://secret/myapp/new.key#value\n"
		if string(fileContent) != expected {
			t.Errorf("unexpected output:\ngot:\n%s\nexpected:\n%s", string(fileContent), expected)
		}
	})

	t.Run("adds flat key when flat keys exist at level", func(t *testing.T) {
		path := filepath.Join(tmpDir, "with_flat.yaml")
		initial := []byte("existing.key: value\n")
		os.WriteFile(path, initial, 0644)

		updated, err := updateCounterpartFile(path, "secret/myapp", []string{"new.key"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !updated {
			t.Fatal("expected updated=true")
		}

		fileContent, _ := os.ReadFile(path)
		// Has flat keys at root, so adds as flat
		expected := "existing.key: value\nnew.key: ref+vault://secret/myapp/new.key#value\n"
		if string(fileContent) != expected {
			t.Errorf("unexpected output:\ngot:\n%s\nexpected:\n%s", string(fileContent), expected)
		}
	})

	t.Run("preserves 2-space indentation", func(t *testing.T) {
		path := filepath.Join(tmpDir, "indent2.yaml")
		initial := []byte("admin:\n  password: placeholder\n")
		os.WriteFile(path, initial, 0644)

		updated, err := updateCounterpartFile(path, "secret/myapp", []string{"admin.password"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !updated {
			t.Fatal("expected updated=true")
		}

		fileContent, _ := os.ReadFile(path)
		expected := "admin:\n  password: ref+vault://secret/myapp/admin.password#value\n"
		if string(fileContent) != expected {
			t.Errorf("indentation not preserved:\ngot:\n%s\nexpected:\n%s", string(fileContent), expected)
		}
	})

	t.Run("skips non-existent file", func(t *testing.T) {
		path := filepath.Join(tmpDir, "nonexistent.yaml")
		updated, err := updateCounterpartFile(path, "secret/test", []string{"key"})
		if err != nil {
			t.Fatalf("expected nil error for non-existent file, got: %v", err)
		}
		if updated {
			t.Fatal("expected updated=false for non-existent file")
		}
	})

	t.Run("adds new key at deepest nested path", func(t *testing.T) {
		path := filepath.Join(tmpDir, "deep_nested.yaml")
		// api.config exists as nested, db.max_conn is a flat key under it
		initial := []byte("api:\n  config:\n    db.max_conn: 1\n")
		os.WriteFile(path, initial, 0644)

		// Adding api.config.db.min_conn should add db.min_conn under api.config
		updated, err := updateCounterpartFile(path, "secret/myapp", []string{"api.config.db.min_conn"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !updated {
			t.Fatal("expected updated=true")
		}

		fileContent, _ := os.ReadFile(path)
		expected := "api:\n  config:\n    db.max_conn: 1\n    db.min_conn: ref+vault://secret/myapp/api.config.db.min_conn#value\n"
		if string(fileContent) != expected {
			t.Errorf("unexpected output:\ngot:\n%s\nexpected:\n%s", string(fileContent), expected)
		}
	})

	t.Run("adds new key at deeper nested path", func(t *testing.T) {
		path := filepath.Join(tmpDir, "deeper_nested.yaml")
		// api.config.repair exists as nested
		initial := []byte("api:\n  config:\n    repair:\n      abc.123: 1\n")
		os.WriteFile(path, initial, 0644)

		// Adding api.config.repair.abc.456 should add abc.456 under api.config.repair
		updated, err := updateCounterpartFile(path, "secret/myapp", []string{"api.config.repair.abc.456"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !updated {
			t.Fatal("expected updated=true")
		}

		fileContent, _ := os.ReadFile(path)
		expected := "api:\n  config:\n    repair:\n      abc.123: 1\n      abc.456: ref+vault://secret/myapp/api.config.repair.abc.456#value\n"
		if string(fileContent) != expected {
			t.Errorf("unexpected output:\ngot:\n%s\nexpected:\n%s", string(fileContent), expected)
		}
	})
}
