import {headers, headersWithContentType} from "./fhir";
import {config} from "../config";

export const taskApi = {
    async getEOverdrachtTasks(patientId) {
        // Query Tasks by code (308292007 = "Overdracht van zorg") from STU3 server
        const url = `${config.fhirStu3BaseURL}/Task?code=308292007${patientId ? `&patient=http://localhost:7050/fhir/sunflower-patients/Patient/${patientId}` : ''}`;
        const res = await fetch(url, {headers});
        if (!res.ok) throw new Error('Get Tasks failed: ' + res.statusText);
        const bundle = await res.json();
        const tasks = (bundle.entry || []).map(e => e.resource).filter(r => r.resourceType === 'Task');

        // Filter by patient reference if provided
        if (patientId) {
            return tasks.filter(task => {
                if (!task.for || !task.for.reference) return false;
                const ref = task.for.reference;
                const id = ref.includes('/') ? ref.split('/').pop() : ref;
                return id === patientId;
            });
        }

        return tasks;
    },

    async createEOverdrachtTask(patientId, patientName, userId, userName, departmentOrgId, departmentOrgName) {
        // Create eOverdracht Task
        const task = {
            resourceType: "Task",
            meta: {
                profile: [
                    "http://nictiz.nl/fhir/StructureDefinition/eOverdracht-Task"
                ]
            },
            status: "in-progress",
            intent: "order",
            code: {
                coding: [
                    {
                        system: "http://snomed.info/sct",
                        code: "308292007",
                        display: "Overdracht van zorg"
                    }
                ]
            },
            for: {
                reference: `http://localhost:7050/fhir/sunflower-patients/Patient/${patientId}`,
                display: patientName
            },
            requester: {
                agent: {
                    reference: `http://localhost:7050/fhir/sunflower-patients/Practitioner/${userId}`,
                    display: userName
                }
            },
            owner: {
                reference: `${config.mcsdQueryBaseURL}/Organization/${departmentOrgId}`,
                display: departmentOrgName
            },
            input: [
                {
                    type: {
                        coding: [
                            {
                                system: "http://snomed.info/sct",
                                code: "11171000146100",
                                display: "verslag van verpleegkundige overdracht"
                            }
                        ]
                    },
                    valueReference: {
                        reference: `Composition/placeholder`,
                        display: `Overdrachtsbericht ${patientName}`
                    }
                }
            ]
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

    async notifyReceivingParty(task, endpoint) {
        // Send notification to receiving party's endpoint
        if (!endpoint || !endpoint.address) {
            throw new Error('Invalid endpoint: no address found');
        }

        console.log('Notifying receiving party at endpoint:', endpoint.address);

        // POST the task to the endpoint address
        const res = await fetch(endpoint.address + '/Task/' + task.id, {
            method: 'POST',
            headers: headersWithContentType
        });

        if (!res.ok) {
            const errorText = await res.text();
            throw new Error(`Notification failed: ${res.statusText} - ${errorText}`);
        }

        return await res.json();
    },

    async deleteTask(taskId) {
        // Delete a Task from the STU3 server
        const url = `${config.fhirStu3BaseURL}/Task/${taskId}`;
        const res = await fetch(url, {
            method: 'DELETE',
            headers: headers
        });

        if (!res.ok) {
            console.error(`Failed to delete task ${taskId}: ${res.statusText}`);
            // Don't throw - deletion failure shouldn't block error handling
        }

        console.log('Task deleted:', taskId);
    }
}
