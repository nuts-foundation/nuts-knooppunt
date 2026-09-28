package pool

// A record a person added during a demo, in De Zonnebloem's EHR
// (sandbox/zonnebloem), carries this tag in meta.tag. The seeded fixtures carry
// none, so a search on the tag finds only what was added. RecyclePatient deletes
// by it: those records have server-assigned ids that a fixed-id re-PUT cannot
// overwrite.
const (
	UserCreatedTagSystem = "https://github.com/nuts-foundation/nuts-knooppunt/sandbox"
	UserCreatedTagCode   = "user-created"
)
