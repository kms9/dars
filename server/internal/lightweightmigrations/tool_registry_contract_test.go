package lightweightmigrations

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var toolRegistryTables = []string{
	"tool_source",
	"tool_source_revision",
	"tool_definition",
	"tool_bundle",
	"tool_bundle_item",
	"agent_tool_bundle_head",
	"tool_source_artifact",
	"tool_source_secret",
}

func TestToolRegistryMigrationContract(t *testing.T) {
	migrationDir, err := ResolveDir()
	if err != nil {
		t.Fatal(err)
	}
	base := readMigration(t, migrationDir, "007_tool_registry_bundle.up.sql")
	upperBase := strings.ToUpper(base)
	for _, table := range toolRegistryTables {
		if !strings.Contains(base, "CREATE TABLE "+table+" ") {
			t.Errorf("base migration does not create %s", table)
		}
	}
	for _, forbidden := range []string{"FOREIGN KEY", "REFERENCES ", " ON DELETE ", " CASCADE"} {
		if strings.Contains(upperBase, forbidden) {
			t.Errorf("base migration contains forbidden relationship clause %q", forbidden)
		}
	}
	for _, column := range []string{"ALTER TABLE agent_task_queue ADD COLUMN tool_bundle_id text", "ALTER TABLE task_token ADD COLUMN tool_bundle_id text"} {
		if !strings.Contains(base, column) {
			t.Errorf("base migration missing %q", column)
		}
	}

	indexName := regexp.MustCompile(`^(17[6-9]|18[0-9]|19[0-9])_.*\.up\.sql$`)
	entries, err := os.ReadDir(migrationDir)
	if err != nil {
		t.Fatal(err)
	}
	indexFiles := 0
	for _, entry := range entries {
		if !indexName.MatchString(entry.Name()) {
			continue
		}
		indexFiles++
		statement := strings.TrimSpace(readMigration(t, migrationDir, entry.Name()))
		if strings.Count(statement, ";") != 1 || !strings.HasSuffix(statement, ";") {
			t.Errorf("%s must contain exactly one statement", entry.Name())
		}
		upper := strings.ToUpper(statement)
		if !strings.HasPrefix(upper, "CREATE INDEX CONCURRENTLY ") && !strings.HasPrefix(upper, "CREATE UNIQUE INDEX CONCURRENTLY ") {
			t.Errorf("%s is not a concurrent index migration", entry.Name())
		}
	}
	if indexFiles != 24 {
		t.Fatalf("tool registry index migration count = %d, want 24", indexFiles)
	}

	for _, upName := range append([]string{"007_tool_registry_bundle", "008_tool_registry_guards", "202_bind_tool_registry_constraints"}, migrationBasenames(entries, indexName)...) {
		downPath := filepath.Join(filepath.Dir(migrationDir), "migrations_down", upName+".down.sql")
		if _, err := os.Stat(downPath); err != nil {
			t.Errorf("missing rollback migration for %s: %v", upName, err)
		}
	}
}

