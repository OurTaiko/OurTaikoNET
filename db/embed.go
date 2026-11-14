// Package db holds the schema of ourtaiko_net: the baseline built by build.sh, and the
// changes made to it since.
package db

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

// Baseline creates the schemas fanmade, official and net in an empty database, at version 1.
//
//go:embed baseline.sql
var Baseline string

//go:embed migrations/*.sql
var migrations embed.FS

// Migration is one change to the schema after the baseline. Its file is
// migrations/<version, four digits>_<name>.sql; versions count on from 2 without gaps.
type Migration struct {
	Version int
	Name    string
	SQL     string
}

// Migrations lists every change after the baseline, in order.
func Migrations() ([]Migration, error) {
	files, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	var list []Migration
	for i, file := range files {
		name := strings.TrimSuffix(strings.TrimPrefix(file, "migrations/"), ".sql")
		number, rest, found := strings.Cut(name, "_")
		version, err := strconv.Atoi(number)
		if !found || err != nil || len(number) != 4 || version != i+2 {
			return nil, fmt.Errorf("migration %q: expected version %04d", file, i+2)
		}
		text, err := migrations.ReadFile(file)
		if err != nil {
			return nil, err
		}
		list = append(list, Migration{Version: version, Name: rest, SQL: string(text)})
	}
	return list, nil
}

// Version is the version of the schema with every migration applied.
func Version() int {
	list, err := Migrations()
	if err != nil {
		panic(err)
	}
	return 1 + len(list)
}
