package main

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func swapStdout(t *testing.T) *bytes.Buffer {
	t.Helper()
	original := stdout
	buf := &bytes.Buffer{}
	stdout = buf
	t.Cleanup(func() { stdout = original })
	return buf
}

func writeSQL(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestRunEmitsReportForDirectory(t *testing.T) {
	dir := t.TempDir()
	writeSQL(t, dir, "schema.sql", "CREATE TABLE t (id int);")
	buf := swapStdout(t)

	require.Equal(t, 0, run([]string{dir}))
	assert.Contains(t, buf.String(), "yze-sql/keywordcase")
	assert.Contains(t, buf.String(), "lowercase")
}

func TestRunAcceptsExplicitFile(t *testing.T) {
	dir := t.TempDir()
	file := writeSQL(t, dir, "schema.sql", "CREATE TABLE t (id int);")
	buf := swapStdout(t)

	require.Equal(t, 0, run([]string{file}))
	assert.Contains(t, buf.String(), `"diagnostics"`)
}

func TestRunFailsOnMissingPath(t *testing.T) {
	assert.Equal(t, 1, run([]string{filepath.Join(t.TempDir(), "absent.sql")}))
}

func TestRunFailsWhenWalkErrors(t *testing.T) {
	original := walkDir
	walkDir = func(root string, fn fs.WalkDirFunc) error { return fn(root, nil, errors.New("walk boom")) }
	t.Cleanup(func() { walkDir = original })

	assert.Equal(t, 1, run([]string{t.TempDir()}))
}

func TestRunFailsWhenReadErrors(t *testing.T) {
	file := writeSQL(t, t.TempDir(), "schema.sql", "create table t ();")
	original := readFile
	readFile = func(string) ([]byte, error) { return nil, errors.New("read boom") }
	t.Cleanup(func() { readFile = original })

	assert.Equal(t, 1, run([]string{file}))
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("write boom") }

func TestRunFailsWhenEncodeErrors(t *testing.T) {
	file := writeSQL(t, t.TempDir(), "schema.sql", "CREATE TABLE t ();")
	original := stdout
	stdout = failWriter{}
	t.Cleanup(func() { stdout = original })

	assert.Equal(t, 1, run([]string{file}))
}

func TestMainExits(t *testing.T) {
	originalExit, originalArgs := osExit, os.Args
	t.Cleanup(func() { osExit, os.Args = originalExit, originalArgs })
	var code int
	osExit = func(c int) { code = c }
	os.Args = []string{"yze-sql-keywordcase", t.TempDir()}

	main()

	assert.Equal(t, 0, code)
}
