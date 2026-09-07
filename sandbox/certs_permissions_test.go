package sandbox

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/json"
	"encoding/pem"
	"io/fs"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nuts-foundation/nuts-node/vcr/pe"
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
// Material is faked wherever a test is only about modes or about the
// wrong-kind guard, and generated for real wherever the early exit is in play.
// That exit no longer merely counts files: it checks that the material hangs
// together, so no placeholder can reach it. The tests that need a healthy set
// share one real run, because a 4096-bit CA key costs a second or more.
const (
	wantPublic = 0o644
	wantKey    = 0o600
	wantDir    = 0o755
)

// The URA the generator puts in the leaf's SAN otherName, which is where the
// bgz descriptor looks for it. Named once here because several tests assert on
// it and the generator has to keep producing exactly this.
const plataanOtherName = "2.16.528.1.1007.99.2110-1-0-S-00000010-00.000-0"

// The template is staged alongside the script because the script reads it: it
// renders the presentation definition with this run's CA fingerprint pinned in.
// A run without it is a run that cannot produce a working sandbox, so every
// test here gets the real one rather than a stand-in.
func stageScript(t *testing.T) string {
	t.Helper()
	source, err := os.ReadFile("generate-demo-certs.sh")
	require.NoError(t, err)
	template, err := os.ReadFile(bgzPolicyTemplate)
	require.NoError(t, err)

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sandbox", "policy"), 0o755))
	script := filepath.Join(dir, "sandbox", "generate-demo-certs.sh")
	require.NoError(t, os.WriteFile(script, source, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "sandbox", bgzPolicyTemplate), template, 0o644))
	return script
}

// stageStaleMaterial writes placeholders at six of the paths the generator
// writes, in the owner-only modes an older version of the script produced.
// None of it is certificate material, so the generator regenerates the set
// rather than accepting it: these tests are about what apply_modes and the
// wrong-kind guard do to an already populated directory, not about the early
// exit, which needs material that validates and gets it from healthyMaterial.
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
	// No compose service mounts this key: its only consumer is the one-shot
	// toolkit container in bootstrap-nuts.sh, whose image runs as root and can
	// therefore read an owner-only bind mount. The fixed-UID argument that
	// forces the two mock keys open does not reach it, and a ten-year signing
	// key readable by every account on the host is the cost of widening it
	// anyway.
	require.Equal(t, os.FileMode(wantKey), mode(t, filepath.Join(out, "plataan-uzi.key")),
		"the UZI key is read by a root container and must stay owner-only")
}

func runScript(t *testing.T, script string) string {
	t.Helper()
	cmd := exec.Command("bash", script)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "generator failed: %s", output)
	return string(output)
}

// runScriptExpectingRefusal is runScript's counterpart for the case where a
// non-zero exit is the behaviour under test rather than a failure.
func runScriptExpectingRefusal(t *testing.T, script string) string {
	t.Helper()
	cmd := exec.Command("bash", script)
	output, err := cmd.CombinedOutput()
	require.Error(t, err, "the generator was expected to refuse, but it succeeded: %s", output)
	return string(output)
}

// stageForeignRoot swaps in a real, self-signed root that did not issue any of
// the material around it, setting Basic Constraints to CA:TRUE only when isCA
// is true. The CA/non-CA distinction is the one axis the paired tests using
// this exist to exercise; building both fixtures from one function keeps that
// distinction visible at the call site instead of buried in duplicated
// certificate boilerplate.
//
// The root's own key, the trust store copy and the chain are rewritten along
// with it, so the result is broken in one way rather than four: with isCA true
// the only relationship left false is that this root issued nothing around it,
// and with isCA false the missing CA:TRUE joins it. A fixture that trips every
// check at once cannot show that the particular check it aims at is still
// there, because any of the others would catch it.
//
// The root and its key come back so a caller can re-issue material underneath
// it, which is how the leaf's issuance gets isolated from mock-dezi's.
func stageForeignRoot(t *testing.T, out string, isCA bool) (*x509.Certificate, crypto.Signer) {
	t.Helper()
	key := newECKey(t)
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
	root := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})

	require.NoError(t, os.WriteFile(filepath.Join(out, "ca.pem"), root, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(out, "ca-only", "gf-sandbox-demo-ca.pem"), root, 0o600))
	writeECKey(t, filepath.Join(out, "ca.key"), key)

	leaf, err := os.ReadFile(filepath.Join(out, "plataan-uzi.pem"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(out, "plataan-uzi-chain.pem"),
		append(leaf, root...), 0o600))

	parsed, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return parsed, key
}

// stageLeafWithTheURAOutsideTheSAN replaces the leaf with one the root really
// did issue, whose key really is beside it, and whose chain really is the leaf
// followed by the root, but which carries the URA in its subject Common Name
// and has no subjectAltName extension at all.
//
// Every relationship except the one under test therefore still holds, so only
// the SAN check can catch it. That is the point: the check this fixture exists
// for used to search the whole certificate DER for the URA bytes, found them in
// the subject, and reported that the value sat in the SAN otherName. A
// validation that reports a relationship it did not check is worse than one
// that checks nothing, because the fast path then accepts the material forever
// and the failure surfaces at the token request, in a service that has never
// heard of this directory.
func stageLeafWithTheURAOutsideTheSAN(t *testing.T, out string) {
	t.Helper()
	issueLeaf(t, out, pkix.Name{
		CommonName:   plataanOtherName,
		Organization: []string{"Ziekenhuis De Plataan"},
	}, nil)
}

