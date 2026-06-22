package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

var (
	dockerCli *client.Client
	dockerMu  sync.Mutex
)
var statsRunningMu sync.Mutex // Add this global variable

// Helper to safely get or initialize the Moby Client session without panicking
func getDockerClient() (*client.Client, error) {
	dockerMu.Lock()
	defer dockerMu.Unlock()

	if dockerCli != nil {
		return dockerCli, nil
	}

	// Create a customized internal HTTP client that forces pure Unix dial rules
	customHttpClient := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				// Hard-code the dial target to skip any hostname resolution completely
				var d net.Dialer
				return d.DialContext(ctx, "unix", "/var/run/docker.sock")
			},
		},
	}

	// Inject our custom transport client directly into the Moby initialization flow
	cli, err := client.New(
		client.WithHTTPClient(customHttpClient),
		client.WithHost("unix:///var/run/docker.sock"),
	)
	if err != nil {
		return nil, err
	}

	// Fast 1-second check to make sure the socket responds immediately
	pingCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	if _, err := cli.Ping(pingCtx, client.PingOptions{}); err != nil {
		return nil, err
	}

	dockerCli = cli
	return dockerCli, nil
}

// Reset client on communication failure so the next loop tick triggers a fresh socket handshake
func resetDockerClient() {
	dockerMu.Lock()
	defer dockerMu.Unlock()
	if dockerCli != nil {
		dockerCli.Close()
		dockerCli = nil
	}
}

// Helper to format common error signatures cleanly inside the panes
func handleDockerError(err error) string {
	resetDockerClient()
	errStr := err.Error()
	if strings.Contains(errStr, "deadline exceeded") ||
		strings.Contains(errStr, "no such file") ||
		strings.Contains(errStr, "connection refused") {
		return "  [Waiting for Moby Daemon...]\n"
	}
	return fmt.Sprintf("  Docker Error:\n  %v\n", err)
}

// ─── Native Docker PS Fetcher ────────────────────────────────────────────────
func FetchDockerPs() string {
	cli, err := getDockerClient()
	if err != nil {
		return "  [Moby Init Waiting...]\n"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	result, err := cli.ContainerList(ctx, client.ContainerListOptions{All: false})
	if err != nil {
		return handleDockerError(err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("  %-20s %-15s\n", "NAME", "STATUS"))

	for _, c := range result.Items {
		name := "unknown"
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		if len(name) > 20 {
			name = name[:17] + "..."
		}

		sb.WriteString(fmt.Sprintf("  %-20s %-15s\n", name, c.Status))
	}

	return sb.String()
}

// ─── Native Docker Stats Fetcher ──────────────────────────────────────────────
func FetchDockerStats() string {
	cli, err := getDockerClient()
	if err != nil {
		return "  [Moby Init Waiting...]\n"
	}

	listCtx, listCancel := context.WithTimeout(context.Background(), 3*time.Second)
	result, err := cli.ContainerList(listCtx, client.ContainerListOptions{All: false})
	listCancel()
	if err != nil {
		return handleDockerError(err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("  %-25s %-7s %-15s\n", "NAME", "CPU%", "MEM USAGE"))

	// Pre-allocate a string slice to preserve the container order safely
	rowResults := make([]string, len(result.Items))
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i, c := range result.Items {
		wg.Add(1)

		// Spawn an independent, concurrent goroutine for each container cgroup sample
		go func(idx int, containerItem container.Summary) {
			defer wg.Done()

			name := "unknown"
			if len(containerItem.Names) > 0 {
				name = strings.TrimPrefix(containerItem.Names[0], "/")
			}
			if len(name) > 25 {
				name = name[:22] + "..."
			}

			// 3-second absolute boundary per container thread
			statsCtx, statsCancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer statsCancel()

			stats, err := cli.ContainerStats(statsCtx, containerItem.ID, client.ContainerStatsOptions{
				Stream:                false,
				IncludePreviousSample: true,
			})
			if err != nil {
				mu.Lock()
				rowResults[idx] = fmt.Sprintf("  %-25s %-7s %-15s\n", name, "--", "Timeout")
				mu.Unlock()
				return
			}
			defer stats.Body.Close()

			var v container.StatsResponse
			if err := json.NewDecoder(stats.Body).Decode(&v); err != nil {
				mu.Lock()
				rowResults[idx] = fmt.Sprintf("  %-25s %-7s %-15s\n", name, "--", "Parse Err")
				mu.Unlock()
				return
			}

			// Calculate CPU Percentage Natively
			cpuPercent := 0.0
			cpuDelta := float64(v.CPUStats.CPUUsage.TotalUsage) - float64(v.PreCPUStats.CPUUsage.TotalUsage)
			systemDelta := float64(v.CPUStats.SystemUsage) - float64(v.PreCPUStats.SystemUsage)
			onlineCPUs := float64(v.CPUStats.OnlineCPUs)
			if onlineCPUs == 0 {
				onlineCPUs = float64(len(v.CPUStats.CPUUsage.PercpuUsage))
			}
			if systemDelta > 0.0 && cpuDelta > 0.0 {
				cpuPercent = (cpuDelta / systemDelta) * onlineCPUs * 100.0
			}

			// Calculate Memory Metrics Natively
			memUsage := float64(v.MemoryStats.Usage)
			if cache, ok := v.MemoryStats.Stats["inactive_file"]; ok {
				memUsage -= float64(cache)
			}
			const mb float64 = 1024 * 1024

			// Save row format into thread-safe slice index
			mu.Lock()
			rowResults[idx] = fmt.Sprintf("  %-25s %-6.2f%% %6.1fMiB\n", name, cpuPercent, memUsage/mb)
			mu.Unlock()
		}(i, c)
	}

	// Block until all container cgroup sampling threads return completely
	wg.Wait()

	// Assemble the final string block cleanly
	for _, row := range rowResults {
		sb.WriteString(row)
	}

	return sb.String()
}
