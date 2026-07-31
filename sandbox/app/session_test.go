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

func TestViewModelRendersEnglishRole(t *testing.T) {
	view := testSession().view()
	require.Equal(t, "SA", view.Initials, "the top bar avatar uses the persona's initials")
	require.Equal(t, "Dr. S. el Amrani", view.Name)
	require.Contains(t, view.Description, "UZI 900001234")
	require.Contains(t, view.Description, "De Plataan Hospital")
}
