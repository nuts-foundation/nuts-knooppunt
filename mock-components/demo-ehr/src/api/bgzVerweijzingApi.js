import {headers, headersWithContentType} from "./fhir";
import {config} from "../config";
import {organizationApi} from "./organizationApi";

// Helper function to get BGZ FHIR query parameter inputs
const getBgzFhirQueryInputs = () => [
    {
        type: { coding: [{ system: "http://loinc.org", code: "79191-3", display: "Patient demographics panel" }] },
        valueString: "Patient?_include=Patient:general-practitioner"
    },
    {
        type: { coding: [{ system: "http://loinc.org", code: "48768-6", display: "Payment sources Document" }] },
        valueString: "Coverage?_include=Coverage:payor:Organization&_include=Coverage:payor:Patient"
    },
    {
        type: { coding: [{ system: "http://snomed.info/sct", code: "11291000146105", display: "Treatment instructions" }] },
        valueString: "Consent?category=http%3A%2F%2Fsnomed.info%2Fsct%7C11291000146105"
    },
    {
        type: { coding: [{ system: "http://snomed.info/sct", code: "11341000146107", display: "Living will and advance directive record" }] },
        valueString: "Consent?category=http%3A%2F%2Fsnomed.info%2Fsct%7C11341000146107"
    },
    {
        type: { coding: [{ system: "http://loinc.org", code: "47420-5", display: "Functional status assessment note" }] },
        valueString: "Observation/$lastn?category=http%3A%2F%2Fsnomed.info%2Fsct%7C118228005,http%3A%2F%2Fsnomed.info%2Fsct%7C384821006"
    },
    {
        type: { coding: [{ system: "http://loinc.org", code: "11450-4", display: "Problem list - Reported" }] },
        valueString: "Condition"
    },
    {
        type: { coding: [{ system: "http://snomed.info/sct", code: "365508006", display: "Residence and accommodation circumstances - finding" }] },
        valueString: "Observation/$lastn?code=http%3A%2F%2Fsnomed.info%2Fsct%7C365508006"
    },
    {
        type: { coding: [{ system: "http://snomed.info/sct", code: "228366006", display: "Finding relating to drug misuse behavior" }] },
        valueString: "Observation?code=http%3A%2F%2Fsnomed.info%2Fsct%7C228366006"
    },
    {
        type: { coding: [{ system: "http://snomed.info/sct", code: "228273003", display: "Finding relating to alcohol drinking behavior" }] },
        valueString: "Observation?code=http%3A%2F%2Fsnomed.info%2Fsct%7C228273003"
    },
    {
        type: { coding: [{ system: "http://snomed.info/sct", code: "365980008", display: "Tobacco use and exposure - finding" }] },
        valueString: "Observation?code=http%3A%2F%2Fsnomed.info%2Fsct%7C365980008"
    },
    {
        type: { coding: [{ system: "http://snomed.info/sct", code: "11816003", display: "Diet education" }] },
        valueString: "NutritionOrder"
    },
    {
        type: { coding: [{ system: "http://loinc.org", code: "75310-3", display: "Health concerns Document" }] },
        valueString: "Flag"
    },
    {
        type: { coding: [{ system: "http://loinc.org", code: "48765-2", display: "Allergies and adverse reactions Document" }] },
        valueString: "AllergyIntolerance"
    },
    {
        type: { coding: [{ system: "http://snomed.info/sct", code: "422979000", display: "Known medication use" }] },
        valueString: "MedicationStatement?category=urn:oid:2.16.840.1.113883.2.4.3.11.60.20.77.5.3|6&_include=MedicationStatement:medication"
    },
    {
        type: { coding: [{ system: "http://snomed.info/sct", code: "16076005", display: "Known medication agreements" }] },
        valueString: "MedicationRequest?category=http%3A%2F%2Fsnomed.info%2Fsct%7C16076005&_include=MedicationRequest:medication"
    },
    {
        type: { coding: [{ system: "http://snomed.info/sct", code: "422037009", display: "Known administration agreements" }] },
        valueString: "MedicationDispense?category=http%3A%2F%2Fsnomed.info%2Fsct%7C422037009&_include=MedicationDispense:medication"
    },
    {
        type: { coding: [{ system: "http://loinc.org", code: "46264-8", display: "Known medical aids" }] },
        valueString: "DeviceUseStatement?_include=DeviceUseStatement:device"
    },
    {
        type: { coding: [{ system: "http://loinc.org", code: "11369-6", display: "History of Immunization Narrative" }] },
        valueString: "Immunization?status=completed"
    },
    {
        type: { coding: [{ system: "http://loinc.org", code: "85354-9", display: "Blood pressure" }] },
        valueString: "Observation/$lastn?code=http://loinc.org|85354-9"
    },
    {
        type: { coding: [{ system: "http://loinc.org", code: "29463-7", display: "Body weight" }] },
        valueString: "Observation/$lastn?code=http://loinc.org|29463-7"
    },
    {
        type: { coding: [{ system: "http://loinc.org", code: "8302-2", display: "Body height" }] },
        valueString: "Observation/$lastn?code=http://loinc.org|8302-2,http://loinc.org|8306-3,http://loinc.org|8308-9"
    },
    {
        type: { coding: [{ system: "http://snomed.info/sct", code: "15220000", display: "Laboratory test" }] },
        valueString: "Observation/$lastn?category=http%3A%2F%2Fsnomed.info%2Fsct%7C275711006&_include=Observation:related-target&_include=Observation:specimen"
    },
    {
        type: { coding: [{ system: "http://loinc.org", code: "47519-4", display: "History of Procedures" }] },
        valueString: "Procedure?category=http%3A%2F%2Fsnomed.info%2Fsct%7C387713003"
    },
    {
        type: { coding: [{ system: "http://loinc.org", code: "47519-4", display: "History of Procedures" }] },
        valueString: "Procedure?category=http%3A%2F%2Fsnomed.info%2Fsct%7C103693007"
    },
    {
        type: { coding: [{ system: "http://loinc.org", code: "47519-4", display: "History of Procedures" }] },
        valueString: "Procedure?category=http%3A%2F%2Fsnomed.info%2Fsct%7C410606002"
    },
    {
        type: { coding: [{ system: "http://loinc.org", code: "47519-4", display: "History of Procedures" }] },
        valueString: "Procedure?category=http%3A%2F%2Fsnomed.info%2Fsct%7C387713003&_include=Procedure:focal-subject&_include=Procedure:performer&_include=Procedure:reason-reference&_include=Procedure:part-of&_include=Procedure:outcome&_include=Procedure:complication&_include=Procedure:body-site"
    },
    {
        type: { coding: [{ system: "http://loinc.org", code: "47519-4", display: "History of Procedures" }] },
        valueString: "Procedure?category=http%3A%2F%2Fsnomed.info%2Fsct%7C103693007&_include=Procedure:focal-subject&_include=Procedure:performer&_include=Procedure:reason-reference&_include=Procedure:part-of&_include=Procedure:outcome&_include=Procedure:complication&_include=Procedure:body-site"
    },
    {
        type: { coding: [{ system: "http://loinc.org", code: "47519-4", display: "History of Procedures" }] },
        valueString: "Procedure?category=http%3A%2F%2Fsnomed.info%2Fsct%7C410606002&_include=Procedure:focal-subject&_include=Procedure:performer&_include=Procedure:reason-reference&_include=Procedure:part-of&_include=Procedure:outcome&_include=Procedure:complication&_include=Procedure:body-site"
    },
    {
        type: { coding: [{ system: "http://snomed.info/sct", code: "10529002", display: "Nursing care plan" }] },
        valueString: "CarePlan?category=http%3A%2F%2Fsnomed.info%2Fsct%7C10529002&_include=CarePlan:subject&_include=CarePlan:care-team&_include=CarePlan:addresses&_include=CarePlan:goal&_include=CarePlan:part-of&_include=CarePlan:replaces&_include=CarePlan:based-on&_include=CarePlan:author&_include=CarePlan:replaces&_include=CarePlan:based-on&_include=CarePlan:replaces&_include=CarePlan:author"
    },
    {
        type: { coding: [{ system: "http://loinc.org", code: "47519-4", display: "History of Procedures" }] },
        valueString: "Procedure?category=http%3A%2F%2Fsnomed.info%2Fsct%7C410606002,Procedure?category=http%3A%2F%2Fsnomed.info%2Fsct%7C103693007,Procedure?category=http%3A%2F%2Fsnomed.info%2Fsct%7C387713003"
    },
    {
        type: { coding: [{ system: "http://snomed.info/sct", code: "373873005", display: "Surgical attendance" }] },
        valueString: "Procedure?code=http%3A%2F%2Fsnomed.info%2Fsct%7C373873005&_include=Procedure:subject&_include=Procedure:encounter&_include=Procedure:performer&_include=Procedure:reason-reference&_include=Procedure:part-of&_include=Procedure:outcome&_include=Procedure:complication&_include=Procedure:body-site"
    },
    {
        type: { coding: [{ system: "http://loinc.org", code: "29762-2", display: "Social history Narrative" }] },
        valueString: "FamilyMemberHistory?_include=FamilyMemberHistory:patient&_include=FamilyMemberHistory:condition"
    },
    {
        type: { coding: [{ system: "http://loinc.org", code: "46264-8", display: "Known medical aids" }] },
        valueString: "DeviceUseStatement?_include=DeviceUseStatement:subject&_include=DeviceUseStatement:device&_include=DeviceUseStatement:context&_include=DeviceUseStatement:derived-from&_include=DeviceUseStatement:reason-reference&_include=DeviceUseStatement:body-site"
    },
    {
        type: { coding: [{ system: "http://loinc.org", code: "11503-0", display: "Medical records" }] },
        valueString: "DocumentReference?category=http%3A%2F%2Floinc.org%7C11503-0,DocumentReference?type=http%3A%2F%2Floinc.org%7C11503-0&_include=DocumentReference:patient&_include=DocumentReference:author&_include=DocumentReference:authenticator&_include=DocumentReference:custodian&_include=DocumentReference:related"
    },
    {
        type: { coding: [{ system: "http://loinc.org", code: "11503-0", display: "Medical records" }] },
        valueString: "DiagnosticReport?category=http%3A%2F%2Floinc.org%7C11503-0,DiagnosticReport?category=http%3A%2F%2Fsnomed.info%2Fsct%7C15220000&_include=DiagnosticReport:result&_include=DiagnosticReport:specimen&_include=DiagnosticReport:imaging-study&_include=DiagnosticReport:media"
    },
    {
        type: { coding: [{ system: "http://loinc.org", code: "11503-0", display: "Medical records" }] },
        valueString: "DocumentReference?category=http%3A%2F%2Floinc.org%7C11503-0&_include=DocumentReference:patient&_include=DocumentReference:author&_include=DocumentReference:authenticator&_include=DocumentReference:custodian&_include=DocumentReference:related"
    }
];

