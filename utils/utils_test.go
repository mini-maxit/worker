package utils_test

import (
	"testing"

	"github.com/mini-maxit/worker/utils"
)

func TestValidateFilename(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		wantErr  bool
	}{
		{
			name:     "valid simple filename",
			filename: "solution.py",
			wantErr:  false,
		},
		{
			name:     "valid filename with underscores",
			filename: "my_solution.cpp",
			wantErr:  false,
		},
		{
			name:     "valid filename with hyphens",
			filename: "my-solution-v2.py",
			wantErr:  false,
		},
		{
			name:     "valid filename with dots",
			filename: "solution.test.cpp",
			wantErr:  false,
		},
		{
			name:     "empty filename",
			filename: "",
			wantErr:  true,
		},
		{
			name:     "filename with semicolon - command injection attempt",
			filename: "solution.py; rm -rf /",
			wantErr:  true,
		},
		{
			name:     "filename with pipe - command injection attempt",
			filename: "solution.py | cat /etc/passwd",
			wantErr:  true,
		},
		{
			name:     "filename with ampersand - command injection attempt",
			filename: "solution.py && malicious",
			wantErr:  true,
		},
		{
			name:     "filename with backtick - command injection attempt",
			filename: "solution`whoami`.py",
			wantErr:  true,
		},
		{
			name:     "filename with dollar sign - variable expansion attempt",
			filename: "solution$PATH.py",
			wantErr:  true,
		},
		{
			name:     "filename with forward slash - path traversal attempt",
			filename: "../../../etc/passwd",
			wantErr:  true,
		},
		{
			name:     "filename with backslash - path traversal attempt",
			filename: "..\\..\\windows\\system32",
			wantErr:  true,
		},
		{
			name:     "filename is dot",
			filename: ".",
			wantErr:  true,
		},
		{
			name:     "filename is double dot",
			filename: "..",
			wantErr:  true,
		},
		{
			name:     "filename with space - shell parsing exploit",
			filename: "solution .py",
			wantErr:  true,
		},
		{
			name:     "filename with single quote - SQL/shell injection",
			filename: "solution'.py",
			wantErr:  true,
		},
		{
			name:     "filename with double quote - shell injection",
			filename: "solution\".py",
			wantErr:  true,
		},
		{
			name:     "filename with asterisk - glob expansion",
			filename: "solution*.py",
			wantErr:  true,
		},
		{
			name:     "filename with question mark - glob expansion",
			filename: "solution?.py",
			wantErr:  true,
		},
		{
			name:     "filename with brackets - shell expansion",
			filename: "solution[0].py",
			wantErr:  true,
		},
		{
			name:     "filename with parentheses - subshell attempt",
			filename: "solution(cmd).py",
			wantErr:  true,
		},
		{
			name:     "filename with redirection - file operation exploit",
			filename: "solution>output.txt",
			wantErr:  true,
		},
		{
			name:     "filename with hash - comment injection",
			filename: "solution#comment.py",
			wantErr:  true,
		},
		{
			name:     "filename with tilde - home directory expansion",
			filename: "~/.bashrc",
			wantErr:  true,
		},
		{
			name:     "filename with exclamation - history expansion",
			filename: "solution!.py",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := utils.ValidateFilename(tt.filename)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateFilename() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
