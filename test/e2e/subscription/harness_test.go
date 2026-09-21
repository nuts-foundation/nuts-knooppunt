package subscription

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/nuts-foundation/nuts-knooppunt/cmd"
	httpcomponent "github.com/nuts-foundation/nuts-knooppunt/component/http"
	"github.com/nuts-foundation/nuts-knooppunt/component/subscription"
	"github.com/stretchr/testify/require"
)

// The permutation matrix (PLAN.md, research question 6) runs two in-process knooppunts —
// Plataan (URA 00000010) as Subscription Server, Zonnebloem (URA 00000020) as
// Subscription Client — with different optional-feature choices per test. Ports are
// allocated per node so pairs can run in one process, unlike http.TestConfig()'s fixed
// 8080/8081.

const (
	plataanURA    = "00000010"
	zonnebloemURA = "00000020"
	taskTopic     = subscription.TaskTopicCanonical
)

type node struct {
	Name        string
	URA         string
	InternalURL string
	PublicURL   string
	config      cmd.Config
}

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

// allocNode reserves ports and prepares a minimal config; call start after wiring the
// partner URLs (which need both nodes' ports).
func allocNode(t *testing.T, name, ura string) *node {
	t.Helper()
	publicPort := freePort(t)
	internalPort := freePort(t)
	n := &node{
		Name:        name,
		URA:         ura,
		InternalURL: fmt.Sprintf("http://localhost:%d", internalPort),
		PublicURL:   fmt.Sprintf("http://localhost:%d", publicPort),
	}
	n.config = cmd.Config{
		HTTP: httpcomponent.Config{
			PublicInterface:   httpcomponent.InterfaceConfig{Address: fmt.Sprintf(":%d", publicPort), BaseURL: n.PublicURL},
			InternalInterface: httpcomponent.InterfaceConfig{Address: fmt.Sprintf(":%d", internalPort), BaseURL: n.InternalURL},
		},
		Subscription: subscription.DefaultConfig(),
	}
	// Fast retries so failure paths resolve within test time.
	n.config.Subscription.Server.RetryBaseDelayMillis = 25
	n.config.Subscription.Server.RetryMaxAttempts = 3
	return n
}

func (n *node) start(t *testing.T) {
	t.Helper()
	errChan := make(chan error, 1)
	go func() {
		if err := cmd.Start(t.Context(), n.config); err != nil {
			errChan <- err
		}
	}()
	require.Eventually(t, func() bool {
		select {
		case err := <-errChan:
			t.Fatalf("%s failed to start: %v", n.Name, err)
		default:
		}
		resp, err := http.Get(n.InternalURL + "/status")
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 15*time.Second, 100*time.Millisecond, "%s did not become ready", n.Name)
}

// startPair allocates, cross-wires and starts a Plataan server node and a Zonnebloem
// client node. mutate can adjust either config before startup (the permutation knobs).
func startPair(t *testing.T, mutate func(server, client *cmd.Config)) (*node, *node) {
	t.Helper()
	server := allocNode(t, "plataan", plataanURA)
	client := allocNode(t, "zonnebloem", zonnebloemURA)

	server.config.Subscription.Server.Enabled = true
	server.config.Subscription.Server.URA = plataanURA
	server.config.Subscription.Server.Partners = map[string]subscription.PartnerConfig{
		zonnebloemURA: {NotificationEndpoint: client.PublicURL + "/fhir/notifications"},
	}

	client.config.Subscription.Client.Enabled = true
	client.config.Subscription.Client.URA = zonnebloemURA
	client.config.Subscription.Client.Partners = map[string]subscription.PartnerConfig{
		plataanURA: {FHIRBaseURL: server.PublicURL + "/fhir"},
	}

	if mutate != nil {
		mutate(&server.config, &client.config)
	}
	server.start(t)
	client.start(t)
	return server, client
}

// --- plain-JSON API helpers (the "vendor" side of the e2e tests speaks only these) ---

func postJSON[T any](t *testing.T, url string, body any) (int, T) {
	t.Helper()
	payload, err := json.Marshal(body)
	require.NoError(t, err)
	resp, err := http.Post(url, "application/json", bytes.NewReader(payload))
	require.NoError(t, err)
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var result T
	if len(responseBody) > 0 {
		_ = json.Unmarshal(responseBody, &result)
	}
	return resp.StatusCode, result
}

func getAs[T any](t *testing.T, url string) (int, T) {
	t.Helper()
	resp, err := http.Get(url)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var result T
	if len(body) > 0 {
		_ = json.Unmarshal(body, &result)
	}
	return resp.StatusCode, result
}
