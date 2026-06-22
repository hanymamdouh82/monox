package fetcher

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/hanymamdouh82/trdc03mon/internal/config"
)

type SyncthingStatus struct {
	Uptime     int `json:"uptime"`
	Goroutines int `json:"goroutines"`
}

type SyncthingSystemConfig struct {
	Folders []struct {
		ID    string `json:"id"`
		Label string `json:"label"`
	} `json:"folders"`
}

type SyncthingCompletion struct {
	Completion float64 `json:"completion"`
}

func FetchSyncthing() string {
	client := &http.Client{
		Timeout: 3 * time.Second, // Lowered slightly to give breathing room
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	var sb strings.Builder

	getJson := func(endpoint string, target interface{}) error {
		url := strings.TrimSuffix(config.AppConfig.Syncthing.URL, "/") + endpoint
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return err
		}
		req.Header.Set("X-API-Key", config.AppConfig.Syncthing.APIKey)

		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		return json.NewDecoder(resp.Body).Decode(target)
	}

	// 1. Query System Diagnostics
	var status SyncthingStatus
	if err := getJson("/rest/system/status", &status); err != nil {
		return fmt.Sprintf("  ERROR: API Unreachable\n  %v", err)
	}

	days := status.Uptime / (24 * 3600)
	hours := (status.Uptime % (24 * 3600)) / 3600
	mins := (status.Uptime % 3600) / 60

	sb.WriteString(fmt.Sprintf("  Uptime     : %dd %dh %dm\n", days, hours, mins))
	sb.WriteString(fmt.Sprintf("  Goroutines : %d\n\n", status.Goroutines))
	sb.WriteString("  ── Folders ──────────────────────────\n")

	// 2. Extract Active Folders Layout
	var sysCfg SyncthingSystemConfig
	if err := getJson("/rest/system/config", &sysCfg); err != nil {
		sb.WriteString(fmt.Sprintf("  Config Error: %v\n", err))
		return sb.String()
	}

	if len(sysCfg.Folders) == 0 {
		sb.WriteString("  No active shared folders found.\n")
		return sb.String()
	}

	// ─── CONCURRENT FOLDER FETCHING ───
	type folderResult struct {
		output string
		index  int
	}

	results := make([]string, len(sysCfg.Folders))
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i, folder := range sysCfg.Folders {
		wg.Add(1)
		go func(idx int, fID, fLabel string) {
			defer wg.Done()

			var comp SyncthingCompletion
			endpoint := fmt.Sprintf("/rest/db/completion?folder=%s", fID)

			displayName := fLabel
			if displayName == "" {
				displayName = fID
			}
			if len(displayName) > 20 {
				displayName = displayName[:17] + "..."
			}

			var line string
			if err := getJson(endpoint, &comp); err != nil {
				line = fmt.Sprintf("  %-20s [ERR]\n", displayName)
			} else {
				line = fmt.Sprintf("  %-20s %5.1f%%\n", displayName, comp.Completion)
			}

			// Store in pre-allocated slice to preserve configuration ordering thread-safely
			mu.Lock()
			results[idx] = line
			mu.Unlock()
		}(i, folder.ID, folder.Label)
	}

	// Wait for all folder HTTP calls to finish concurrently
	wg.Wait()

	// Append the ordered results to the string builder
	for _, resLine := range results {
		sb.WriteString(resLine)
	}

	return sb.String()
}
