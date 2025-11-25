package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"unicode/utf8"

	"fortio.org/terminal/ansipixels/tcolor"
)

type config struct {
	trim       bool
	file       string
	outputPath string
	re         *regexp.Regexp
	args       []string
	workers    int
}

func newConfig(re *regexp.Regexp, trim bool, file string, outputPath string, args []string, workers int) *config {
	return &config{
		trim:       trim,
		file:       file,
		outputPath: outputPath,
		re:         re,
		args:       args,
		workers:    workers,
	}
}

func Configure() (*config, error) {
	return ConfigureWithArgs(os.Args)
}

func ConfigureWithArgs(args []string) (*config, error) {
	if len(args) < 2 {
		return nil, errors.New("gorep needs a pattern to match")
	}
	
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	noTrim := fs.Bool("no-trim", false,
		"disable trimming leading indentation in each line when printed")
	fileFlag := fs.String("f", "", "take input from a file or directory")
	outputFile := fs.String("o", "", "save the matches to a file")
	workers := fs.Int("workers", runtime.NumCPU(), "number of concurrent workers for directory search")
	
	if err := fs.Parse(args[1:]); err != nil {
		return nil, err
	}
	
	parsedArgs := fs.Args()
	if len(parsedArgs) == 0 {
		return nil, errors.New("pattern argument required")
	}
	
	re, err := regexp.Compile(parsedArgs[0])
	if err != nil {
		return nil, fmt.Errorf("invalid regular expression: %w", err)
	}
	
	if *workers < 1 {
		*workers = 1
	}
	
	return newConfig(re, !*noTrim, *fileFlag, *outputFile, parsedArgs, *workers), nil
}

var (
	GREEN = tcolor.Green.Foreground()
	WHITE = tcolor.White.Foreground()
	RED   = tcolor.Red.Foreground()
	BLUE  = tcolor.Blue.Foreground()
)

type fileJob struct {
	path string
	name string
}

type matchResult struct {
	filename string
	output   string
	hasMatch bool
}

func (c *config) Main(ctx context.Context) int {
	var opf *os.File
	if c.outputPath != "" {
		_, err := os.Stat(c.outputPath)
		if err == nil {
			log.Println("output file already exists")
			return 1
		}
		opf, err = os.Create(c.outputPath)
		if err != nil {
			log.Println("output file couldn't be created")
			return 1
		}
		defer opf.Close()
	}

	var str string
	if len(c.args) > 1 {
		str = strings.Join(c.args[1:], " ")
	}
	
	switch {
	case c.file != "":
		info, err := os.Stat(c.file)
		if err != nil {
			log.Println("can't open given file or directory")
			return 1
		}
		if info.IsDir() {
			absPath, err := filepath.Abs(c.file)
			if err != nil {
				log.Printf("failed to get absolute path: %v\n", err)
				return 1
			}
			if err := c.searchDirectory(ctx, absPath, opf); err != nil {
				log.Printf("error searching directory: %v\n", err)
				return 1
			}
			return 0
		}
		content, err := os.ReadFile(c.file)
		if err != nil {
			log.Println("can't open given file")
			return 1
		}
		str = string(content)
	case len(c.args) < 2:
		scanner := bufio.NewScanner(os.Stdin)
		var builder strings.Builder
		for scanner.Scan() {
			builder.Write(scanner.Bytes())
			builder.WriteByte('\n')
		}
		if err := scanner.Err(); err != nil {
			log.Println("invalid input")
			return 1
		}
		str = builder.String()
	}
	
	c.match(str, "", opf)
	return 0
}

