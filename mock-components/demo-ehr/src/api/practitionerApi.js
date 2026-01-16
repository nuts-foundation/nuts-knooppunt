import {headers, headersWithContentType} from "./fhir";
import {config} from "../config";

export const practitionerApi = {
    async searchByIdentifier(userId) {
        // Search for Practitioner by identifier (userId from OIDC token)
        const url = `${config.fhirBaseURL}/Practitioner?identifier=urn:oid:2.16.840.1.113883.2.4.6.3|${userId}`;
        const res = await fetch(url, {headers});

        if (!res.ok) {
            throw new Error(`Search Practitioner failed: ${res.statusText}`);
        }

        const bundle = await res.json();
        const practitioners = (bundle.entry || []).map(e => e.resource).filter(r => r.resourceType === 'Practitioner');

        // Return first match if found
        return practitioners.length > 0 ? practitioners[0] : null;
    },

    async createPractitioner(userId, userName, userEmail) {
        // Create Practitioner resource
        const practitioner = {
            resourceType: "Practitioner",
            identifier: [
                {
                    system: "urn:oid:2.16.840.1.113883.2.4.6.3",
                    value: userId
                }
            ],
            name: [
                {
                    use: "official",
                    text: userName,
                    family: userName.split(' ').pop() || userName,
                    given: userName.split(' ').slice(0, -1).length > 0
                        ? userName.split(' ').slice(0, -1)
                        : [userName]
                }
            ],
            telecom: userEmail ? [
                {
                    system: "email",
                    value: userEmail,
                    use: "work"
                }
            ] : []
        };

        const url = `${config.fhirBaseURL}/Practitioner`;
        const res = await fetch(url, {
            method: 'POST',
            headers: headersWithContentType,
            body: JSON.stringify(practitioner)
        });

        if (!res.ok) {
            const errorText = await res.text();
            throw new Error(`Create Practitioner failed: ${res.statusText} - ${errorText}`);
        }

        return await res.json();
    }
}
