import {headers} from "./fhir";
import {config} from "../config";

export const bgzVisualizationApi = {
    // Helper to build patient parameter for queries
    buildPatientParam(patientId) {
        return `patient=${patientId}`;
    },

    // Helper to fetch and parse bundle
    async fetchBundle(url) {
        const res = await fetch(url, {headers});
        if (!res.ok) {
            console.warn(`Query failed: ${url} - ${res.statusText}`);
            return [];
        }
        const bundle = await res.json();
        return (bundle.entry || []).map(e => e.resource);
    },

    async getPatientSummary(patientId) {
        const base = config.fhirStu3BaseURL;
        const patientParam = this.buildPatientParam(patientId);

        const summary = {
            patient: null,
            paymentDetails: [],
            treatmentDirectives: [],
            advanceDirectives: [],
            functionalStatus: [],
            problems: [],
            socialHistory: {
                livingSituation: [],
                drugUse: [],
                alcoholUse: [],
                tobaccoUse: [],
                nutritionAdvice: []
            },
            alerts: [],
            allergies: [],
            medication: {
                medicationUse: [],
                medicationAgreement: [],
                administrationAgreement: []
            },
            medicalAids: [],
            vaccinations: [],
            vitalSigns: {
                bloodPressure: [],
                bodyWeight: [],
                bodyHeight: []
            },
            results: [],
            procedures: [],
            encounters: [],
            plannedCare: []
        };

        try {
            // 1. Patient information
            const patientRes = await fetch(`${base}/Patient?_id=${patientId}&_include=Patient:general-practitioner`, {headers});
            if (patientRes.ok) {
                const bundle = await patientRes.json();
                summary.patient = (bundle.entry || []).find(e => e.resource.resourceType === 'Patient')?.resource;
            }

            // 2. Payment details
            summary.paymentDetails = await this.fetchBundle(
                `${base}/Coverage?${patientParam}&_include=Coverage:payor:Patient&_include=Coverage:payor:Organization`
            );

            // 3. Treatment directives
            summary.treatmentDirectives = await this.fetchBundle(
                `${base}/Consent?${patientParam}&category=http://snomed.info/sct|11291000146105`
            );
            summary.advanceDirectives = await this.fetchBundle(
                `${base}/Consent?${patientParam}&category=http://snomed.info/sct|11341000146107`
            );

            // 5. Functional status
            summary.functionalStatus = await this.fetchBundle(
                `${base}/Observation?${patientParam}&category=http://snomed.info/sct|118228005,http://snomed.info/sct|384821006`
            );

            // 6. Problems
            summary.problems = await this.fetchBundle(`${base}/Condition?${patientParam}`);

            // 7. Social history
            summary.socialHistory.livingSituation = await this.fetchBundle(
                `${base}/Observation?${patientParam}&code=http://snomed.info/sct|365508006`
            );
            summary.socialHistory.drugUse = await this.fetchBundle(
                `${base}/Observation?${patientParam}&code=http://snomed.info/sct|228366006`
            );
            summary.socialHistory.alcoholUse = await this.fetchBundle(
                `${base}/Observation?${patientParam}&code=http://snomed.info/sct|228273003`
            );
            summary.socialHistory.tobaccoUse = await this.fetchBundle(
                `${base}/Observation?${patientParam}&code=http://snomed.info/sct|365980008`
            );
            summary.socialHistory.nutritionAdvice = await this.fetchBundle(
                `${base}/NutritionOrder?${patientParam}`
            );

            // 8. Alerts
            summary.alerts = await this.fetchBundle(`${base}/Flag?${patientParam}`);

            // 9. Allergies
            summary.allergies = await this.fetchBundle(`${base}/AllergyIntolerance?${patientParam}`);

            // 10. Medication
            summary.medication.medicationUse = await this.fetchBundle(
                `${base}/MedicationStatement?${patientParam}&category=urn:oid:2.16.840.1.113883.2.4.3.11.60.20.77.5.3|6&_include=MedicationStatement:medication`
            );
            summary.medication.medicationAgreement = await this.fetchBundle(
                `${base}/MedicationRequest?${patientParam}&category=http://snomed.info/sct|16076005&_include=MedicationRequest:medication`
            );

            // 11. Medical aids
            summary.medicalAids = await this.fetchBundle(
                `${base}/DeviceUseStatement?${patientParam}&_include=DeviceUseStatement:device`
            );

            // 12. Vaccinations
            summary.vaccinations = await this.fetchBundle(
                `${base}/Immunization?${patientParam}&status=completed`
            );

            // 13. Vital signs
            summary.vitalSigns.bloodPressure = await this.fetchBundle(
                `${base}/Observation?${patientParam}&code=http://loinc.org|85354-9`
            );
            summary.vitalSigns.bodyWeight = await this.fetchBundle(
                `${base}/Observation?${patientParam}&code=http://loinc.org|29463-7`
            );
            summary.vitalSigns.bodyHeight = await this.fetchBundle(
                `${base}/Observation?${patientParam}&code=http://loinc.org|8302-2,http://loinc.org|8306-3,http://loinc.org|8308-9`
            );

            // 14. Results
            summary.results = await this.fetchBundle(
                `${base}/Observation?${patientParam}&category=http://snomed.info/sct|275711006&_include=Observation:specimen`
            );

            // 15. Procedures
            summary.procedures = await this.fetchBundle(
                `${base}/Procedure?${patientParam}&category=http://snomed.info/sct|387713003`
            );

            // 16. Encounters
            summary.encounters = await this.fetchBundle(
                `${base}/Encounter?${patientParam}&class=http://hl7.org/fhir/v3/ActCode|IMP,http://hl7.org/fhir/v3/ActCode|ACUTE,http://hl7.org/fhir/v3/ActCode|NONAC`
            );

            // 17. Planned care
            const plannedCare = await Promise.all([
                this.fetchBundle(`${base}/ImmunizationRecommendation?${patientParam}`),
                this.fetchBundle(`${base}/DeviceRequest?${patientParam}&status=active&_include=DeviceRequest:device`),
                this.fetchBundle(`${base}/Appointment?${patientParam}&status=booked,pending,proposed`)
            ]);
            summary.plannedCare = plannedCare.flat();

        } catch (err) {
            console.error('Error fetching patient summary:', err);
        }

        return summary;
    }
}
