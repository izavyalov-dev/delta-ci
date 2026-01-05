package migrations

import _ "embed"

// Migration represents a single SQL migration to apply in order.
type Migration struct {
	ID     string
	Script string
}

//go:embed 0001_initial.sql
var initial string

// All lists migrations in application order.
var All = []Migration{
	{ID: "0001_initial", Script: initial},
}
