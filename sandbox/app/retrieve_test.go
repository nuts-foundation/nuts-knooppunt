package main

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func grantedFrom(source string, sections ...string) sourceRetrieval {
	retrieval := sourceRetrieval{Outcome: chainGranted, Source: sourceAddress{URA: "00000020", Name: source}}
	for _, section := range sections {
		retrieval.Items = append(retrieval.Items, recordItem{Section: section, Title: section + " item", Source: source})
	}
	return retrieval
}

func TestRetrievalStore_WithdrawRemovesEverySessionsCopyAndMarksWhatWasRemoved(t *testing.T) {
	store := newRetrievalStore()
	require.True(t, store.putIfCurrent("owner-1", "anna", store.generationOf("anna"), grantedFrom("Zorgcentrum De Zonnebloem", "Allergies", "Medication")))
	require.True(t, store.putIfCurrent("owner-2", "anna", store.generationOf("anna"), grantedFrom("Zorgcentrum De Zonnebloem", "Conditions")))
	require.True(t, store.putIfCurrent("owner-1", "bram", store.generationOf("bram"), grantedFrom("Zorgcentrum De Zonnebloem", "Allergies")))
	require.True(t, store.holds("owner-1", "anna"))
	held, removed := store.copyState("owner-1", "anna")
	require.True(t, held && !removed)

	require.True(t, store.withdraw("anna"))
	held, removed = store.copyState("owner-1", "anna")
	require.True(t, !held && removed, "one read gives both, and they never contradict each other")

	for _, owner := range []string{"owner-1", "owner-2"} {
		_, ok := store.get(owner, "anna")
		require.Falsef(t, ok, "%s still holds a copy", owner)
		require.False(t, store.holds(owner, "anna"))
	}
	require.Equal(t, removal{Sources: []string{"Zorgcentrum De Zonnebloem"}, Sections: []string{"Allergies", "Conditions", "Medication"}}, store.marks("anna"))
	require.Equal(t, "withdrawal", store.discardCause("anna"))
	_, ok := store.get("owner-1", "bram")
	require.True(t, ok, "another patient's copy stays")
	require.Empty(t, store.marks("bram").Sources)
}

func TestRetrievalStore_ASecondWithdrawalKeepsTheMarksOfTheFirst(t *testing.T) {
	store := newRetrievalStore()
	store.putIfCurrent("owner-1", "anna", store.generationOf("anna"), grantedFrom("Zorgcentrum De Zonnebloem", "Allergies"))
	require.True(t, store.withdraw("anna"))
	require.False(t, store.withdraw("anna"), "nothing was left to remove")
	require.Equal(t, []string{"Allergies"}, store.marks("anna").Sections, "and the earlier marks are still there")
}

func TestRetrievalStore_ARefusedOrEmptyCopyLeavesNoMark(t *testing.T) {
	store := newRetrievalStore()
	store.putIfCurrent("owner-1", "anna", store.generationOf("anna"), sourceRetrieval{Outcome: chainRefused, Source: sourceAddress{Name: "Zorgcentrum De Zonnebloem"}})
	store.putIfCurrent("owner-2", "anna", store.generationOf("anna"), grantedFrom("Zorgcentrum De Zonnebloem")) // granted, and nothing returned
	require.False(t, store.holds("owner-1", "anna"), "a refusal is not a held copy")
	require.False(t, store.holds("owner-2", "anna"), "nor is an answer without items")
	require.False(t, store.withdraw("anna"))
	require.Empty(t, store.marks("anna").Sources)
	for _, owner := range []string{"owner-1", "owner-2"} {
		_, ok := store.get(owner, "anna")
		require.Falsef(t, ok, "what %s stored goes with the withdrawal all the same", owner)
	}
}

