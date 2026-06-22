package fetcher

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type ProcessStats struct {
	PID    int
	Name   string
	CPU    float64
	Memory float64
}

// Internal structures to track cpu time deltas across updates
type procCpuTime struct {
	utime int64
	stime int64
}

var (
	lastSystemTotal int64
	lastProcTimes   = make(map[int]procCpuTime)
	lastUpdateTime  time.Time
	totalMemKb      float64
)

// Helper to get total system memory natively once
func getTotalMemory() float64 {
	if totalMemKb > 0 {
		return totalMemKb
	}
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 1.0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				val, _ := strconv.ParseFloat(fields[1], 64)
				totalMemKb = val
				return totalMemKb
			}
		}
	}
	return 1.0
}

// Helper to get total global CPU jiffies from /proc/stat
func getSystemTotalCpu() int64 {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 {
		return 0
	}
	fields := strings.Fields(lines[0]) // cpu line
	if len(fields) < 5 {
		return 0
	}
	var total int64
	for i := 1; i < len(fields); i++ {
		val, _ := strconv.ParseInt(fields[i], 10, 64)
		total += val
	}
	return total
}

func FetchSystemLoad() string {
	var sb strings.Builder

	// 1. Load Averages
	if data, err := os.ReadFile("/proc/loadavg"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) >= 3 {
			sb.WriteString("-- Load Average --\n")
			sb.WriteString(fmt.Sprintf("  1 min: %s, 5 min: %s, 15 min: %s\n\n", fields[0], fields[1], fields[2]))
		}
	}

	// 2. System Uptime
	if data, err := os.ReadFile("/proc/uptime"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) > 0 {
			if upSecs, err := strconv.ParseFloat(fields[0], 64); err == nil {
				secs := int64(upSecs)
				days := secs / (24 * 3600)
				hours := (secs % (24 * 3600)) / 3600
				mins := (secs % 3600) / 60
				sb.WriteString("-- Uptime --\n")
				sb.WriteString(fmt.Sprintf("  %dd %dh %dm\n\n", days, hours, mins))
			}
		}
	}

	sb.WriteString("-- Top Processes (CPU) --\n")
	sb.WriteString(fmt.Sprintf("  %-6s %-15s %-6s %-6s\n", "PID", "COMMAND", "CPU%", "MEM%"))

	// 3. Delta Setup
	currentSystemTotal := getSystemTotalCpu()
	systemDelta := currentSystemTotal - lastSystemTotal
	lastSystemTotal = currentSystemTotal

	memTotal := getTotalMemory()
	currentProcTimes := make(map[int]procCpuTime)

	files, err := os.ReadDir("/proc")
	if err != nil {
		return sb.String()
	}

	var plist []ProcessStats

	for _, f := range files {
		pid, err := strconv.Atoi(f.Name())
		if err != nil {
			continue // Skip non-process entries
		}

		// Read /proc/[pid]/stat
		statData, err := os.ReadFile(filepath.Join("/proc", f.Name(), "stat"))
		if err != nil {
			continue
		}

		statStr := string(statData)
		startIdx := strings.Index(statStr, "(")
		endIdx := strings.LastIndex(statStr, ")")
		if startIdx == -1 || endIdx == -1 || endIdx <= startIdx {
			continue
		}

		name := statStr[startIdx+1 : endIdx]
		tail := strings.Fields(statStr[endIdx+1:])
		if len(tail) < 22 {
			continue
		}

		// Parse CPU times (fields 14 and 15 in the tail slice are utime and stime)
		utime, _ := strconv.ParseInt(tail[11], 10, 64)
		stime, _ := strconv.ParseInt(tail[12], 10, 64)
		currentProcTimes[pid] = procCpuTime{utime: utime, stime: stime}

		// Calculate CPU Percentage via system delta context tracking
		var cpuPercent float64
		if oldTime, exists := lastProcTimes[pid]; exists && systemDelta > 0 {
			procDelta := (utime + stime) - (oldTime.utime + oldTime.stime)
			cpuPercent = (float64(procDelta) / float64(systemDelta)) * 100.0
		}

		// Parse Memory RSS (field 24 in raw /proc/[pid]/stat, index 21 in tail)
		rssPages, _ := strconv.ParseFloat(tail[21], 64)
		memRssKb := rssPages * 4.0 // 4KB page size on standard architectures
		memPercent := (memRssKb / memTotal) * 100.0

		plist = append(plist, ProcessStats{
			PID:    pid,
			Name:   name,
			CPU:    cpuPercent,
			Memory: memPercent,
		})
	}

	// Update the global state maps cleanly for the next execution frame tick
	lastProcTimes = currentProcTimes

	// 4. Sort Processes by CPU usage descending
	sort.Slice(plist, func(i, j int) bool {
		return plist[i].CPU > plist[j].CPU
	})

	limit := 8
	if len(plist) < limit {
		limit = len(plist)
	}

	for i := 0; i < limit; i++ {
		proc := plist[i]
		cmdName := proc.Name
		if len(cmdName) > 15 {
			cmdName = cmdName[:12] + "..."
		}
		sb.WriteString(fmt.Sprintf("  %-6d %-15s %-6.1f %-6.1f\n", proc.PID, cmdName, proc.CPU, proc.Memory))
	}

	return sb.String()
}
