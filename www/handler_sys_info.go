package www

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"time"

	_ "embed"
)

type sysInfo struct {
	CurrentVersion string
	RuntimeVersion string
	LatestVersion  string
}

func NewSysInfoHandler(logger *slog.Logger, tm *TemplateManager, currentVersion string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")

		latestVersion, err := getLatestVersion(r.Context())
		if err != nil {
			latestVersion = currentVersion
			logger.Error("getting current release tag", slog.Any("error", err))
		}

		sysInfo := sysInfo{
			CurrentVersion: currentVersion,
			RuntimeVersion: runtime.Version(),
			LatestVersion:  latestVersion,
		}

		if err := tm.ExecuteToWriter("sys_info.html", sysInfo, &w); err != nil {
			logger.Error("handling log request", slog.Any("error", err))
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

func getLatestVersion(ctx context.Context) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/repos/angas/solarplant-go/tags", nil)
	client := http.Client{Timeout: 10 * time.Second}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "solarplant-go")
	res, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error getting current release tag: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("got status code %d while fetching current release tag", res.StatusCode)
	}

	type GithubTag struct {
		Name string `json:"name"`
	}
	var tags []GithubTag
	if err := json.NewDecoder(res.Body).Decode(&tags); err != nil {
		return "", err
	}

	if len(tags) == 0 {
		return "", nil
	}

	return tags[0].Name, nil
}