// A leaf whose subjectAltName is marked critical is a perfectly valid leaf, and
// RFC 5280 section 4.2.1.6 requires the extension to be critical when the
// subject is empty. The generator does not issue one, so nothing else here
// would notice a check that cannot read one.
//
// It matters anyway, and in the expensive direction. Rejecting a valid leaf
// sends the script down the generation path, so a false negative costs the
// operator their whole PKI on every single run, which is worse than the
// misplaced-URA acceptance the check exists to stop. The first version of that
// check read the line immediately after the extension's OID, and a critical
// extension puts a BOOLEAN there before the OCTET STRING, so it silently found
// nothing.
// Valid leaves the generator must not touch. Every case here is a false
// negative if it fails, and a false negative is the expensive direction: it
// sends the script down the generation path, so it rotates the operator's
// entire PKI on every run rather than once.
func TestGeneratorAcceptsValidLeavesItDidNotIssueItself(t *testing.T) {
	for name, stage := range map[string]func(*testing.T, string) []byte{
		// RFC 5280 section 4.2.1.6 requires a critical subjectAltName when the
		// subject is empty, so this shape is not exotic. The first version of
		// the SAN check read the line straight after the extension OID, where
		// a critical extension puts its BOOLEAN, and passing that offset to
		// -strparse segfaults LibreSSL 3.3.6.
		"the SAN is marked critical": stageLeafWithACriticalSAN,

		// The extension is found by matching the friendly name openssl renders
		// for the SAN OID. A certificate may carry that same string anywhere,
		// including its subject, where it renders as a UTF8STRING well before
		// the extensions. Matching it there selects an earlier extension's
		// OCTET STRING and answers about the wrong extension entirely.
		"the subject names the SAN extension": stageLeafWhoseSubjectNamesTheSANExtension,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			healthy := healthyMaterial(t)
			script := stageScript(t)
			out := stageMaterial(t, script, healthy)
			staged := stage(t, out)

			output := runScript(t, script)

			require.Contains(t, output, "already present",
				"a valid leaf must not send the generator down the rotation path")
			onDisk, err := os.ReadFile(filepath.Join(out, "plataan-uzi.pem"))
			require.NoError(t, err)
			require.Equal(t, staged, onDisk,
				"the leaf was rotated, so the check rejected a certificate it should have accepted")
		})
	}
}

// uraOtherNameType is the otherName type-id the did:x509 resolver looks for.
// It appends a SAN value only when the otherName carries exactly this OID
// (nuts-node vdr/didx509/x509_utils.go), so the same string under any other one
// is a value the node cannot see.
var uraOtherNameType = asn1.ObjectIdentifier{2, 5, 5, 5}

// issueLeaf reissues the leaf under root, keeping the certificate, the key
// beside it and the chain file consistent, and returns the PEM it wrote so a
// caller can tell "accepted unchanged" from "regenerated".
func issueLeaf(t *testing.T, out string, subject pkix.Name, extensions []pkix.Extension) []byte {
	t.Helper()
	root := parseCert(t, filepath.Join(out, "ca.pem"))
	rootKey, ok := parsePrivateKey(t, filepath.Join(out, "ca.key")).(crypto.Signer)
	require.True(t, ok, "the CA key must be a signing key")

	key := newECKey(t)
	der, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{
		SerialNumber:          big.NewInt(9),
		Subject:               subject,
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		BasicConstraintsValid: true,
		ExtraExtensions:       extensions,
	}, root, &key.PublicKey, rootKey)
	require.NoError(t, err)

	leaf := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	require.NoError(t, os.WriteFile(filepath.Join(out, "plataan-uzi.pem"), leaf, 0o600))
	writeECKey(t, filepath.Join(out, "plataan-uzi.key"), key)

	rootPEM, err := os.ReadFile(filepath.Join(out, "ca.pem"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(out, "plataan-uzi-chain.pem"),
		append(leaf, rootPEM...), 0o600))
	return leaf
}

// stageLeafWithACriticalSAN reissues the leaf with the URA in a critical
// subjectAltName otherName. Everything else about the material stays true.
func stageLeafWithACriticalSAN(t *testing.T, out string) []byte {
	t.Helper()
	return issueLeaf(t, out,
		pkix.Name{CommonName: "plataan", Organization: []string{"Ziekenhuis De Plataan"}},
		[]pkix.Extension{uraSANExtension(t, uraOtherNameType, true)})
}

