package lightweightapi

import (
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/kms9/dars/pkg/protocol"
)

const lightweightEdition = "dars_lightweight"

type appConfigResponse struct {
	Edition                   string `json:"edition"`
	ServerVersion             string `json:"server_version,omitempty"`
	AuthMode                  string `json:"auth_mode"`
	DaemonProtocol            string `json:"daemon_protocol"`
	RealtimeEnabled           bool   `json:"realtime_enabled"`
	AllowSignup               bool   `json:"allow_signup"`
	WorkspaceCreationDisabled bool   `json:"workspace_creation_disabled"`
	DaemonServerURL           string `json:"daemon_server_url,omitempty"`
	DaemonAppURL              string `json:"daemon_app_url,omitempty"`
}

func (h *Handler) GetConfig(w http.ResponseWriter, _ *http.Request) {
	serverURL, appURL := lightweightDaemonSetupURLs()
	writeJSON(w, http.StatusOK, appConfigResponse{
		Edition: lightweightEdition, ServerVersion: h.cfg.ServerVersion,
		AuthMode: "email_code", DaemonProtocol: protocol.DaemonProtocolVersion,
		RealtimeEnabled: true, AllowSignup: h.cfg.AllowSignup,
		WorkspaceCreationDisabled: h.cfg.WorkspaceCreationDisabled,
		DaemonServerURL:           serverURL, DaemonAppURL: appURL,
	})
}

func lightweightDaemonSetupURLs() (string, string) {
	appURL := normalizePublicURL(os.Getenv("DARS_APP_URL"))
	if appURL == "" {
		appURL = normalizePublicURL(os.Getenv("FRONTEND_ORIGIN"))
	}
	if appURL == "" || publicURLHost(appURL) == "dars.ai" {
		return "", ""
	}
	serverURL := normalizePublicURL(os.Getenv("DARS_PUBLIC_URL"))
	if serverURL == "" {
		serverURL = appURL
	}
	return serverURL, appURL
}

func normalizePublicURL(raw string) string {
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}

func publicURLHost(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if parsed.Hostname() == "" && !strings.Contains(raw, "://") {
		parsed, err = url.Parse("https://" + raw)
		if err != nil {
			return ""
		}
	}
	return strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
}
