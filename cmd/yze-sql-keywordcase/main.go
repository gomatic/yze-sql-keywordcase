// Command yze-sql-keywordcase reports PostgreSQL keywords that are not written in
// lowercase in the given .sql files and directories, emitting the lean
// stickler-json report the stickler runner consumes.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	keywordcase "github.com/gomatic/yze-sql-keywordcase"
)

// Injected collaborators, so the command is testable without real I/O.
var (
	osExit             = os.Exit
	readFile           = os.ReadFile
	walkDir            = filepath.WalkDir
	stdout   io.Writer = os.Stdout
)

func main() { osExit(run(os.Args[1:])) }

// run expands the arguments to .sql files, runs the analyzer, and emits the report.
func run(args []string) int {
	files, err := sqlFiles(args)
	if err != nil {
		return fail(err)
	}
	report, err := keywordcase.Report(readFile, files)
	if err != nil {
		return fail(err)
	}
	if err := json.NewEncoder(stdout).Encode(report); err != nil {
		return fail(err)
	}
	return 0
}

// fail prints err to stderr and returns the failure exit code.
func fail(err error) int {
	_, _ = fmt.Fprintln(os.Stderr, "yze-sql-keywordcase:", err)
	return 1
}

// sqlFiles expands each argument: a directory contributes its *.sql files
// (recursively), and any other path is taken verbatim.
func sqlFiles(args []string) ([]string, error) {
	var files []string
	for _, arg := range args {
		info, err := os.Stat(arg)
		switch {
		case err != nil:
			return nil, err
		case info.IsDir():
			found, walkErr := sqlFilesUnder(dirParam(arg))
			if walkErr != nil {
				return nil, walkErr
			}
			files = append(files, found...)
		default:
			files = append(files, arg)
		}
	}
	return files, nil
}

// dirParam names the dir parameter of sqlFilesUnder; rename it to the real domain concept.
type dirParam string

// sqlFilesUnder walks dir collecting every *.sql file.
func sqlFilesUnder(dir dirParam) ([]string, error) {
	var files []string
	err := walkDir(string(dir), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".sql") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}
