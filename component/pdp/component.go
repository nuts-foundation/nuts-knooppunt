package pdp

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/nuts-foundation/nuts-knooppunt/component"
)

type Config struct {
	Enabled bool
}

func DefaultConfig() Config {
	return Config{
		Enabled: true,
	}
}

var _ component.Lifecycle = (*Component)(nil)

type Component struct {
	Config Config
}

// New creates an instance of the pdp component, which provides a simple policy decision endpoint.
func New(config Config) (*Component, error) {
	return &Component{
		Config: config,
	}, nil
}

func (c Component) Start() error {
	// Nothing to do
	return nil
}

func (c Component) Stop(ctx context.Context) error {
	// Nothing to do
	return nil
}

func (c Component) RegisterHttpHandlers(publicMux *http.ServeMux, internalMux *http.ServeMux) {
	internalMux.HandleFunc("POST /pdp", mainPolicyHandler)
}

type MainPolicyInput struct {
	Method string   `json:"method"`
	Path   []string `json:"path"`
	User   string   `json:"user"`
}

type MainPolicyResponse struct {
	Result MainPolicyResult `json:"result"`
}

type MainPolicyResult struct {
	Allow bool `json:"allow"`
}

func mainPolicyHandler(w http.ResponseWriter, r *http.Request) {
	resp := MainPolicyResponse{
		Result: MainPolicyResult{
			Allow: false,
		},
	}

	b, err := json.Marshal(resp)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(b)
}
