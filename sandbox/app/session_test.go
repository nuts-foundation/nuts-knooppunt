package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func testSession() authSession {
	return authSession{
		DeziNumber:  "900001234",
		Initials:    "S.",
		Surname:     "el Amrani",
		OrgURA:      "00000010",
		OrgName:     "Ziekenhuis De Plataan",
		RoleCode:    "01.022",
		RoleName:    "Klinisch geriater",
		Attestation: "header.payload.signature",
		ExpiresAt:   time.Now().Add(time.Hour),
	}
}

func TestSessionRoundTrip(t *testing.T) {
	store := newSessionStore()
	id := store.create(testSession())
	require.NotEmpty(t, id)

	got, ok := store.get(id)
	require.True(t, ok)
	require.Equal(t, "900001234", got.DeziNumber)
	require.Equal(t, "01.022", got.RoleCode)
}

func TestSessionIDsAreUnique(t *testing.T) {
	store := newSessionStore()
	require.NotEqual(t, store.create(testSession()), store.create(testSession()))
}

func TestExpiredSessionIsNotReturned(t *testing.T) {
	store := newSessionStore()
	expired := testSession()
	expired.ExpiresAt = time.Now().Add(-time.Minute)
	id := store.create(expired)

	_, ok := store.get(id)
	require.False(t, ok, "an expired session must not be usable")
}

func TestExpiredSessionIsReapedOnRead(t *testing.T) {
	store := newSessionStore()
	expired := testSession()
	expired.ExpiresAt = time.Now().Add(-time.Minute)
	id := store.create(expired)
	require.Len(t, store.sessions, 1)

	// Rejecting the read is not enough on its own: without dropping the entry
	// a long-running sandbox keeps every expired session in memory.
	_, ok := store.get(id)
	require.False(t, ok)
	require.Empty(t, store.sessions, "reading an expired session must also remove it")
}

func TestDropRemovesOneSession(t *testing.T) {
	store := newSessionStore()
	keep := store.create(testSession())
	remove := store.create(testSession())

	store.drop(remove)

	_, ok := store.get(remove)
	require.False(t, ok)
	_, ok = store.get(keep)
	require.True(t, ok, "dropping one session must not affect another")
}

func TestDropAllClearsEverySession(t *testing.T) {
	store := newSessionStore()
	first := store.create(testSession())
	second := store.create(testSession())

	store.dropAll()

	_, ok := store.get(first)
	require.False(t, ok)
	_, ok = store.get(second)
	require.False(t, ok)
}

// The view shows what the attestation carries, and nothing it does not. A v0.7
// verklaring has initials, a surname prefix and a surname, and no title; it
// names the role and the organization in Dutch. Both of those used to be
// dressed up here, with a hardcoded "Dr." and a lookup table of English labels
// that only ever covered the demo persona.
func TestViewModelShowsOnlyWhatTheAttestationCarries(t *testing.T) {
	view := testSession().view()
	require.Equal(t, "SA", view.Initials, "the top bar avatar uses the persona's initials")
	require.Equal(t, "S. el Amrani", view.Name)
	require.NotContains(t, view.Name, "Dr.",
		"the attestation carries no title, and an EHR has no way to know one either")
	require.Equal(t, "Klinisch geriater · UZI 900001234 · Ziekenhuis De Plataan", view.Description,
		"the role and organization are shown as Dezi spells them")
}

func TestFirstLetterHandlesMultibyteRunes(t *testing.T) {
	require.Equal(t, "Ö", firstLetter("özdemir"),
		"a multi-byte leading rune must decode whole, not truncate to an invalid byte")
}

func TestCreateSweepsExpiredSessions(t *testing.T) {
	store := newSessionStore()
	expired := testSession()
	expired.ExpiresAt = time.Now().Add(-time.Minute)
	abandoned := store.create(expired)
	require.Len(t, store.sessions, 1)

	// Reaping on read never reaches a session whose cookie was thrown away, so
	// the sweep on insert is what actually bounds this map.
	store.create(testSession())
	require.Len(t, store.sessions, 1, "creating a session must sweep the expired ones")
	_, ok := store.sessions[abandoned]
	require.False(t, ok, "the abandoned session must be the one dropped")
}

func TestSecureCookiesAcceptsAnUppercaseScheme(t *testing.T) {
	// RFC 3986 section 3.1 makes the scheme case-insensitive, so this is a
	// valid https URL and must not silently yield a cookie without Secure.
	t.Setenv("SANDBOX_PUBLIC_URL", "HTTPS://sandbox.example.com")
	require.True(t, secureCookies())

	t.Setenv("SANDBOX_PUBLIC_URL", "http://localhost:8091")
	require.False(t, secureCookies())
}
