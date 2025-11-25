import {headers} from "./fhir";
import {config} from "../config";

export const consentApi = {
    async list(patientReference) {
        // Optionally filter by patient reference if provided
        const url = new URL(config.fhirBaseURL + '/Consent');
        if (patientReference) {
            url.searchParams.set('patient', patientReference);
        }
        const res = await fetch(url.toString(), {headers});
        if (!res.ok) throw new Error('List consents failed: ' + res.statusText);
        const bundle = await res.json();
        return (bundle.entry || []).map(e => e.resource).filter(r => r.resourceType === 'Consent');
    }, async create(form) {
        // Minimal FHIR R4 Consent structure
        const resource = this.toResource(form)
        const res = await fetch(`${config.fhirBaseURL}/Consent`, {
            method: 'POST', headers, body: JSON.stringify(resource)
        });
        if (!res.ok) throw new Error('Create consent failed: ' + res.status + ' ' + res.statusText);
        return await res.json();
    }, async update(id, updated) {
        const res = await fetch(`${config.fhirBaseURL}/Consent/${id}`, {
            method: 'PUT', headers, body: JSON.stringify(updated)
        });
        if (!res.ok) throw new Error('Update consent failed: ' + res.status + ' ' + res.statusText);
        return await res.json();
    }, async delete(id) {
        const res = await fetch(`${config.fhirBaseURL}/Consent/${id}`, {method: 'DELETE', headers});
        if (!res.ok && res.status !== 204) throw new Error('Delete consent failed: ' + res.status + ' ' + res.statusText);
        return true;
    }, toEditable(consent) {
        return {
            id: consent.id,
            status: consent.status,
            patientReference: consent.patient?.reference || '',
            provisionActorsOrgURAs: (consent.provision?.actor || []).map(a => a.reference?.identifier?.value || a.reference?.reference || ''),
            dateTime: consent.dateTime || '',
            categoryCodes: (consent.category || []).flatMap(c => (c.coding || []).map(cd => ({
                system: cd.system, code: cd.code, display: cd.display
            })))
        };
    }, toResource(form) {
        return {
            resourceType: 'Consent',
            status: form.status || 'active',
            scope: {coding: [{system: 'http://terminology.hl7.org/CodeSystem/consentscope', code: 'patient-privacy'}]},
            category: (form.categoryCodes || []).map(c => ({
                coding: [{
                    system: c.system || 'http://loinc.org', code: c.code, display: c.display
                }]
            })),
            patient: {reference: form.patientReference},
            dateTime: new Date(form.dateTime).toISOString(),
            controller: [{
                type: 'Organization', identifier: {
                    system: 'http://fhir.nl/fhir/NamingSystem/ura', value: config.organizationURA,
                }
            }],
            provision: {
                type: 'permit',
                actor: (form.provisionActorsOrgURAs || []).map(ura => ({
                    role: {coding: [{system: 'http://terminology.hl7.org/CodeSystem/consentaction', code: 'access'}]},
                    reference: {
                        type: 'Organization',
                        identifier: {
                            system: 'http://fhir.nl/fhir/NamingSystem/ura',
                            value: ura
                        }
                    }
                })),
            }
        };
    }
};
