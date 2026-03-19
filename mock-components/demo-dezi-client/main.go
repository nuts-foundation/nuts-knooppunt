package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-jose/go-jose/v4"
)

// Configuration
var (
	deziAuthority   = getEnv("DEZI_AUTHORITY", "https://auth.dezi.nl")
	clientID        = getEnv("DEZI_CLIENT_ID", "")
	redirectURI     = getEnv("DEZI_REDIRECT_URI", "")
	serverPort      = getEnv("SERVER_PORT", "8090")
	keyFile         = getEnv("KEY_FILE", "./cert-key.pem")
	frontendBaseURL = getEnv("FRONTEND_BASE_URL", "http://localhost:3000")
)

// Session storage (in-memory, for demo purposes)
var sessions = make(map[string]*Session)

type Session struct {
	CodeVerifier string
	State        string
	Nonce        string
	ReturnURL    string
	CreatedAt    time.Time
}

type OIDCConfig struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	UserinfoEndpoint                  string   `json:"userinfo_endpoint"`
	JwksURI                           string   `json:"jwks_uri"`
	ResponseTypesSupported            []string `json:"response_types_supported"`
	SubjectTypesSupported             []string `json:"subject_types_supported"`
	IDTokenSigningAlgValuesSupported  []string `json:"id_token_signing_alg_values_supported"`
	ScopesSupported                   []string `json:"scopes_supported"`
	GrantTypesSupported               []string `json:"grant_types_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported"`
}

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	IDToken     string `json:"id_token"`
	IDTokenType string `json:"id_token_type,omitempty"`
	Scope       string `json:"scope"`
}

type UserinfoEnvelope struct {
	JTI          string `json:"jti"`
	IAT          int64  `json:"iat"`
	EXP          int64  `json:"exp"`
	ISS          string `json:"iss"`
	AUD          string `json:"aud"`
	LOAAuthn     string `json:"loa_authn"`
	JSONSchema   string `json:"json_schema"`
	VerklaringID string `json:"verklaring_id"`
	Verklaring   string `json:"verklaring"`
}

type Verklaring struct {
	JTI           string  `json:"jti"`
	ISS           string  `json:"iss"`
	EXP           int64   `json:"exp"`
	NBF           int64   `json:"nbf"`
	JSONSchema    string  `json:"json_schema"`
	LOADezi       string  `json:"loa_dezi"`
	VerklaringID  string  `json:"verklaring_id"`
	DeziNummer    string  `json:"dezi_nummer"`
	Voorletters   string  `json:"voorletters"`
	Voorvoegsel   *string `json:"voorvoegsel"`
	Achternaam    string  `json:"achternaam"`
	AbonneeNummer string  `json:"abonnee_nummer"`
	AbonneeNaam   string  `json:"abonnee_naam"`
	RolCode       string  `json:"rol_code"`
	RolNaam       string  `json:"rol_naam"`
	RolCodeBron   string  `json:"rol_code_bron"`
	StatusURI     string  `json:"status_uri"`
}

type UserInfo struct {
	Sub           string      `json:"sub"`
	DeziNummer    string      `json:"dezi_nummer"`
	Name          string      `json:"name"`
	GivenName     string      `json:"given_name"`
	FamilyName    string      `json:"family_name"`
	AbonneeNummer string      `json:"abonnee_nummer"`
	AbonneeNaam   string      `json:"abonnee_naam"`
	RolCode       string      `json:"rol_code"`
	RolNaam       string      `json:"rol_naam"`
	VerklaringID  string      `json:"verklaring_id"`
	Verklaring    *Verklaring `json:"verklaring_details,omitempty"`
}

func main() {
	log.Printf("Starting Dezi OIDC Client on port %s", serverPort)
	log.Printf("Dezi Authority: %s", deziAuthority)
	log.Printf("Client ID: %s", clientID)
	log.Printf("Redirect URI: %s", redirectURI)

	// Setup routes
	http.HandleFunc("/login", handleLogin)
	http.HandleFunc("/callback", handleCallback)
	http.HandleFunc("/userinfo", handleUserinfo)
	http.HandleFunc("/logout", handleLogout)
	http.HandleFunc("/.well-known/openid-configuration", handleOIDCConfig)

	// CORS middleware
	handler := corsMiddleware(http.DefaultServeMux)

	log.Printf("Server listening on http://localhost:%s", serverPort)
	if err := http.ListenAndServe(":"+serverPort, handler); err != nil {
		log.Fatal(err)
	}
}