func TestRetrievalStore_AGrantedRetrievalAndAWithdrawalCannotUndoEachOtherOutOfOrder(t *testing.T) {
	t.Run("the retrieval started before the withdrawal", func(t *testing.T) {
		store := newRetrievalStore()
		generation := store.generationOf("anna")
		store.withdraw("anna")
		require.False(t, store.putIfCurrent("owner-1", "anna", generation, grantedFrom("Zorgcentrum De Zonnebloem", "Allergies")),
			"an answer on its way when consent was withdrawn is not stored")
		require.False(t, store.holds("owner-1", "anna"))
	})
	t.Run("the retrieval is stored, then consent is withdrawn", func(t *testing.T) {
		store := newRetrievalStore()
		store.putIfCurrent("owner-1", "anna", store.generationOf("anna"), grantedFrom("Zorgcentrum De Zonnebloem", "Allergies"))
		store.withdraw("anna")
		require.Equal(t, []string{"Allergies"}, store.marks("anna").Sections)
		require.False(t, store.holds("owner-1", "anna"), "marks and a held copy never coexist")
	})
	t.Run("a granted retrieval after a withdrawal clears the marks", func(t *testing.T) {
		store := newRetrievalStore()
		store.putIfCurrent("owner-1", "anna", store.generationOf("anna"), grantedFrom("Zorgcentrum De Zonnebloem", "Allergies"))
		store.withdraw("anna")
		require.True(t, store.putIfCurrent("owner-1", "anna", store.generationOf("anna"), grantedFrom("Zorgcentrum De Zonnebloem", "Allergies")))
		require.Empty(t, store.marks("anna").Sources)
		require.True(t, store.holds("owner-1", "anna"))
	})
	t.Run("a granted retrieval that returned nothing clears them too", func(t *testing.T) {
		store := newRetrievalStore()
		store.putIfCurrent("owner-1", "anna", store.generationOf("anna"), grantedFrom("Zorgcentrum De Zonnebloem", "Allergies"))
		store.withdraw("anna")
		require.True(t, store.putIfCurrent("owner-1", "anna", store.generationOf("anna"), grantedFrom("Zorgcentrum De Zonnebloem")))
		require.Empty(t, store.marks("anna").Sources, "the source served the request, so consent is back, whatever it had to return")
		require.False(t, store.holds("owner-1", "anna"), "and there is nothing to hold")
	})
	t.Run("a refused retrieval after a withdrawal keeps them", func(t *testing.T) {
		store := newRetrievalStore()
		store.putIfCurrent("owner-1", "anna", store.generationOf("anna"), grantedFrom("Zorgcentrum De Zonnebloem", "Allergies"))
		store.withdraw("anna")
		store.putIfCurrent("owner-1", "anna", store.generationOf("anna"), sourceRetrieval{Outcome: chainRefused})
		require.Equal(t, []string{"Allergies"}, store.marks("anna").Sections)
	})
}

// A notification's check and commit are one step; runtime_test.go pins the
// boundary between them with a probe. This is the net under that test: an older
// and a newer notification handled at the same moment, many times over. On
// correct code no interleaving ends in the older state, so it cannot fail by
// chance. It fails within a few rounds when anything happens between a check
// and its commit (a yield, a call, I/O), and the race detector reports a commit
// made without the mutex. A gap with nothing in it is not hit in practice, by
// this test or otherwise, which is why the probe test exists.
func TestRetrievalStore_AnOlderAndANewerNotificationAtOnceAlwaysEndInTheNewerState(t *testing.T) {
	for round := 0; round < 2000; round++ {
		state, store := newRuntimeState(), newRetrievalStore()
		store.putIfCurrent("owner-1", "anna", store.generationOf("anna"), grantedFrom("Zorgcentrum De Zonnebloem", "Allergies"))
		start := make(chan struct{})
		var done sync.WaitGroup
		for _, n := range []consentNotification{notificationAt("deny", t0), notificationAt("permit", t0.Add(time.Second))} {
			done.Go(func() {
				<-start
				state.accept("anna", n, func() {
					if n.Decision == "deny" {
						store.withdraw("anna")
					} else {
						store.clearMarks("anna")
					}
				})
			})
		}
		close(start)
		done.Wait()

		latest, _ := state.notification("anna")
		require.Equalf(t, "permit", latest.Decision, "round %d", round)
		require.Emptyf(t, store.marks("anna").Sources, "round %d: a withdrawal was applied after the restoration that overtook it", round)
		revision, _ := state.recordState("anna")
		require.Equalf(t, state.receivedCount("anna"), revision, "round %d: one advance per accepted notification", round)
	}
}

func TestRetrievalStore_ClearPatientAndClearMarks(t *testing.T) {
	store := newRetrievalStore()
	store.putIfCurrent("owner-1", "anna", store.generationOf("anna"), grantedFrom("Zorgcentrum De Zonnebloem", "Allergies"))
	store.withdraw("anna")
	store.clearMarks("anna")
	require.Empty(t, store.marks("anna").Sources, "a permit notification clears the marks")

	store.putIfCurrent("owner-1", "anna", store.generationOf("anna"), grantedFrom("Zorgcentrum De Zonnebloem", "Allergies"))
	store.withdraw("anna")
	store.clearPatient("anna")
	require.Empty(t, store.marks("anna").Sources, "a recycle clears them too")
	require.Equal(t, "reset", store.discardCause("anna"))
}
