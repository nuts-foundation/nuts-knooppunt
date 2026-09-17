// Command seed populates a fresh deployment with demo data: FHIR test
// resources, a Nuts subject and X509Credential per demo organization, and
// NVI localization records.
//
// This used to be three separate containers/steps: a data seed (create
// subjects, write each one's DID to a shared directory), per-organization
// credential issuance via an external go-didx509-toolkit container reading
// those DID files, and a third step storing the resulting credentials and
// registering discovery. The split existed because a did:web DID gets a
// fresh UUID per subject creation, so a credential can't be minted before
// the subject exists - across separate containers that meant a file-based
// handoff. Consolidated into one process, it's just call order: create the
// subject, then mint its credential, in the same function.
//
// go-didx509-toolkit's credential_issuer package is imported directly
// (credential_issuer.IssueX509Credential) instead of shelling out to its
// CLI in a separate container.
package main

import (
	"bytes"
	"context"
	"crypto"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/nuts-foundation/go-didx509-toolkit/credential_issuer"
	"github.com/nuts-foundation/go-didx509-toolkit/x509_cert"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/plataan"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/sunflower"
)

// discoveryServiceID is the Nuts discovery service the demo organizations
// register on. It must match a definition in config/discovery/.
const discoveryServiceID = "bgz-test"

// caFingerprintDN is the subject DN of the CA that issued every demo
// organization's certificate (see test/e2e/pep/certs/README.md). Pinned in
// config/discovery/bgz-test.json by that CA's public key hash, so it must
// keep matching the committed certificates exactly.
const caFingerprintDN = "CN=Fake UZI Root CA"

// organization is a demo organization that participates in the
// authorization chain: it has a Nuts subject, an X509Credential minted from
// a test UZI certificate, and a discovery registration.
type organization struct {
	// certKey is the short name used for the organization's certificate
	// files in test/e2e/pep/certs (<certKey>.key, <certKey>-chain.pem).
	certKey string
	// subject is the Nuts subject name, which is the organization's URA.
	subject string
	// fhirBaseURL is advertised on discovery as where this organization
	// serves FHIR. It is deployment-dependent: in compose (and equivalently
	// in Kubernetes) it points at the organization's PEP, so a resolved
	// address leads through the authorization chain rather than around it.
	fhirBaseURL string
}

// organizations returns the demo organizations, in seed order.
func organizations() []organization {
	return []organization{
		// Ziekenhuis De Plataan (consumer/requester).
		{
			certKey:     "plataan",
			subject:     plataan.URA,
			fhirBaseURL: plataan.EndpointAddress(),
		},
		// Zorgcentrum De Zonnebloem (source / data holder).
		{
			certKey:     "zonnebloem",
			subject:     *sunflower.Organization().Identifier[0].Value,
			fhirBaseURL: sunflower.EndpointAddress(),
		},
	}
}

func main() {
	if len(os.Args) < 4 {
		panic("Usage: " + os.Args[0] + " <internal API base path> <HAPI FHIR multitenant base URL> <certs dir>")
	}
	internalAPI := strings.TrimRight(os.Args[1], "/")
	hapiBaseURL, err := url.Parse(os.Args[2])
	if err != nil {
		panic("invalid HAPI FHIR base URL: " + err.Error())
	}
	certsDir := os.Args[3]

	println("Loading FHIR testdata into HAPI...")
	if _, err := vectors.Load(hapiBaseURL); err != nil {
		panic("Unable to load testdata: " + err.Error())
	}

	for _, org := range organizations() {
		// The seed is re-runnable against a node that already has the
		// subject: creation fails and the existing DID is read back instead.
		// Every failure in this loop panics rather than skipping to the next
		// organization: the seed is meant to be re-run wholesale on failure,
		// not resumed per-organization.
		did, err := createNutsSubject(internalAPI, org.subject)
		if err != nil {
			println("Note: could not create Nuts subject " + org.subject + " (" + err.Error() + "); looking up existing subject")
			did, err = resolveSubjectDID(internalAPI, org.subject)
			if err != nil {
				panic("no DID available for subject " + org.subject + ": " + err.Error())
			}
			println("Using existing Nuts subject " + org.subject + " (" + did + ")")
		} else {
			println("Created Nuts subject " + org.subject + " (" + did + ")")
		}

		credential, err := issueX509Credential(certsDir, org.certKey, did)
		if err != nil {
			panic("unable to issue X509Credential for " + org.subject + ": " + err.Error())
		}

		if err := storeCredential(internalAPI, org.subject, credential); err != nil {
			panic("unable to store credential for " + org.subject + ": " + err.Error())
		}
		println("Stored X509Credential in wallet of " + org.subject)

		if err := registerOnDiscovery(internalAPI, org.subject, org.fhirBaseURL); err != nil {
			panic("unable to register " + org.subject + " on discovery '" + discoveryServiceID + "': " + err.Error())
		}
		println("Registered " + org.subject + " on discovery service " + discoveryServiceID + " at " + org.fhirBaseURL)
	}

	// Register each pool patient's NVI localization Lists through the
	// Knooppunt. This is part of the core seed and must succeed.
	println("Registering NVI localization records...")
	internalBaseURL, err := url.Parse(internalAPI)
	if err != nil {
		panic("invalid internal API base URL: " + err.Error())
	}
	if err := vectors.SeedNVI(context.Background(), internalBaseURL); err != nil {
		panic("Unable to seed NVI: " + err.Error())
	}

	// Run the mCSD sync so the seeded admin directories flow into the query
	// directory. Without this the demo organizations exist but are not
	// resolvable to an address, and there is no timer that would eventually
	// do it - AC1 requires the deployment to be addressable with no manual
	// step.
	println("Running mCSD update...")
	if err := invokeMCSDUpdate(internalAPI); err != nil {
		panic("Unable to run mCSD update: " + err.Error())
	}

	println("Seed complete.")
}

