package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/hanymamdouh82/monox/internal/config"
)

// Native struct map for smartctl -json telemetry output
type SmartReport struct {
	SmartStatus struct {
		Passed bool `json:"passed"`
	} `json:"smart_status"`
	Temperature struct {
		Current int `json:"current"`
	} `json:"temperature"`
	PowerOnTime struct {
		Hours int `json:"hours"`
	} `json:"power_on_time"`
	AtaSmartAttributes struct {
		Table []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
			Raw  struct {
				Value int `json:"value"`
			} `json:"raw"`
		} `json:"table"`
	} `json:"ata_smart_attributes"`
}

func FetchDiskUsage() string {
	mounts := config.AppConfig.Disk.Mounts
	var sb strings.Builder

	// Header: 15s (Mount) | 11s (Total) | 11s (Used) | 11s (Avail) | 6s (Use%)
	sb.WriteString(fmt.Sprintf("  %-15s %11s %11s %11s %6s\n", "MOUNT", "TOTAL", "USED", "AVAIL", "USE%"))

	const gb float64 = 1024 * 1024 * 1024

	for _, path := range mounts {
		var stat syscall.Statfs_t
		if err := syscall.Statfs(path, &stat); err != nil {
			displayName := path
			if len(displayName) > 15 {
				displayName = displayName[:12] + "..."
			}
			sb.WriteString(fmt.Sprintf("  %-15s %s\n", displayName, "[Statfs Error]"))
			continue
		}

		totalBytes := stat.Blocks * uint64(stat.Bsize)
		freeBytes := stat.Bfree * uint64(stat.Bsize)
		availBytes := stat.Bavail * uint64(stat.Bsize)
		usedBytes := totalBytes - freeBytes

		var usePercent float64
		if totalBytes > 0 {
			usePercent = (float64(usedBytes) / float64(totalBytes)) * 100
		}

		displayName := path
		if len(displayName) > 15 {
			displayName = displayName[:12] + "..."
		}

		// Helper strings to encapsulate numbers with units before padding
		totalStr := fmt.Sprintf("%.1fGiB", float64(totalBytes)/gb)
		usedStr := fmt.Sprintf("%.1fGiB", float64(usedBytes)/gb)
		availStr := fmt.Sprintf("%.1fGiB", float64(availBytes)/gb)
		pctStr := fmt.Sprintf("%.1f%%", usePercent)

		// Data Row matches Header spacing exactly: 15s | 11s | 11s | 11s | 6s
		sb.WriteString(fmt.Sprintf(
			"  %-15s %11s %11s %11s %6s\n",
			displayName,
			totalStr,
			usedStr,
			availStr,
			pctStr,
		))
	}

	return sb.String()
}

func FetchSmart() string {
	drives := config.AppConfig.Smart.Drives
	var sb strings.Builder

	// Header alignment: 10s (Drive) | 10s (Health) | 8s (Temp) | 12s (PowerOn) | 14s (Realloc) | 10s (Pending)
	sb.WriteString(fmt.Sprintf("  %-10s %-15s %8s %12s %14s %10s\n", "DRIVE", "HEALTH", "TEMP", "POWER ON", "REALLOCATED", "PENDING"))

	rowResults := make([]string, len(drives))
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i, drive := range drives {
		wg.Add(1)

		go func(idx int, diskName string) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			cmd := exec.CommandContext(ctx, "sudo", "-n", "smartctl", "-a", "-json", "/dev/"+diskName)
			out, err := cmd.Output()

			displayName := "/dev/" + diskName
			if len(displayName) > 10 {
				displayName = displayName[:10]
			}

			if err != nil {
				var errMsg string
				if ctx.Err() == context.DeadlineExceeded {
					errMsg = "Timeout"
				} else if len(out) == 0 {
					errMsg = "Access Denied"
				} else {
					errMsg = "Cmd Error"
				}

				mu.Lock()
				rowResults[idx] = fmt.Sprintf("  %-10s %-15s %8s %12s %14s %10s\n", displayName, errMsg, "--", "--", "--", "--")
				mu.Unlock()
				return
			}

			var report SmartReport
			if err := json.Unmarshal(out, &report); err != nil {
				mu.Lock()
				rowResults[idx] = fmt.Sprintf("  %-10s %-10s %8s %12s %14s %10s\n", displayName, "Parse Err", "--", "--", "--", "--")
				mu.Unlock()
				return
			}

			statusStr := "PASSED"
			if !report.SmartStatus.Passed {
				statusStr = "FAILED"
			}

			var reallocated, pending int
			for _, attr := range report.AtaSmartAttributes.Table {
				switch attr.ID {
				case 5:
					reallocated = attr.Raw.Value
				case 197:
					pending = attr.Raw.Value
				}
			}

			// Pre-format numeric variables to bundle their strings/units before computing column padding
			tempStr := fmt.Sprintf("%d°C", report.Temperature.Current)
			powerStr := fmt.Sprintf("%d hrs", report.PowerOnTime.Hours)
			reallocStr := fmt.Sprintf("%d", reallocated)
			pendingStr := fmt.Sprintf("%d", pending)

			// Construct the aligned row using identical width values to the header layout map
			row := fmt.Sprintf(
				"  %-10s %-15s %8s %12s %14s %10s\n",
				displayName,
				statusStr,
				tempStr,
				powerStr,
				reallocStr,
				pendingStr,
			)

			mu.Lock()
			rowResults[idx] = row
			mu.Unlock()
		}(i, drive)
	}

	// Block until all disk telemetry shell lookups complete in parallel
	wg.Wait()

	// Assemble final screen output string buffer
	for _, row := range rowResults {
		sb.WriteString(row)
	}

	return sb.String()
}
