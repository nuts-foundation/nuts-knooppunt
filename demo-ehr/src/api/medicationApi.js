// FHIR API client for fetching medication-related resources
import { headers } from "./fhir";
import { config } from "../config";

export const medicationApi = {
    /**
     * Fetch MedicationRequest resources for a specific patient
     * @param {string} patientId - FHIR Patient ID
     * @returns {Promise<Array>} Array of MedicationRequest resources
     */
    async getMedicationRequests(patientId) {
        try {
            const response = await fetch(
                `${config.fhirBaseURL}/MedicationRequest?patient=${patientId}`,
                {
                    method: 'GET',
                    headers,
                }
            );

            if (!response.ok) {
                throw new Error(`Failed to fetch medication requests: ${response.statusText}`);
            }

            const bundle = await response.json();

            if (bundle.resourceType !== 'Bundle') {
                throw new Error('Invalid response: expected a Bundle');
            }

            return (bundle.entry || [])
                .map(entry => entry.resource)
                .filter(r => r.resourceType === 'MedicationRequest');
        } catch (error) {
            console.error('Error fetching medication requests:', error);
            throw error;
        }
    },

    /**
     * Fetch MedicationDispense resources for a specific patient
     * @param {string} patientId - FHIR Patient ID
     * @returns {Promise<Array>} Array of MedicationDispense resources
     */
    async getMedicationDispenses(patientId) {
        try {
            const response = await fetch(
                `${config.fhirBaseURL}/MedicationDispense?patient=${patientId}`,
                {
                    method: 'GET',
                    headers,
                }
            );

            if (!response.ok) {
                throw new Error(`Failed to fetch medication dispenses: ${response.statusText}`);
            }

            const bundle = await response.json();

            if (bundle.resourceType !== 'Bundle') {
                throw new Error('Invalid response: expected a Bundle');
            }

            return (bundle.entry || [])
                .map(entry => entry.resource)
                .filter(r => r.resourceType === 'MedicationDispense');
        } catch (error) {
            console.error('Error fetching medication dispenses:', error);
            throw error;
        }
    },

    /**
     * Format medication name from medicationCodeableConcept or medicationReference
     * @param {Object} resource - MedicationRequest or MedicationDispense resource
     * @returns {string} Medication name
     */
    formatMedication(resource) {
        if (resource.medicationCodeableConcept) {
            const coding = resource.medicationCodeableConcept.coding?.[0];
            if (coding?.display) return coding.display;
            if (coding?.code) return coding.code;
            if (resource.medicationCodeableConcept.text) return resource.medicationCodeableConcept.text;
        }
        if (resource.medicationReference?.display) {
            return resource.medicationReference.display;
        }
        if (resource.medicationReference?.reference) {
            return resource.medicationReference.reference;
        }
        return 'Unknown medication';
    },

    /**
     * Format dosage instruction
     * @param {Object} resource - MedicationRequest or MedicationDispense resource
     * @returns {string} Dosage instruction
     */
    formatDosage(resource) {
        if (resource.dosageInstruction && resource.dosageInstruction.length > 0) {
            const dosage = resource.dosageInstruction[0];
            if (dosage.text) return dosage.text;

            const parts = [];
            if (dosage.doseAndRate?.[0]?.doseQuantity) {
                const dose = dosage.doseAndRate[0].doseQuantity;
                parts.push(`${dose.value} ${dose.unit || dose.code || ''}`);
            }
            if (dosage.timing?.repeat?.frequency && dosage.timing?.repeat?.period) {
                parts.push(`${dosage.timing.repeat.frequency}x per ${dosage.timing.repeat.periodUnit || dosage.timing.repeat.period}`);
            }
            if (parts.length > 0) return parts.join(', ');
        }
        return '-';
    },

    /**
     * Format date
     * @param {string} dateString - ISO date string
     * @returns {string} Formatted date
     */
    formatDate(dateString) {
        if (!dateString) return '-';
        try {
            return new Date(dateString).toLocaleDateString('nl-NL');
        } catch {
            return dateString;
        }
    },

    /**
     * Get status badge class
     * @param {string} status - Resource status
     * @returns {string} CSS class name
     */
    getStatusClass(status) {
        const statusMap = {
            'active': 'status-active',
            'completed': 'status-completed',
            'cancelled': 'status-cancelled',
            'draft': 'status-draft',
            'stopped': 'status-stopped',
            'on-hold': 'status-on-hold',
        };
        return statusMap[status?.toLowerCase()] || 'status-unknown';
    },
};

