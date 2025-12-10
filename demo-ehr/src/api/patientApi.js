// FHIR API client for fetching patient data
import {headers} from "./fhir";
import {config} from "../config";

export const patientApi = {
    /**
     * Fetch all patients from the FHIR server
     * @returns {Promise<Array>} Array of patient objects
     */
    async list() {
        try {
            const response = await fetch(`${config.fhirBaseURL}/Patient`, {
                method: 'GET',
                headers,
            });

            if (!response.ok) {
                throw new Error(`Failed to fetch patients: ${response.statusText}`);
            }

            const bundle = await response.json();

            if (bundle.resourceType !== 'Bundle') {
                throw new Error('Invalid response: expected a Bundle');
            }

            // Extract patient resources from the bundle
            return (bundle.entry || []).map(entry => entry.resource).filter(r => r.resourceType === 'Patient');
        } catch (error) {
            console.error('Error fetching patients:', error);
            throw error;
        }
    },

    /**
     * Search for patients by BSN
     * @param {string} bsn - BSN to search for
     * @returns {Promise<Array>} Array of matching patient objects
     */
    async searchByBSN(bsn) {
        if (!bsn || bsn.trim() === '') {
            return [];
        }

        try {
            const searchUrl = `${config.fhirBaseURL}/Patient?identifier=http://fhir.nl/fhir/NamingSystem/bsn|${encodeURIComponent(bsn.trim())}`;
            const response = await fetch(searchUrl, {
                method: 'GET',
                headers,
            });

            if (!response.ok) {
                throw new Error(`Failed to search patients by BSN: ${response.statusText}`);
            }

            const bundle = await response.json();

            if (bundle.resourceType !== 'Bundle') {
                throw new Error('Invalid response: expected a Bundle');
            }

            // Extract patient resources from the bundle
            return (bundle.entry || []).map(entry => entry.resource).filter(r => r.resourceType === 'Patient');
        } catch (error) {
            console.error('Error searching patients by BSN:', error);
            throw error;
        }
    },

    /**
     * Extract BSN (Dutch citizen service number) from patient identifiers
     * @param {Object} patient - FHIR Patient resource
     * @returns {string|null} BSN or null if not found
     */
    getByBSN(patient) {
        if (!patient.identifier) return null;

        // BSN is typically identified by the system "http://fhir.nl/fhir/NamingSystem/bsn"
        const bsnIdentifier = patient.identifier.find(
            id => id.system === 'http://fhir.nl/fhir/NamingSystem/bsn' ||
                id.system === 'urn:oid:2.16.840.1.113883.2.4.6.3'
        );

        return bsnIdentifier ? bsnIdentifier.value : null;
    },

    /**
     * Get formatted patient name
     * @param {Object} patient - FHIR Patient resource
     * @returns {string} Formatted name
     */
    formatName(patient) {
        if (!patient.name || patient.name.length === 0) {
            return 'Unknown';
        }

        const officialName = patient.name.find(n => n.use === 'official') || patient.name[0];

        const parts = [];
        if (officialName.given) parts.push(officialName.given.join(' '));
        if (officialName.prefix) parts.push(officialName.prefix.join(' '));
        if (officialName.family) parts.push(officialName.family);

        return parts.join(' ') || 'Unknown';
    },

    /**
     * Get patient birth date
     * @param {Object} patient - FHIR Patient resource
     * @returns {string|null} Birth date or null
     */
    formatBirthDate(patient) {
        return patient.birthDate || null;
    },

    /**
     * Get patient gender
     * @param {Object} patient - FHIR Patient resource
     * @returns {string} Gender
     */
    formatGender(patient) {
        return patient.gender || 'unknown';
    },

    /**
     * Create a new Patient resource (no BSN validation performed)
     */
    async create({bsn, given, family, prefix = [], birthDate, gender}) {
        const resource = {
            resourceType: 'Patient',
            identifier: bsn ? [{system: 'http://fhir.nl/fhir/NamingSystem/bsn', value: bsn}] : [],
            name: [
                {
                    use: 'official',
                    family,
                    given,
                    ...(prefix.length ? {prefix} : {}),
                },
            ],
            gender,
            birthDate,
        };
        const response = await fetch(`${config.fhirBaseURL}/Patient`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/fhir+json',
                'Accept': 'application/fhir+json',
            },
            body: JSON.stringify(resource),
        });
        if (!response.ok) {
            const text = await response.text();
            throw new Error(`Create patient failed: ${response.status} ${response.statusText} - ${text}`);
        }
        return await response.json();
    },
    /**
     * Update an existing Patient resource by ID (PUT).
     * @param {string} id
     * @param {Object} input same shape as createPatient
     */
    async update(id, {bsn, given, family, prefix = [], birthDate, gender}) {
        if (!id) throw new Error('Missing patient id');
        const resource = {
            resourceType: 'Patient',
            id,
            identifier: bsn ? [{system: 'http://fhir.nl/fhir/NamingSystem/bsn', value: bsn}] : [],
            name: [
                {
                    use: 'official',
                    family,
                    given,
                    ...(prefix.length ? {prefix} : {}),
                },
            ],
            gender,
            birthDate,
        };
        const response = await fetch(`${config.fhirBaseURL}/Patient/${id}`, {
            method: 'PUT',
            headers: {
                'Content-Type': 'application/fhir+json',
                'Accept': 'application/fhir+json',
            },
            body: JSON.stringify(resource),
        });
        if (!response.ok) {
            const text = await response.text();
            throw new Error(`Update patient failed: ${response.status} ${response.statusText} - ${text}`);
        }
        return await response.json();
    },
    /**
     * Convert a Patient resource to form fields used by the UI.
     */
    toForm(patient) {
        const nameObj = (patient.name || []).find(n => n.use === 'official') || (patient.name || [])[0] || {};
        return {
            bsn: patientApi.getByBSN(patient) || '',
            given: Array.isArray(nameObj.given) ? nameObj.given.join(' ') : '',
            family: nameObj.family || '',
            prefix: Array.isArray(nameObj.prefix) ? nameObj.prefix.join(' ') : '',
            birthDate: patient.birthDate || '',
            gender: patient.gender || 'unknown',
        };
    },
    async delete(id) {
        if (!id) throw new Error('Missing patient id');
        const response = await fetch(`${config.fhirBaseURL}/Patient/${id}`, {
            method: 'DELETE',
            headers: {
                'Accept': 'application/fhir+json',
            },
        });
        if (!response.ok && response.status !== 204) {
            const text = await response.text();
            throw new Error(`Delete patient failed: ${response.status} ${response.statusText} - ${text}`);
        }
        return true;
    },

    /**
     * Search for patients by BSN on STU3 endpoint
     * @param {string} bsn - BSN to search for
     * @returns {Promise<Array>} Array of matching patient objects
     */
    async searchByBSNOnStu3(bsn) {
        if (!bsn || bsn.trim() === '') {
            return [];
        }

        try {
            const searchUrl = `${config.fhirStu3BaseURL}/Patient?identifier=http://fhir.nl/fhir/NamingSystem/bsn|${encodeURIComponent(bsn.trim())}`;
            const response = await fetch(searchUrl, {
                method: 'GET',
                headers,
            });

            if (!response.ok) {
                throw new Error(`Failed to search patients by BSN on STU3: ${response.statusText}`);
            }

            const bundle = await response.json();

            if (bundle.resourceType !== 'Bundle') {
                throw new Error('Invalid response: expected a Bundle');
            }

            // Extract patient resources from the bundle
            return (bundle.entry || []).map(entry => entry.resource).filter(r => r.resourceType === 'Patient');
        } catch (error) {
            console.error('Error searching patients by BSN on STU3:', error);
            throw error;
        }
    },

    /**
     * Create a new Patient resource on STU3 endpoint
     */
    async createOnStu3({bsn, given, family, prefix = [], birthDate, gender}) {
        const resource = {
            resourceType: 'Patient',
            identifier: bsn ? [{system: 'http://fhir.nl/fhir/NamingSystem/bsn', value: bsn}] : [],
            name: [
                {
                    use: 'official',
                    family,
                    given,
                    ...(prefix.length ? {prefix} : {}),
                },
            ],
            gender,
            birthDate,
        };
        const response = await fetch(`${config.fhirStu3BaseURL}/Patient`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/fhir+json',
                'Accept': 'application/fhir+json',
            },
            body: JSON.stringify(resource),
        });
        if (!response.ok) {
            const text = await response.text();
            throw new Error(`Create patient on STU3 failed: ${response.status} ${response.statusText} - ${text}`);
        }
        return await response.json();
    },
};
