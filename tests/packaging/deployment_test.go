package packaging

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func unitText(t *testing.T, root, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, "deploy", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(raw)
}

func requireUnitLines(t *testing.T, name, text string, lines ...string) {
	t.Helper()
	for _, line := range lines {
		if !strings.Contains(text, line) {
			t.Fatalf("%s is missing %q", name, line)
		}
	}
}

func TestDeploymentUnits(t *testing.T) {
	root := repoRoot(t)
	service := unitText(t, root, "media-core.service")
	backup := unitText(t, root, "media-core-backup.service")
	timer := unitText(t, root, "media-core-backup.timer")

	requireUnitLines(t, "media-core.service", service,
		"User=media-core", "Group=media-core", "WorkingDirectory=/opt/media-core",
		"ExecStart=/opt/media-core/media-core serve",
		"NoNewPrivileges=true", "ProtectSystem=strict", "ProtectHome=true",
		"PrivateTmp=true", "ReadWritePaths=/var/lib/media-core", "UMask=0077")
	requireUnitLines(t, "media-core-backup.service", backup,
		"Environment=MEDIA_CORE_BIN=/opt/media-core/media-core",
		"User=media-core", "Group=media-core", "WorkingDirectory=/opt/media-core",
		"ExecStart=/opt/media-core/scripts/backup-smoke.sh /var/lib/media-core /var/backups/media-core",
		"Type=oneshot", "NoNewPrivileges=true", "ProtectSystem=strict", "ProtectHome=true",
		"PrivateTmp=true", "ReadWritePaths=/var/lib/media-core /var/backups/media-core", "UMask=0077")
	requireUnitLines(t, "media-core-backup.timer", timer,
		"OnCalendar=daily", "Persistent=true", "Unit=media-core-backup.service",
		"WantedBy=timers.target")

	for name, text := range map[string]string{"service": service, "backup": backup, "timer": timer} {
		for _, secret := range []string{"SECRET=", "PASSWORD=", "TOKEN=", "owner_secret"} {
			if strings.Contains(strings.ToLower(text), strings.ToLower(secret)) {
				t.Fatalf("%s contains a secret-like value %q", name, secret)
			}
		}
	}

	scriptPath := filepath.Join(root, "scripts", "backup-smoke.sh")
	info, err := os.Stat(scriptPath)
	if err != nil {
		t.Fatalf("stat backup script: %v", err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatal("backup script is not executable")
	}
	for _, dir := range []string{"deploy", "docs/engineering"} {
		if stat, err := os.Stat(filepath.Join(root, dir)); err != nil || !stat.IsDir() {
			t.Fatalf("required directory %s is absent: %v", dir, err)
		}
	}

	if analyzer, err := exec.LookPath("systemd-analyze"); err == nil {
		out, err := exec.Command(analyzer, "verify",
			filepath.Join(root, "deploy", "media-core.service"),
			filepath.Join(root, "deploy", "media-core-backup.service"),
			filepath.Join(root, "deploy", "media-core-backup.timer")).CombinedOutput()
		if err != nil {
			text := string(out)
			for _, line := range strings.Split(text, "\n") {
				if line == "" || strings.Contains(line, "is not executable: No such file or directory") {
					continue
				}
				t.Fatalf("systemd-analyze verify: %v (%s)", err, text)
			}
		}
	}

	if testing.Short() {
		t.Skip("skipping systemd-like integration smoke in short mode")
	}
	bin := filepath.Join(t.TempDir(), "media-core")
	build := exec.Command("go", "build", "-o", bin, "./cmd/media-core")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build media-core: %v (%s)", err, out)
	}
	state := filepath.Join(t.TempDir(), "state")
	dest := filepath.Join(t.TempDir(), "backups")
	for _, dir := range []string{state, dest} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	if out, err := exec.Command(bin, "init", "--state-dir", state).CombinedOutput(); err != nil {
		t.Fatalf("init scratch state: %v (%s)", err, out)
	}
	baseEnv := []string{}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "MEDIA_CORE_BIN=") {
			baseEnv = append(baseEnv, kv)
		}
	}
	runSmoke := func(value string) (string, error) {
		cmd := exec.Command("bash", scriptPath, state, dest)
		cmd.Dir = root
		cmd.Env = append(append([]string{}, baseEnv...), "MEDIA_CORE_BIN="+value)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	out, err := runSmoke(bin)
	if err != nil || !strings.Contains(out, "backup smoke ok") {
		t.Fatalf("systemd-like smoke: err = %v, out = %s", err, out)
	}
	if out, err := runSmoke("media-core"); err == nil || !strings.Contains(out, "must be an absolute path") {
		t.Fatalf("relative binary path must fail: err = %v, out = %s", err, out)
	}
	if out, err := runSmoke(filepath.Join(t.TempDir(), "missing")); err == nil || !strings.Contains(out, "missing or not executable") {
		t.Fatalf("missing binary must fail: err = %v, out = %s", err, out)
	}
}
