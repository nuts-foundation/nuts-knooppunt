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
}