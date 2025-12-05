import {headers} from "./fhir";
import {config} from "../config";

export const organizationApi = {
    async list() {
        // Query Organizations from mCSD Query Directory
        const url = config.mcsdQueryBaseURL + '/Organization';
        const res = await fetch(url, {headers});
        if (!res.ok) throw new Error('List organizations failed: ' + res.statusText);
        const bundle = await res.json();
        return (bundle.entry || []).map(e => e.resource).filter(r => r.resourceType === 'Organization');
    },

    async getById(id) {
        // Query single Organization by ID
        const url = `${config.mcsdQueryBaseURL}/Organization/${id}`;
        const res = await fetch(url, {headers});
        if (!res.ok) {
            if (res.status === 404) {
                return null;
            }
            throw new Error('Get organization by ID failed: ' + res.statusText);
        }
        return await res.json();
    },

    async getByIds(ids) {
        if (!ids || ids.length === 0) {
            return [];
        }
        // Query Organizations by multiple IDs using _id parameter
        const idsParam = ids.join(',');
        const url = `${config.mcsdQueryBaseURL}/Organization?_id=${idsParam}`;
        const res = await fetch(url, {headers});
        if (!res.ok) throw new Error('Get organizations by IDs failed: ' + res.statusText);
        const bundle = await res.json();
        return (bundle.entry || []).map(e => e.resource).filter(r => r.resourceType === 'Organization');
    },

    async getSubOrganizations(parentOrgId) {
        // Query sub-organizations (departments) by partOf reference
        const url = `${config.mcsdQueryBaseURL}/Organization?partof=${parentOrgId}`;
        const res = await fetch(url, {headers});
        if (!res.ok) throw new Error('Get sub-organizations failed: ' + res.statusText);
        const bundle = await res.json();
        return (bundle.entry || []).map(e => e.resource).filter(r => r.resourceType === 'Organization');
    },

    formatName(org) {
        return org.name || 'Unnamed Organization';
    },

    isActive(org) {
        return org.active === true;
    },

    // Get URA identifier
    getURA(org) {
        if (!org.identifier || !Array.isArray(org.identifier)) {
            return null;
        }
        const uraIdentifier = org.identifier.find(
            id => id.system === 'http://fhir.nl/fhir/NamingSystem/ura'
        );
        return uraIdentifier?.value || null;
    },

    // Format address
    formatAddress(org) {
        if (!org.address || !Array.isArray(org.address) || org.address.length === 0) {
            return '-';
        }
        const addr = org.address[0]; // Take first address
        const parts = [];

        if (addr.line && Array.isArray(addr.line) && addr.line.length > 0) {
            parts.push(addr.line.join(' '));
        }
        if (addr.city) {
            parts.push(addr.city);
        }
        if (addr.postalCode) {
            parts.push(addr.postalCode);
        }

        return parts.length > 0 ? parts.join(', ') : '-';
    },

    // Format type
    formatType(org) {
        if (!org.type || !Array.isArray(org.type) || org.type.length === 0) {
            return '-';
        }
        const types = org.type.map(t => {
            if (t.coding && Array.isArray(t.coding) && t.coding.length > 0) {
                return t.coding[0].display || t.coding[0].code || '-';
            }
            if (t.text) {
                return t.text;
            }
            return '-';
        }).filter(t => t !== '-');

        return types.length > 0 ? types.join(', ') : '-';
    },

    // Format telecom (returns array of contact points)
    formatTelecom(org) {
        if (!org.telecom || !Array.isArray(org.telecom) || org.telecom.length === 0) {
            return [];
        }
        return org.telecom.map(t => ({
            system: t.system || 'unknown',
            value: t.value || '-',
            use: t.use
        }));
    },

    // Format telecom as string
    formatTelecomString(org) {
        const contacts = this.formatTelecom(org);
        if (contacts.length === 0) {
            return '-';
        }
        return contacts.map(c => {
            const icon = c.system === 'phone' ? '📞' : c.system === 'email' ? '📧' : '📱';
            return `${icon} ${c.value}`;
        }).join(', ');
    },

    // Get endpoint for organization, traversing up partOf hierarchy if needed
    // Returns endpoint with payloadType.code='eOverdracht-notification'
    async getEndpoint(orgId) {
        try {
            let currentOrg = await this.getById(orgId);
            if (!currentOrg) {
                return null;
            }

            const visited = new Set([orgId]); // Prevent infinite loops

            // Traverse up the hierarchy to find an endpoint with correct payloadType
            while (currentOrg) {
                // Check if this organization has endpoints
                if (currentOrg.endpoint && Array.isArray(currentOrg.endpoint) && currentOrg.endpoint.length > 0) {
                    // Check all endpoints for matching payloadType
                    for (const endpointRef of currentOrg.endpoint) {
                        const ref = endpointRef.reference;
                        const endpointId = ref.includes('/') ? ref.split('/').pop() : ref;

                        // Fetch the endpoint resource
                        const endpointUrl = `${config.mcsdQueryBaseURL}/Endpoint/${endpointId}`;
                        const res = await fetch(endpointUrl, {headers});

                        if (res.ok) {
                            const endpoint = await res.json();

                            // Check if endpoint has payloadType with code='eOverdracht-notification'
                            if (endpoint.payloadType && Array.isArray(endpoint.payloadType)) {
                                const hasCorrectPayloadType = endpoint.payloadType.some(pt => {
                                    if (pt.coding && Array.isArray(pt.coding)) {
                                        return pt.coding.some(coding => coding.code === 'eOverdracht-notification');
                                    }
                                    return false;
                                });

                                if (hasCorrectPayloadType) {
                                    console.log('Found endpoint with eOverdracht-notification payloadType');
                                    return endpoint;
                                }
                            }
                        }
                    }
                }

                // If no matching endpoint found, traverse up to parent
                if (currentOrg.partOf && currentOrg.partOf.reference) {
                    const parentRef = currentOrg.partOf.reference;
                    const parentId = parentRef.includes('/') ? parentRef.split('/').pop() : parentRef;

                    // Check for circular references
                    if (visited.has(parentId)) {
                        console.warn('Circular partOf reference detected for organization:', parentId);
                        break;
                    }
                    visited.add(parentId);

                    currentOrg = await this.getById(parentId);
                } else {
                    // No parent to check
                    break;
                }
            }

            console.warn('No endpoint with eOverdracht-notification payloadType found');
            return null;
        } catch (err) {
            console.error('Error getting endpoint for organization:', orgId, err);
            return null;
        }
    },
}