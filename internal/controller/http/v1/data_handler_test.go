package v1

import "testing"

func TestSanitizePathSegment(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"normal filename", "data.csv", "data.csv"},
		{"with directory", "/etc/passwd", "passwd"},
		{"traversal attack", "../../etc/passwd", "passwd"},
		{"deep traversal", "../../../secret.txt", "secret.txt"},
		{"dot dot only", "..", ""},
		{"single dot", ".", ""},
		{"empty string", "", ""},
		{"hidden file", ".hidden", "hidden"},
		{"leading dashes", "---file.txt", "file.txt"},
		{"leading dots and dashes", "..-..-file.txt", "file.txt"},
		{"normal with spaces", "my file.csv", "my file.csv"},
		{"unicode filename", "数据.csv", "数据.csv"},
		{"nested path", "a/b/c/file.txt", "file.txt"},
		{"backslash traversal", `..\..\secret.txt`, `\..\secret.txt`},
		{"all dots", "...", ""},
		{"slash only", "/", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizePathSegment(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizePathSegment(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