// stageLeafWhoseSubjectNamesTheSANExtension gives the leaf a subject Common
// Name equal to the friendly text openssl prints for the SAN OID, while its
// actual SAN is correct. BasicConstraintsValid puts a second extension ahead of
// the SAN, so a scanner that starts on the subject and takes the next OCTET
// STRING lands on Basic Constraints and answers about that instead.
func stageLeafWhoseSubjectNamesTheSANExtension(t *testing.T, out string) []byte {
	t.Helper()
	return issueLeaf(t, out,
		pkix.Name{CommonName: "X509v3 Subject Alternative Name", Organization: []string{"Ziekenhuis De Plataan"}},
		[]pkix.Extension{uraSANExtension(t, uraOtherNameType, false)})
}

// stageLeafWithTheURAUnderAnotherTypeID puts the URA in a real SAN otherName
// under an OID the resolver does not look at.
func stageLeafWithTheURAUnderAnotherTypeID(t *testing.T, out string) {
	t.Helper()
	issueLeaf(t, out,
		pkix.Name{CommonName: "plataan", Organization: []string{"Ziekenhuis De Plataan"}},
		[]pkix.Extension{uraSANExtension(t, asn1.ObjectIdentifier{1, 2, 3, 4}, false)})
}

// stageRootClaimingCAInItsSubject swaps in a root that is not a CA and carries
// the text CA:TRUE in its subject instead of in Basic Constraints, then
// reissues everything beneath it so that this is the only relationship left
// false. LibreSSL 3.3.6 accepts a non-CA certificate as an explicit -CAfile
// anchor, so the issuance checks do not contradict it either.
func stageRootClaimingCAInItsSubject(t *testing.T, out string) {
	t.Helper()
	key := newECKey(t)
	der, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{
		SerialNumber: big.NewInt(10),
		Subject:      pkix.Name{CommonName: "CA:TRUE"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
	}, &x509.Certificate{
		SerialNumber: big.NewInt(10),
		Subject:      pkix.Name{CommonName: "CA:TRUE"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
	}, &key.PublicKey, key)
	require.NoError(t, err)
	root := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})

	require.NoError(t, os.WriteFile(filepath.Join(out, "ca.pem"), root, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(out, "ca-only", "gf-sandbox-demo-ca.pem"), root, 0o600))
	writeECKey(t, filepath.Join(out, "ca.key"), key)

	parsed, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	require.False(t, parsed.IsCA, "the fixture must not be a CA, or it proves nothing")
	stageMockDezi(t, out, parsed, key)
	issueLeaf(t, out,
		pkix.Name{CommonName: "plataan", Organization: []string{"Ziekenhuis De Plataan"}},
		[]pkix.Extension{uraSANExtension(t, uraOtherNameType, false)})
}

// uraSANExtension builds the extension Go's x509 package will not: an otherName
// carrying the URA as a UTF8String under the given type-id. Go drops otherName
// entries it does not recognise rather than emitting them, so this is assembled
// with encoding/asn1, mirroring how sanOtherName takes one apart.
//
// The type-id is a parameter because it is load-bearing and invisible in the
// rendered value: the same string under the wrong OID looks identical in
// openssl's output and is unreadable to the node.
func uraSANExtension(t *testing.T, typeID asn1.ObjectIdentifier, critical bool) pkix.Extension {
	t.Helper()
	return uraSANExtensionAs(t, typeID, critical, "utf8")
}

// uraSANExtensionAs is uraSANExtension with the ASN.1 string encoding as a
// parameter, for the case that asserts the generator issues UTF8String and
// treats anything else as material it did not produce.
func uraSANExtensionAs(t *testing.T, typeID asn1.ObjectIdentifier, critical bool, encoding string) pkix.Extension {
	t.Helper()
	value, err := asn1.MarshalWithParams(plataanOtherName, encoding)
	require.NoError(t, err)

	otherName, err := asn1.Marshal(struct {
		TypeID asn1.ObjectIdentifier
		Value  asn1.RawValue `asn1:"tag:0"`
	}{
		TypeID: typeID,
		Value:  asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: value},
	})
	require.NoError(t, err)

	// GeneralName's otherName is "[0] IMPLICIT OtherName", so the context tag
	// replaces the SEQUENCE tag rather than wrapping it, which is why the
	// content bytes are lifted out and re-tagged.
	var sequence asn1.RawValue
	_, err = asn1.Unmarshal(otherName, &sequence)
	require.NoError(t, err)

	names, err := asn1.Marshal([]asn1.RawValue{{
		Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: sequence.Bytes,
	}})
	require.NoError(t, err)

	return pkix.Extension{Id: asn1.ObjectIdentifier{2, 5, 29, 17}, Critical: critical, Value: names}
}

// stageMockDezi replaces the mock-dezi pair with a fresh certificate and its
// matching key, issued by parent when there is one and self-signed when there
// is not. The generator asks only two things of this certificate, that the key
// beside it matches and that the root issued it, so a fixture that changes
// exactly one of those isolates the check that has to catch it.
func stageMockDezi(t *testing.T, out string, parent *x509.Certificate, parentKey crypto.Signer) {
	t.Helper()
	key := newECKey(t)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "mock-dezi"},
		DNSNames:     []string{"mock-dezi", "localhost"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
	}
	issuer, issuerKey := template, crypto.Signer(key)
	if parent != nil {
		issuer, issuerKey = parent, parentKey
	}
	der, err := x509.CreateCertificate(rand.Reader, template, issuer, &key.PublicKey, issuerKey)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(out, "mock-dezi.pem"),
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600))
	writeECKey(t, filepath.Join(out, "mock-dezi.key"), key)
}