export const bgzVerweijzingApi = {
    async getBgZWorkflowTasks(patientBsn) {
        // Fetch all BGZ workflow tasks for a patient by BSN
        const url = `${config.fhirStu3BaseURL}/Task?for:identifier=http://fhir.nl/fhir/NamingSystem/bsn|${patientBsn}&code=http://snomed.info/sct|3457005`;

        const res = await fetch(url, {
            method: 'GET',
            headers: headers
        });

        if (!res.ok) {
            throw new Error(`Failed to fetch BGZ workflow tasks: ${res.statusText}`);
        }

        const bundle = await res.json();
        // Filter out OperationOutcome and other non-Task resources
        return bundle.entry
            ? bundle.entry
                .map(e => e.resource)
                .filter(resource => resource && resource.resourceType === 'Task')
            : [];
    },

    async createBgZWorkflowTask(bsn, patientName, userId, userName, departmentOrgId, departmentOrgName) {
        // Create BgZ Workflow Task (Verwijzing)
        const task = {
            resourceType: "Task",
            meta: {
                profile: [
                    "http://nictiz.nl/fhir/StructureDefinition/BgZ-verwijzing-Task"
                ]
            },
            intent: "proposal",
            status: "requested",
            code: {
                coding: [
                    {
                        system: "http://snomed.info/sct",
                        code: "3457005",
                        display: "verwijzen van patiënt "
                    }
                ],
                text: "Verwijzing"
            },
            requester: {
                agent: {
                    reference: `http://localhost:7050/fhir/sunflower-patients/Practitioner/${userId}`,
                    display: userName
                }
            },
            for: {
                identifier: {
                    system: 'http://fhir.nl/fhir/NamingSystem/bsn',
                    value: bsn
                }
            },
            owner: {
                reference: `${config.mcsdQueryBaseURL}/Organization/${departmentOrgId}`,
                display: departmentOrgName
            },
            input: getBgzFhirQueryInputs()
        };

        const url = `${config.fhirStu3BaseURL}/Task`;
        const res = await fetch(url, {
            method: 'POST',
            headers: headersWithContentType,
            body: JSON.stringify(task)
        });

        if (!res.ok) {
            const errorText = await res.text();
            throw new Error(`Create Task failed: ${res.statusText} - ${errorText}`);
        }

        return await res.json();
    },

    async createBgZNotificatonTask(patientBsn, workflowTaskId, authorizationBase, user, departmentOrgId,
        notificationEndpoint, selectedOrganization) {
        // Create BgZ Notification Task
        const uuid = () => {
            try {
                return (typeof crypto !== 'undefined' && crypto.randomUUID) ? crypto.randomUUID() : null;
            } catch (e) { return null; }
        };

        // Get logged-in user's organization URA from user profile or config
        const loggedInOrgURA = user?.profile?.sub;

        console.log('Logged-in user organization URA:', loggedInOrgURA);

        // Get receiving organization URA from selectedOrganization
        let receivingOrgURA = null;
        if (selectedOrganization) {
            receivingOrgURA = organizationApi.getURA(selectedOrganization);
            console.log('Receiving organization URA:', receivingOrgURA);

            if (!receivingOrgURA) {
                console.warn('Could not extract URA from selected organization:', selectedOrganization.id);
            }
        } else {
            console.warn('No selected organization provided');
        }

        const now = new Date();
        const authoredOn = now.toISOString();
        const restrictionEnd = new Date(now.getTime() + 7 * 24 * 60 * 60 * 1000).toISOString(); // +7 days

        const task = {
            resourceType: "Task",
            groupIdentifier: {
                system: "urn:ietf:rfc:3986",
                value: `urn:uuid:${uuid() || Math.random().toString(36).slice(2)}`
            },
            identifier: [
                {
                    system: "urn:ietf:rfc:3986",
                    value: `urn:uuid:${uuid() || Math.random().toString(36).slice(2)}`
                }
            ],
            status: "requested",
            intent: "proposal",
            code: {
                coding: [
                    {
                        system: "http://fhir.nl/fhir/NamingSystem/TaskCode",
                        code: "pull-notification"
                    }
                ]
            },
            restriction: {
                period: {
                    end: restrictionEnd
                }
            },
            for: {
                identifier: {
                    system: "http://fhir.nl/fhir/NamingSystem/bsn",
                    value: patientBsn
                }
            },
            authoredOn: authoredOn,
            requester: {
                agent: {
                    identifier: {
                        system: "http://example.com/fhir/NamingSystem/dummy",
                        value: "demo-ehr-app" // system identifier for demo-ehr-app
                    }
                },
                onBehalfOf: {
                    identifier: {
                        system: "http://fhir.nl/fhir/NamingSystem/ura",
                        value: loggedInOrgURA
                    }
                }
            },
            owner: {
                identifier: {
                    system: "http://fhir.nl/fhir/NamingSystem/ura",
                    value: receivingOrgURA || String(departmentOrgId) // fallback to departmentOrgId if URA not found
                }
            },
            ...(workflowTaskId && {
                basedOn: [
                    {
                        reference: `Task/${workflowTaskId.id}`
                    }
                ]
            }),
            input: [
                {
                    type: {
                        coding: [
                            {
                                system: "http://fhir.nl/fhir/NamingSystem/TaskParameter",
                                code: "authorization-base"
                            }
                        ]
                    },
                    valueString: authorizationBase
                },
                {
                    type: {
                        coding: [
                            {
                                system: "http://fhir.nl/fhir/NamingSystem/TaskParameter",
                                code: "get-workflow-task"
                            }
                        ]
                    },
                    valueBoolean: !!workflowTaskId
                },
                // If no workflow task, include all FHIR query parameters
                ...(!workflowTaskId ? getBgzFhirQueryInputs() : [])
            ]
        };

        // Use dynamic proxy to handle any endpoint address
        // Extract the base URL from the endpoint and append /Task
        const endpointAddress = notificationEndpoint.address || notificationEndpoint;

        console.log('Sending notification task to endpoint:', endpointAddress);

        const url = '/api/dynamic-proxy/Task';
        const headers = {
            ...headersWithContentType,
            'X-Target-URL': endpointAddress
        };

        const res = await fetch(url, {
            method: 'POST',
            headers: headers,
            body: JSON.stringify(task)
        });

        if (!res.ok) {
            const errorText = await res.text();
            throw new Error(`Create Notification Task failed: ${res.statusText} - ${errorText}`);
        }

        return await res.json();
    }
};
