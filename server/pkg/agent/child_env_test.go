package agent

import (
	"slices"
	"testing"
)

func TestMergeEnvRemovesInheritedDARSNamespaceAndKeepsProviderCredentials(t *testing.T) {
	env := mergeEnv([]string{
		"PATH=/usr/bin",
		"DARS_DAEMON_TOKEN=ddt_secret",
		"dars_workspace_id=workspace",
		"XAI_API_KEY=provider-secret",
	}, map[string]string{"DARS_TOKEN": "dat_task", "DARS_TASK_ID": "task"})
	for _, forbidden := range []string{"DARS_DAEMON_TOKEN=ddt_secret", "dars_workspace_id=workspace"} {
		if slices.Contains(env, forbidden) {
			t.Fatalf("inherited daemon environment leaked: %s", forbidden)
		}
	}
	for _, required := range []string{"PATH=/usr/bin", "XAI_API_KEY=provider-secret", "DARS_TOKEN=dat_task", "DARS_TASK_ID=task"} {
		if !slices.Contains(env, required) {
			t.Fatalf("required child environment missing: %s", required)
		}
	}
}
