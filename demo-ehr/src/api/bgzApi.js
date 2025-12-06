import {headersWithContentType} from "./fhir";
import {config} from "../config";
import bgzExampleBundle from "../bgz-example.json";

export const bgzApi = {
    // Get the number of resources in the BGZ bundle
    getBGZResourceCount() {
        return bgzExampleBundle.entry ? bgzExampleBundle.entry.length : 0;
    },

    async generateBGZ(patientId) {
        // Deep clone the example bundle to avoid modifying the original
        const bundle = JSON.parse(JSON.stringify(bgzExampleBundle));

        // Replace all Patient references with the current patient ID
        const replacePatientReferences = (obj) => {
            if (typeof obj !== 'object' || obj === null) {
                return;
            }

            // Check if this is a reference object
            if (obj.reference && typeof obj.reference === 'string' && obj.reference.startsWith('Patient/')) {
                obj.reference = `Patient/${patientId}`;
            }

            // Recursively process all properties
            Object.keys(obj).forEach(key => {
                replacePatientReferences(obj[key]);
            });
        };

        replacePatientReferences(bundle);

        // POST the bundle to the FHIR server
        const url = config.fhirBaseURL;
        const res = await fetch(url, {
            method: 'POST',
            headers: headersWithContentType,
            body: JSON.stringify(bundle)
        });

        console.log(JSON.stringify(bundle))

        if (!res.ok) {
            const errorText = await res.text();
            throw new Error(`Generate BGZ failed: ${res.statusText} - ${errorText}`);
        }

        return await res.json();
    }
}