func TestToolRegistryDatabaseGuardsOnFreshCheckDatabase(t *testing.T) {
	pool := openFreshCheckDatabase(t)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)

	var tableCount int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = ANY($1)
	`, toolRegistryTables).Scan(&tableCount); err != nil {
		t.Fatal(err)
	}
	if tableCount != len(toolRegistryTables) {
		t.Fatalf("registry table count = %d, want %d", tableCount, len(toolRegistryTables))
	}
	rows, err := tx.Query(ctx, `
		SELECT table_name FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name <> 'schema_migrations'
		ORDER BY table_name
	`)
	if err != nil {
		t.Fatal(err)
	}
	var actualTables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		actualTables = append(actualTables, tableName)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		t.Fatal(err)
	}
	rows.Close()
	expectedTables := SchemaTableManifest()
	sort.Strings(expectedTables)
	if !slices.Equal(actualTables, expectedTables) {
		t.Fatalf("schema table manifest mismatch\nactual: %v\nexpected: %v", actualTables, expectedTables)
	}
	var foreignKeys int
	if err := tx.QueryRow(ctx, `
		SELECT count(*)
		FROM pg_constraint constraint_record
		JOIN pg_class relation ON relation.oid = constraint_record.conrelid
		WHERE constraint_record.contype = 'f' AND relation.relname = ANY($1)
	`, append(toolRegistryTables, "agent_task_queue", "task_token")).Scan(&foreignKeys); err != nil {
		t.Fatal(err)
	}
	if foreignKeys != 0 {
		t.Fatalf("registry schema has %d foreign keys", foreignKeys)
	}

	workspaceA := "10000000-0000-0000-0000-000000000001"
	workspaceB := "10000000-0000-0000-0000-000000000002"
	userID := "20000000-0000-0000-0000-000000000001"
	agentID := "30000000-0000-0000-0000-000000000001"
	sourceID := "40000000-0000-0000-0000-000000000001"
	revisionID := "50000000-0000-0000-0000-000000000001"
	definitionID := "60000000-0000-0000-0000-000000000001"
	artifactID := "70000000-0000-0000-0000-000000000001"
	secretID := "80000000-0000-0000-0000-000000000001"
	bundleID := "tb_freshcheck001"
	secondBundleID := "tb_freshcheck002"
	taskID := "90000000-0000-0000-0000-000000000001"

	execSQL(t, tx, `INSERT INTO agent (id, workspace_id, owner_id, name, permission_mode) VALUES ($1, $2, $3, 'fixture-agent', 'private')`, agentID, workspaceA, userID)
	execSQL(t, tx, `INSERT INTO tool_source (id, workspace_id, name, kind, created_by) VALUES ($1, $2, 'fixture-source', 'grpc', $3)`, sourceID, workspaceA, userID)
	execSQL(t, tx, `INSERT INTO tool_source_artifact (id, workspace_id, source_id, sha256, media_type, size_bytes, content) VALUES ($1, $2, $3, $4, 'application/protobuf', 2, decode('0102', 'hex'))`, artifactID, workspaceA, sourceID, strings.Repeat("a", 64))
	execSQL(t, tx, `INSERT INTO tool_source_secret (id, workspace_id, source_id, envelope, key_id) VALUES ($1, $2, $3, '{"v":1}'::jsonb, 'test')`, secretID, workspaceA, sourceID)
	execSQL(t, tx, `INSERT INTO tool_source_revision (id, workspace_id, source_id, revision, endpoint, artifact_id, secret_id, created_by) VALUES ($1, $2, $3, 1, 'https://fixture.test', $4, $5, $6)`, revisionID, workspaceA, sourceID, artifactID, secretID, userID)
	execSQL(t, tx, `INSERT INTO tool_definition (id, workspace_id, source_id, source_revision_id, public_name, upstream_name, input_schema) VALUES ($1, $2, $3, $4, 'fixture.call', 'Fixture/Call', '{"type":"object"}'::jsonb)`, definitionID, workspaceA, sourceID, revisionID)
	execSQL(t, tx, `UPDATE tool_source_revision SET status = 'ready', published_at = now() WHERE id = $1`, revisionID)
	execSQL(t, tx, `UPDATE tool_source SET current_revision = $1, enabled = true WHERE id = $2`, revisionID, sourceID)
	execSQL(t, tx, `INSERT INTO tool_bundle (id, workspace_id, manifest_hash, created_by) VALUES ($1, $2, $3, $4)`, bundleID, workspaceA, strings.Repeat("b", 64), userID)
	execSQL(t, tx, `INSERT INTO tool_bundle_item (workspace_id, bundle_id, ordinal, public_name, source_id, source_revision_id, tool_definition_id, artifact_id, definition_snapshot, invocation_plan) VALUES ($1, $2, 0, 'fixture.call', $3, $4, $5, $6, '{"input":{"type":"object"}}'::jsonb, '{"method":"Fixture/Call"}'::jsonb)`, workspaceA, bundleID, sourceID, revisionID, definitionID, artifactID)
	execSQL(t, tx, `INSERT INTO agent_tool_bundle_head (workspace_id, agent_id, bundle_id, updated_by) VALUES ($1, $2, $3, $4)`, workspaceA, agentID, bundleID, userID)
	execSQL(t, tx, `INSERT INTO agent_task_queue (id, workspace_id, agent_id, runtime_id, issue_id, status, tool_bundle_id) VALUES ($1, $2, $3, $4, $5, 'dispatched', $6)`, taskID, workspaceA, agentID, "a0000000-0000-0000-0000-000000000001", "b0000000-0000-0000-0000-000000000001", bundleID)
	execSQL(t, tx, `INSERT INTO task_token (token_hash, task_id, agent_id, workspace_id, user_id, tool_bundle_id, expires_at) VALUES ('hash-fixture', $1, $2, $3, $4, $5, now() + interval '1 hour')`, taskID, agentID, workspaceA, userID, bundleID)

	expectSQLFailure(t, tx, `INSERT INTO agent_tool_bundle_head (workspace_id, agent_id, bundle_id, updated_by) VALUES ($1, $2, $3, $4)`, workspaceB, agentID, bundleID, userID)
	expectSQLFailure(t, tx, `UPDATE tool_bundle_item SET invocation_plan = '{"changed":true}'::jsonb WHERE bundle_id = $1`, bundleID)
	execSQL(t, tx, `INSERT INTO tool_bundle (id, workspace_id, manifest_hash, created_by) VALUES ($1, $2, $3, $4)`, secondBundleID, workspaceA, strings.Repeat("c", 64), userID)
	expectSQLFailure(t, tx, `UPDATE agent_task_queue SET tool_bundle_id = $1 WHERE id = $2`, secondBundleID, taskID)
	expectSQLFailure(t, tx, `INSERT INTO task_token (token_hash, task_id, agent_id, workspace_id, user_id, tool_bundle_id, expires_at) VALUES ('hash-mismatch', $1, $2, $3, $4, $5, now() + interval '1 hour')`, taskID, agentID, workspaceA, userID, secondBundleID)
	expectSQLFailure(t, tx, `DELETE FROM tool_source_artifact WHERE id = $1`, artifactID)

	execSQL(t, tx, `DELETE FROM task_token WHERE task_id = $1`, taskID)
	execSQL(t, tx, `DELETE FROM agent_task_queue WHERE id = $1`, taskID)
	execSQL(t, tx, `DELETE FROM agent_tool_bundle_head WHERE workspace_id = $1 AND agent_id = $2`, workspaceA, agentID)
	for _, id := range []string{bundleID, secondBundleID} {
		execSQL(t, tx, `UPDATE tool_bundle SET status = 'revoked', revoked_at = now() WHERE id = $1`, id)
		execSQL(t, tx, `DELETE FROM tool_bundle_item WHERE bundle_id = $1`, id)
		execSQL(t, tx, `DELETE FROM tool_bundle WHERE id = $1`, id)
	}
	execSQL(t, tx, `UPDATE tool_source SET enabled = false, current_revision = NULL WHERE id = $1`, sourceID)
	execSQL(t, tx, `DELETE FROM tool_definition WHERE source_id = $1`, sourceID)
	execSQL(t, tx, `DELETE FROM tool_source_revision WHERE source_id = $1`, sourceID)
	execSQL(t, tx, `DELETE FROM tool_source_artifact WHERE source_id = $1`, sourceID)
	execSQL(t, tx, `DELETE FROM tool_source_secret WHERE source_id = $1`, sourceID)
	execSQL(t, tx, `DELETE FROM tool_source WHERE id = $1`, sourceID)
}

func openFreshCheckDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	raw := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if raw == "" {
		t.Skip("DATABASE_URL is not set; Fresh DB contract runs in make check")
	}
	config, err := pgxpool.ParseConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(config.ConnConfig.Database, "dars_lightweight_check_") {
		t.Skipf("refusing database mutation outside isolated check DB: %s", config.ConnConfig.Database)
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func execSQL(t *testing.T, tx pgx.Tx, sql string, args ...any) {
	t.Helper()
	if _, err := tx.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("SQL failed: %v\n%s", err, sql)
	}
}

func expectSQLFailure(t *testing.T, tx pgx.Tx, sql string, args ...any) {
	t.Helper()
	if _, err := tx.Exec(context.Background(), "SAVEPOINT expected_failure"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(context.Background(), sql, args...); err == nil {
		t.Fatalf("expected SQL failure:\n%s", sql)
	}
	if _, err := tx.Exec(context.Background(), "ROLLBACK TO SAVEPOINT expected_failure"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(context.Background(), "RELEASE SAVEPOINT expected_failure"); err != nil {
		t.Fatal(err)
	}
}

func readMigration(t *testing.T, migrationDir, name string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(migrationDir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func migrationBasenames(entries []os.DirEntry, pattern *regexp.Regexp) []string {
	result := make([]string, 0)
	for _, entry := range entries {
		if pattern.MatchString(entry.Name()) {
			result = append(result, strings.TrimSuffix(entry.Name(), ".up.sql"))
		}
	}
	return result
}
