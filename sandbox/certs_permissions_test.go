package sandbox

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// The containers read the generated demo material as fixed non-root UIDs while
// a Linux bind mount passes the host file's owner and mode straight through, so
// these modes are what decides whether the stack starts at all. Two earlier
// attempts at this shipped broken: the first left everything owner-only, and
// the second repaired only freshly generated material while telling anyone with
// existing material there was nothing to do. Both passed every test that
// existed at the time, because nothing covered the script.
//
// The material is faked rather than generated so this stays a permissions test
// and does not spend seconds on a 4096-bit key. The generator's early exit only
// checks that the six files exist.
const (
	wantPublic = 0o644
	wantKey    = 0o600
	wantDir    = 0o755
)

func stageScript(t *testing.T) string {
	t.Helper()
	source, err := os.ReadFile("generate-demo-certs.sh")
	require.NoError(t, err)

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sandbox"), 0o755))
	script := filepath.Join(dir, "sandbox", "generate-demo-certs.sh")
	require.NoError(t, os.WriteFile(script, source, 0o755))
	return script
}

// stageStaleMaterial writes the six files the generator looks for, in the
// owner-only modes an older version of the script produced.
func stageStaleMaterial(t *testing.T, script string) string {
	t.Helper()
	out := filepath.Join(filepath.Dir(script), ".certs")
	require.NoError(t, os.MkdirAll(filepath.Join(out, "ca-only"), 0o700))
	for _, name := range []string{"ca.pem", "ca.key", "mock-dezi.pem", "mock-dezi.key", "dezi-signing.key"} {
		require.NoError(t, os.WriteFile(filepath.Join(out, name), []byte("stale\n"), 0o600))
	}
	require.NoError(t, os.WriteFile(filepath.Join(out, "ca-only", "gf-sandbox-demo-ca.pem"), []byte("stale\n"), 0o600))
	require.NoError(t, os.Chmod(out, 0o700))
	return out
}

func mode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	return info.Mode().Perm()
}

func requireContainerReadable(t *testing.T, out string) {
	t.Helper()
	require.Equal(t, os.FileMode(wantDir), mode(t, out), ".certs must be traversable by the container UID")
	require.Equal(t, os.FileMode(wantDir), mode(t, filepath.Join(out, "ca-only")))
	for _, name := range []string{"ca.pem", "mock-dezi.pem", "mock-dezi.key", "dezi-signing.key"} {
		require.Equal(t, os.FileMode(wantPublic), mode(t, filepath.Join(out, name)), "%s is mounted and must be readable", name)
	}
	require.Equal(t, os.FileMode(wantPublic), mode(t, filepath.Join(out, "ca-only", "gf-sandbox-demo-ca.pem")))
	require.Equal(t, os.FileMode(wantKey), mode(t, filepath.Join(out, "ca.key")), "the CA key is mounted nowhere and must stay owner-only")
}

func runScript(t *testing.T, script string) string {
	t.Helper()
	cmd := exec.Command("bash", script)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "generator failed: %s", output)
	return string(output)
}

func TestGeneratorRepairsStaleMaterialInPlace(t *testing.T) {
	script := stageScript(t)
	out := stageStaleMaterial(t, script)

	output := runScript(t, script)

	require.Contains(t, output, "already present", "the six files exist, so this must take the early exit")
	requireContainerReadable(t, out)
}

func TestGeneratorLeavesUnrelatedPEMsAlone(t *testing.T) {
	script := stageScript(t)
	out := stageStaleMaterial(t, script)
	unrelated := filepath.Join(out, "unexpected-private.pem")
	require.NoError(t, os.WriteFile(unrelated, []byte("someone else's key\n"), 0o600))

	runScript(t, script)

	require.Equal(t, os.FileMode(wantKey), mode(t, unrelated),
		"only the files this script generates may be widened, not whatever else sits in the directory")
	requireContainerReadable(t, out)
}

func TestGeneratorSurvivesADanglingSymlink(t *testing.T) {
	script := stageScript(t)
	out := stageStaleMaterial(t, script)
	require.NoError(t, os.Symlink(filepath.Join(out, "gone.pem"), filepath.Join(out, "stale.pem")))

	// A glob would hand this to chmod, which fails on a dangling symlink and
	// under `set -e` abandons the rest of the repair, leaving the directory
	// unsearchable and the outage in place.
	runScript(t, script)

	requireContainerReadable(t, out)
}
