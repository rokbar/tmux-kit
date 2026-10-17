package main

import (
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
)

// buildWtui builds wtui with extra `go build` flags and returns the binary's path.
func buildWtui(t *testing.T, flags ...string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "wtui")
	args := append([]string{"build", "-o", bin}, flags...)
	if out, err := exec.Command("go", append(args, ".")...).CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

// versionOf runs `<bin> --version` and returns what it printed.
func versionOf(t *testing.T, bin string) string {
	t.Helper()
	out, err := exec.Command(bin, "--version").Output()
	if err != nil {
		t.Fatalf("%s --version: %v", bin, err)
	}
	return strings.TrimSpace(string(out))
}

func TestVersionIsTheOneBakedInAtBuildTime(t *testing.T) {
	bin := buildWtui(t, "-ldflags", "-X main.version=v1.2.3")
	if got, want := versionOf(t, bin), "wtui v1.2.3"; got != want {
		t.Errorf("--version = %q, want %q", got, want)
	}
}

func TestPlainLocalBuildReportsDev(t *testing.T) {
	bin := buildWtui(t)
	if got, want := versionOf(t, bin), "wtui dev"; got != want {
		t.Errorf("--version = %q, want %q", got, want)
	}
}

// `go install github.com/rokbar/tmux-kit/wtui@latest` bakes nothing in, but Go
// records the module version it built. Faking that build needs a module proxy,
// so this drives the version rule with the build info such a build carries.
func TestGoInstallBuildFallsBackToTheModuleVersion(t *testing.T) {
	info := &debug.BuildInfo{Main: debug.Module{
		Path:    "github.com/rokbar/tmux-kit/wtui",
		Version: "v0.0.0-20261016184854-9b83ce9fc023",
	}}
	if got, want := resolveVersion("dev", info), "v0.0.0-20261016184854-9b83ce9fc023"; got != want {
		t.Errorf("resolveVersion = %q, want %q", got, want)
	}
}