// P-256 because it costs nothing and the generator compares public keys, so
// what the algorithm is never comes into it.
func newECKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	return key
}

func writeECKey(t *testing.T, path string, key *ecdsa.PrivateKey) {
	t.Helper()
	der, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path,
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), 0o600))
}

// The regression this covers cost two shipped-broken attempts: material that
// was fine apart from its modes, and a script that answered "nothing to do"
// while changing nothing. What the earlier version of this test never checked
// is the other half of the promise in its name. Repairing in place means the
// material survives the repair, and rotation is not a harmless extra here:
// bootstrap-nuts.sh matches the wallet on credential type alone, so a rotated
// leaf leaves the node holding a credential issued from the chain that no
// longer exists, and the demo fails at token request time pointing at the node.
func TestGeneratorRepairsPermissionsWithoutRotatingMaterial(t *testing.T) {
	healthy := healthyMaterial(t)
	script := stageScript(t)
	// stageMaterial writes the set owner-only, which is what the older script
	// left behind and the state this has to converge.
	out := stageMaterial(t, script, healthy)

	output := runScript(t, script)

	require.Contains(t, output, "already present",
		"material that works must be accepted as it stands rather than regenerated")
	requireContainerReadable(t, out)
	requireMaterialMatches(t, healthy, out)
}

