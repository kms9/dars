package lightweightapi

import (
	"net/http"
	"strings"
	"time"
)

type skillSearchCandidateResponse struct {
	Name         string  `json:"name"`
	URL          string  `json:"url"`
	Source       string  `json:"source"`
	Repo         *string `json:"repo"`
	InstallCount *int64  `json:"install_count"`
	GitHubStars  *int64  `json:"github_stars"`
	Description  string  `json:"description"`
}

func (h *Handler) SearchSkills(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "query is required"})
		return
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}
	candidates, err := searchClawHubSkills(httpClient, query)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"code":  "upstream_unavailable",
			"error": err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, candidates)
}
