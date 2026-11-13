package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const unitName = "px.service"

// Install registers and starts the systemd system service for the daemon.
func Install(exe, configPath string) error {
	if _, err := os.Stat("/run/systemd/system"); err != nil {
		return fmt.Errorf("systemd is not available, run px run with your init instead")
	}
	if os.Geteuid() != 0 {
		return fmt.Errorf("install requires root, run with sudo")
	}
	unit := unitText(exe, configPath)
	unitPath := filepath.Join("/etc/systemd/system", unitName)
	if err := os.WriteFile(unitPath, []byte(unit), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", unitPath, err)
	}
	for _, cmd := range [][]string{
		{"systemctl", "daemon-reload"},
		{"systemctl", "enable", "px"},
		{"systemctl", "restart", "px"},
	} {
		if out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput(); err != nil {
			return fmt.Errorf("%s failed: %v (%s)", strings.Join(cmd, " "), err, strings.TrimSpace(string(out)))
		}
	}
	time.Sleep(time.Second)
	out, _ := exec.Command("systemctl", "is-active", "px").Output()
	status := strings.TrimSpace(string(out))
	if status != "active" {
		return fmt.Errorf("service is %q after start, check journalctl -u px", status)
	}
	return nil
}

// Uninstall stops and removes the systemd system service.
func Uninstall() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("uninstall requires root, run with sudo")
	}
	exec.Command("systemctl", "stop", "px").Run()
	exec.Command("systemctl", "disable", "px").Run()
	os.Remove("/tmp/px.sock")
	unitPath := filepath.Join("/etc/systemd/system", unitName)
	if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", unitPath, err)
	}
	exec.Command("systemctl", "daemon-reload").Run()
	return nil
}

func unitText(exe, configPath string) string {
	return fmt.Sprintf(`[Unit]
Description=px mihomo controller daemon
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=%s run --config %s
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`, exe, configPath)
}