// Every case starts from one healthy run and breaks exactly one relationship,
// so a case that stops failing names the check that went missing rather than
// leaving a fixture that trips six of them at once. Before this, all of these
// took the early exit: the files were all present and ca.pem was still a CA,
// so the generator reported material it had never looked inside as usable, and
// no later run ever repaired it.
func TestGeneratorRegeneratesMaterialThatDoesNotHangTogether(t *testing.T) {
	healthy := healthyMaterial(t)

	broken := []struct {
		name   string
		mutate func(t *testing.T, out string)
	}{
		{
			// The reproduction from the review: the state an interrupt leaves
			// between writing plataan-uzi.key and re-issuing the certificate
			// beside it. The toolkit then signs with this key while embedding
			// the old leaf, and the credential fails against its own chain.
			name: "the UZI key is not the leaf's key",
			mutate: func(t *testing.T, out string) {
				writeECKey(t, filepath.Join(out, "plataan-uzi.key"), newECKey(t))
			},
		},
		{
			name: "the CA key is not the root's key",
			mutate: func(t *testing.T, out string) {
				writeECKey(t, filepath.Join(out, "ca.key"), newECKey(t))
			},
		},
		{
			name: "the mock-dezi key is not its certificate's key",
			mutate: func(t *testing.T, out string) {
				writeECKey(t, filepath.Join(out, "mock-dezi.key"), newECKey(t))
			},
		},
		{
			// mock-dezi's certificate is the one piece of this set nothing
			// else refers to, so nothing else notices when it stops
			// descending from the root: the knooppunt trusts the root and
			// then rejects the TLS handshake.
			name:   "the root did not issue the mock-dezi certificate",
			mutate: func(t *testing.T, out string) { stageMockDezi(t, out, nil, nil) },
		},
		{
			// A real certificate, not a placeholder: the knooppunt trusts
			// whatever sits here, so "it parses" is not the property that
			// matters. Anchoring the demo on a certificate that signs nothing
			// rejects every chain the node is given.
			name: "the trust store copy is not the root",
			mutate: func(t *testing.T, out string) {
				leaf, err := os.ReadFile(filepath.Join(out, "plataan-uzi.pem"))
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(
					filepath.Join(out, "ca-only", "gf-sandbox-demo-ca.pem"), leaf, 0o600))
			},
		},
		{
			// The CA is written first, so every interrupt after it leaves
			// this: a root the material around it does not descend from.
			// mock-dezi is re-issued underneath the new root so that the leaf
			// is the only thing left unissued by it, which is what makes this
			// case name the leaf's own check rather than mock-dezi's.
			name: "the root did not issue the leaf",
			mutate: func(t *testing.T, out string) {
				root, key := stageForeignRoot(t, out, true)
				stageMockDezi(t, out, root, key)
			},
		},
		{
			// bootstrap-nuts.sh feeds this file to the toolkit, and the
			// did:x509 resolver needs the root in it to anchor the chain.
			name: "the chain is not the leaf followed by the root",
			mutate: func(t *testing.T, out string) {
				leaf, err := os.ReadFile(filepath.Join(out, "plataan-uzi.pem"))
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(
					filepath.Join(out, "plataan-uzi-chain.pem"), leaf, 0o600))
			},
		},
		{
			// The mock-dezi certificate stands in as the leaf: same key pair,
			// same issuer, same chain shape, no URA. It is the shape of what
			// a leaf from an older version of this script leaves behind, and
			// the presentation definition is the only thing that would ever
			// notice, three services downstream.
			name: "the leaf does not carry the URA",
			mutate: func(t *testing.T, out string) {
				dezi, err := os.ReadFile(filepath.Join(out, "mock-dezi.pem"))
				require.NoError(t, err)
				deziKey, err := os.ReadFile(filepath.Join(out, "mock-dezi.key"))
				require.NoError(t, err)
				root, err := os.ReadFile(filepath.Join(out, "ca.pem"))
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(filepath.Join(out, "plataan-uzi.pem"), dezi, 0o600))
				require.NoError(t, os.WriteFile(filepath.Join(out, "plataan-uzi.key"), deziKey, 0o600))
				require.NoError(t, os.WriteFile(filepath.Join(out, "plataan-uzi-chain.pem"),
					append(dezi, root...), 0o600))
			},
		},
		{
			// mock-dezi reads this key at startup and exits fatally on a key
			// it cannot parse, which a partially written file is.
			name: "the signing key does not parse",
			mutate: func(t *testing.T, out string) {
				require.NoError(t, os.WriteFile(
					filepath.Join(out, "dezi-signing.key"), []byte("-----BEGIN RSA PRIVATE KEY-----\n"), 0o600))
			},
		},
		{
			// A real, parseable private key of the wrong type. mock-dezi
			// type-asserts *rsa.PrivateKey and exits with "is a %T, not an RSA
			// private key" (mock-components/dezi/keys.go), so "it parses" is
			// not the property that decides whether the stack starts.
			name: "the signing key is not RSA",
			mutate: func(t *testing.T, out string) {
				writeECKey(t, filepath.Join(out, "dezi-signing.key"), newECKey(t))
			},
		},
		{
			// The URA is present in the certificate, and in the wrong place.
			// This is the case a check that searches the whole DER cannot
			// distinguish from a correct leaf, and the presentation definition
			// reads $.credentialSubject.san.otherName, so the wrong place is
			// no place at all.
			name:   "the URA is in the subject rather than the SAN",
			mutate: stageLeafWithTheURAOutsideTheSAN,
		},
		{
			// In the SAN, in an otherName, and still invisible to the node.
			// The did:x509 resolver appends a SAN value only when the
			// otherName's type-id is exactly 2.5.5.5
			// (nuts-node vdr/didx509/x509_utils.go), so the same string under
			// any other OID resolves to no san:otherName at all and the
			// credential fails against its own policy.
			name:   "the URA is under another otherName type-id",
			mutate: stageLeafWithTheURAUnderAnotherTypeID,
		},
		{
			// The URA, under the right type-id, in a string type this script
			// does not issue. Regenerating is the intended answer and not a
			// near miss: every other check in validate_material asks "is this
			// the set I produced", and a leaf encoded some other way is not.
			//
			// It is worth pinning because the resolver is more permissive than
			// this: it unmarshals into a Go string, and encoding/asn1 takes
			// IA5String, GeneralString, T61String, NumericString and BMPString
			// too. Someone reading only that could widen this check to match,
			// which would make it depend on openssl printing the value, and
			// openssl prints nothing at all for GeneralString and BMPString.
			name: "the URA is not a UTF8String",
			mutate: func(t *testing.T, out string) {
				issueLeaf(t, out,
					pkix.Name{CommonName: "plataan", Organization: []string{"Ziekenhuis De Plataan"}},
					[]pkix.Extension{uraSANExtensionAs(t, uraOtherNameType, false, "ia5")})
			},
		},
		{
			// A root that says CA:TRUE without being a CA. The text appears in
			// the subject, nowhere near Basic Constraints, and LibreSSL 3.3.6
			// then accepts the certificate as an explicit -CAfile anchor too,
			// so nothing else in the set contradicts it. The did:x509 resolver
			// checks IsCA on the certificate the fingerprint names
			// (nuts-node vdr/didx509/resolver.go) and rejects the chain.
			name:   "the root only claims CA:TRUE in its subject",
			mutate: stageRootClaimingCAInItsSubject,
		},
	}

	for _, broken := range broken {
		t.Run(broken.name, func(t *testing.T) {
			// Each case is an independent temp directory and an independent
			// bash run, and every one of them pays for a 4096-bit CA key.
			// Sequentially that is most of this package's runtime.
			t.Parallel()
			script := stageScript(t)
			out := stageMaterial(t, script, healthy)
			broken.mutate(t, out)

			output := runScript(t, script)

			require.NotContains(t, output, "already present",
				"material that does not hang together must never be reported as usable")
			requireMaterialHangsTogether(t, out)
			requireContainerReadable(t, out)
		})
	}
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
	require.Equal(t, plataanOtherName, sanOtherName(t, leaf),
		"the URA must sit in the SAN otherName where the presentation definition looks for it")

	// Task 6 feeds this chain to the did:x509 resolver, which requires a
	// chain sorted leaf to root; a silent reversal here would otherwise only
	// surface there as a confusing failure.
	chain := parseCertChain(t, filepath.Join(out, "plataan-uzi-chain.pem"))
	require.Len(t, chain, 2, "the chain must contain exactly the leaf and the root")
	require.True(t, chain[0].Equal(leaf), "the chain's first certificate must be the leaf")
	require.True(t, chain[1].Equal(root), "the chain's second certificate must be the root")

	requireContainerReadable(t, out)
	for _, name := range []string{"plataan-uzi.pem", "plataan-uzi-chain.pem"} {
		require.Equal(t, os.FileMode(wantPublic), mode(t, filepath.Join(out, name)),
			"%s is mounted into the bootstrap and must be readable", name)
	}
}

