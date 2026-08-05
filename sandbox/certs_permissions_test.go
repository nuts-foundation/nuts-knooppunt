package sandbox

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

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
// and does not spend seconds on a 4096-bit key. The generator's early exit
// checks that eight files exist and that ca.pem is a CA certificate.
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

// stageStaleMaterial writes six of the eight files the generator's early
// exit looks for, in the owner-only modes an older version of the script
// produced. Callers that need the exit to fire supply the remaining two
// plataan files themselves.
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

// stageRoot overwrites ca.pem with a real, self-signed root certificate,
// setting Basic Constraints to CA:TRUE only when isCA is true. The CA/non-CA
// distinction is the one axis the paired tests using this exist to exercise;
// building both fixtures from one function keeps that distinction visible at
// the call site instead of buried in duplicated certificate boilerplate.
func stageRoot(t *testing.T, out string, isCA bool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	cn := "test non-CA root"
	if isCA {
		cn = "test CA"
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
	}
	if isCA {
		template.IsCA = true
		template.BasicConstraintsValid = true
		template.KeyUsage = x509.KeyUsageCertSign
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(out, "ca.pem"),
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600))
}

// stageValidCARoot overwrites ca.pem with a real, minimal self-signed CA
// certificate. stageStaleMaterial's placeholder content ("stale\n") is not a
// certificate at all, which this task's root_is_ca check correctly treats as
// not a CA; a test that means to exercise the early exit itself, rather than
// the regeneration it now falls back to, needs a root openssl also accepts.
func stageValidCARoot(t *testing.T, out string) {
	t.Helper()
	stageRoot(t, out, true)
}

// stageValidNonCARoot overwrites ca.pem with a real, self-signed certificate
// that is not a CA: openssl parses it without error, so only root_is_ca's
// CA:TRUE check, not mere parseability, can tell it apart from a usable root.
// This is also what material generated before this task leaves behind: a
// parseable, self-signed root with no Basic Constraints.
func stageValidNonCARoot(t *testing.T, out string) {
	t.Helper()
	stageRoot(t, out, false)
}

