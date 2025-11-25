package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestInvalidArgs(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		shouldError bool
	}{
		{
			name:        "NoArguments",
			args:        []string{"gorep"},
			shouldError: true,
		},
		{
			name:        "InvalidRegex",
			args:        []string{"gorep", "[invalid", "valid", "test.txt"},
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ConfigureWithArgs(tt.args)
			if tt.shouldError && err == nil {
				t.Errorf("Expected error for test case: %s", tt.name)
			}
			if !tt.shouldError && err != nil {
				t.Errorf("Unexpected error for test case: %s: %v", tt.name, err)
			}
		})
	}
}

func TestValidArgs(t *testing.T) {
	pwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("can't get pwd")
	}
	
	ctx := context.Background()
	
	// Test inline string search
	c, err := ConfigureWithArgs([]string{"gorep", "test", "this is a test"})
	if err != nil {
		t.Fatalf("Configure failed: %v", err)
	}
	if c.Main(ctx) != 0 {
		t.Error("Main should return 0 for valid inline search")
	}
	
	// Test directory search
	tempDir := t.TempDir()
	os.WriteFile(tempDir+"/test1.txt", []byte("const test\nconst value"), 0o644)
	os.WriteFile(tempDir+"/test2.txt", []byte("const another"), 0o644)
	
	c = newConfig(regexp.MustCompile("const[a-z]{3}"), true, tempDir, "", nil, runtime.NumCPU())
	if c.Main(ctx) != 0 {
		t.Error("Main should return 0 for valid directory search")
	}
	t.Chdir(pwd)
	
	// Test file search
	c.file = "main_test.go"
	c.outputPath = ""
	if c.Main(ctx) != 0 {
		t.Error("Main should return 0 for valid file search")
	}
}

func TestMatchToString(t *testing.T) {
	c := newConfig(regexp.MustCompile("test"), true, "", "", nil, 1)
	
	tests := []struct {
		name      string
		input     string
		preString string
		wantEmpty bool
	}{
		{
			name:      "NoMatch",
			input:     "hello world",
			preString: "",
			wantEmpty: true,
		},
		{
			name:      "SingleMatch",
			input:     "this is a test",
			preString: "",
			wantEmpty: false,
		},
		{
			name:      "MultipleMatches",
			input:     "test test test",
			preString: "",
			wantEmpty: false,
		},
		{
			name:      "WithPreString",
			input:     "test line",
			preString: "file.txt: \n",
			wantEmpty: false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := c.matchToString(tt.input, tt.preString)
			if tt.wantEmpty && len(result) > 0 {
				t.Errorf("Expected empty result, got: %s", result)
			}
			if !tt.wantEmpty && len(result) == 0 {
				t.Error("Expected non-empty result, got empty")
			}
			if !tt.wantEmpty && tt.preString != "" && !strings.Contains(result, tt.preString) {
				t.Errorf("Expected result to contain preString %q", tt.preString)
			}
		})
	}
}

func TestSearchDirectory(t *testing.T) {
	pwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("can't get pwd: %v", err)
	}
	defer t.Chdir(pwd)
	
	ctx := context.Background()
	
	// Create temp directory with test files
	tempDir := t.TempDir()
	os.WriteFile(tempDir+"/file1.txt", []byte("hello world\ntest line"), 0o644)
	os.WriteFile(tempDir+"/file2.txt", []byte("another test"), 0o644)
	
	nestedDir := tempDir + "/nested"
	os.Mkdir(nestedDir, 0o755)
	os.WriteFile(nestedDir+"/file3.txt", []byte("nested test content"), 0o644)
	
	c := newConfig(regexp.MustCompile("test"), true, "", "", nil, 2)
	
	absPath, err := filepath.Abs(tempDir)
	if err != nil {
		t.Fatalf("failed to get abs path: %v", err)
	}
	
	err = c.searchDirectory(ctx, absPath, nil)
	if err != nil {
		t.Errorf("searchDirectory failed: %v", err)
	}
}

func TestConcurrentSearch(t *testing.T) {
	pwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("can't get pwd: %v", err)
	}
	defer t.Chdir(pwd)
	
	ctx := context.Background()
	
	// Create temp directory with many files
	tempDir := t.TempDir()
	for i := 0; i < 20; i++ {
		content := fmt.Sprintf("file %d with test content\nmore lines\ntest again", i)
		os.WriteFile(fmt.Sprintf("%s/file%d.txt", tempDir, i), []byte(content), 0o644)
	}
	
	c := newConfig(regexp.MustCompile("test"), true, tempDir, "", nil, 4)
	
	absPath, err := filepath.Abs(tempDir)
	if err != nil {
		t.Fatalf("failed to get abs path: %v", err)
	}
	
	err = c.searchDirectory(ctx, absPath, nil)
	if err != nil {
		t.Errorf("concurrent search failed: %v", err)
	}
}
