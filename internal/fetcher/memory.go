package fetcher

import (
	"fmt"
	"syscall"
)

func FetchMemory() string {
	var si syscall.Sysinfo_t

	// Direct Linux kernel system call
	if err := syscall.Sysinfo(&si); err != nil {
		return fmt.Sprintf("Error executing sysinfo syscall:\n%v", err)
	}

	// si.Unit provides the memory block scale size factor (typically 1 byte on modern kernels,
	// but can vary, so we multiply explicitly to guarantee absolute byte parity).
	unit := uint64(si.Unit)

	memTotal := uint64(si.Totalram) * unit
	memFree := uint64(si.Freeram) * unit
	memBuffer := uint64(si.Bufferram) * unit

	// On modern Linux kernels, cached memory isn't explicitly exposed in Sysinfo_t.
	// For precise "Available Memory" matching `free -h`, /proc/meminfo is technically more accurate,
	// but standard host-calculation treats Used = Total - Free - Buffers
	memUsed := memTotal - memFree - memBuffer

	swapTotal := uint64(si.Totalswap) * unit
	swapFree := uint64(si.Freeswap) * unit
	swapUsed := swapTotal - swapFree

	// Conversion constant to gigabytes
	const gb float64 = 1024 * 1024 * 1024

	return fmt.Sprintf(
		"Mem Total: %7.1f GiB\nMem Used:  %7.1f GiB\nMem Free:  %7.1f GiB\nBuffers:   %7.1f GiB\n\nSwap Total:%7.1f GiB\nSwap Used: %7.1f GiB",
		float64(memTotal)/gb,
		float64(memUsed)/gb,
		float64(memFree)/gb,
		float64(memBuffer)/gb,
		float64(swapTotal)/gb,
		float64(swapUsed)/gb,
	)
}
