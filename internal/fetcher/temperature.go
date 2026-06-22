package fetcher

import (
	"bytes"
	"fmt"
	"os/exec"
)

func FetchTemps() string {
	cmd := exec.Command("sensors")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return fmt.Sprintf("Error:\n%v", err)
	}
	return out.String()
}