// A root generated before this material became did:x509 anchored is a real,
// parseable, self-signed certificate with no Basic Constraints, and the
// resolver rejects every chain it anchors. The fixture is otherwise healthy
// material with such a root swapped in, rather than placeholders that fail on
// parseability alone, so only the missing CA:TRUE distinguishes it from a
// usable root.
func TestGeneratorReplacesANonCARoot(t *testing.T) {
	healthy := healthyMaterial(t)
	script := stageScript(t)
	out := stageMaterial(t, script, healthy)
	stageForeignRoot(t, out, false)

	runScript(t, script)

	root := parseCert(t, filepath.Join(out, "ca.pem"))
	require.True(t, root.IsCA, "a root that is not a usable CA must be regenerated, not accepted")
	requireMaterialHangsTogether(t, out)
}

// Docker creates a directory wherever a bind-mount source is missing, so a
// compose stack brought up before this script has ever run leaves one at each
// of the paths compose mounts out of .certs. A directory staged with MkdirAll
// is indistinguishable from one Docker created, which is why this needs no
// daemon.
//
// The overlay's policy mount is absent from the cases below because it is the
// one mount whose source is a directory: Docker creating it is the state the
// generator writes into, not a wedged one.
//
// The generator's presence probe is -f, which a directory fails, so before the
// guard it read the material as absent and took the generation path: it
// rewrote ca.key and ca.pem and only then died on the first directory it could
// not overwrite. Asserting a non-zero exit alone would not catch that, because
// three of these four cases exited non-zero already; what was wrong was that
// the CA had been rotated by the time they did. Hence the ca.key assertion,
// which is the regression that actually costs the operator something.
func TestGeneratorRefusesADirectoryWhereAFileBelongs(t *testing.T) {
	// docker-compose.yml mounts the first three, docker-compose.sandbox.yml
	// the fourth.
	for _, mounted := range []string{
		"mock-dezi.pem",
		"mock-dezi.key",
		"dezi-signing.key",
		"ca-only/gf-sandbox-demo-ca.pem",
	} {
		t.Run(mounted, func(t *testing.T) {
			script := stageScript(t)
			out := stageStaleMaterial(t, script)
			wedged := filepath.Join(out, mounted)
			require.NoError(t, os.Remove(wedged))
			require.NoError(t, os.Mkdir(wedged, 0o755))
			caKeyBefore, err := os.ReadFile(filepath.Join(out, "ca.key"))
			require.NoError(t, err)

			output := runScriptExpectingRefusal(t, script)

			require.Contains(t, output, wedged,
				"the refusal must name the path that is the wrong kind, or the operator cannot act on it")
			require.Contains(t, output, "Delete "+out,
				"the refusal must give the recovery, which is documented nowhere else")

			caKeyAfter, err := os.ReadFile(filepath.Join(out, "ca.key"))
			require.NoError(t, err)
			require.Equal(t, caKeyBefore, caKeyAfter,
				"the CA must not be rotated before the refusal: a half-rotated PKI is the expensive half of this failure")
			// stageStaleMaterial leaves .certs at 0700 and apply_modes is what
			// widens it to 0755, so this is what proves the guard runs ahead of
			// apply_modes rather than merely ahead of key generation.
			require.Equal(t, os.FileMode(0o700), mode(t, out),
				"the guard must refuse before apply_modes touches a wedged tree")
		})
	}
}

// The seam between a shell pipeline and a Go constant, and the one place a
// silent mismatch could hide. Everything else that touches the fingerprint
// works from whichever side of it the reader happens to be on: the script
// computes it with openssl and never parses the policy back, and the policy
// tests substitute a fingerprint they minted themselves and never run the
// script. A digest computed one way and pinned the other is invisible to both,
// and shows up two services away as a token request that fails to match a
// credential the node holds and considers valid.
//
// So this asserts the whole pattern, not just the digest: what the shell wrote
// has to be exactly what Go computes from the same ca.pem, wrapped in the
// pattern the template carries.
func TestGeneratorRendersThePolicyForTheCAItGenerated(t *testing.T) {
	script := stageScript(t)
	out := filepath.Join(filepath.Dir(script), ".certs")

	runScript(t, script)

	require.Equal(t, issuerPattern(caFingerprintOnDisk(t, out)), renderedIssuerPattern(t, out),
		"the rendered policy must pin the CA this run produced")
	require.Equal(t, os.FileMode(wantDir), mode(t, filepath.Join(out, "policy")),
		"the knooppunt reads the policy directory as UID 18081 and must be able to traverse it")
	require.Equal(t, os.FileMode(wantPublic), mode(t, filepath.Join(out, "policy", "bgz.json")),
		"the policy is bind-mounted into the knooppunt and must be readable")
}

