import {headers} from "./fhir";
import {apiBase} from "../config";

export const taskShared = {
    async deleteTask(taskId) {
        // Delete a Task from the STU3 server
        const url = `${apiBase.fhirStu3}/Task/${taskId}`;
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
};
