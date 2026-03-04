package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"

	"github.com/google/uuid"
	"github.com/zitadel/oidc/v3/pkg/client/rp"
	zitadelHTTP "github.com/zitadel/oidc/v3/pkg/http"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"golang.org/x/oauth2"
)

func main() {
	deziURL := os.Getenv("DEZI_URL")
	if deziURL == "" {
		deziURL = "https://max.proeftuin.uzi-online.irealisatie.nl"
	}
	clientID := "TODO"
	ctx := context.Background()
	insecureKey := make([]byte, 32)
	cookieHandler := zitadelHTTP.NewCookieHandler(insecureKey, insecureKey, zitadelHTTP.WithUnsecure())

	relyingParty, err := rp.NewRelyingPartyOIDC(ctx, deziURL, clientID, "", "http://localhost:9090/callback", []string{"openid"},
		rp.WithPKCE(cookieHandler))
	if err != nil {
		panic(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /auth", rp.AuthURLHandler(func() string {
		return uuid.NewString()
	}, relyingParty, func() []oauth2.AuthCodeOption {
		nonce := make([]byte, 16)
		_, _ = rand.Read(nonce)
		return []oauth2.AuthCodeOption{
			oauth2.SetAuthURLParam("nonce", hex.EncodeToString(nonce)),
		}
	}))
	mux.HandleFunc("GET /callback", rp.CodeExchangeHandler(rp.UserinfoCallback(handleCallbackResult), relyingParty))
	const addr = ":9090"
	println("Listening on", addr)
	println("Open http://localhost" + addr + "/auth to start the authentication flow")
	if err := http.ListenAndServe(addr, mux); err != nil {
		panic(err)
	}
}

func handleCallbackResult(w http.ResponseWriter, r *http.Request, tokens *oidc.Tokens[*oidc.IDTokenClaims], state string, rp rp.RelyingParty, info *oidc.UserInfo) {
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(struct {
		Tokens *oidc.Tokens[*oidc.IDTokenClaims] `json:"tokens"`
		User   *oidc.UserInfo                    `json:"user"`
	}{
		Tokens: tokens,
		User:   info,
	}); err != nil {
		panic(err)
	}
}