// CORS middleware to allow frontend requests
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", frontendBaseURL)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Handle login initiation
func handleLogin(w http.ResponseWriter, r *http.Request) {
	log.Println("Handling login request")

	// Get return URL from query params (where to redirect after successful login)
	returnURL := r.URL.Query().Get("return_url")
	if returnURL == "" {
		returnURL = frontendBaseURL
	}

	// Generate PKCE parameters
	codeVerifier, err := generateCodeVerifier()
	if err != nil {
		http.Error(w, "Failed to generate code verifier", http.StatusInternalServerError)
		return
	}

	codeChallenge := generateCodeChallenge(codeVerifier)

	// Generate state and nonce
	state := generateRandomString(32)
	nonce := generateRandomString(32)

	// Store session with state as key (simpler for demo)
	sessions[state] = &Session{
		CodeVerifier: codeVerifier,
		State:        state,
		Nonce:        nonce,
		ReturnURL:    returnURL,
		CreatedAt:    time.Now(),
	}

	// Build authorization URL
	authURL := fmt.Sprintf("%s/authorize?response_type=code&client_id=%s&redirect_uri=%s&scope=openid&state=%s&code_challenge=%s&code_challenge_method=S256&nonce=%s&display=page&prompt=login",
		deziAuthority,
		url.QueryEscape(clientID),
		url.QueryEscape(redirectURI),
		url.QueryEscape(state),
		url.QueryEscape(codeChallenge),
		url.QueryEscape(nonce),
	)

	log.Printf("Redirecting to Dezi: %s", authURL)

	// Redirect to Dezi
	http.Redirect(w, r, authURL, http.StatusFound)
}

