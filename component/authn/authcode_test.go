package authn

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthRequest_Authenticate(t *testing.T) {
	t.Run("successful authentication", func(t *testing.T) {
		authReq := &AuthRequest{
			ID:       "test-auth-req-1",
			AuthDone: false,
		}

		// Create a valid Dezi token
		token := jwt.New()
		_ = token.Set("dezi_nummer", "123456789")
		_ = token.Set("voorletters", "A.B.")
		_ = token.Set("achternaam", "Test")

		signingKey, _ := rsa.GenerateKey(rand.Reader, 2048)
		serializer := jwt.NewSerializer().Sign(jwt.WithKey(jwa.RS256, signingKey))
		deziToken, err := serializer.Serialize(token)
		require.NoError(t, err)

		// Authenticate
		err = authReq.Authenticate(string(deziToken))
		require.NoError(t, err)

		// Verify the auth request was updated correctly
		assert.True(t, authReq.AuthDone)
		assert.Equal(t, "123456789", authReq.Subject)
		assert.Equal(t, string(deziToken), authReq.DeziToken)
		assert.NotNil(t, authReq.ParsedDeziToken)
		assert.False(t, authReq.AuthTime.IsZero())
		assert.WithinDuration(t, time.Now(), authReq.AuthTime, 1*time.Second)

		// Verify parsed token contains claims
		claims := (*authReq.ParsedDeziToken).PrivateClaims()
		assert.Equal(t, "123456789", claims["dezi_nummer"])
		assert.Equal(t, "A.B.", claims["voorletters"])
		assert.Equal(t, "Test", claims["achternaam"])
	})

	t.Run("already authenticated error", func(t *testing.T) {
		authReq := &AuthRequest{
			ID:       "test-auth-req-2",
			AuthDone: true, // Already authenticated
		}

		token := jwt.New()
		_ = token.Set("dezi_nummer", "123456789")
		signingKey, _ := rsa.GenerateKey(rand.Reader, 2048)
		serializer := jwt.NewSerializer().Sign(jwt.WithKey(jwa.RS256, signingKey))
		deziToken, _ := serializer.Serialize(token)

		// Attempt to authenticate again
		err := authReq.Authenticate(string(deziToken))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already authenticated")
	})

	t.Run("invalid JWT token", func(t *testing.T) {
		authReq := &AuthRequest{
			ID:       "test-auth-req-3",
			AuthDone: false,
		}

		// Invalid JWT
		err := authReq.Authenticate("invalid.jwt.token")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse Dezi token")

		// Auth should not be marked as done
		assert.False(t, authReq.AuthDone)
		assert.Empty(t, authReq.Subject)
	})

	t.Run("missing dezi_nummer claim", func(t *testing.T) {
		authReq := &AuthRequest{
			ID:       "test-auth-req-4",
			AuthDone: false,
		}

		// Create token without dezi_nummer
		token := jwt.New()
		_ = token.Set("voorletters", "A.B.")
		_ = token.Set("achternaam", "Test")

		signingKey, _ := rsa.GenerateKey(rand.Reader, 2048)
		serializer := jwt.NewSerializer().Sign(jwt.WithKey(jwa.RS256, signingKey))
		deziToken, _ := serializer.Serialize(token)

		// Authenticate
		err := authReq.Authenticate(string(deziToken))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "dezi_nummer claim missing or invalid")

		// Auth should not be marked as done
		assert.False(t, authReq.AuthDone)
		assert.Empty(t, authReq.Subject)
	})

	t.Run("invalid dezi_nummer claim type", func(t *testing.T) {
		authReq := &AuthRequest{
			ID:       "test-auth-req-5",
			AuthDone: false,
		}

		// Create token with invalid dezi_nummer type
		token := jwt.New()
		_ = token.Set("dezi_nummer", 123456789) // number instead of string
		_ = token.Set("achternaam", "Test")

		signingKey, _ := rsa.GenerateKey(rand.Reader, 2048)
		serializer := jwt.NewSerializer().Sign(jwt.WithKey(jwa.RS256, signingKey))
		deziToken, _ := serializer.Serialize(token)

		// Authenticate
		err := authReq.Authenticate(string(deziToken))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "dezi_nummer claim missing or invalid")

		// Auth should not be marked as done
		assert.False(t, authReq.AuthDone)
		assert.Empty(t, authReq.Subject)
	})
}
