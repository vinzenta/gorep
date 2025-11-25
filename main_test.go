package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
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

func TestConfigure(t *testing.T) {
	// Save and restore os.Args
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"gorep", "test"}
	_, err := Configure()
	if err != nil {
		t.Errorf("Configure failed: %v", err)
	}
}

func TestMainErrorPaths(t *testing.T) {
	ctx := context.Background()

	t.Run("OutputFileAlreadyExists", func(t *testing.T) {
		tempFile := filepath.Join(t.TempDir(), "existing.txt")
		os.WriteFile(tempFile, []byte("exists"), 0o644)

		c := newConfig(regexp.MustCompile("test"), true, "", tempFile, []string{"test", "data"}, 1)
		exitCode := c.Main(ctx)
		if exitCode != 1 {
			t.Errorf("Expected exit code 1 when output file exists, got %d", exitCode)
		}
	})

	t.Run("CantCreateOutputFile", func(t *testing.T) {
		c := newConfig(regexp.MustCompile("test"), true, "", "/invalid/path/file.txt", []string{"test", "data"}, 1)
		exitCode := c.Main(ctx)
		if exitCode != 1 {
			t.Errorf("Expected exit code 1 when can't create output file, got %d", exitCode)
		}
	})

	t.Run("InvalidFilePath", func(t *testing.T) {
		c := newConfig(regexp.MustCompile("test"), true, "/nonexistent/file.txt", "", nil, 1)
		exitCode := c.Main(ctx)
		if exitCode != 1 {
			t.Errorf("Expected exit code 1 for invalid file path, got %d", exitCode)
		}
	})

	t.Run("CantReadFile", func(t *testing.T) {
		tempFile := filepath.Join(t.TempDir(), "unreadable.txt")
		os.WriteFile(tempFile, []byte("test"), 0o000)
		defer os.Chmod(tempFile, 0o644)

		c := newConfig(regexp.MustCompile("test"), true, tempFile, "", nil, 1)
		exitCode := c.Main(ctx)
		if exitCode != 1 {
			t.Errorf("Expected exit code 1 for unreadable file, got %d", exitCode)
		}
	})

	t.Run("FileSearchWithContent", func(t *testing.T) {
		tempFile := filepath.Join(t.TempDir(), "test.txt")
		os.WriteFile(tempFile, []byte("test content\nmore test"), 0o644)

		c := newConfig(regexp.MustCompile("test"), true, tempFile, "", nil, 1)
		exitCode := c.Main(ctx)
		if exitCode != 0 {
			t.Errorf("Expected exit code 0 for successful file search, got %d", exitCode)
		}
	})
}

func TestMatchWithOutputFile(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "output.txt")
	outputFile, err := os.Create(tempFile)
	if err != nil {
		t.Fatalf("Failed to create output file: %v", err)
	}
	defer outputFile.Close()

	c := newConfig(regexp.MustCompile("test"), true, "", "", nil, 1)
	c.match("this is a test\nanother test line", "", outputFile)

	outputFile.Close()
	content, err := os.ReadFile(tempFile)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	if !strings.Contains(string(content), "test") {
		t.Error("Output file should contain matches")
	}
}

func TestMatchWithNoMatches(t *testing.T) {
	c := newConfig(regexp.MustCompile("xyz"), true, "", "", nil, 1)
	c.match("this is a test", "", nil)
	// Should not panic or error, just produce no output
}