func TestGeneratorRepairsStaleMaterialInPlace(t *testing.T) {
	script := stageScript(t)
	out := stageStaleMaterial(t, script)
	// root_is_ca now guards the early exit, and the exit also now checks for
	// this task's two new files, so the fixture needs both or it falls
	// through to a full regeneration instead of the repair this test covers.
	stageValidCARoot(t, out)
	for _, name := range []string{"plataan-uzi-chain.pem", "plataan-uzi.key"} {
		require.NoError(t, os.WriteFile(filepath.Join(out, name), []byte("stale\n"), 0o600))
	}

	output := runScript(t, script)

	require.Contains(t, output, "already present", "the eight files exist and ca.pem is a CA, so this must take the early exit")
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

func TestGeneratorProducesACARootAndUZILeaf(t *testing.T) {
	script := stageScript(t)
	out := filepath.Join(filepath.Dir(script), ".certs")

	runScript(t, script)

	// did:x509 fingerprints the root and rejects it unless it is a CA, so a
	// root without Basic Constraints cannot anchor the chain at all.
	root := parseCert(t, filepath.Join(out, "ca.pem"))
	require.True(t, root.IsCA, "the demo root must be a CA certificate")
	require.True(t, root.BasicConstraintsValid, "Basic Constraints must be present, not merely implied")

	leaf := parseCert(t, filepath.Join(out, "plataan-uzi.pem"))
	require.False(t, leaf.IsCA, "the leaf must not be a CA")
	require.Len(t, leaf.Subject.Organization, 1)
	require.Equal(t, "Ziekenhuis De Plataan", leaf.Subject.Organization[0])

	// The descriptor extracts the URA from this SAN with a single capture
	// group, so the exact shape is load-bearing, not cosmetic. A substring
	// check would still pass with an unwanted prefix or suffix around the
	// expected value, e.g. a leading "urn:X" or a trailing "-x", even though
	// config/policy/policy.json's "^[0-9.]+-\d+-\d+-S-(\d+)-00\.000-\d+$"
	// pattern rejects both, so the otherName must equal the expected value
	// exactly.
	require.Equal(t, "2.16.528.1.1007.99.2110-1-0-S-00000010-00.000-0", sanOtherName(t, leaf),
		"the URA must sit in the SAN otherName where the presentation definition looks for it")

	// Task 6 feeds this chain to the did:x509 resolver, which requires a
	// chain sorted leaf to root; a silent reversal here would otherwise only
	// surface there as a confusing failure.
	chain := parseCertChain(t, filepath.Join(out, "plataan-uzi-chain.pem"))
	require.Len(t, chain, 2, "the chain must contain exactly the leaf and the root")
	require.True(t, chain[0].Equal(leaf), "the chain's first certificate must be the leaf")
	require.True(t, chain[1].Equal(root), "the chain's second certificate must be the root")

	requireContainerReadable(t, out)
	for _, name := range []string{"plataan-uzi.pem", "plataan-uzi-chain.pem", "plataan-uzi.key"} {
		require.Equal(t, os.FileMode(wantPublic), mode(t, filepath.Join(out, name)),
			"%s is mounted into the bootstrap and must be readable", name)
	}
}

func TestGeneratorReplacesANonCARoot(t *testing.T) {
	script := stageScript(t)
	out := stageStaleMaterial(t, script)
	// All eight files the early exit now looks for must be present, or a
	// missing plataan file forces the fallthrough to regeneration on its own
	// and the assertion below no longer isolates the root_is_ca guard.
	// ca.pem is replaced with a real, self-signed, non-CA certificate:
	// stageStaleMaterial's "stale\n" placeholder is not a certificate at all,
	// so it cannot distinguish root_is_ca's CA:TRUE check from a check that
	// merely confirms ca.pem parses.
	stageValidNonCARoot(t, out)
	for _, name := range []string{"plataan-uzi-chain.pem", "plataan-uzi.key"} {
		require.NoError(t, os.WriteFile(filepath.Join(out, name), []byte("stale\n"), 0o600))
	}

	runScript(t, script)

	root := parseCert(t, filepath.Join(out, "ca.pem"))
	require.True(t, root.IsCA, "a root that is not a usable CA must be regenerated, not accepted")
}

func parseCert(t *testing.T, path string) *x509.Certificate {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	block, _ := pem.Decode(raw)
	require.NotNil(t, block, "%s is not PEM encoded", path)
	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	return cert
}

// parseCertChain parses every PEM-encoded certificate in path, in file order.
func parseCertChain(t *testing.T, path string) []*x509.Certificate {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var certs []*x509.Certificate
	for {
		var block *pem.Block
		block, raw = pem.Decode(raw)
		if block == nil {
			break
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		require.NoError(t, err)
		certs = append(certs, cert)
	}
	return certs
}

// sanOtherName decodes the certificate's SAN extension and returns the
// UTF8String value of its otherName entry. Go's x509 parser drops otherName
// entries it does not recognise, so the extension is decoded directly with
// encoding/asn1 rather than through cert.Extensions helpers.
func sanOtherName(t *testing.T, cert *x509.Certificate) string {
	t.Helper()
	for _, ext := range cert.Extensions {
		if ext.Id.String() != "2.5.29.17" {
			continue
		}

		// SubjectAltName ::= GeneralNames ::= SEQUENCE OF GeneralName, and
		// GeneralName is a CHOICE, so each entry can carry a different tag;
		// RawValue captures whichever one is on the wire without needing to
		// know it ahead of time.
		var names []asn1.RawValue
		_, err := asn1.Unmarshal(ext.Value, &names)
		require.NoError(t, err)

		for _, name := range names {
			// otherName is GeneralName's "[0] IMPLICIT OtherName"; every
			// other GeneralName choice carries a different tag.
			if name.Class != asn1.ClassContextSpecific || name.Tag != 0 {
				continue
			}

			// OtherName ::= SEQUENCE { type-id OID, value [0] EXPLICIT ANY }.
			var other struct {
				TypeID asn1.ObjectIdentifier
				Value  asn1.RawValue `asn1:"tag:0"`
			}
			_, err := asn1.UnmarshalWithParams(name.FullBytes, &other, "tag:0")
			require.NoError(t, err)

			var value string
			_, err = asn1.Unmarshal(other.Value.Bytes, &value)
			require.NoError(t, err)
			return value
		}
		t.Fatalf("subjectAltName extension has no otherName entry")
	}
	t.Fatalf("certificate has no subjectAltName extension")
	return ""
}
