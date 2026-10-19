package coding

import (
	"strings"
	"testing"
)

func TestUnifiedDiff(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		before  string
		after   string
		existed bool
		want    string
	}{
		{
			name:    "replace",
			path:    "hello.txt",
			before:  "alpha\n",
			after:   "beta\n",
			existed: true,
			want: "" +
				"--- a/hello.txt\n" +
				"+++ b/hello.txt\n" +
				"@@ -1,1 +1,1 @@\n" +
				"-alpha\n" +
				"+beta\n",
		},
		{
			name:   "create",
			path:   "hello.txt",
			before: "",
			after:  "hi\n",
			want: "" +
				"--- /dev/null\n" +
				"+++ b/hello.txt\n" +
				"@@ -0,0 +1,1 @@\n" +
				"+hi\n",
		},
		{
			name:    "insert",
			path:    "note.txt",
			before:  "a\nb\nc\n",
			after:   "a\nx\nb\nc\n",
			existed: true,
			want: "" +
				"--- a/note.txt\n" +
				"+++ b/note.txt\n" +
				"@@ -1,3 +1,4 @@\n" +
				" a\n" +
				"+x\n" +
				" b\n" +
				" c\n",
		},
		{
			name:    "delete",
			path:    "note.txt",
			before:  "a\nb\nc\n",
			after:   "a\nc\n",
			existed: true,
			want: "" +
				"--- a/note.txt\n" +
				"+++ b/note.txt\n" +
				"@@ -1,3 +1,2 @@\n" +
				" a\n" +
				"-b\n" +
				" c\n",
		},
		{
			name:    "newline only",
			path:    "hello.txt",
			before:  "alpha",
			after:   "alpha\n",
			existed: true,
			want: "" +
				"--- a/hello.txt\n" +
				"+++ b/hello.txt\n" +
				"@@ -1,1 +1,1 @@\n" +
				"-alpha\n" +
				`\ No newline at end of file` + "\n" +
				"+alpha\n",
		},
		{
			name:    "identical",
			path:    "hello.txt",
			before:  "alpha\n",
			after:   "alpha\n",
			existed: true,
		},
		{
			name:    "absolute path",
			path:    "/tmp/hello.txt",
			before:  "a\n",
			after:   "b\n",
			existed: true,
			want: "" +
				"--- /tmp/hello.txt\n" +
				"+++ /tmp/hello.txt\n" +
				"@@ -1,1 +1,1 @@\n" +
				"-a\n" +
				"+b\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := unifiedDiff(tt.path, tt.before, tt.after, tt.existed)
			if got != tt.want {
				t.Fatalf("diff\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestUnifiedDiffTwoHunks(t *testing.T) {
	var before, after strings.Builder
	for i := range 20 {
		line := "l" + string(rune('a'+i))
		before.WriteString(line + "\n")
		if i == 0 {
			after.WriteString("X\n")
			continue
		}
		if i == 19 {
			after.WriteString("Y\n")
			continue
		}
		after.WriteString(line + "\n")
	}
	got := unifiedDiff("wide.txt", before.String(), after.String(), true)
	if strings.Count(got, "\n@@ ") != 2 {
		t.Fatalf("hunks\n%s", got)
	}
	if !strings.Contains(got, "-la\n") || !strings.Contains(got, "+X\n") || !strings.Contains(got, "+Y\n") {
		t.Fatalf("diff\n%s", got)
	}
	if strings.Contains(got, " lj\n") {
		t.Fatalf("middle line leaked into the diff\n%s", got)
	}
}

func TestUnifiedDiffLongFile(t *testing.T) {
	before := strings.Repeat("a\n", maxDiffLines+1)
	after := before + "z\n"
	got := unifiedDiff("long.txt", before, after, true)
	if !strings.Contains(got, "too long for a line diff") || strings.Contains(got, "@@") {
		t.Fatalf("preview\n%s", got)
	}
}

func TestUnifiedDiffTruncatesRunes(t *testing.T) {
	line := strings.Repeat("a", maxDiffRunes)
	got := unifiedDiff("big.txt", "", line+"\n", false)
	if !strings.Contains(got, "diff truncated") {
		t.Fatal("huge line was not capped")
	}
	if len([]rune(got)) > maxDiffRunes+len("\n… diff truncated\n")+10 {
		t.Fatalf("len %d", len([]rune(got)))
	}
}
