package coding

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Workspace is a directory the coding tools are allowed to touch.
type Workspace struct {
	root string
}

// Open resolves root and checks that it is a directory.
// Later path checks use the symlink-resolved root.
func Open(root string) (*Workspace, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("workspace %s: %w", abs, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workspace %s is not a directory", abs)
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, err
	}
	return &Workspace{root: real}, nil
}

// Root is the absolute, symlink-resolved workspace directory.
func (w *Workspace) Root() string {
	return w.root
}

// Resolve turns path into an absolute path inside the workspace.
// Relative paths start at the workspace root. Symlinks are followed,
// and a target outside the root is rejected.
func (w *Workspace) Resolve(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("path is empty")
	}
	if strings.ContainsRune(path, 0) {
		return "", fmt.Errorf("path is invalid")
	}

	abs := path
	if !filepath.IsAbs(path) {
		abs = filepath.Join(w.root, path)
	}
	abs = filepath.Clean(abs)

	real, err := nearestExisting(abs)
	if err != nil {
		return "", err
	}
	if !w.withinRoot(real) {
		return "", fmt.Errorf("%q is outside the workspace", path)
	}
	return real, nil
}

func (w *Workspace) withinRoot(path string) bool {
	rel, err := filepath.Rel(w.root, path)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// nearestExisting resolves symlinks without requiring the final element to exist.
// Missing parents are walked until an existing directory is found, then the
// missing suffix is joined back on.
func nearestExisting(path string) (string, error) {
	if _, err := os.Lstat(path); err == nil {
		return filepath.EvalSymlinks(path)
	} else if !os.IsNotExist(err) {
		return "", err
	}

	parent := filepath.Dir(path)
	if parent == path {
		return path, nil
	}
	realParent, err := nearestExisting(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(realParent, filepath.Base(path)), nil
}
