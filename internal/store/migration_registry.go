package store

import (
	"context"
	"database/sql"
	"strconv"
)

const MinimumMigratableSchema = 7

// Both classification and execution use this registry. A future schema bump
// without a complete registered path is never advertised as migratable.
var schemaMigrations = map[string]struct {
	next string
	run  func(context.Context, *sql.DB) error
}{
	"7":  {next: "8", run: migrateSchema7To8},
	"8":  {next: "9", run: migrateSchema8To9},
	"9":  {next: "10", run: migrateSchema9To10},
	"10": {next: "11", run: migrateSchema10To11},
	"11": {next: "12", run: migrateSchema11To12},
	"12": {next: "13", run: migrateSchema12To13},
	"13": {next: "14", run: migrateSchema13To14},
	"14": {next: "15", run: migrateSchema14To15},
	"15": {next: "16", run: migrateSchema15To16},
	"16": {next: "17", run: migrateSchema16To17},
	"17": {next: "18", run: migrateSchema17To18},
	"18": {next: "19", run: migrateSchema18To19},
	"19": {next: "20", run: migrateSchema19To20},
	"20": {next: "21", run: migrateSchema20To21},
	"21": {next: "22", run: migrateSchema21To22},
	"22": {next: "23", run: migrateSchema22To23},
	"23": {next: "24", run: migrateSchema23To24},
	"24": {next: "25", run: migrateSchema24To25},
	"25": {next: "26", run: migrateSchema25To26},
}

func canMigrateSchema(version int) bool {
	if version < MinimumMigratableSchema || version >= SchemaVersion {
		return false
	}
	current := strconv.Itoa(version)
	for steps := 0; steps < len(schemaMigrations); steps++ {
		migration, ok := schemaMigrations[current]
		if !ok || migration.run == nil {
			return false
		}
		current = migration.next
		if current == schemaVersion {
			return true
		}
	}
	return false
}
