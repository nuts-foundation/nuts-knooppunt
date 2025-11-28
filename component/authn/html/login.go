package html

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net/http"

	"github.com/google/uuid"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"github.com/rs/zerolog/log"
)

func RenderLogin(httpResponse http.ResponseWriter, httpRequest *http.Request) {
	httpResponse.Header().Add("Content-Type", "text/html; charset=utf-8")
	err := loginTemplate.Execute(httpResponse, struct {
		AuthRequestID string
	}{
		AuthRequestID: httpRequest.URL.Query().Get("authRequestID"),
	})
	if err != nil {
		log.Ctx(httpRequest.Context()).Err(err).Msg("Failed to render login template")
		http.Error(httpResponse, "Internal Server Error", http.StatusInternalServerError)
	}
}

func HandleLoginSubmit(callbackURLFunc func(context.Context, string) string, completeLoginFunc func(authRequestID string, deziToken string) error) func(http.ResponseWriter, *http.Request) {
	sigingKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	return func(httpResponse http.ResponseWriter, httpRequest *http.Request) {
		if err := httpRequest.ParseForm(); err != nil {
			log.Ctx(httpRequest.Context()).Err(err).Msg("Failed to parse login form")
			http.Error(httpResponse, "Bad Request", http.StatusBadRequest)
			return
		}

		authRequestID := httpRequest.FormValue("authRequestID")
		if authRequestID == "" {
			log.Ctx(httpRequest.Context()).Error().Msg("Missing authRequestID")
			http.Error(httpResponse, "Bad Request: missing authRequestID", http.StatusBadRequest)
			return
		}

		// Check if user denied
		action := httpRequest.FormValue("action")
		if action == "deny" {
			log.Ctx(httpRequest.Context()).Info().Msg("User denied authorization")
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

		serializer := jwt.NewSerializer().Sign(jwt.WithKey(jwa.RS256, sigingKey))
		deziTokenBytes, err := serializer.Serialize(token)
		if err != nil {
			log.Ctx(httpRequest.Context()).Err(err).Msg("Failed to serialize Dezi token")
			http.Error(httpResponse, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		if err := completeLoginFunc(authRequestID, string(deziTokenBytes)); err != nil {
			log.Ctx(httpRequest.Context()).Err(err).Msg("Failed to complete login")
			http.Error(httpResponse, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		http.Redirect(httpResponse, httpRequest, callbackURLFunc(httpRequest.Context(), authRequestID), http.StatusFound)
	}
}
