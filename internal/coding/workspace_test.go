package coding

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolve(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	ws, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}

	inside, err := ws.Resolve("a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if inside != filepath.Join(ws.Root(), "a.txt") {
		t.Fatalf("inside path %s", inside)
	}
	if _, err := ws.Resolve("./sub/../a.txt"); err != nil {
		t.Fatal(err)
	}
	absInside := filepath.Join(ws.Root(), "a.txt")
	if _, err := ws.Resolve(absInside); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.Resolve("sub/new.txt"); err != nil {
		t.Fatal(err)
	}

	blocked := []string{
		"../secret",
		filepath.Join(root, "..", "secret"),
		"/etc/passwd",
		"sub/../../etc/passwd",
		"",
	}
	for _, path := range blocked {
		if _, err := ws.Resolve(path); err == nil {
			t.Fatalf("accepted %q", path)
		}
	}
}

func TestSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(root, "leak.txt")); err != nil {
		t.Skip(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "outdir")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "real.txt"), []byte("yes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real.txt", filepath.Join(root, "alias.txt")); err != nil {
		t.Fatal(err)
	}

	ws, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ws.Resolve("leak.txt"); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatal(err)
	}
	if _, err := ws.Resolve("outdir/secret.txt"); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatal(err)
	}
	got, err := ws.Resolve("alias.txt")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(got) != "real.txt" {
		t.Fatalf("alias resolved to %s", got)
	}
}
