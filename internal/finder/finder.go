package finder

import (
	"os/exec"
	"strings"
)

func ListFiles() ([]string, error) {
	cmd := exec.Command("fd", "--type", "f")
	out, err := cmd.Output()
	if err != nil {
		cmd = exec.Command("rg", "--files")
		out, err = cmd.Output()
		if err != nil {
			return nil, err
		}
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil, nil
	}
	return lines, nil
}
