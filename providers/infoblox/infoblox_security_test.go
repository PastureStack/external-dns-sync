package infoblox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadSecretFileFromRootAcceptsRelativeAndAbsoluteMountedFiles(t *testing.T) {
	root := t.TempDir()
	secretPath := filepath.Join(root, "infoblox-password")
	if err := os.WriteFile(secretPath, []byte("secret-value\n"), 0o400); err != nil {
		t.Fatal(err)
	}
	for _, configuredPath := range []string{"infoblox-password", secretPath} {
		got, err := readSecretFileFromRoot(root, configuredPath)
		if err != nil {
			t.Fatalf("mounted secret %q was rejected: %v", configuredPath, err)
		}
		if got != "secret-value\n" {
			t.Fatalf("secret payload = %q", got)
		}
	}
}

func TestReadSecretFileFromRootRejectsTraversalAndEscapingSymlink(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside-secret")
	if err := os.WriteFile(outside, []byte("must-not-read"), 0o400); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(root, "escaping-link")
	if err := os.Symlink(outside, symlink); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}

	for _, configuredPath := range []string{
		"../outside-secret",
		outside,
		"escaping-link",
		root,
	} {
		if _, err := readSecretFileFromRoot(root, configuredPath); err == nil {
			t.Fatalf("unsafe secret path %q was accepted", configuredPath)
		}
	}
}

func TestReadSecretFileFromRootRejectsEmptyAndOversizedFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "empty"), nil, 0o400); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "large"), []byte(strings.Repeat("x", maxSecretLen+1)), 0o400); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"empty", "large"} {
		if _, err := readSecretFileFromRoot(root, name); err == nil {
			t.Fatalf("invalid secret file %q was accepted", name)
		}
	}
}
