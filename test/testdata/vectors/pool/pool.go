// Package pool holds the GF Sandbox demo-patient pool: a small, fixed set of
// synthetic patients (Anna Jansen plus structural clones), each a complete
// two-source fixture. Every patient has hospital-side data at Ziekenhuis De
// Plataan (URA 00000010), a BGZ at Zorgcentrum De Zonnebloem (URA 00000020),
// and an NVI localization registration under both custodians.
//
// All FHIR resource IDs are derived deterministically from the patient Key so
// that seeding is an idempotent PUT-by-fixed-id upsert. Counts shown in any UI
// must be derived from these resources, never hard-coded (DESIGN §5.3).
package pool

import (
	"fmt"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/nvi"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/plataan"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/sunflower"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

const (
	bsnSystem = "http://fhir.nl/fhir/NamingSystem/bsn"

	// snomedSystem is used for the clinical codes. Real values are structured as
	// BGZ sections so a real BGZ dataset can replace them (DESIGN §5.3).
	snomedSystem = "http://snomed.info/sct"

	// Source Device identifier systems, one per custodian side. The NVI Source
	// records which system published the registration.
	plataanSourceSystem    = "https://plataan.example/device-identifiers"
	zonnebloemSourceSystem = "https://zonnebloem.example/device-identifiers"
)

// PoolPatient is one demo patient: a stable Key, an elfproef-valid synthetic
// BSN, demographics, and the fixed FHIR resource IDs for its Plataan-side and
// Zonnebloem-side Patient resources.
type PoolPatient struct {
	Key       string // stable pool key: sandbox address + lock key + recycle key
	BSN       string // elfproef-valid synthetic BSN, pending RvIG verification
	Name      fhir.HumanName
	BirthDate string

	PlataanPatientID    string
	ZonnebloemPatientID string

	// variant drives the superficial per-patient variation of the clinical data.
	variant variant
}

// variant is a small table of per-patient differences layered over the shared
// BGZ structure, so repeated demos do not look identical (DESIGN §5.3). Every
// patient keeps the same resource types and sections; only surface values move.
type variant struct {
	apixabanDose   string // Plataan anticoagulant dose
	afibOnsetYear  string
	allergyCode    string // SNOMED code of the Zonnebloem allergy substance
	allergyDisplay string
	metoprololDose string
	metforminDose  string
	dm2OnsetYear   string
	htnOnsetYear   string
}

// Patients returns the demo pool: Anna (the canonical persona) plus five
// clones. All BSNs pass the elfproef and none is 999911120 (Nictiz-fixture
// collision); they are marked "pending RvIG verification" (DESIGN §10).
func Patients() []PoolPatient {
	return []PoolPatient{
		newPatient("anna", "999900006", "Anna", "Jansen", "1944-03-12", variant{
			apixabanDose:   "5 mg 2dd",
			afibOnsetYear:  "2025",
			allergyCode:    "373270004",
			allergyDisplay: "Penicilline",
			metoprololDose: "50 mg 1dd",
			metforminDose:  "500 mg 2dd",
			dm2OnsetYear:   "2012",
			htnOnsetYear:   "2009",
		}),
		newPatient("pool-02", "999900018", "Bram", "de Vries", "1951-07-04", variant{
			apixabanDose:   "2.5 mg 2dd",
			afibOnsetYear:  "2023",
			allergyCode:    "294505008",
			allergyDisplay: "Amoxicilline",
			metoprololDose: "25 mg 1dd",
			metforminDose:  "850 mg 2dd",
			dm2OnsetYear:   "2015",
			htnOnsetYear:   "2011",
		}),
		newPatient("pool-03", "999900031", "Cornelia", "Bakker", "1939-11-23", variant{
			apixabanDose:   "5 mg 2dd",
			afibOnsetYear:  "2024",
			allergyCode:    "373270004",
			allergyDisplay: "Penicilline",
			metoprololDose: "100 mg 1dd",
			metforminDose:  "500 mg 3dd",
			dm2OnsetYear:   "2008",
			htnOnsetYear:   "2005",
		}),
		newPatient("pool-04", "999900043", "Dirk", "Visser", "1957-02-18", variant{
			apixabanDose:   "2.5 mg 2dd",
			afibOnsetYear:  "2022",
			allergyCode:    "294530006",
			allergyDisplay: "Cefalosporine",
			metoprololDose: "50 mg 2dd",
			metforminDose:  "1000 mg 2dd",
			dm2OnsetYear:   "2018",
			htnOnsetYear:   "2013",
		}),
		newPatient("pool-05", "999900055", "Elisabeth", "Smit", "1948-09-30", variant{
			apixabanDose:   "5 mg 2dd",
			afibOnsetYear:  "2025",
			allergyCode:    "373270004",
			allergyDisplay: "Penicilline",
			metoprololDose: "25 mg 1dd",
			metforminDose:  "500 mg 2dd",
			dm2OnsetYear:   "2010",
			htnOnsetYear:   "2007",
		}),
		newPatient("pool-06", "999900067", "Frederik", "Meijer", "1953-05-15", variant{
			apixabanDose:   "2.5 mg 2dd",
			afibOnsetYear:  "2024",
			allergyCode:    "294505008",
			allergyDisplay: "Amoxicilline",
			metoprololDose: "100 mg 1dd",
			metforminDose:  "850 mg 3dd",
			dm2OnsetYear:   "2016",
			htnOnsetYear:   "2014",
		}),
	}
}

func newPatient(key, bsn, given, family, birthDate string, v variant) PoolPatient {
	return PoolPatient{
		Key:       key,
		BSN:       bsn,
		BirthDate: birthDate,
		Name: fhir.HumanName{
			Given:  []string{given},
			Family: to.Ptr(family),
		},
		PlataanPatientID:    fmt.Sprintf("pool-%s-plataan-patient", key),
		ZonnebloemPatientID: fmt.Sprintf("pool-%s-zonnebloem-patient", key),
		variant:             v,
	}
}

// PatientByKey returns the pool patient with the given key, or false.
func PatientByKey(key string) (PoolPatient, bool) {
	for _, p := range Patients() {
		if p.Key == key {
			return p, true
		}
	}
	return PoolPatient{}, false
}

// resourceID builds a fixed, human-readable FHIR id derived from the patient
// key, so PUT-upsert is a no-op on re-seed.
func (p PoolPatient) resourceID(side, suffix string) string {
	return fmt.Sprintf("pool-%s-%s-%s", p.Key, side, suffix)
}

func (p PoolPatient) bsnIdentifier() fhir.Identifier {
	return fhir.Identifier{
		System: to.Ptr(bsnSystem),
		Value:  to.Ptr(p.BSN),
	}
}

// plataanPatient is De Plataan's own Patient record (own local id, same BSN).
func (p PoolPatient) plataanPatient() fhir.Patient {
	return fhir.Patient{
		Id:         to.Ptr(p.PlataanPatientID),
		Identifier: []fhir.Identifier{p.bsnIdentifier()},
		Name:       []fhir.HumanName{p.Name},
		BirthDate:  to.Ptr(p.BirthDate),
	}
}

// zonnebloemPatient is De Zonnebloem's own Patient record (own local id, same BSN).
func (p PoolPatient) zonnebloemPatient() fhir.Patient {
	return fhir.Patient{
		Id:         to.Ptr(p.ZonnebloemPatientID),
		Identifier: []fhir.Identifier{p.bsnIdentifier()},
		Name:       []fhir.HumanName{p.Name},
		BirthDate:  to.Ptr(p.BirthDate),
	}
}

func codeableConcept(system, code, display string) *fhir.CodeableConcept {
	return &fhir.CodeableConcept{
		Coding: []fhir.Coding{
			{
				System:  to.Ptr(system),
				Code:    to.Ptr(code),
				Display: to.Ptr(display),
			},
		},
		Text: to.Ptr(display),
	}
}

func clinicalStatusActive() *fhir.CodeableConcept {
	return codeableConcept("http://terminology.hl7.org/CodeSystem/condition-clinical", "active", "Active")
}

// PlataanResources returns De Plataan's hospital-side data for this patient:
// Patient + Condition (atrial fibrillation) + MedicationRequest (apixaban),
// per DESIGN §5.3. All IDs are fixed for idempotent PUT-upsert.
func (p PoolPatient) PlataanResources() []fhir.HasId {
	subject := fhir.Reference{
		Reference: to.Ptr("Patient/" + p.PlataanPatientID),
		Type:      to.Ptr("Patient"),
	}

	afib := fhir.Condition{
		Id:             to.Ptr(p.resourceID("plataan", "condition-afib")),
		ClinicalStatus: clinicalStatusActive(),
		Code:           codeableConcept(snomedSystem, "49436004", "Atriumfibrilleren"),
		Subject:        subject,
		OnsetDateTime:  to.Ptr(p.variant.afibOnsetYear),
		RecordedDate:   to.Ptr(p.variant.afibOnsetYear),
	}

	apixaban := fhir.MedicationRequest{
		Id:                        to.Ptr(p.resourceID("plataan", "medication-apixaban")),
		Status:                    "active",
		Intent:                    "order",
		MedicationCodeableConcept: codeableConcept(snomedSystem, "764141005", "Apixaban"),
		Subject:                   subject,
		DosageInstruction: []fhir.Dosage{
			{Text: to.Ptr("Apixaban " + p.variant.apixabanDose)},
		},
	}

	return []fhir.HasId{
		to.Ptr(p.plataanPatient()),
		to.Ptr(afib),
		to.Ptr(apixaban),
	}
}

// ZonnebloemResources returns De Zonnebloem's BGZ data for this patient:
// Patient + AllergyIntolerance (penicillin-class) + MedicationRequests
// (metoprolol, metformin) + Conditions (type 2 diabetes, hypertension), per
// DESIGN §5.3. All IDs are fixed for idempotent PUT-upsert.
func (p PoolPatient) ZonnebloemResources() []fhir.HasId {
	subject := fhir.Reference{
		Reference: to.Ptr("Patient/" + p.ZonnebloemPatientID),
		Type:      to.Ptr("Patient"),
	}

	allergy := fhir.AllergyIntolerance{
		Id:             to.Ptr(p.resourceID("zonnebloem", "allergy")),
		ClinicalStatus: codeableConcept("http://terminology.hl7.org/CodeSystem/allergyintolerance-clinical", "active", "Active"),
		VerificationStatus: codeableConcept(
			"http://terminology.hl7.org/CodeSystem/allergyintolerance-verification", "unconfirmed", "Unconfirmed"),
		Type:         to.Ptr(fhir.AllergyIntoleranceTypeAllergy),
		Category:     []fhir.AllergyIntoleranceCategory{fhir.AllergyIntoleranceCategoryMedication},
		Code:         codeableConcept(snomedSystem, p.variant.allergyCode, p.variant.allergyDisplay),
		Patient:      subject,
		RecordedDate: to.Ptr("2019"),
	}

	metoprolol := fhir.MedicationRequest{
		Id:                        to.Ptr(p.resourceID("zonnebloem", "medication-metoprolol")),
		Status:                    "active",
		Intent:                    "order",
		MedicationCodeableConcept: codeableConcept(snomedSystem, "372826007", "Metoprolol"),
		Subject:                   subject,
		DosageInstruction: []fhir.Dosage{
			{Text: to.Ptr("Metoprolol " + p.variant.metoprololDose)},
		},
	}

	metformin := fhir.MedicationRequest{
		Id:                        to.Ptr(p.resourceID("zonnebloem", "medication-metformin")),
		Status:                    "active",
		Intent:                    "order",
		MedicationCodeableConcept: codeableConcept(snomedSystem, "372567009", "Metformine"),
		Subject:                   subject,
		DosageInstruction: []fhir.Dosage{
			{Text: to.Ptr("Metformine " + p.variant.metforminDose)},
		},
	}

	dm2 := fhir.Condition{
		Id:             to.Ptr(p.resourceID("zonnebloem", "condition-dm2")),
		ClinicalStatus: clinicalStatusActive(),
		Code:           codeableConcept(snomedSystem, "44054006", "Diabetes mellitus type 2"),
		Subject:        subject,
		OnsetDateTime:  to.Ptr(p.variant.dm2OnsetYear),
		RecordedDate:   to.Ptr(p.variant.dm2OnsetYear),
	}

	hypertension := fhir.Condition{
		Id:             to.Ptr(p.resourceID("zonnebloem", "condition-hypertension")),
		ClinicalStatus: clinicalStatusActive(),
		Code:           codeableConcept(snomedSystem, "38341003", "Hypertensie"),
		Subject:        subject,
		OnsetDateTime:  to.Ptr(p.variant.htnOnsetYear),
		RecordedDate:   to.Ptr(p.variant.htnOnsetYear),
	}

	return []fhir.HasId{
		to.Ptr(p.zonnebloemPatient()),
		to.Ptr(allergy),
		to.Ptr(metoprolol),
		to.Ptr(metformin),
		to.Ptr(dm2),
		to.Ptr(hypertension),
	}
}

// NVILists returns the NVI localization registrations for this patient: one per
// custodian (De Plataan 00000010 and De Zonnebloem 00000020), so the patient is
// findable from either side by BSN (pseudonymized).
func (p PoolPatient) NVILists() []nvi.SeedList {
	return []nvi.SeedList{
		{
			CustodianURA: plataan.URA,
			BSN:          p.BSN,
			SourceSystem: plataanSourceSystem,
			SourceValue:  "PLATAAN-EHR-" + p.Key,
		},
		{
			CustodianURA: *sunflower.Organization().Identifier[0].Value,
			BSN:          p.BSN,
			SourceSystem: zonnebloemSourceSystem,
			SourceValue:  "ZONNEBLOEM-EHR-" + p.Key,
		},
	}
}

// PlataanResources returns the Plataan-side FHIR resources for the whole pool.
func PlataanResources() []fhir.HasId {
	var resources []fhir.HasId
	for _, p := range Patients() {
		resources = append(resources, p.PlataanResources()...)
	}
	return resources
}

// ZonnebloemResources returns the Zonnebloem-side FHIR resources for the whole
// pool. These land in the existing sunflower-patients tenant.
func ZonnebloemResources() []fhir.HasId {
	var resources []fhir.HasId
	for _, p := range Patients() {
		resources = append(resources, p.ZonnebloemResources()...)
	}
	return resources
}