// issueX509Credential mints an X509Credential for subjectDID from the
// organization's pre-generated certificate chain and private key
// (<certsDir>/<certKey>-chain.pem, <certsDir>/<certKey>.key - see
// test/e2e/pep/certs/README.md), returning the credential as a JWT.
func issueX509Credential(certsDir, certKey, subjectDID string) (string, error) {
	chain, err := loadCertChain(filepath.Join(certsDir, certKey+"-chain.pem"))
	if err != nil {
		return "", fmt.Errorf("load cert chain: %w", err)
	}
	var caFingerprintCert *x509.Certificate
	for _, cert := range chain {
		if cert.Subject.String() == caFingerprintDN {
			caFingerprintCert = cert
			break
		}
	}
	if caFingerprintCert == nil {
		return "", fmt.Errorf("no certificate with subject %q in %s-chain.pem", caFingerprintDN, certKey)
	}

	key, err := loadPrivateKey(filepath.Join(certsDir, certKey+".key"))
	if err != nil {
		return "", fmt.Errorf("load private key: %w", err)
	}

	// go-didx509-toolkit's CLI defaults --subject_attr to O,L (organization
	// name and locality) when issuing a VC, via a kong flag default -
	// IssueX509Credential itself defaults to an empty subject attribute
	// list when called directly with no options, which omits
	// credentialSubject.subject.O. config/discovery/bgz-test.json's
	// presentation definition requires exactly that field, so it must be
	// passed explicitly here to match the CLI's actual behavior.
	credential, err := credential_issuer.IssueX509Credential(chain, caFingerprintCert, key, subjectDID,
		credential_issuer.SubjectAttributes(x509_cert.SubjectTypeOrganization, x509_cert.SubjectTypeLocality),
	)
	if err != nil {
		return "", err
	}
	return credential.Raw(), nil
}

// loadCertChain parses every certificate in a PEM file, in file order.
// go-didx509-toolkit's own chain-ordering logic (finding parents for an
// unordered certificate list) isn't needed here: the committed chain files
// are already leaf-then-root (see test/e2e/pep/certs/README.md), which is
// the order credential_issuer.IssueX509Credential expects (chain[0] is the
// signing certificate).
func loadCertChain(path string) ([]*x509.Certificate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var certs []*x509.Certificate
	for {
		var block *pem.Block
		block, data = pem.Decode(data)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, err
		}
		certs = append(certs, cert)
	}
	if len(certs) == 0 {
		return nil, errors.New("no certificates found")
	}
	return certs, nil
}

// loadPrivateKey parses a PKCS8- or PKCS1-encoded RSA private key from a PEM
// file, matching test/e2e/pep/certs/issue-cert.sh's output format.
func loadPrivateKey(path string) (crypto.Signer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("no PEM block found")
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		signer, ok := key.(crypto.Signer)
		if !ok {
			return nil, fmt.Errorf("PKCS8 key is not a crypto.Signer (%T)", key)
		}
		return signer, nil
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("not a valid PKCS8 or PKCS1 RSA key: %w", err)
	}
	return key, nil
}