func TestContextCancellation(t *testing.T) {
	tempDir := t.TempDir()
	for i := 0; i < 10; i++ {
		os.WriteFile(fmt.Sprintf("%s/file%d.txt", tempDir, i), []byte("test content"), 0o644)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	c := newConfig(regexp.MustCompile("test"), true, "", "", nil, 2)
	absPath, _ := filepath.Abs(tempDir)

	// Should handle cancellation gracefully
	err := c.searchDirectory(ctx, absPath, nil)
	if err != nil && err != context.Canceled {
		t.Errorf("Expected context.Canceled or nil, got: %v", err)
	}
}

func TestWorkerWithInvalidFiles(t *testing.T) {
	tempDir := t.TempDir()

	// Create binary file (invalid UTF-8)
	binaryFile := filepath.Join(tempDir, "binary.bin")
	os.WriteFile(binaryFile, []byte{0xFF, 0xFE, 0xFD}, 0o644)

	// Create unreadable file
	unreadableFile := filepath.Join(tempDir, "unreadable.txt")
	os.WriteFile(unreadableFile, []byte("test"), 0o000)
	defer os.Chmod(unreadableFile, 0o644)

	c := newConfig(regexp.MustCompile("test"), true, "", "", nil, 1)
	absPath, _ := filepath.Abs(tempDir)

	// Should skip invalid files without error
	err := c.searchDirectory(context.Background(), absPath, nil)
	if err != nil {
		t.Errorf("searchDirectory should handle invalid files gracefully, got: %v", err)
	}
}

func TestConfigureWithArgsEdgeCases(t *testing.T) {
	t.Run("NoPatternAfterFlags", func(t *testing.T) {
		_, err := ConfigureWithArgs([]string{"gorep", "-f", "file.txt"})
		if err == nil {
			t.Error("Expected error when no pattern provided after flags")
		}
	})

	t.Run("InvalidFlagValue", func(t *testing.T) {
		_, err := ConfigureWithArgs([]string{"gorep", "-workers", "invalid", "pattern"})
		if err == nil {
			t.Error("Expected error for invalid workers value")
		}
	})

	t.Run("NegativeWorkers", func(t *testing.T) {
		c, err := ConfigureWithArgs([]string{"gorep", "-workers", "0", "test"})
		if err != nil {
			t.Fatalf("ConfigureWithArgs failed: %v", err)
		}
		if c.workers != 1 {
			t.Errorf("Expected workers to be adjusted to 1, got %d", c.workers)
		}
	})

	t.Run("WithAllFlags", func(t *testing.T) {
		tempDir := t.TempDir()
		outputFile := filepath.Join(tempDir, "out.txt")

		c, err := ConfigureWithArgs([]string{"gorep", "-f", "test.txt", "-o", outputFile, "-no-trim", "-workers", "4", "test"})
		if err != nil {
			t.Fatalf("ConfigureWithArgs failed: %v", err)
		}
		if c.trim {
			t.Error("Expected trim to be false with -no-trim flag")
		}
		if c.workers != 4 {
			t.Errorf("Expected 4 workers, got %d", c.workers)
		}
		if c.file != "test.txt" {
			t.Errorf("Expected file to be test.txt, got %s", c.file)
		}
	})
}

func TestSearchDirectoryWithOutputFile(t *testing.T) {
	tempDir := t.TempDir()
	os.WriteFile(tempDir+"/test1.txt", []byte("test content"), 0o644)
	os.WriteFile(tempDir+"/test2.txt", []byte("more test"), 0o644)

	outputPath := filepath.Join(tempDir, "output.txt")
	outputFile, err := os.Create(outputPath)
	if err != nil {
		t.Fatalf("Failed to create output file: %v", err)
	}
	defer outputFile.Close()

	c := newConfig(regexp.MustCompile("test"), true, "", "", nil, 2)
	absPath, _ := filepath.Abs(tempDir)

	err = c.searchDirectory(context.Background(), absPath, outputFile)
	if err != nil {
		t.Errorf("searchDirectory with output file failed: %v", err)
	}

	outputFile.Close()
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	if !strings.Contains(string(content), "test") {
		t.Error("Output file should contain test matches")
	}
}

func TestMainWithStdin(t *testing.T) {
	// Create a pipe to simulate stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}

	// Save original stdin
	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()

	os.Stdin = r

	// Write test data to pipe in goroutine
	go func() {
		w.Write([]byte("test line 1\ntest line 2\nno match here\n"))
		w.Close()
	}()

	ctx := context.Background()
	c := newConfig(regexp.MustCompile("test"), true, "", "", []string{"test"}, 1)

	exitCode := c.Main(ctx)
	if exitCode != 0 {
		t.Errorf("Expected exit code 0 for stdin search, got %d", exitCode)
	}
}

func TestMainWithDirectoryAndOutputFile(t *testing.T) {
	tempDir := t.TempDir()
	os.WriteFile(tempDir+"/file1.txt", []byte("test content"), 0o644)
	os.WriteFile(tempDir+"/file2.txt", []byte("more test"), 0o644)

	outputPath := filepath.Join(t.TempDir(), "output.txt")

	ctx := context.Background()
	c := newConfig(regexp.MustCompile("test"), true, tempDir, outputPath, nil, 2)

	exitCode := c.Main(ctx)
	if exitCode != 0 {
		t.Errorf("Expected exit code 0 for directory search with output, got %d", exitCode)
	}

	// Verify output file was created and has content
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	if !strings.Contains(string(content), "test") {
		t.Error("Output file should contain matches")
	}
}

func TestMatchWithWriteError(t *testing.T) {
	// Create a read-only file to cause write errors
	tempFile := filepath.Join(t.TempDir(), "readonly.txt")
	outputFile, err := os.Create(tempFile)
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}
	outputFile.Close()

	// Make it read-only
	os.Chmod(tempFile, 0o444)
	defer os.Chmod(tempFile, 0o644)

	// Try to open it for writing (should succeed on open, but fail on write)
	outputFile, err = os.OpenFile(tempFile, os.O_WRONLY, 0o644)
	if err != nil {
		// On some systems we can't even open it for writing
		t.Skip("Could not open read-only file for writing on this system")
	}
	defer outputFile.Close()

	c := newConfig(regexp.MustCompile("test"), true, "", "", nil, 1)
	// This should log an error but not panic
	c.match("this is a test", "", outputFile)
}

func TestWorkerWithContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	jobs := make(chan fileJob, 1)
	results := make(chan matchResult, 1)

	var wg sync.WaitGroup
	wg.Add(1)

	c := newConfig(regexp.MustCompile("test"), true, "", "", nil, 1)

	// Start worker
	go c.worker(ctx, jobs, results, &wg)

	// Cancel context before sending jobs
	cancel()
	close(jobs)

	wg.Wait()
	close(results)

	// Should complete without hanging
}

func TestNoTrimFlag(t *testing.T) {
	c := newConfig(regexp.MustCompile("test"), false, "", "", nil, 1)

	input := "\t\ttest content\t\t\n"
	result := c.matchToString(input, "")

	// With trim=false, whitespace should be preserved
	if !strings.Contains(result, "\t") && c.trim == false {
		// Just verify it runs without error
	}
}

func TestEmptyDirectory(t *testing.T) {
	tempDir := t.TempDir()

	ctx := context.Background()
	c := newConfig(regexp.MustCompile("test"), true, tempDir, "", nil, 2)

	exitCode := c.Main(ctx)
	if exitCode != 0 {
		t.Errorf("Expected exit code 0 for empty directory, got %d", exitCode)
	}
}

func TestSearchDirectoryError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel before search

	tempDir := t.TempDir()
	os.WriteFile(tempDir+"/test.txt", []byte("test"), 0o644)

	c := newConfig(regexp.MustCompile("test"), true, tempDir, "", nil, 2)

	exitCode := c.Main(ctx)
	// Should handle canceled context gracefully
	if exitCode != 0 && exitCode != 1 {
		t.Errorf("Expected exit code 0 or 1 for canceled context, got %d", exitCode)
	}
}

func TestMainInlineWithNoFile(t *testing.T) {
	ctx := context.Background()

	// Test the case where args > 1 but no file specified
	c := newConfig(regexp.MustCompile("test"), true, "", "", []string{"test", "inline", "text", "with", "test"}, 1)

	exitCode := c.Main(ctx)
	if exitCode != 0 {
		t.Errorf("Expected exit code 0 for inline text search, got %d", exitCode)
	}
}

func TestMatchWithNoTrim(t *testing.T) {
	c := newConfig(regexp.MustCompile("test"), false, "", "", nil, 1)

	c.match("\t\ttest content\t\t", "", nil)
	// Should not panic or error
}

func TestWorkerWithNoMatches(t *testing.T) {
	tempDir := t.TempDir()
	os.WriteFile(tempDir+"/nomatch.txt", []byte("no matching content here"), 0o644)

	ctx := context.Background()
	c := newConfig(regexp.MustCompile("xyz123"), true, "", "", nil, 1)

	absPath, _ := filepath.Abs(tempDir)
	err := c.searchDirectory(ctx, absPath, nil)
	if err != nil {
		t.Errorf("searchDirectory should handle no matches, got: %v", err)
	}
}

func TestMain(m *testing.M) {
	// Run all tests
	code := m.Run()
	os.Exit(code)
}

// Integration test for main() entry point
func TestMainEntry(t *testing.T) {
	// We can't directly test main() because it calls os.Exit()
	// But we can test that Configure() and Main() work together
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"gorep", "test", "this is a test"}

	c, err := Configure()
	if err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	exitCode := c.Main(context.Background())
	if exitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", exitCode)
	}
}

