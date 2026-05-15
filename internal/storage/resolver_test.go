package storage

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestEncodeCWD(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"/Users/alice/myproject", "-Users-alice-myproject"},
		{"/", "-"},
		{"/a", "-a"},
		{"/a/b/c", "-a-b-c"},
		{"/path with space/x", "-path with space-x"},
	}
	for _, c := range cases {
		if got := EncodeCWD(c.in); got != c.want {
			t.Errorf("EncodeCWD(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestResolverPaths(t *testing.T) {
	r := newResolver("/Users/alice/proj", "/Users/alice")

	wantProject := filepath.Join("/Users/alice", ".tgconn", "projects", "-Users-alice-proj")
	if r.ProjectDir() != wantProject {
		t.Errorf("ProjectDir = %q, want %q", r.ProjectDir(), wantProject)
	}
	if r.CWD() != "/Users/alice/proj" {
		t.Errorf("CWD = %q", r.CWD())
	}
	if !strings.HasSuffix(r.Subdir("tmp"), "/-Users-alice-proj/tmp") {
		t.Errorf("Subdir(tmp) = %q", r.Subdir("tmp"))
	}
	if r.LegacyDir() != "/Users/alice/proj/.tgconn" {
		t.Errorf("LegacyDir = %q", r.LegacyDir())
	}
}
