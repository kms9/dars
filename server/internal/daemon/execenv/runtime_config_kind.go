package execenv

type taskKind int

const (
	kindIssue taskKind = iota
	kindChat
)

func classifyTask(ctx TaskContextForEnv) taskKind {
	if ctx.ChatSessionID != "" {
		return kindChat
	}
	return kindIssue
}
