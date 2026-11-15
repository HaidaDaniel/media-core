package cli

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"

	"github.com/HaidaDaniel/media-core/internal/config"
	"github.com/HaidaDaniel/media-core/internal/runtime"
	"github.com/HaidaDaniel/media-core/internal/store"
)


func init() {
	runtime.RegisterCommand("doctor", runDoctor)
}

const doctorUsage = "usage: media-core doctor [--state-dir <dir>]"

const (
	doctorPass = "pass"
	doctorWarn = "warn"
	doctorFail = "fail"
)

// doctorCheck is one named read-only probe.
type doctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// doctorReport is the diagnostic verdict. ok is false when any check
// fails; warnings never fail the report because the next normal open
// repairs the modes it covers.
type doctorReport struct {
	OK       bool          `json:"ok"`
	StateDir string        `json:"state_dir"`
	Checks   []doctorCheck `json:"checks"`
}

type doctorArgs struct {
	libCommand
}

func parseDoctorArgs(args []string) (*doctorArgs, error) {
	a := &doctorArgs{}
	rest, err := a.parseCommon(args)
	if err != nil {
		return nil, err
	}
	if len(rest) != 0 {
		return nil, fmt.Errorf("media-core doctor: unknown argument %q; %s", rest[0], doctorUsage)
	}
	return a, nil
}

// runDoctor diagnoses the state directory read-only. It creates nothing —
// no state directory, no lock file, no database sidecar — takes no
// maintenance lock, and opens the database through a read-only connection,
// so a running server keeps working and the files are never touched. The
// command exits 0 whenever the diagnosis completes; the verdict lives in
// the report. Exit 2 is argument misuse and 1 is a failed configuration
// resolution or an unexpected I/O fault of the diagnosis itself.
func runDoctor(args []string, out io.Writer) error {
	a, err := parseDoctorArgs(args)
	if err != nil {
		return &runtime.ExitCodeError{Exit: ExitUsage, Err: err}
	}
	cfg, err := config.Resolve(config.Options{StateDir: a.stateDir})
	if err != nil {
		return &runtime.ExitCodeError{Exit: ExitError, Err: err}
	}
	report := diagnoseState(cfg.StateDir)
	return writeEnvelope(out, report, nil)
}

// diagnoseState runs the probes. Every unexpected I/O fault is classified
// into the check that met it, so a completed diagnosis always yields a
// report rather than an opaque crash.
func diagnoseState(stateDir string) doctorReport {
	report := doctorReport{
		OK:       true,
		StateDir: stateDir,
		Checks:   []doctorCheck{{Name: "config", Status: doctorPass, Detail: "configuration resolved"}},
	}
	add := func(c doctorCheck) {
		report.Checks = append(report.Checks, c)
		if c.Status == doctorFail {
			report.OK = false
		}
	}

	dirOK, dirDetail := probeDir(stateDir)
	add(doctorCheck{Name: "state_dir", Status: passFail(dirOK), Detail: dirDetail})

	dbPath := filepath.Join(stateDir, store.DBFileName)
	dbOK, dbDetail := probeFile(dbPath, "database file")
	add(doctorCheck{Name: "database", Status: passFail(dbOK), Detail: dbDetail})

	schemaStatus, schemaDetail := probeSchema(dbPath, dbOK, dirOK)
	add(doctorCheck{Name: "schema", Status: schemaStatus, Detail: schemaDetail})

	permStatus, permDetail := probePermissions(stateDir, dbPath, dirOK)
	add(doctorCheck{Name: "permissions", Status: permStatus, Detail: permDetail})

	return report
}

// probeDir classifies the state path: a real directory is ok, a missing
// path and a symlinked or non-directory path are diagnosable failures.
func probeDir(stateDir string) (bool, string) {
	info, err := os.Lstat(stateDir)
	if errors.Is(err, os.ErrNotExist) {
		return false, "state directory is missing; run media-core init"
	}
	if err != nil {
		return false, "state directory cannot be read: " + err.Error()
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false, "state directory is a symlink; use a real directory"
	}
	if !info.IsDir() {
		return false, "state path is not a directory"
	}
	return true, "state directory exists"
}