func (c *config) searchDirectory(ctx context.Context, path string, outputFile *os.File) error {
	jobs := make(chan fileJob, c.workers*2)
	results := make(chan matchResult, c.workers*2)
	
	var wg sync.WaitGroup
	
	// Start worker pool
	for i := 0; i < c.workers; i++ {
		wg.Add(1)
		go c.worker(ctx, jobs, results, &wg)
	}
	
	// Start result collector
	done := make(chan struct{})
	var outputMutex sync.Mutex
	go func() {
		for result := range results {
			if result.hasMatch {
				outputMutex.Lock()
				fmt.Print(result.output)
				if outputFile != nil {
					cleanOutput := result.output
					for _, toRemove := range []string{GREEN, RED, BLUE, WHITE} {
						cleanOutput = strings.ReplaceAll(cleanOutput, toRemove, "")
					}
					outputFile.WriteString(cleanOutput)
				}
				outputMutex.Unlock()
			}
		}
		close(done)
	}()
	
	// Walk directory and send jobs
	go func() {
		defer close(jobs)
		filepath.WalkDir(path, func(filePath string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			
			if d.IsDir() {
				return nil
			}
			
			jobs <- fileJob{path: filePath, name: d.Name()}
			return nil
		})
	}()
	
	wg.Wait()
	close(results)
	<-done
	
	return nil
}

func (c *config) worker(ctx context.Context, jobs <-chan fileJob, results chan<- matchResult, wg *sync.WaitGroup) {
	defer wg.Done()
	
	for job := range jobs {
		select {
		case <-ctx.Done():
			return
		default:
		}
		
		content, err := os.ReadFile(job.path)
		if err != nil || !utf8.Valid(content) {
			continue
		}
		
		output := c.matchToString(string(content), fmt.Sprintf("%s%s: \n", BLUE, job.name))
		results <- matchResult{
			filename: job.name,
			output:   output,
			hasMatch: len(output) > 0,
		}
	}
}

func (c *config) match(str string, preString string, output *os.File) {
	result := c.matchToString(str, preString)
	if len(result) > 0 {
		fmt.Print(result)
		if output != nil {
			cleanOutput := result
			for _, toRemove := range []string{GREEN, RED, BLUE, WHITE} {
				cleanOutput = strings.ReplaceAll(cleanOutput, toRemove, "")
			}
			if _, err := output.WriteString(cleanOutput); err != nil {
				log.Println("couldn't write output")
			}
		}
	}
}

func (c *config) matchToString(str string, preString string) string {
	var printBuilder strings.Builder
	lineNum := 0
	matchCount := 0
	
	for line := range strings.Lines(str) {
		lineNum++
		
		// Only run regex once, get indices
		indices := c.re.FindAllStringIndex(line, -1)
		if len(indices) == 0 {
			continue
		}
		
		matchCount++
		
		// Build line number prefix
		printBuilder.WriteString(RED)
		fmt.Fprintf(&printBuilder, "%d. ", lineNum)
		printBuilder.WriteString(WHITE)
		
		// Build line with highlighted matches
		curI := 0
		for j, idx := range indices {
			start, end := idx[0], idx[1]
			
			// Pre-match text
			pre := line[curI:start]
			if c.trim && j == 0 {
				pre = strings.TrimLeft(pre, "\t ")
			}
			printBuilder.WriteString(pre)
			
			// Match text (highlighted)
			printBuilder.WriteString(GREEN)
			printBuilder.WriteString(line[start:end])
			printBuilder.WriteString(WHITE)
			
			curI = end
			
			// Post-match text (on last match)
			if j == len(indices)-1 {
				post := line[end:]
				if c.trim {
					post = strings.TrimRight(post, "\t\n ")
				}
				printBuilder.WriteString(post)
			}
		}
		
		printBuilder.WriteByte('\n')
	}
	
	if matchCount == 0 {
		return ""
	}
	
	// Prepend filename if provided
	if preString != "" {
		return preString + printBuilder.String()
	}
	return printBuilder.String()
}

func main() {
	c, err := Configure()
	if err != nil {
		log.Fatal(err)
	}
	
	ctx := context.Background()
	os.Exit(c.Main(ctx))
}