// The early exit is the path an operator whose material predates the policy
// step takes, and it is the path that costs nothing to get wrong: the material
// validates, the script says there is nothing to do, and the sandbox comes up
// with no bgz scope at all. apply_modes runs on both paths for the same reason
// and gives the same argument; this is that argument applied to the render.
func TestGeneratorRendersThePolicyOnTheEarlyExitPath(t *testing.T) {
	healthy := healthyMaterial(t)
	script := stageScript(t)
	out := stageMaterial(t, script, healthy)
	require.NoError(t, os.RemoveAll(filepath.Join(out, "policy")))

	output := runScript(t, script)

	require.Contains(t, output, "already present", "the material must still be accepted as it stands")
	require.Equal(t, issuerPattern(caFingerprintOnDisk(t, out)), renderedIssuerPattern(t, out),
		"a run that repairs nothing else must still render the policy")
}

// A policy left pinned to a CA that no longer exists is worse than an absent
// one: absent fails closed at startup with invalid_scope, while stale fails at
// the token request, against a credential the node holds and considers valid,
// naming nothing that points back here. The fingerprint is 43 base64url
// characters of an unpadded 32-byte digest, so the stale value is a real one
// rather than a placeholder; a shorter string would be rejected by length alone
// and would not show that the script re-derives rather than merely repairs.
func TestGeneratorRepinsAPolicyLeftOnAnotherCA(t *testing.T) {
	healthy := healthyMaterial(t)
	script := stageScript(t)
	out := stageMaterial(t, script, healthy)

	stale := caFingerprintOf([]byte("a certificate this material never descended from"))
	require.Len(t, stale, 43)
	rendered := filepath.Join(out, "policy", "bgz.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(rendered), 0o700))
	template, err := os.ReadFile(bgzPolicyTemplate)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(rendered,
		[]byte(strings.ReplaceAll(string(template), caFingerprintPlaceholder, stale)), 0o600))

	output := runScript(t, script)

	require.Contains(t, output, "already present")
	require.Equal(t, issuerPattern(caFingerprintOnDisk(t, out)), renderedIssuerPattern(t, out),
		"the policy must follow the CA on disk, not whatever a previous run pinned")
}

// issuerPattern is the whole of what the descriptor's $.issuer filter contains
// for a given CA. Written once, so a test cannot agree with the template about
// the fingerprint while disagreeing about the shape around it.
func issuerPattern(fingerprint string) string {
	return "^did:x509:0:sha256:" + fingerprint + "::.*$"
}

// caFingerprintOnDisk computes what a did:x509 issued under out/ca.pem would
// carry, with crypto/x509 and encoding/base64 rather than by shelling out to
// the openssl pipeline the generator uses. Asserting with the same tool and the
// same reasoning as the code under test would pass on whatever that pipeline
// happens to emit, including the empty string two failed openssl calls produce.
func caFingerprintOnDisk(t *testing.T, out string) string {
	t.Helper()
	fingerprint := caFingerprintOf(parseCert(t, filepath.Join(out, "ca.pem")).Raw)
	require.Len(t, fingerprint, 43, "an unpadded base64url SHA-256 is 43 characters; anything else is not a digest")
	return fingerprint
}

// renderedIssuerPattern reads the generated policy back through the node's own
// types, so a render that is no longer a loadable presentation definition fails
// here rather than at node startup.
func renderedIssuerPattern(t *testing.T, out string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(out, "policy", "bgz.json"))
	require.NoError(t, err)

	var mapping map[string]pe.WalletOwnerMapping
	require.NoError(t, json.Unmarshal(raw, &mapping))
	for _, descriptor := range mapping["bgz"]["organization"].InputDescriptors {
		if descriptor.Id != "id_uzicert_uracredential" {
			continue
		}
		for _, field := range descriptor.Constraints.Fields {
			if len(field.Path) != 1 || field.Path[0] != "$.issuer" {
				continue
			}
			require.NotNil(t, field.Filter)
			require.NotNil(t, field.Filter.Pattern)
			return *field.Filter.Pattern
		}
	}
	t.Fatal("the rendered policy does not constrain $.issuer, so it pins no CA at all")
	return ""
}

// healthyMaterialCache holds one complete generator run, produced on first use
// and shared by every test that needs a valid set to start from: they need the
// same healthy material rather than different material, and a 4096-bit CA key
// costs a second or more per run. Top-level tests run one at a time and the
// parallel subtests above take their copy from the parent before they fan out,
// so this is only ever written sequentially.
var healthyMaterialCache map[string][]byte

func healthyMaterial(t *testing.T) map[string][]byte {
	t.Helper()
	if healthyMaterialCache == nil {
		script := stageScript(t)
		runScript(t, script)
		healthyMaterialCache = snapshotMaterial(t, filepath.Join(filepath.Dir(script), ".certs"))
	}
	return healthyMaterialCache
}