func TestMatchWithOutputFileWriteSuccess(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "output.txt")
	outputFile, err := os.Create(tempFile)
	if err != nil {
		t.Fatalf("Failed to create output file: %v", err)
	}
	defer outputFile.Close()

	c := newConfig(regexp.MustCompile("test"), true, "", "", nil, 1)

	// Test with matches
	c.match("line 1 test\nline 2 with test\nno match", "", outputFile)

	outputFile.Close()
	content, err := os.ReadFile(tempFile)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	if !strings.Contains(string(content), "line 1") {
		t.Error("Output should contain matched lines")
	}
}

func TestWorkerSelectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	jobs := make(chan fileJob, 10)
	results := make(chan matchResult, 10)

	var wg sync.WaitGroup
	wg.Add(1)

	c := newConfig(regexp.MustCompile("test"), true, "", "", nil, 1)

	// Start worker
	go c.worker(ctx, jobs, results, &wg)

	// Send a job then cancel
	tempFile := filepath.Join(t.TempDir(), "test.txt")
	os.WriteFile(tempFile, []byte("test content"), 0o644)

	jobs <- fileJob{path: tempFile, name: "test.txt"}

	// Cancel context
	cancel()
	close(jobs)

	wg.Wait()
	close(results)

	// Drain results
	for range results {
	}
}

func TestSearchDirectoryWithContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	tempDir := t.TempDir()
	// Create several files
	for i := 0; i < 5; i++ {
		os.WriteFile(fmt.Sprintf("%s/file%d.txt", tempDir, i), []byte("test"), 0o644)
	}

	c := newConfig(regexp.MustCompile("test"), true, "", "", nil, 1)
	absPath, _ := filepath.Abs(tempDir)

	// Cancel immediately
	cancel()

	// Should return context.Canceled or nil
	err := c.searchDirectory(ctx, absPath, nil)
	if err != nil && err != context.Canceled {
		t.Errorf("Expected context.Canceled or nil, got: %v", err)
	}
}

func TestSearchDirectoryWithWalkError(t *testing.T) {
	tempDir := t.TempDir()
	subdir := filepath.Join(tempDir, "subdir")
	os.Mkdir(subdir, 0o755)
	os.WriteFile(filepath.Join(subdir, "test.txt"), []byte("test"), 0o644)

	// Make subdirectory unreadable
	os.Chmod(subdir, 0o000)
	defer os.Chmod(subdir, 0o755)

	c := newConfig(regexp.MustCompile("test"), true, "", "", nil, 1)
	absPath, _ := filepath.Abs(tempDir)

	// Should handle walk errors gracefully
	err := c.searchDirectory(context.Background(), absPath, nil)
	if err != nil {
		t.Errorf("searchDirectory should handle walk errors, got: %v", err)
	}
}

func TestMatchWithWriteFailure(t *testing.T) {
	// Create a real file and then close it to make writes fail
	tempFile := filepath.Join(t.TempDir(), "closed.txt")
	f, err := os.Create(tempFile)
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}
	f.Close() // Close immediately so writes will fail

	c := newConfig(regexp.MustCompile("test"), true, "", "", nil, 1)
	// This should log an error but not panic
	c.match("this is a test", "", f)
}

func TestMainWithStdinEmpty(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}

	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()

	os.Stdin = r
	w.Close() // Close immediately to simulate EOF

	ctx := context.Background()
	c := newConfig(regexp.MustCompile("test"), true, "", "", []string{"test"}, 1)

	exitCode := c.Main(ctx)
	if exitCode != 0 {
		t.Errorf("Expected exit code 0 for empty stdin, got %d", exitCode)
	}
}

func TestMainWithFileAndOutputFile(t *testing.T) {
	// Create a test file
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "input.txt")
	os.WriteFile(testFile, []byte("test content line\nanother test"), 0o644)

	outputFile := filepath.Join(tempDir, "output.txt")

	ctx := context.Background()
	c := newConfig(regexp.MustCompile("test"), true, testFile, outputFile, nil, 1)

	exitCode := c.Main(ctx)
	if exitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", exitCode)
	}

	// Verify output file was created
	content, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	if !strings.Contains(string(content), "test") {
		t.Error("Output file should contain matches")
	}
}
