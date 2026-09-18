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

// categoriesOf's type switch has no default, so a resource type nobody mapped
// vanishes from the registration silently. This turns that into a failure the
// moment the pool seeds a new type.
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

// The BGZ policy authorizes a MedicationRequest search only when it carries
// category=http://snomed.info/sct|16076005 (component/pdp/policies/bgz/policy.rego).
// A seeded MedicationRequest without that category is invisible to every query
// the policy permits, so the demo's medication section would come back empty
// while each call in the chain reported success.
func TestMedicationRequests_CarryTheBGZCategory(t *testing.T) {
	for _, p := range Patients() {
		for _, resource := range append(p.PlataanResources(), p.ZonnebloemResources()...) {
			request, isMedication := resource.(*fhir.MedicationRequest)
			if !isMedication {
				continue
			}
			if !hasCoding(request.Category, snomedSystem, bgzMedicationCategory) {
				t.Errorf("patient %s: MedicationRequest %s carries no %s|%s category, so the authorized BGZ query cannot find it",
					p.Key, *request.Id, snomedSystem, bgzMedicationCategory)
			}
		}
	}
}

func hasCoding(concepts []fhir.CodeableConcept, system, code string) bool {
	for _, concept := range concepts {
		for _, coding := range concept.Coding {
			if coding.System != nil && *coding.System == system && coding.Code != nil && *coding.Code == code {
				return true
			}
		}
	}
	return false
}
