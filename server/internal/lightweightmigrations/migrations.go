package lightweightmigrations

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/kms9/dars/internal/selfexec"
)

const (
	DefaultDatabaseName    = "dars_lightweight"
	Edition                = "dars_lightweight"
	SupportedSchemaVersion = 1
	MinimumPostgresVersion = 150000
	maxSearchDepth         = 4
)

var candidateLeaves = []string{
	filepath.Join("lightweight", "migrations"),
	filepath.Join("server", "lightweight", "migrations"),
}

type identityQueryer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// ValidateDatabaseIdentity is deliberately read-only and must run before the
// migration runner creates schema_migrations or executes target DDL.
func ValidateDatabaseIdentity(ctx context.Context, db identityQueryer, expectedName string) error {
	expectedName = strings.TrimSpace(expectedName)
	if expectedName == "" {
		return fmt.Errorf("expected database name is empty")
	}

	var databaseName string
	var applicationTables int
	var markerExists bool
	var serverVersion int
	if err := db.QueryRow(ctx, `
		SELECT current_database(),
		       (SELECT COUNT(*)
		        FROM pg_catalog.pg_class c
		        JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
		        WHERE n.nspname = 'public'
		          AND c.relkind IN ('r', 'p')
		          AND c.relname <> 'schema_migrations'),
		       to_regclass('public.schema_metadata') IS NOT NULL,
		       current_setting('server_version_num')::integer
	`).Scan(&databaseName, &applicationTables, &markerExists, &serverVersion); err != nil {
		return fmt.Errorf("probe database identity: %w", err)
	}
	if serverVersion < MinimumPostgresVersion {
		return fmt.Errorf("PostgreSQL server version %d is below required %d", serverVersion, MinimumPostgresVersion)
	}
	if databaseName != expectedName {
		return fmt.Errorf("database name %q does not match expected %q", databaseName, expectedName)
	}
	if applicationTables == 0 {
		return nil
	}
	if !markerExists {
		return fmt.Errorf("non-empty database has no schema_metadata marker")
	}

	var edition string
	var schemaVersion int
	if err := db.QueryRow(ctx, `
		SELECT edition, schema_version
		FROM schema_metadata
		WHERE id = 1
	`).Scan(&edition, &schemaVersion); err != nil {
		return fmt.Errorf("read schema_metadata marker: %w", err)
	}
	if edition != Edition || schemaVersion != SupportedSchemaVersion {
		return fmt.Errorf("unsupported schema marker edition=%q version=%d", edition, schemaVersion)
	}
	return nil
}

// ValidateRuntimeDatabaseIdentity is the stricter Server startup gate. Unlike
// the migration preflight, an empty database is not runnable: the marker must
// already exist and identify the one supported Lightweight schema. This check
// is read-only and is intended to run before listeners or background workers
// are constructed.
func ValidateRuntimeDatabaseIdentity(ctx context.Context, db identityQueryer, expectedName string) error {
	if err := ValidateDatabaseIdentity(ctx, db, expectedName); err != nil {
		return err
	}
	var edition string
	var schemaVersion int
	if err := db.QueryRow(ctx, `
		SELECT edition, schema_version
		FROM schema_metadata
		WHERE id = 1
	`).Scan(&edition, &schemaVersion); err != nil {
		return fmt.Errorf("runtime schema marker is unavailable: %w", err)
	}
	if edition != Edition || schemaVersion != SupportedSchemaVersion {
		return fmt.Errorf("unsupported runtime schema marker edition=%q version=%d", edition, schemaVersion)
	}
	return nil
}

func ResolveDir() (string, error) {
	seen := make(map[string]bool)
	for _, root := range searchRoots() {
		base := root
		for range maxSearchDepth + 1 {
			for _, leaf := range candidateLeaves {
				dir := filepath.Clean(filepath.Join(base, leaf))
				if seen[dir] {
					continue
				}
				seen[dir] = true
				if info, err := os.Stat(dir); err == nil && info.IsDir() {
					return dir, nil
				}
			}
			base = filepath.Join(base, "..")
		}
	}
	return "", fmt.Errorf("lightweight migrations directory not found")
}

func searchRoots() []string {
	roots := []string{"."}
	if executable, err := selfexec.Resolve(); err == nil {
		roots = append(roots, filepath.Dir(executable))
	}
	return roots
}

func Files(direction string) ([]string, error) {
	if direction != "up" && direction != "down" {
		return nil, fmt.Errorf("invalid migration direction %q", direction)
	}
	dir, err := ResolveDir()
	if err != nil {
		return nil, err
	}
	files, err := filepath.Glob(filepath.Join(dir, "*."+direction+".sql"))
	if err != nil {
		return nil, err
	}
	if direction == "down" {
		sort.Sort(sort.Reverse(sort.StringSlice(files)))
	} else {
		sort.Strings(files)
	}
	return files, nil
}

// ExtractVersion returns the migration basename without the direction suffix.
func ExtractVersion(path string) string {
	base := filepath.Base(path)
	base = strings.TrimSuffix(base, ".up.sql")
	return strings.TrimSuffix(base, ".down.sql")
}
