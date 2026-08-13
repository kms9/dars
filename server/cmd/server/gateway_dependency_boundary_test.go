package main

import (
	"bufio"
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var gatewayDependencyPrefixes = []string{
	"github.com/kms9/dars/internal/mcpgateway",
	"github.com/modelcontextprotocol/go-sdk",
	"google.golang.org/grpc",
	"github.com/bufbuild/protocompile",
	"github.com/getkin/kin-openapi",
}

func TestDARSCLIImportGraphExcludesGatewayDependencies(t *testing.T) {
	serverRoot := serverModuleRoot(t)
	command := exec.Command("go", "list", "-deps", "./cmd/dars")
	command.Dir = serverRoot
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps ./cmd/dars: %v\n%s", err, output)
	}
	imports := make(map[string]struct{})
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		imports[strings.TrimSpace(scanner.Text())] = struct{}{}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	for imported := range imports {
		for _, prefix := range gatewayDependencyPrefixes {
			if imported == prefix || strings.HasPrefix(imported, prefix+"/") {
				t.Errorf("cmd/dars unexpectedly depends on %s", imported)
			}
		}
	}
}

func TestGatewayDependenciesAreDirectServerRequirements(t *testing.T) {
	content, err := os.ReadFile(filepath.Join(serverModuleRoot(t), "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	for _, module := range gatewayDependencyPrefixes[1:] {
		found := false
		for _, line := range strings.Split(string(content), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, module+" ") && !strings.Contains(trimmed, "// indirect") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s is not a direct Server requirement", module)
		}
	}
}

func TestFrozenTargetsDoNotImportGatewayImplementation(t *testing.T) {
	repoRoot := filepath.Dir(serverModuleRoot(t))
	targets := []string{
		"server/internal/daemon",
		"server/pkg/agent",
		"server/pkg/protocol",
		"server/cmd/dars",
		"apps/web",
		"apps/desktop",
		"apps/mobile",
		"apps/docs",
		"packages/core",
		"packages/ui",
		"packages/views",
	}
	for _, target := range targets {
		root := filepath.Join(repoRoot, target)
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				switch entry.Name() {
				case "node_modules", ".next", ".turbo", "dist", "build":
					return filepath.SkipDir
				}
				return nil
			}
			switch filepath.Ext(path) {
			case ".go", ".js", ".jsx", ".ts", ".tsx":
			default:
				return nil
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, prefix := range gatewayDependencyPrefixes {
				if bytes.Contains(content, []byte(prefix)) {
					t.Errorf("frozen target %s imports Gateway dependency %s", path, prefix)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", target, err)
		}
	}
}

func serverModuleRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}
