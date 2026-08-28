package lightweightmigrations

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func TestDaemonTokenMetadataMigrationIsReversibleAndConservative(t *testing.T) {
	migrationDir, err := ResolveDir()
	if err != nil {
		t.Fatal(err)
	}
	up := readMigrationFile(t, filepath.Join(migrationDir, "203_daemon_token_metadata.up.sql"))
	for _, expected := range []string{
		"ADD COLUMN user_id uuid",
		"ADD COLUMN name text",
		"ADD COLUMN token_prefix text",
		"HAVING COUNT(DISTINCT owner_id) = 1",
	} {
		if !strings.Contains(up, expected) {
			t.Errorf("metadata migration missing %q", expected)
		}
	}
	for _, forbidden := range []string{"FOREIGN KEY", "REFERENCES ", "CREATE INDEX", " CASCADE"} {
		if strings.Contains(strings.ToUpper(up), forbidden) {
			t.Errorf("metadata migration contains forbidden clause %q", forbidden)
		}
	}

	downFiles, err := Files("down")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(filepath.Dir(migrationDir), "migrations_down", "203_daemon_token_metadata.down.sql")
	found := false
	for _, file := range downFiles {
		if file == want {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("rollback migration %s is not discoverable", want)
	}
	down := readMigrationFile(t, want)
	for _, expected := range []string{"DROP COLUMN token_prefix", "DROP COLUMN name", "DROP COLUMN user_id"} {
		if !strings.Contains(down, expected) {
			t.Errorf("rollback migration missing %q", expected)
		}
	}
}

func TestSchemaColumnsFixtureMatchesFreshCheckDatabase(t *testing.T) {
	migrationDir, err := ResolveDir()
	if err != nil {
		t.Fatal(err)
	}
	fixturePath := filepath.Join(migrationDir, "..", "..", "..", "openspec", "contracts", "schema-columns.tsv")
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}

	type columnContract struct {
		typeName string
		nullable bool
	}
	expected := make(map[string]columnContract)
	expectedTables := make(map[string]bool)
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	if len(lines) < 2 || lines[0] != "table\tcolumn\ttype\tnullable\tdefault\tenum" {
		t.Fatalf("invalid schema column fixture header")
	}
	for lineNumber, line := range lines[1:] {
		fields := strings.Split(line, "\t")
		if len(fields) != 6 {
			t.Fatalf("schema column fixture line %d has %d fields, want 6", lineNumber+2, len(fields))
		}
		nullable, err := strconv.ParseBool(fields[3])
		if err != nil {
			t.Fatalf("schema column fixture line %d nullable: %v", lineNumber+2, err)
		}
		key := fields[0] + "." + fields[1]
		if _, exists := expected[key]; exists {
			t.Fatalf("duplicate schema column fixture key %s", key)
		}
		expected[key] = columnContract{typeName: fields[2], nullable: nullable}
		expectedTables[fields[0]] = true
	}

	wantTables := SchemaTableManifest()
	for _, table := range wantTables {
		if !expectedTables[table] {
			t.Errorf("schema column fixture is missing table %s", table)
		}
	}
	if len(expectedTables) != len(wantTables) {
		t.Fatalf("schema column fixture table count = %d, want %d", len(expectedTables), len(wantTables))
	}

	pool := openFreshCheckDatabase(t)
	rows, err := pool.Query(context.Background(), `
		SELECT table_name, column_name, udt_name, is_nullable = 'YES'
		FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name <> 'schema_migrations'
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	actual := make(map[string]columnContract)
	for rows.Next() {
		var tableName, columnName, udtName string
		var nullable bool
		if err := rows.Scan(&tableName, &columnName, &udtName, &nullable); err != nil {
			t.Fatal(err)
		}
		actual[tableName+"."+columnName] = columnContract{typeName: normalizePostgresType(udtName), nullable: nullable}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	var mismatches []string
	for key, want := range expected {
		got, exists := actual[key]
		if !exists {
			mismatches = append(mismatches, "missing database column "+key)
			continue
		}
		if got != want {
			mismatches = append(mismatches, fmt.Sprintf("%s = %+v, want %+v", key, got, want))
		}
	}
	for key := range actual {
		if _, exists := expected[key]; !exists {
			mismatches = append(mismatches, "missing fixture column "+key)
		}
	}
	if len(mismatches) > 0 {
		sort.Strings(mismatches)
		t.Fatalf("schema column fixture mismatch:\n%s", strings.Join(mismatches, "\n"))
	}
}

func normalizePostgresType(udtName string) string {
	switch udtName {
	case "int2":
		return "smallint"
	case "int4":
		return "integer"
	case "int8":
		return "bigint"
	case "bool":
		return "boolean"
	case "_uuid":
		return "uuid[]"
	default:
		return udtName
	}
}

func readMigrationFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