// snapshotMaterial reads every file under dir into memory, keyed by its path
// relative to dir. Modes are deliberately not captured: the point of a
// snapshot is the material, and every test that uses one wants it staged in
// the owner-only modes that are the starting state under test.
func snapshotMaterial(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	material := map[string][]byte{}
	require.NoError(t, filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		name, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		material[name] = content
		return nil
	}))
	require.NotEmpty(t, material, "a generator run that wrote nothing is not a fixture")
	return material
}

// stageMaterial writes a snapshot into the .certs directory beside script, in
// the owner-only modes an older version of the generator left behind.
func stageMaterial(t *testing.T, script string, material map[string][]byte) string {
	t.Helper()
	out := filepath.Join(filepath.Dir(script), ".certs")
	for name, content := range material {
		path := filepath.Join(out, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, content, 0o600))
	}
	require.NoError(t, os.Chmod(out, 0o700))
	return out
}

// requireMaterialMatches fails unless every file in the snapshot is still on
// disk with the same content, which is what distinguishes repairing material
// from replacing it.
func requireMaterialMatches(t *testing.T, material map[string][]byte, out string) {
	t.Helper()
	for name, want := range material {
		got, err := os.ReadFile(filepath.Join(out, name))
		require.NoError(t, err)
		require.Equal(t, want, got, "%s was rewritten, so the material was rotated rather than repaired", name)
	}
}

// requireMaterialHangsTogether asserts the relationships the generator's own
// validation exists to guarantee, and does it with crypto/x509 rather than by
// shelling out to openssl the way the generator does. Asserting with the same
// tool and the same reasoning as the code under test would pass on anything
// that code happens to accept, including the mismatches it was accepting
// before this existed.
func requireMaterialHangsTogether(t *testing.T, out string) {
	t.Helper()
	root := parseCert(t, filepath.Join(out, "ca.pem"))
	require.True(t, root.IsCA, "the root must be a CA certificate")
	requireKeyBelongsTo(t, filepath.Join(out, "ca.key"), root)

	dezi := parseCert(t, filepath.Join(out, "mock-dezi.pem"))
	requireKeyBelongsTo(t, filepath.Join(out, "mock-dezi.key"), dezi)
	require.NoError(t, dezi.CheckSignatureFrom(root), "mock-dezi.pem must be issued by the root")

	leaf := parseCert(t, filepath.Join(out, "plataan-uzi.pem"))
	requireKeyBelongsTo(t, filepath.Join(out, "plataan-uzi.key"), leaf)
	require.NoError(t, leaf.CheckSignatureFrom(root), "plataan-uzi.pem must be issued by the root")
	require.Equal(t, plataanOtherName, sanOtherName(t, leaf),
		"the leaf must carry the URA the presentation definition looks for")

	root0, err := os.ReadFile(filepath.Join(out, "ca.pem"))
	require.NoError(t, err)
	trusted, err := os.ReadFile(filepath.Join(out, "ca-only", "gf-sandbox-demo-ca.pem"))
	require.NoError(t, err)
	require.Equal(t, root0, trusted, "the knooppunt's trust store copy must be the root itself")

	chain := parseCertChain(t, filepath.Join(out, "plataan-uzi-chain.pem"))
	require.Len(t, chain, 2, "the chain must contain exactly the leaf and the root")
	require.True(t, chain[0].Equal(leaf), "the chain must start with the leaf on disk")
	require.True(t, chain[1].Equal(root), "the chain must end with the root on disk")

	// The type, not merely that it parses: mock-components/dezi/keys.go
	// type-asserts *rsa.PrivateKey and exits fatally on anything else, so an
	// EC key here is a stack that does not start.
	require.IsType(t, &rsa.PrivateKey{}, parsePrivateKey(t, filepath.Join(out, "dezi-signing.key")),
		"mock-dezi accepts only an RSA signing key")
}

// requireKeyBelongsTo fails unless the private key at path is the one the
// certificate was issued for.
func requireKeyBelongsTo(t *testing.T, path string, cert *x509.Certificate) {
	t.Helper()
	public, ok := cert.PublicKey.(interface{ Equal(crypto.PublicKey) bool })
	require.True(t, ok, "%T cannot be compared", cert.PublicKey)
	signer, ok := parsePrivateKey(t, path).(crypto.Signer)
	require.True(t, ok, "%s is not a signing key", path)
	require.True(t, public.Equal(signer.Public()),
		"%s is not the key %s was issued for", path, cert.Subject.CommonName)
}

// parsePrivateKey accepts the three encodings this material can be in: PKCS#1
// for the keys openssl writes with -traditional, PKCS#8 for the ones it writes
// without, and SEC 1 for the fixtures above.
func parsePrivateKey(t *testing.T, path string) crypto.PrivateKey {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	block, _ := pem.Decode(raw)
	require.NotNil(t, block, "%s is not PEM encoded", path)

	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		return key
	}
	key, err := x509.ParseECPrivateKey(block.Bytes)
	require.NoError(t, err, "%s is not a private key in any encoding this material uses", path)
	return key
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
