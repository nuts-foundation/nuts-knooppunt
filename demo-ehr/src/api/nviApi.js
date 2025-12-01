// NVI API client for querying DocumentReferences
import { headers } from "./fhir";
import { config } from "../config";

export const nviApi = {
    /**
     * Search for DocumentReferences by patient BSN to find organizations that have data
     * @param {string} bsn - Patient BSN identifier
     * @param {string} abonneeNummer - Abonnee nummer from token for tenant identification
     * @returns {Promise<Array>} Array of unique organizations with their URA and name
     */
    async searchOrganizationsByPatient(bsn, abonneeNummer) {
        if (!bsn) {
            return [];
        }

        try {
            // Query DocumentReferences using patient:identifier parameter with BSN
            const searchParams = new URLSearchParams({
                'patient:identifier': `http://fhir.nl/fhir/NamingSystem/bsn|${bsn}`,
                '_count': '100', // Get up to 100 results
            });

            console.log("adding header:","X-Tenant-ID: http://fhir.nl/fhir/NamingSystem/ura|"+abonneeNummer)
            const response = await fetch(
                `/api/knooppunt/nvi/DocumentReference?${searchParams}`,
                {
                    method: 'GET',
                    headers: {
                        ...headers,
                        'X-Tenant-ID': 'http://fhir.nl/fhir/NamingSystem/ura|'+abonneeNummer,
                    },
                }
            );

            if (!response.ok) {
                throw new Error(`Failed to search DocumentReferences: ${response.statusText}`);
            }

            const bundle = await response.json();

            if (bundle.resourceType !== 'Bundle') {
                throw new Error('Invalid response: expected a Bundle');
            }

            // Extract unique custodian organizations from DocumentReferences
            const organizationsMap = new Map();

            (bundle.entry || []).forEach(entry => {
                const docRef = entry.resource;
                if (docRef.resourceType === 'DocumentReference' && docRef.custodian) {
                    // Extract organization identifier (URA)
                    const custodian = docRef.custodian;
                    let ura = null;
                    let name = null;

                    if (custodian.identifier) {
                        // URA is typically in the identifier with system "urn:oid:2.16.840.1.113883.2.4.6.1"
                        ura = custodian.identifier.value;
                    }

                    if (custodian.display) {
                        name = custodian.display;
                    } else if (custodian.reference) {
                        // Use reference as fallback
                        name = custodian.reference;
                    }

                    // Only add if we have a URA
                    if (ura && !organizationsMap.has(ura)) {
                        organizationsMap.set(ura, {
                            ura,
                            name: name || 'Unknown Organization',
                            documentCount: 1,
                        });
                    } else if (ura) {
                        // Increment document count
                        const org = organizationsMap.get(ura);
                        org.documentCount += 1;
                    }
                }
            });

            // Convert map to array and sort by name
            return Array.from(organizationsMap.values()).sort((a, b) =>
                a.name.localeCompare(b.name)
            );
        } catch (error) {
            console.error('Error searching DocumentReferences:', error);
            throw error;
        }
    },

    /**
     * Format URA for display
     * @param {string} ura - Organization URA identifier
     * @returns {string} Formatted URA
     */
    formatURA(ura) {
        if (!ura) return '-';
        return ura;
    },
};

