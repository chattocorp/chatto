package memlimit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestContainerMemoryLimit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		v2, v1  string // file content; empty means the file does not exist
		want    int64
		wantErr bool
	}{
		{name: "no cgroup files"},
		{name: "cgroup v2 limit", v2: "268435456\n", want: 268435456},
		{name: "cgroup v2 without limit", v2: "max\n"},
		{name: "cgroup v2 before v1", v2: "268435456\n", v1: "536870912\n", want: 268435456},
		{name: "cgroup v1 limit", v1: "536870912\n", want: 536870912},
		{name: "cgroup v1 without limit", v1: "9223372036854771712\n"},
		{name: "malformed limit", v2: "lots\n", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			v2 := filepath.Join(dir, "memory.max")
			v1 := filepath.Join(dir, "memory.limit_in_bytes")
			for path, content := range map[string]string{v2: tt.v2, v1: tt.v1} {
				if content == "" {
					continue
				}
				if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			got, err := containerMemoryLimit([]string{v2, v1})
			if (err != nil) != tt.wantErr {
				t.Fatalf("containerMemoryLimit() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("containerMemoryLimit() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestGoMemoryLimit(t *testing.T) {
	t.Parallel()
	if got, want := goMemoryLimit(256<<20), int64(241591910); got != want {
		t.Fatalf("goMemoryLimit(256 MiB) = %d, want %d", got, want)
	}
}
