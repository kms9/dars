package lightweightmigrations

// schemaTableManifest is the exact checked-in Lightweight application-table
// contract. schema_migrations is runner bookkeeping and is intentionally not
// part of this list.
var schemaTableManifest = []string{
	"activity_log",
	"agent",
	"agent_builder_draft",
	"agent_invocation_target",
	"agent_runtime",
	"agent_skill",
	"agent_task_queue",
	"agent_tool_bundle_head",
	"chat_draft_restore",
	"chat_message",
	"chat_session",
	"comment",
	"daemon_token",
	"issue",
	"member",
	"personal_access_token",
	"runtime_profile",
	"schema_metadata",
	"skill",
	"skill_file",
	"squad",
	"squad_member",
	"task_message",
	"task_token",
	"task_usage",
	"tool_bundle",
	"tool_bundle_item",
	"tool_definition",
	"tool_source",
	"tool_source_artifact",
	"tool_source_revision",
	"tool_source_secret",
	"user",
	"verification_code",
	"workspace",
}

// SchemaTableManifest returns a copy of the exact sorted table contract.
func SchemaTableManifest() []string {
	return append([]string(nil), schemaTableManifest...)
}
