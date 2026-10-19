package coding

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zyy17/pigo/internal/skills"
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
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, err
	}
	return &Workspace{root: resolved}, nil
}

// Root is the absolute, symlink-resolved workspace directory.
func (w *Workspace) Root() string {
	return w.root
}

// Resolve turns path into an absolute path inside the workspace.
// Relative paths start at the workspace root. The check uses the
// symlink-resolved path, so a link inside the root cannot escape it.
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

	resolved, err := nearestExisting(abs)
	if err != nil {
		return "", err
	}
	if !w.withinRoot(resolved) {
		return "", fmt.Errorf("%q is outside the workspace", path)
	}
	return resolved, nil
}

func (w *Workspace) withinRoot(path string) bool {
	return skills.Within(w.root, path)
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

// readablePath resolves a path for read_file.
// The workspace check still comes first. A discovered skill directory is an
// extra root so the model can open SKILL.md that lives outside the project,
// such as ~/.pi/agent/skills. write_file and edit_file do not use this.
func readablePath(ws *Workspace, skillDirs []string, path string) (string, error) {
	abs, err := ws.Resolve(path)
	if err == nil {
		return abs, nil
	}
	if filepath.IsAbs(path) {
		if skillAbs, skillErr := resolveSkillPath(path, skillDirs); skillErr == nil {
			return skillAbs, nil
		}
	}
	return "", err
}

func resolveSkillPath(path string, roots []string) (string, error) {
	resolved, err := nearestExisting(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	for _, root := range roots {
		resolvedRoot, err := nearestExisting(root)
		if err != nil {
			continue
		}
		if skills.Within(resolvedRoot, resolved) {
			return resolved, nil
		}
	}
	return "", fmt.Errorf("%q is outside the workspace", path)
}
