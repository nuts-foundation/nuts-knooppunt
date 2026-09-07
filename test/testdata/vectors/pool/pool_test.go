package pool

import (
	"slices"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/nvi"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// The pool must start unshared from De Plataan's side: publishing that side is
// the action E3 demonstrates (DESIGN §5.7).
func TestNVIRegistrations_ExcludePlataan(t *testing.T) {
	for _, p := range Patients() {
		for _, reg := range p.NVIRegistrations() {
			if reg.CustodianURA == "00000010" {
				t.Errorf("patient %s must start unshared from De Plataan", p.Key)
			}
		}
	}
}

func TestCategories_DerivedFromResources(t *testing.T) {
	anna, ok := PatientByKey("anna")
	if !ok {
		t.Fatal("the pool must contain anna")
	}

	wantPlataan := []string{nvi.CategoryCondition, nvi.CategoryMedicationRequest, nvi.CategoryPatient}
	if got := anna.PlataanCategories(); !slices.Equal(got, wantPlataan) {
		t.Errorf("PlataanCategories = %v, want %v", got, wantPlataan)
	}
	wantZonnebloem := []string{nvi.CategoryAllergyIntolerance, nvi.CategoryCondition,
		nvi.CategoryMedicationRequest, nvi.CategoryPatient}
	if got := anna.ZonnebloemCategories(); !slices.Equal(got, wantZonnebloem) {
		t.Errorf("ZonnebloemCategories = %v, want %v", got, wantZonnebloem)
	}
}

// categoriesOf is a type switch with no default, so a resource type nobody
// mapped contributes nothing and silently vanishes from the registration. This
// is the guard that turns that into a failure the moment the pool seeds a new
// type.
func TestCategoriesOf_MapsEveryPooledResourceType(t *testing.T) {
	for _, p := range Patients() {
		for _, resource := range append(p.PlataanResources(), p.ZonnebloemResources()...) {
			if len(categoriesOf([]fhir.HasId{resource})) == 0 {
				t.Errorf("resource %T is seeded but maps to no NVI data category; add it to categoriesOf", resource)
			}
		}
	}
}

func TestNVIRegistrations_ZonnebloemMatchesItsData(t *testing.T) {
	anna, ok := PatientByKey("anna")
	if !ok {
		t.Fatal("the pool must contain anna")
	}

	regs := anna.NVIRegistrations()
	if len(regs) != 1 {
		t.Fatalf("got %d registrations, want 1 (Zonnebloem only)", len(regs))
	}
	if !slices.Equal(regs[0].Categories, anna.ZonnebloemCategories()) {
		t.Errorf("categories = %v, want %v", regs[0].Categories, anna.ZonnebloemCategories())
	}
	if regs[0].ClientID != ZonnebloemClientID || regs[0].BSN != anna.BSN {
		t.Errorf("client/bsn = %q/%q", regs[0].ClientID, regs[0].BSN)
	}
}