// probeFile classifies the database file.
func probeFile(path string, label string) (bool, string) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, label + " is missing; run media-core init"
	}
	if err != nil {
		return false, label + " cannot be read: " + err.Error()
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false, label + " is a symlink"
	}
	if !info.Mode().IsRegular() {
		return false, label + " is not a regular file"
	}
	return true, "database file exists"
}

func passFail(ok bool) string {
	if ok {
		return doctorPass
	}
	return doctorFail
}

// probeSchema classifies the ledger state from a throwaway copy of the
// database file. Even a read-only open would materialize WAL sidecars into
// the state directory, so the copy — made in the system temp dir and
// deleted afterwards — is what gets opened; the state directory is never
// touched. The main file is always self-consistent (WAL checkpoints write
// it whole), and the schema only changes through the exclusive
// init/migrate maintenance, so reading it without the live WAL is sound.
func probeSchema(dbPath string, dbOK, dirOK bool) (string, string) {
	if !dirOK {
		return doctorFail, "not checked: state directory is missing"
	}
	if !dbOK {
		return doctorFail, "not checked: database file is missing"
	}
	tmp, err := os.CreateTemp("", "media-core-doctor-*")
	if err != nil {
		return doctorFail, "cannot stage a temporary copy: " + err.Error()
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	src, err := os.Open(dbPath)
	if err != nil {
		tmp.Close()
		return doctorFail, "cannot read the database: " + err.Error()
	}
	if _, err := io.Copy(tmp, src); err != nil {
		src.Close()
		tmp.Close()
		return doctorFail, "cannot stage a temporary copy: " + err.Error()
	}
	src.Close()
	if err := tmp.Close(); err != nil {
		return doctorFail, "cannot stage a temporary copy: " + err.Error()
	}
	u := &url.URL{Scheme: "file", Path: tmpPath}
	db, err := sql.Open("sqlite", u.String()+"?mode=ro")
	if err != nil {
		return doctorFail, "cannot open the copy: " + err.Error()
	}
	defer db.Close()
	err = store.CheckSchema(db)
	switch {
	case err == nil:
		return doctorPass, "schema is current for this build"
	case errors.Is(err, store.ErrSchemaMissing):
		return doctorFail, "schema is missing; run media-core init"
	case errors.Is(err, store.ErrSchemaPending):
		return doctorFail, "schema is behind this build; run media-core migrate"
	case errors.Is(err, store.ErrSchemaNewer):
		return doctorFail, "schema is newer than this build; use a newer version of media-core"
	default:
		return doctorFail, "schema check failed: " + err.Error()
	}
}

// probePermissions reports loose modes as warnings: the next normal open
// tightens them, so doctor never treats them as faults.
func probePermissions(stateDir, dbPath string, dirOK bool) (string, string) {
	if !dirOK {
		return doctorFail, "not checked: state directory is missing"
	}
	issues := []string{}
	wantDir, wantFile := os.FileMode(0o700), os.FileMode(0o600)
	check := func(path string, want os.FileMode, label string) {
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return
		}
		if perm := info.Mode().Perm(); perm != want {
			issues = append(issues, fmt.Sprintf("%s mode %o, want %o", label, perm, want))
		}
	}
	check(stateDir, wantDir, "state directory")
	check(dbPath, wantFile, "database file")
	check(filepath.Join(stateDir, store.MaintenanceLockFileName), wantFile, "maintenance lock")
	check(dbPath+"-wal", wantFile, "wal sidecar")
	check(dbPath+"-shm", wantFile, "shm sidecar")
	if len(issues) > 0 {
		detail := ""
		for i, s := range issues {
			if i > 0 {
				detail += "; "
			}
			detail += s
		}
		return doctorWarn, detail
	}
	return doctorPass, "file modes are restrictive"
}
