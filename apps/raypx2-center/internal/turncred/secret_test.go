package turncred

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSharedSecretRejectsWorldReadable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret")
	if err := os.WriteFile(path, []byte("shared-secret"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSharedSecret(path); err == nil {
		t.Fatal("expected error for 0644 secret")
	}
}

func TestLoadSharedSecretAccepts0640(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret")
	if err := os.WriteFile(path, []byte("shared-secret\n"), 0640); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSharedSecret(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "shared-secret" {
		t.Fatalf("got %q", got)
	}
}

func TestEvaluateTURNEnablesWithValidSecret(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret")
	if err := os.WriteFile(path, []byte("shared-secret"), 0640); err != nil {
		t.Fatal(err)
	}
	enabled, secret, reason, err := EvaluateTURN(Config{
		TURNURLs:         []string{"turn:turn.example.com:3478?transport=udp"},
		SharedSecretFile: path,
	})
	if err != nil || !enabled || string(secret) != "shared-secret" || reason != "" {
		t.Fatalf("enabled=%v secret=%q reason=%q err=%v", enabled, secret, reason, err)
	}
}
