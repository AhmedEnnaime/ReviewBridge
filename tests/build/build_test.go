package build_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func moduleRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		t.Fatalf("go env GOMOD: %v", err)
	}
	return filepath.Dir(strings.TrimSpace(string(out)))
}

func buildFor(t *testing.T, goos, goarch string) {
	t.Helper()
	root := moduleRoot(t)
	out := filepath.Join(t.TempDir(), "reviewbridge-"+goos+"-"+goarch)
	cmd := exec.Command("go", "build", "-o", out, "./cmd/reviewbridge")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOOS="+goos, "GOARCH="+goarch, "CGO_ENABLED=0")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build %s/%s failed: %v\n%s", goos, goarch, err, output)
	}
}

func TestBuildCompilesDarwinAMD64(t *testing.T) {
	buildFor(t, "darwin", "amd64")
}

func TestBuildCompilesDarwinARM64(t *testing.T) {
	buildFor(t, "darwin", "arm64")
}

func TestBuildCompilesLinuxAMD64(t *testing.T) {
	buildFor(t, "linux", "amd64")
}

func TestNoCGODependencies(t *testing.T) {
	root := moduleRoot(t)
	out := filepath.Join(t.TempDir(), "reviewbridge-nocgo")
	cmd := exec.Command("go", "build", "-o", out, "./cmd/reviewbridge")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("CGO_ENABLED=0 build failed: %v\n%s", err, output)
	}
}

func TestDockerBuildSucceeds(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}
	root := moduleRoot(t)
	cmd := exec.Command("docker", "build", "-f", "Dockerfile.build", "--no-cache", "-t", "reviewbridge-test-build", ".")
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("docker build failed: %v\n%s", err, output)
	}
}
