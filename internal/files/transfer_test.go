package files

import (
	"path"
	"strings"
	"testing"
)

func TestPathSanitization(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantErr  bool
		errMsg   string
	}{
		{
			name:    "simple filename",
			input:   "test.txt",
			wantErr: false,
		},
		{
			name:    "filename with subdirectory",
			input:   "subdir/test.txt",
			wantErr: false,
		},
		{
			name:    "path traversal attempt",
			input:   "../etc/passwd",
			wantErr: true,
			errMsg:  "path traversal",
		},
		{
			name:    "path traversal in middle",
			input:   "foo/../../etc/passwd",
			wantErr: true,
			errMsg:  "path traversal",
		},
		{
			name:    "double dot in path",
			input:   "foo/../bar",
			wantErr: false, // path.Clean resolves this to "foo/bar" effectively - no ".." remains
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanPath := path.Clean(tt.input)
			hasTraversal := strings.Contains(cleanPath, "..")

			if tt.wantErr && !hasTraversal {
				t.Errorf("expected path traversal to be detected for %q (cleaned: %q)", tt.input, cleanPath)
			}
			if !tt.wantErr && hasTraversal {
				t.Errorf("unexpected path traversal detected for %q (cleaned: %q)", tt.input, cleanPath)
			}
		})
	}
}

func TestWorkspaceConstants(t *testing.T) {
	if WorkspaceVolumeName != "workspace" {
		t.Errorf("WorkspaceVolumeName = %q, want %q", WorkspaceVolumeName, "workspace")
	}
	if WorkspaceMountPath != "/workspace" {
		t.Errorf("WorkspaceMountPath = %q, want %q", WorkspaceMountPath, "/workspace")
	}
	if AppContainerName != "app" {
		t.Errorf("AppContainerName = %q, want %q", AppContainerName, "app")
	}
}
