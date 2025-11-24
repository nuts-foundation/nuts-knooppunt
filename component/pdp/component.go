package pdp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/nuts-foundation/nuts-knooppunt/component"
	"github.com/nuts-foundation/nuts-knooppunt/component/mitz"
)

func DefaultConfig() Config {
	return Config{
		Enabled: true,
	}
}

var _ component.Lifecycle = (*Component)(nil)

// New creates an instance of the pdp component, which provides a simple policy decision endpoint.
func New(config Config, mitzcomp *mitz.Component) (*Component, error) {
	return &Component{
		Config: config,
		Mitz:   mitzcomp,
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
	internalMux.HandleFunc("POST /pdp", http.HandlerFunc(c.HandleMainPolicy))
	internalMux.HandleFunc("POST /pdp/v1/data/{package}/{rule}", http.HandlerFunc(c.HandlePolicy))
}

func (c Component) HandleMainPolicy(w http.ResponseWriter, r *http.Request) {
	var reqBody MainPolicyRequest
	err := json.NewDecoder(r.Body).Decode(&reqBody)
	input := reqBody.Input
	if err != nil {
		http.Error(w, "unable to parse request body", http.StatusBadRequest)
		return
	}

	resp := MainPolicyResponse{
		EvalMitzPolicy(c, input),
	}

	b, err := json.Marshal(resp)
	if err != nil {
		http.Error(w, "failed to encode json output", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(b)
}

func (c Component) HandlePolicy(w http.ResponseWriter, r *http.Request) {
	pack := r.PathValue("package")
	if pack != "knooppunt" {
		http.Error(w, "invalid package", http.StatusBadRequest)
		return
	}

	policy := r.PathValue("rule")
	switch policy {
	case "authz":
		c.HandleMainPolicy(w, r)
	default:
		http.Error(w, fmt.Sprintf("unknown rule %s", policy), http.StatusBadRequest)
	}
}
