import {headers} from "./fhir";
import {config} from "../config";

export const healthcareServiceApi = {
    async list() {
        // Query HealthcareServices from mCSD Query Directory
        const url = config.mcsdQueryBaseURL + '/HealthcareService';
        const res = await fetch(url, {headers});
        if (!res.ok) throw new Error('List healthcare services failed: ' + res.statusText);
        const bundle = await res.json();
        return (bundle.entry || []).map(e => e.resource).filter(r => r.resourceType === 'HealthcareService');
    },

    // Helper to format service name
    formatName(service) {
        return service.name || 'Unnamed Service';
    },

    // Helper to check if service is active
    isActive(service) {
        return service.active === true;
    },

    // Group services by name
    groupByName(services) {
        const grouped = {};
        services.forEach(service => {
            const name = this.formatName(service);
            if (!grouped[name]) {
                grouped[name] = {
                    name: name,
                    services: [],
                    count: 0,
                    hasActive: false,
                    types: new Set()
                };
            }
            grouped[name].services.push(service);
            grouped[name].count += 1;
            if (this.isActive(service)) {
                grouped[name].hasActive = true;
            }

            // Extract individual types and add to Set to avoid duplicates
            if (service.type && Array.isArray(service.type)) {
                service.type.forEach(t => {
                    let typeDisplay = null;
                    if (t.coding && Array.isArray(t.coding) && t.coding.length > 0) {
                        typeDisplay = t.coding[0].display || t.coding[0].code;
                    } else if (t.text) {
                        typeDisplay = t.text;
                    }
                    if (typeDisplay) {
                        grouped[name].types.add(typeDisplay);
                    }
                });
            }
        });

        // Convert to array and format types
        return Object.values(grouped).map(group => ({
            ...group,
            types: Array.from(group.types).join(', ') || '-'
        }));
    },

    // Extract organization IDs from providedBy references
    getOrganizationIds(services) {
        const orgIds = new Set();
        services.forEach(service => {
            if (service.providedBy && service.providedBy.reference) {
                // Extract ID from reference (e.g., "Organization/123" -> "123")
                const ref = service.providedBy.reference;
                const id = ref.includes('/') ? ref.split('/').pop() : ref;
                if (id) {
                    orgIds.add(id);
                }
            }
        });
        return Array.from(orgIds);
    },
}
