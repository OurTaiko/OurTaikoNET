// Package db holds the schema of ourtaiko_net as built by build.sh.
package db

import _ "embed"

// Baseline creates the schemas fanmade, official and net in an empty database.
//
//go:embed baseline.sql
var Baseline string
