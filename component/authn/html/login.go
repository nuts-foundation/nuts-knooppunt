package html

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

func RenderLogin(httpResponse http.ResponseWriter, httpRequest *http.Request) {
	httpResponse.Header().Add("Content-Type", "text/html; charset=utf-8")
	err := loginTemplate.Execute(httpResponse, struct {
		AuthRequestID string
	}{
		AuthRequestID: httpRequest.URL.Query().Get("authRequestID"),
	})
	if err != nil {
		slog.ErrorContext(httpRequest.Context(), "Failed to render login template", "error", err)
		http.Error(httpResponse, "Internal Server Error", http.StatusInternalServerError)
	}
}

func HandleLoginSubmit(callbackURLFunc func(context.Context, string) string, completeLoginFunc func(ctx context.Context, authRequestID string, deziToken string) error) func(http.ResponseWriter, *http.Request) {
	signingKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	return func(httpResponse http.ResponseWriter, httpRequest *http.Request) {
		if err := httpRequest.ParseForm(); err != nil {
			slog.ErrorContext(httpRequest.Context(), "Failed to parse login form", "error", err)
			http.Error(httpResponse, "Bad Request", http.StatusBadRequest)
			return
		}

		authRequestID := httpRequest.FormValue("authRequestID")
		if authRequestID == "" {
			slog.ErrorContext(httpRequest.Context(), "Missing authRequestID")
			http.Error(httpResponse, "Bad Request: missing authRequestID", http.StatusBadRequest)
			return
		}

		// Check if user denied
		action := httpRequest.FormValue("action")
		if action == "deny" {
			slog.InfoContext(httpRequest.Context(), "User denied authorization")
			http.Error(httpResponse, "Authorization denied", http.StatusForbidden)
			return
		}

		// Build JWT token with Dezi claims
		token := jwt.New()

		// Set Dezi claims
		verklaringID := uuid.NewString()
		_ = token.Set("verklaring_id", verklaringID)
		_ = token.Set("loa_dezi", httpRequest.FormValue("loa_dezi"))
		_ = token.Set("dezi_nummer", httpRequest.FormValue("dezi_nummer"))
		_ = token.Set("voorletters", httpRequest.FormValue("voorletters"))
		_ = token.Set("voorvoegsel", httpRequest.FormValue("voorvoegsel"))
		_ = token.Set("achternaam", httpRequest.FormValue("achternaam"))
		_ = token.Set("abonnee_nummer", httpRequest.FormValue("abonnee_nummer"))
		_ = token.Set("abonnee_naam", httpRequest.FormValue("abonnee_naam"))
		_ = token.Set("rol_code", httpRequest.FormValue("rol_code"))
		_ = token.Set("rol_naam", httpRequest.FormValue("rol_naam"))
		_ = token.Set("rol_code_bron", "http://www.dezi.nl/rol_code_bron/big")
		_ = token.Set("rol_code_bron", "https://auth.dezi.nl/revocatie/"+verklaringID)

		serializer := jwt.NewSerializer().Sign(jwt.WithKey(jwa.RS256, signingKey))
		deziTokenBytes, err := serializer.Serialize(token)
		if err != nil {
			slog.ErrorContext(httpRequest.Context(), "Failed to serialize Dezi token", "error", err)
			http.Error(httpResponse, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		if err := completeLoginFunc(httpRequest.Context(), authRequestID, string(deziTokenBytes)); err != nil {
			slog.ErrorContext(httpRequest.Context(), "Failed to complete login", "error", err)
			http.Error(httpResponse, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		http.Redirect(httpResponse, httpRequest, callbackURLFunc(httpRequest.Context(), authRequestID), http.StatusFound)
	}
}
