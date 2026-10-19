package coding

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/zyy17/pigo/internal/agent"
)

func TestFileTools(t *testing.T) {
	ws := openTemp(t)
	ctx := context.Background()
	write := mustTool(t, ws, "write_file")
	edit := mustTool(t, ws, "edit_file")
	read := mustTool(t, ws, "read_file")
	list := mustTool(t, ws, "list_dir")

	if _, err := write.Execute(ctx, jsonArg(t, map[string]any{
		"path":    "notes/hello.txt",
		"content": "alpha beta alpha\n",
	})); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(ws.Root(), "notes", "hello.txt"))
	if err != nil || string(body) != "alpha beta alpha\n" {
		t.Fatalf("file %q err %v", body, err)
	}

	if _, err := edit.Execute(ctx, jsonArg(t, map[string]any{
		"path": "notes/hello.txt", "old_string": "alpha", "new_string": "gamma",
	})); err == nil || !strings.Contains(err.Error(), "matched") {
		t.Fatal(err)
	}
	if _, err := edit.Execute(ctx, jsonArg(t, map[string]any{
		"path": "notes/hello.txt", "old_string": "beta", "new_string": "delta",
	})); err != nil {
		t.Fatal(err)
	}
	if _, err := edit.Execute(ctx, jsonArg(t, map[string]any{
		"path": "notes/hello.txt", "old_string": "alpha", "new_string": "ALPHA", "replace_all": true,
	})); err != nil {
		t.Fatal(err)
	}

	got, err := read.Execute(ctx, jsonArg(t, map[string]string{"path": "notes/hello.txt"}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "ALPHA delta ALPHA") {
		t.Fatalf("read %q", got)
	}

	listing, err := list.Execute(ctx, []byte("{}"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(listing, "notes/") {
		t.Fatalf("list %q", listing)
	}

	if _, err := read.Execute(ctx, []byte("{")); err == nil {
		t.Fatal("expected invalid JSON to fail")
	}
	if _, err := read.Execute(ctx, jsonArg(t, map[string]string{"path": "../secret"})); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("read escape: %v", err)
	}
	if _, err := write.Execute(ctx, jsonArg(t, map[string]string{
		"path": "/etc/pigo-should-not-exist", "content": "x",
	})); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("write escape: %v", err)
	}
}

func TestReadTruncatedUTF8(t *testing.T) {
	ws := openTemp(t)
	var body strings.Builder
	body.WriteString(strings.Repeat("a", maxFileBytes-1))
	body.WriteString("世")
	if err := os.WriteFile(filepath.Join(ws.Root(), "wide.txt"), []byte(body.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	read := mustTool(t, ws, "read_file")
	got, err := read.Execute(context.Background(), jsonArg(t, map[string]string{"path": "wide.txt"}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "truncated") || strings.Contains(got, "世") {
		t.Fatal("truncated read was not valid text ending before the cut rune")
	}

	if err := os.WriteFile(filepath.Join(ws.Root(), "wide.txt"), []byte{0xff, 0xfe}, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := read.Execute(context.Background(), jsonArg(t, map[string]string{"path": "wide.txt"})); err == nil || !strings.Contains(err.Error(), "UTF-8") {
		t.Fatalf("binary read: %v", err)
	}
}

func TestRunCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell test uses bash")
	}
	ws := openTemp(t)
	tool := mustTool(t, ws, "run_command")
	ctx := context.Background()

	out, err := tool.Execute(ctx, jsonArg(t, map[string]string{"command": "pwd"}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != ws.Root() {
		t.Fatalf("pwd %q want %s", out, ws.Root())
	}

	out, err = tool.Execute(ctx, jsonArg(t, map[string]string{"command": "[[ 1 == 1 ]] && echo ok"}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "ok" {
		t.Fatalf("bash [[ ]] %q", out)
	}

	_, err = tool.Execute(ctx, jsonArg(t, map[string]string{"command": "echo nope; exit 3"}))
	if err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatal(err)
	}

	t.Setenv("OPENAI_API_KEY", "super-secret-value")
	out, err = tool.Execute(ctx, jsonArg(t, map[string]string{"command": "printenv OPENAI_API_KEY || true"}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "super-secret-value") {
		t.Fatalf("api key leaked: %q", out)
	}

	old := commandTimeout
	commandTimeout = 200 * time.Millisecond
	t.Cleanup(func() { commandTimeout = old })
	start := time.Now()
	_, err = tool.Execute(ctx, jsonArg(t, map[string]string{"command": "sleep 5"}))
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatal(err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatalf("timeout took %s", time.Since(start))
	}
}

func openTemp(t *testing.T) *Workspace {
	t.Helper()
	ws, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return ws
}

func mustTool(t *testing.T, ws *Workspace, name string) agent.Tool {
	t.Helper()
	for _, tool := range Tools(ws, nil) {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("missing tool %s", name)
	return agent.Tool{}
}

func jsonArg(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
