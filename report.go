package keywordcase

import (
	errs "github.com/gomatic/go-error"
	sql "github.com/gomatic/go-sql"
	goyze "github.com/gomatic/go-yze"
)

// ErrReadFile reports that a SQL source file could not be read.
const ErrReadFile errs.Const = "cannot read SQL file"

// FileReader reads a file's bytes; injected so aggregation is testable without the
// filesystem.
type FileReader func(path string) ([]byte, error)

// Report runs the keyword-case check over each file and aggregates the diagnostics
// into the lean stickler-json report. A read failure aborts with ErrReadFile; a
// lexical scan failure aborts with the wrapped sql.ErrScan.
func Report(read FileReader, files []string) (goyze.Report, error) {
	report := goyze.Report{}
	for _, file := range files {
		data, err := read(file)
		if err != nil {
			return goyze.Report{}, ErrReadFile.With(err, "path", file)
		}
		diags, scanErr := Diagnostics(Path(file), sql.SQL(data))
		if scanErr != nil {
			return goyze.Report{}, scanErr
		}
		report.Diagnostics = append(report.Diagnostics, diags...)
	}
	return report, nil
}