// invokeMCSDUpdate triggers a synchronization of the configured mCSD
// administration directories into the query directory. The sync is
// request-driven (there is no background timer), so the seed has to ask for it.
func invokeMCSDUpdate(internalAPI string) error {
	httpResponse, err := http.Post(internalAPI+"/mcsd/update", "application/json", nil)
	if err != nil {
		return err
	}
	defer httpResponse.Body.Close()

	if httpResponse.StatusCode != http.StatusOK {
		responseData, _ := io.ReadAll(httpResponse.Body)
		return fmt.Errorf("unexpected status code (status=%s, expected=200)\nResponse data:\n%s",
			httpResponse.Status, strings.TrimSpace(string(responseData)))
	}
	return nil
}

// resolveSubjectDID returns the preferred DID of an existing Nuts subject.
func resolveSubjectDID(internalAPI, subject string) (string, error) {
	httpResponse, err := http.Get(internalAPI + "/nuts/internal/vdr/v2/subject/" + subject)
	if err != nil {
		return "", err
	}
	defer httpResponse.Body.Close()

	dids, err := readJSONResponse[[]string](httpResponse, http.StatusOK)
	if err != nil {
		return "", fmt.Errorf("failed to look up Nuts subject: %w", err)
	}
	if len(dids) == 0 {
		return "", fmt.Errorf("subject %s has no DIDs", subject)
	}
	return dids[0], nil
}

// createNutsSubject creates a Nuts subject, returning its preferred DID.
func createNutsSubject(internalAPI string, subject string) (string, error) {
	httpResponse, err := http.Post(internalAPI+"/nuts/internal/vdr/v2/subject", "application/json", strings.NewReader(`{"subject":"`+subject+`"}`))
	if err != nil {
		return "", err
	}
	defer httpResponse.Body.Close()

	type ResultDocument struct {
		ID string `json:"id"`
	}
	type Result struct {
		Documents []ResultDocument `json:"documents"`
	}
	result, err := readJSONResponse[Result](httpResponse, http.StatusOK)
	if err != nil {
		return "", fmt.Errorf("failed to create Nuts subject: %w", err)
	}
	if len(result.Documents) == 0 {
		return "", fmt.Errorf("subject created but no documents returned")
	}
	return result.Documents[0].ID, err
}

// storeCredential loads a Verifiable Credential into a subject's wallet. The
// credential is a JWT, and the endpoint takes it as a bare JSON string.
func storeCredential(internalAPI, subject, credential string) error {
	httpResponse, err := http.Post(
		internalAPI+"/nuts/internal/vcr/v2/holder/"+subject+"/vc",
		"application/json",
		strings.NewReader(`"`+credential+`"`),
	)
	if err != nil {
		return err
	}
	defer httpResponse.Body.Close()

	// Storing an already-stored credential is not an error: the seed is
	// re-runnable, and a conflict means the wallet already holds it.
	if httpResponse.StatusCode == http.StatusConflict {
		return nil
	}
	return expectStatus(httpResponse, http.StatusNoContent)
}

// registerOnDiscovery activates the discovery service for the subject, so it
// is findable by other participants.
func registerOnDiscovery(internalAPI, subject, fhirBaseURL string) error {
	body, err := json.Marshal(map[string]any{
		"registrationParameters": map[string]string{
			"fhirBaseURL": fhirBaseURL,
		},
	})
	if err != nil {
		return fmt.Errorf("marshal discovery registration request: %w", err)
	}

	httpResponse, err := http.Post(
		internalAPI+"/nuts/internal/discovery/v1/"+discoveryServiceID+"/"+subject,
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return err
	}
	defer httpResponse.Body.Close()

	return expectStatus(httpResponse, http.StatusOK)
}

func expectStatus(httpResponse *http.Response, expectedStatus int) error {
	if httpResponse.StatusCode != expectedStatus {
		responseData, _ := io.ReadAll(httpResponse.Body)
		return fmt.Errorf("unexpected status code (status=%s, expected=%d, url=%s)\nResponse data:\n----------------\n%s\n----------------",
			httpResponse.Status, expectedStatus, httpResponse.Request.URL, strings.TrimSpace(string(responseData)))
	}
	return nil
}

func readJSONResponse[T any](httpResponse *http.Response, expectedStatus int) (T, error) {
	var result T
	if err := expectStatus(httpResponse, expectedStatus); err != nil {
		return result, err
	}
	if err := json.NewDecoder(httpResponse.Body).Decode(&result); err != nil {
		return result, fmt.Errorf("failed to decode response body: %w", err)
	}
	return result, nil
}
