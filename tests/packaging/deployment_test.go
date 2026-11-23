package packaging

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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

const trustedLanDropIn = "media-core.service.d/trusted-lan.conf"

func TestDeploymentUnits(t *testing.T) {
	root := repoRoot(t)
	service := unitText(t, root, "media-core.service")
	backup := unitText(t, root, "media-core-backup.service")
	timer := unitText(t, root, "media-core-backup.timer")
	dropIn := unitText(t, root, trustedLanDropIn)

	requireUnitLines(t, "media-core.service", service,
		"User=media-core", "Group=media-core", "WorkingDirectory=/opt/media-core",
		"ExecStart=/opt/media-core/media-core serve",
		"NoNewPrivileges=true", "ProtectSystem=strict", "ProtectHome=true",
		"PrivateTmp=true", "ReadWritePaths=/var/lib/media-core", "UMask=0077")
	requireUnitLines(t, "media-core-backup.service", backup,
		"Environment=MEDIA_CORE_BIN=/opt/media-core/media-core",
		"User=media-core", "Group=media-core", "WorkingDirectory=/opt/media-core",
		"ExecStart=/opt/media-core/scripts/backup-smoke.sh /var/lib/media-core /var/backups/media-core media-core-auto-",
		"Type=oneshot", "NoNewPrivileges=true", "ProtectSystem=strict", "ProtectHome=true",
		"PrivateTmp=true", "ReadWritePaths=/var/lib/media-core /var/backups/media-core", "UMask=0077")
	requireUnitLines(t, "media-core-backup.timer", timer,
		"OnCalendar=daily", "Persistent=true", "Unit=media-core-backup.service",
		"WantedBy=timers.target")

	// The default unit stays loopback-only; Trusted LAN is a verified
	// optional drop-in that the operator installs deliberately.
	requireUnitLines(t, "media-core.service", service,
		"ExecStart=/opt/media-core/media-core serve --state-dir /var/lib/media-core --listen 127.0.0.1:8787")
	requireUnitLines(t, trustedLanDropIn, dropIn,
		"Wants=network-online.target", "After=network-online.target",
		"ExecStart=",
		"ExecStart=/opt/media-core/media-core serve --state-dir /var/lib/media-core --trusted-lan --listen 192.168.1.100:8787 --lan-host media.home.arpa")
	if strings.Count(dropIn, "ExecStart=") != 2 {
		t.Fatalf("%s must clear the default ExecStart before setting the LAN one", trustedLanDropIn)
	}

	for name, text := range map[string]string{"service": service, "backup": backup, "timer": timer, "trusted LAN drop-in": dropIn} {
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

	units := map[string]string{
		"media-core.service":        service,
		"media-core-backup.service": backup,
		"media-core-backup.timer":   timer,
		trustedLanDropIn:            dropIn,
	}
	if out, err := verifyUnitsInSandbox(t, units); err != nil {
		t.Fatalf("systemd-analyze verify of project units: %v (%s)", err, out)
	}
}

// sandboxUnitText rewrites production paths to files inside a temporary root
// so systemd-analyze can check our unit syntax without installed binaries.
func sandboxUnitText(text, base string) string {
	replacer := strings.NewReplacer(
		"/opt/media-core/scripts/backup-smoke.sh", filepath.Join(base, "opt", "media-core", "scripts", "backup-smoke.sh"),
		"/opt/media-core/media-core", filepath.Join(base, "opt", "media-core", "media-core"),
		"WorkingDirectory=/opt/media-core", "WorkingDirectory="+filepath.Join(base, "opt", "media-core"),
		"/var/lib/media-core", filepath.Join(base, "var", "lib", "media-core"),
		"/var/backups/media-core", filepath.Join(base, "var", "backups", "media-core"),
	)
	return replacer.Replace(text)
}

// verifyUnitsInSandbox runs systemd-analyze on temporary copies of the unit
// files with executable stubs at every referenced path. No system directory
// is touched and no host unit warnings can affect the result.
func verifyUnitsInSandbox(t *testing.T, units map[string]string) (string, error) {
	t.Helper()
	analyzer, err := exec.LookPath("systemd-analyze")
	if err != nil {
		t.Fatalf("systemd-analyze is required for unit verification: %v", err)
	}
	base := t.TempDir()
	for _, dir := range []string{"opt/media-core/scripts", "var/lib/media-core", "var/backups/media-core", "units"} {
		if err := os.MkdirAll(filepath.Join(base, filepath.FromSlash(dir)), 0o700); err != nil {
			t.Fatalf("mkdir sandbox %s: %v", dir, err)
		}
	}
	for _, path := range []string{"opt/media-core/media-core", "opt/media-core/scripts/backup-smoke.sh"} {
		if err := os.WriteFile(filepath.Join(base, filepath.FromSlash(path)), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatalf("write stub %s: %v", path, err)
		}
	}
	paths := []string{}
	for name, text := range units {
		path := filepath.Join(base, "units", filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("mkdir unit dir for %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(sandboxUnitText(text, base)), 0o600); err != nil {
			t.Fatalf("write unit copy %s: %v", name, err)
		}
		if filepath.Ext(name) == ".service" || filepath.Ext(name) == ".timer" {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	args := append([]string{"verify", "--recursive-errors=no"}, paths...)
	out, err := exec.Command(analyzer, args...).CombinedOutput()
	return string(out), err
}

func TestDeploymentUnitsDetectCorruption(t *testing.T) {
	root := repoRoot(t)
	base := map[string]string{
		"media-core.service":        unitText(t, root, "media-core.service"),
		"media-core-backup.service": unitText(t, root, "media-core-backup.service"),
		"media-core-backup.timer":   unitText(t, root, "media-core-backup.timer"),
		trustedLanDropIn:            unitText(t, root, trustedLanDropIn),
	}
	cases := []struct {
		name   string
		file   string
		mutate func(string) string
	}{
		{"unknown section", "media-core.service", func(text string) string {
			return strings.Replace(text, "[Service]", "[Bogus]", 1)
		}},
		{"empty ExecStart", "media-core.service", func(text string) string {
			lines := strings.Split(text, "\n")
			for i, line := range lines {
				if strings.HasPrefix(line, "ExecStart=") {
					lines[i] = "ExecStart="
				}
			}
			return strings.Join(lines, "\n")
		}},
		{"unknown key", "media-core.service", func(text string) string {
			return text + "\nThisKeyDoesNotExist=yes\n"
		}},
		{"broken timer", "media-core-backup.timer", func(text string) string {
			return strings.Replace(text, "[Timer]", "[Broken]", 1)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			units := map[string]string{}
			for name, text := range base {
				units[name] = text
			}
			units[tc.file] = tc.mutate(units[tc.file])
			if tc.name == "empty ExecStart" {
				// A drop-in legitimately replaces ExecStart, so the base
				// unit must be verified alone for this counterexample.
				delete(units, trustedLanDropIn)
			}
			out, err := verifyUnitsInSandbox(t, units)
			if err == nil {
				t.Fatalf("corrupted %s passed verification (%s)", tc.file, out)
			}
		})
	}
}