// Handle OAuth callback
func handleCallback(w http.ResponseWriter, r *http.Request) {
	log.Println("Handling callback from Dezi")

	// Get state from query param
	state := r.URL.Query().Get("state")
	if state == "" {
		http.Error(w, "Missing state parameter", http.StatusBadRequest)
		return
	}

	// Look up session by state
	session, exists := sessions[state]
	if !exists {
		http.Error(w, "Invalid or expired session", http.StatusBadRequest)
		return
	}

	// Get authorization code
	code := r.URL.Query().Get("code")
	if code == "" {
		errorParam := r.URL.Query().Get("error")
		errorDesc := r.URL.Query().Get("error_description")
		log.Printf("Error from Dezi: %s - %s", errorParam, errorDesc)
		http.Error(w, fmt.Sprintf("Authorization error: %s", errorDesc), http.StatusBadRequest)
		return
	}

	log.Printf("Received authorization code: %s", code)

	// Exchange code for tokens
	tokenResp, err := exchangeCodeForToken(code, session.CodeVerifier)
	if err != nil {
		log.Printf("Failed to exchange code for token: %v", err)
		http.Error(w, fmt.Sprintf("Token exchange failed: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("Successfully obtained access token")

	// Store the access token in a new session
	accessSessionID := generateRandomString(32)
	sessions[accessSessionID] = &Session{
		CodeVerifier: tokenResp.AccessToken, // Reuse CodeVerifier field to store access token
		Nonce:        session.Nonce,
		ReturnURL:    session.ReturnURL,
		CreatedAt:    time.Now(),
	}

	// Set access token cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "sessionID",
		Value:    accessSessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   tokenResp.ExpiresIn,
	})

	// Clean up the original session
	delete(sessions, state)

	// Redirect back to frontend
	returnURL := session.ReturnURL
	if returnURL == "" {
		returnURL = frontendBaseURL
	}

	log.Printf("Redirecting to: %s", returnURL)
	http.Redirect(w, r, returnURL, http.StatusFound)
}

// Handle userinfo request
func handleUserinfo(w http.ResponseWriter, r *http.Request) {
	log.Println("Handling userinfo request")

	// Get access token from session cookie
	cookie, err := r.Cookie("sessionID")
	if err != nil {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	session, exists := sessions[cookie.Value]
	if !exists {
		http.Error(w, "Invalid session", http.StatusUnauthorized)
		return
	}

	accessToken := session.CodeVerifier // We stored the access token here

	// Get userinfo from Dezi
	userinfo, err := getUserinfo(accessToken)
	if err != nil {
		log.Printf("Failed to get userinfo: %v", err)
		http.Error(w, fmt.Sprintf("Failed to get userinfo: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(userinfo)
}

// Handle logout
func handleLogout(w http.ResponseWriter, r *http.Request) {
	log.Println("Handling logout")

	// Clear session cookie
	http.SetCookie(w, &http.Cookie{
		Name:   "sessionID",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "logged out"})
}

// Provide OIDC configuration for the demo-ehr frontend
func handleOIDCConfig(w http.ResponseWriter, r *http.Request) {
	baseURL := fmt.Sprintf("http://localhost:%s", serverPort)
	config := map[string]interface{}{
		"issuer":                                baseURL,
		"authorization_endpoint":                baseURL + "/login",
		"token_endpoint":                        baseURL + "/token",
		"userinfo_endpoint":                     baseURL + "/userinfo",
		"end_session_endpoint":                  baseURL + "/logout",
		"response_types_supported":              []string{"code"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"scopes_supported":                      []string{"openid"},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}

// Exchange authorization code for access token
func exchangeCodeForToken(code, codeVerifier string) (*TokenResponse, error) {
	log.Println("Exchanging code for token")

	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)
	data.Set("client_id", clientID)
	data.Set("code_verifier", codeVerifier)

	req, err := http.NewRequest("POST", deziAuthority+"/token", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send token request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	log.Printf("Token exchange successful, expires in %d seconds", tokenResp.ExpiresIn)
	log.Printf("ID Token: %s", tokenResp.IDToken)
	return &tokenResp, nil
}

// Get userinfo from Dezi
func getUserinfo(accessToken string) (*UserInfo, error) {
	log.Println("Getting userinfo from Dezi")

	req, err := http.NewRequest("GET", deziAuthority+"/userinfo", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create userinfo request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send userinfo request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read userinfo response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("userinfo request failed with status %d: %s", resp.StatusCode, string(body))
	}

	log.Printf("Received userinfo response")
	log.Printf("Content-Type: %s", resp.Header.Get("Content-Type"))
	log.Printf("Response length: %d bytes", len(body))
	log.Printf("Full encrypted/signed response:\n%s", string(body))

	// The response is a JWE token - decrypt it
	envelope, err := decryptUserinfoJWE(string(body))
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt userinfo JWE: %w", err)
	}

	log.Printf("Successfully decrypted userinfo envelope")

	// Parse the verklaring (declaration) JWT
	verklaring, err := parseVerklaringJWT(envelope.Verklaring)
	if err != nil {
		log.Printf("Warning: failed to parse verklaring: %v", err)
		// Continue anyway - we can still return basic info
	}

	// Build UserInfo response
	userInfo := &UserInfo{
		Sub:          fmt.Sprintf("dezi:%s", envelope.VerklaringID),
		VerklaringID: envelope.VerklaringID,
		Verklaring:   verklaring,
	}

	if verklaring != nil {
		userInfo.DeziNummer = verklaring.DeziNummer
		userInfo.Name = formatName(verklaring.Voorletters, verklaring.Voorvoegsel, verklaring.Achternaam)
		userInfo.GivenName = verklaring.Voorletters
		userInfo.FamilyName = verklaring.Achternaam
		userInfo.AbonneeNummer = verklaring.AbonneeNummer
		userInfo.AbonneeNaam = verklaring.AbonneeNaam
		userInfo.RolCode = verklaring.RolCode
		userInfo.RolNaam = verklaring.RolNaam
	}

	return userInfo, nil
}

// Decrypt JWE userinfo response (or parse JWT if not encrypted)
func decryptUserinfoJWE(jweToken string) (*UserinfoEnvelope, error) {
	log.Printf("Attempting to decrypt/parse userinfo response")
	log.Printf("Response length: %d bytes", len(jweToken))
	log.Printf("Full response:\n%s", jweToken)

	// Count the parts to determine format
	parts := strings.Split(jweToken, ".")
	log.Printf("Response has %d parts", len(parts))

	if len(parts) == 3 {
		// This is a JWT (JWS - signed), not JWE (encrypted)
		log.Printf("Response is a signed JWT (3 parts), not an encrypted JWE (5 parts)")
		log.Printf("Parsing as JWT instead...")

		// Parse as JWT - decode the payload directly
		payload, err := base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			return nil, fmt.Errorf("failed to decode JWT payload: %w", err)
		}

		log.Printf("Decoded JWT payload: %s", string(payload))

		var envelope UserinfoEnvelope
		if err := json.Unmarshal(payload, &envelope); err != nil {
			return nil, fmt.Errorf("failed to parse envelope: %w", err)
		}

		// Log the envelope
		envelopeJSON, _ := json.MarshalIndent(envelope, "", "  ")
		log.Printf("Userinfo Envelope:\n%s", string(envelopeJSON))

		return &envelope, nil
	}

	if len(parts) != 5 {
		return nil, fmt.Errorf("invalid format: expected 5 parts (JWE) or 3 parts (JWT), got %d", len(parts))
	}

	log.Printf("Response is a JWE (5 parts), decrypting...")

	// Load private key for JWE decryption
	keyData, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key: %w", err)
	}

	block, _ := pem.Decode(keyData)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	var privateKey *rsa.PrivateKey

	// Try parsing as PKCS8
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if rsaKey, ok := key.(*rsa.PrivateKey); ok {
			privateKey = rsaKey
		}
	}

	// Try parsing as PKCS1
	if privateKey == nil {
		if rsaKey, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
			privateKey = rsaKey
		}
	}

	if privateKey == nil {
		return nil, fmt.Errorf("failed to parse private key")
	}

	// Parse and decrypt JWE
	jwe, err := jose.ParseEncrypted(jweToken, []jose.KeyAlgorithm{jose.RSA_OAEP}, []jose.ContentEncryption{jose.A256CBC_HS512})
	if err != nil {
		return nil, fmt.Errorf("failed to parse JWE: %w", err)
	}

	decrypted, err := jwe.Decrypt(privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt JWE: %w", err)
	}

	// Parse the envelope
	var envelope UserinfoEnvelope
	if err := json.Unmarshal(decrypted, &envelope); err != nil {
		return nil, fmt.Errorf("failed to parse envelope: %w", err)
	}

	// Log the decrypted envelope
	envelopeJSON, _ := json.MarshalIndent(envelope, "", "  ")
	log.Printf("Decrypted Userinfo Envelope:\n%s", string(envelopeJSON))

	return &envelope, nil
}

// Parse verklaring JWT
func parseVerklaringJWT(verklaringToken string) (*Verklaring, error) {
	// The verklaring is a signed JWT (JWS)
	// For now, we'll just parse it without verification (in production, verify the signature)

	parts := strings.Split(verklaringToken, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT format")
	}

	// Decode the payload (second part)
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to decode JWT payload: %w", err)
	}

	var verklaring Verklaring
	if err := json.Unmarshal(payload, &verklaring); err != nil {
		return nil, fmt.Errorf("failed to parse verklaring: %w", err)
	}

	// Log the decoded verklaring
	verklaringJSON, _ := json.MarshalIndent(verklaring, "", "  ")
	log.Printf("Decoded Verklaring:\n%s", string(verklaringJSON))

	return &verklaring, nil
}

// Helper functions

func generateCodeVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func generateCodeChallenge(verifier string) string {
	hash := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}

func generateRandomString(length int) string {
	b := make([]byte, length)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)[:length]
}

func formatName(voorletters string, voorvoegsel *string, achternaam string) string {
	name := voorletters
	if voorvoegsel != nil && *voorvoegsel != "" {
		name += " " + *voorvoegsel
	}
	name += " " + achternaam
	return name
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
