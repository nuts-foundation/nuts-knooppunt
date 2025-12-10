import React, { useState, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { useAuth } from '../AuthProvider';
import { patientApi } from '../api/patientApi';
import { medicationApi } from '../api/medicationApi';
import { nviApi } from '../api/nviApi';
import { healthcareServiceApi } from '../api/healthcareServiceApi';
import { organizationApi } from '../api/organizationApi';
import { bgzVerweijzingApi } from '../api/bgzVerweijzingApi';
import { eOverdrachtApi } from '../api/eOverdrachtApi';
import { taskShared } from '../api/taskShared';
import { bgzApi } from '../api/bgzApi';
import { bgzVisualizationApi } from '../api/bgzVisualizationApi';

function PatientPage() {
  const { patientId } = useParams();
  const navigate = useNavigate();
  const { isAuthenticated, logout, user, practitionerId } = useAuth();

  const [patient, setPatient] = useState(null);
  const [medicationRequests, setMedicationRequests] = useState([]);
  const [medicationDispenses, setMedicationDispenses] = useState([]);
  const [careNetworkOrganizations, setCareNetworkOrganizations] = useState([]);

  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [medicationRequestsLoading, setMedicationRequestsLoading] = useState(true);
  const [medicationDispensesLoading, setMedicationDispensesLoading] = useState(true);
  const [medicationRequestsError, setMedicationRequestsError] = useState(null);
  const [medicationDispensesError, setMedicationDispensesError] = useState(null);
  const [careNetworkLoading, setCareNetworkLoading] = useState(true);
  const [careNetworkError, setCareNetworkError] = useState(null);
  const [showEOverdrachtModal, setShowEOverdrachtModal] = useState(false);
  const [eOverdrachtStep, setEOverdrachtStep] = useState('services'); // 'services', 'organizations', 'departments', or 'confirmation'
  const [selectedServiceGroup, setSelectedServiceGroup] = useState(null);
  const [selectedOrganization, setSelectedOrganization] = useState(null);
  const [selectedDepartment, setSelectedDepartment] = useState(null);
  const [healthcareServices, setHealthcareServices] = useState([]);
  const [groupedHealthcareServices, setGroupedHealthcareServices] = useState([]);
  const [healthcareServicesLoading, setHealthcareServicesLoading] = useState(false);
  const [healthcareServicesError, setHealthcareServicesError] = useState(null);
  const [organizations, setOrganizations] = useState([]);
  const [organizationsLoading, setOrganizationsLoading] = useState(false);
  const [organizationsError, setOrganizationsError] = useState(null);
  const [departments, setDepartments] = useState([]);
  const [departmentsLoading, setDepartmentsLoading] = useState(false);
  const [departmentsError, setDepartmentsError] = useState(null);
  const [taskCreating, setTaskCreating] = useState(false);
  const [taskSuccess, setTaskSuccess] = useState(false);
  const [taskError, setTaskError] = useState(null);
  const [eOverdrachtTasks, setEOverdrachtTasks] = useState([]);
  const [eOverdrachtTasksLoading, setEOverdrachtTasksLoading] = useState(true);
  const [eOverdrachtTasksError, setEOverdrachtTasksError] = useState(null);
  const [taskOrganizations, setTaskOrganizations] = useState({});
  const [expandedTaskJson, setExpandedTaskJson] = useState(null);
  const [bgzGenerating, setBgzGenerating] = useState(false);
  const [bgzError, setBgzError] = useState(null);
  const [bgzSuccess, setBgzSuccess] = useState(false);
  const [showBgzConfirmModal, setShowBgzConfirmModal] = useState(false);
  const [bgzSummary, setBgzSummary] = useState(null);
  const [bgzSummaryLoading, setBgzSummaryLoading] = useState(true);
  const [bgzSummaryError, setBgzSummaryError] = useState(null);
  const [showBgzDeleteModal, setShowBgzDeleteModal] = useState(false);
  const [bgzDeleting, setBgzDeleting] = useState(false);
  const [bgzDeleteError, setBgzDeleteError] = useState(null);
  const [isBgzReferral, setIsBgzReferral] = useState(false);
  const [createWorkflowTask, setCreateWorkflowTask] = useState(true);
  const [contextLaunchEndpoint, setContextLaunchEndpoint] = useState(null);
  const [workflowTaskId, setWorkflowTaskId] = useState(null);
  const [contextLaunchBSN, setContextLaunchBSN] = useState(null);

  useEffect(() => {
    if (isAuthenticated && patientId) {
      loadPatientData();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isAuthenticated, patientId]);

  useEffect(() => {
    if (showEOverdrachtModal) {
      loadHealthcareServices();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [showEOverdrachtModal]);

  const loadPatientData = async () => {
    setLoading(true);
    setError(null);

    try {
      // Fetch patient details
      const response = await fetch(`${require('../config').config.fhirBaseURL}/Patient/${patientId}`, {
        method: 'GET',
        headers: {
          'Accept': 'application/fhir+json',
          'Content-Type': 'application/fhir+json',
          'Cache-Control': 'no-cache',
        },
      });

      if (!response.ok) {
        throw new Error(`Failed to fetch patient: ${response.statusText}`);
      }

      const patientData = await response.json();
      setPatient(patientData);

      // Load medication data in parallel
      loadMedicationRequests(patientId);
      loadMedicationDispenses(patientId);

      // Load care network organizations
      const patientBSN = patientApi.getByBSN(patientData);
      if (patientBSN) {
        loadCareNetwork(patientBSN);
      } else {
        setCareNetworkLoading(false);
      }

      // Load eOvedracht tasks
      loadEOverdrachtTasks(patientId);

      // Load BGZ summary
      loadBGZSummary(patientId);
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  const loadBGZSummary = async (patientId) => {
    setBgzSummaryLoading(true);
    setBgzSummaryError(null);
    try {
      // Get the patient's BSN to look up the STU3 patient
      const response = await fetch(`${require('../config').config.fhirBaseURL}/Patient/${patientId}`, {
        method: 'GET',
        headers: require('../api/fhir').headers,
      });

      if (!response.ok) {
        throw new Error('Failed to fetch patient');
      }

      const patientData = await response.json();
      const bsn = patientApi.getByBSN(patientData);

      if (!bsn) {
        throw new Error('Patient does not have a BSN identifier');
      }

      // Search for the patient on STU3 endpoint by BSN
      const stu3Patients = await patientApi.searchByBSNOnStu3(bsn);

      if (stu3Patients.length === 0) {
        // No STU3 patient found, no BGZ summary available
        setBgzSummary(null);
        return;
      }

      const stu3PatientId = stu3Patients[0].id;
      const summary = await bgzVisualizationApi.getPatientSummary(stu3PatientId);
      setBgzSummary(summary);
    } catch (err) {
      setBgzSummaryError(err.message);
    } finally {
      setBgzSummaryLoading(false);
    }
  };

  const loadMedicationRequests = async (patientId) => {
    setMedicationRequestsLoading(true);
    setMedicationRequestsError(null);
    try {
      const data = await medicationApi.getMedicationRequests(patientId);
      setMedicationRequests(data);
    } catch (err) {
      setMedicationRequestsError(err.message);
    } finally {
      setMedicationRequestsLoading(false);
    }
  };

  const loadMedicationDispenses = async (patientId) => {
    setMedicationDispensesLoading(true);
    setMedicationDispensesError(null);
    try {
      const data = await medicationApi.getMedicationDispenses(patientId);
      setMedicationDispenses(data);
    } catch (err) {
      setMedicationDispensesError(err.message);
    } finally {
      setMedicationDispensesLoading(false);
    }
  };

  const loadCareNetwork = async (bsn) => {
    setCareNetworkLoading(true);
    setCareNetworkError(null);
    try {
      // Extract abonnee_nummer from the OIDC token (stored in 'sub' claim)
      const abonneeNummer = user?.profile?.sub;
      const organizations = await nviApi.searchOrganizationsByPatient(bsn, abonneeNummer);
      setCareNetworkOrganizations(organizations);
    } catch (err) {
      setCareNetworkError(err.message);
    } finally {
      setCareNetworkLoading(false);
    }
  };

  const loadEOverdrachtTasks = async (patientId) => {
    setEOverdrachtTasksLoading(true);
    setEOverdrachtTasksError(null);
    try {
      const tasks = await eOverdrachtApi.getEOverdrachtTasks(patientId);
      setEOverdrachtTasks(tasks);

      // Load organization details for each task
      const orgMap = {};
      for (const task of tasks) {
        if (task.owner?.reference) {
          try {
            // Extract organization ID from reference (format: Organization/id or full URL)
            const ref = task.owner.reference;
            const orgId = ref.includes('/') ? ref.split('/').pop() : ref;

            // Fetch organization details
            let org = await organizationApi.getById(orgId);
            if (org) {
              // Traverse up the partOf hierarchy to find the root organization
              let rootOrg = org;
              const visited = new Set(); // Prevent infinite loops

              while (rootOrg.partOf && rootOrg.partOf.reference) {
                const parentRef = rootOrg.partOf.reference;
                const parentId = parentRef.includes('/') ? parentRef.split('/').pop() : parentRef;

                // Check for circular references
                if (visited.has(parentId)) {
                  console.warn('Circular partOf reference detected for organization:', parentId);
                  break;
                }
                visited.add(parentId);

                try {
                  const parentOrg = await organizationApi.getById(parentId);
                  if (parentOrg) {
                    rootOrg = parentOrg;
                  } else {
                    break;
                  }
                } catch (err) {
                  console.warn('Failed to load parent organization:', parentId, err);
                  break;
                }
              }

              // Store the root organization
              orgMap[orgId] = rootOrg;
            }
          } catch (err) {
            console.warn('Failed to load organization for task:', task.id, err);
          }
        }
      }
      setTaskOrganizations(orgMap);
    } catch (err) {
      setEOverdrachtTasksError(err.message);
    } finally {
      setEOverdrachtTasksLoading(false);
    }
  };

  const loadHealthcareServices = async () => {
    setHealthcareServicesLoading(true);
    setHealthcareServicesError(null);
    try {
      const services = await healthcareServiceApi.list();
      setHealthcareServices(services);
      // Group services by name
      const grouped = healthcareServiceApi.groupByName(services);
      setGroupedHealthcareServices(grouped);
    } catch (err) {
      setHealthcareServicesError(err.message);
    } finally {
      setHealthcareServicesLoading(false);
    }
  };

  const loadOrganizationsForServiceGroup = async (serviceGroup) => {
    setOrganizationsLoading(true);
    setOrganizationsError(null);
    try {
      // Extract organization IDs from providedBy references
      const orgIds = healthcareServiceApi.getOrganizationIds(serviceGroup.services);
      if (orgIds.length === 0) {
        setOrganizations([]);
        return;
      }
      // Fetch organizations by IDs
      const orgs = await organizationApi.getByIds(orgIds);

      // Traverse to root organizations (with URA and no partOf)
      const rootOrgMap = new Map();
      for (const org of orgs) {
        try {
          let currentOrg = org;
          const visited = new Set([currentOrg.id]); // Prevent infinite loops

          // Traverse up the partOf hierarchy until we find a root organization
          while (currentOrg) {
            const hasURA = organizationApi.getURA(currentOrg);
            const hasPartOf = currentOrg.partOf && currentOrg.partOf.reference;

            // Check if this is a root organization (has URA and no partOf)
            if (hasURA && !hasPartOf) {
              rootOrgMap.set(currentOrg.id, currentOrg);
              break;
            }

            // If has partOf, traverse up
            if (hasPartOf) {
              const parentRef = currentOrg.partOf.reference;
              const parentId = parentRef.includes('/') ? parentRef.split('/').pop() : parentRef;

              // Check for circular references
              if (visited.has(parentId)) {
                console.warn('Circular partOf reference detected for organization:', parentId);
                break;
              }
              visited.add(parentId);

              try {
                const parentOrg = await organizationApi.getById(parentId);
                if (parentOrg) {
                  currentOrg = parentOrg;
                } else {
                  break;
                }
              } catch (err) {
                console.warn('Failed to load parent organization:', parentId, err);
                break;
              }
            } else {
              // Has no partOf but also no URA - stop here
              break;
            }
          }
        } catch (err) {
          console.warn('Failed to traverse organization hierarchy:', org.id, err);
        }
      }

      // Set deduplicated root organizations
      setOrganizations(Array.from(rootOrgMap.values()));
    } catch (err) {
      setOrganizationsError(err.message);
    } finally {
      setOrganizationsLoading(false);
    }
  };

  const loadDepartments = async (organizationId) => {
    setDepartmentsLoading(true);
    setDepartmentsError(null);
    try {
      const deps = await organizationApi.getSubOrganizations(organizationId);
      setDepartments(deps);
    } catch (err) {
      setDepartmentsError(err.message);
    } finally {
      setDepartmentsLoading(false);
    }
  };

  const handleServiceSelect = async (group) => {
    setSelectedServiceGroup(group);
    setEOverdrachtStep('organizations');
    await loadOrganizationsForServiceGroup(group);
  };

  const handleOrganizationSelect = async (org) => {
    setSelectedOrganization(org);
    setEOverdrachtStep('departments');
    await loadDepartments(org.id);
  };

  const handleCloseModal = () => {
    setShowEOverdrachtModal(false);
    // Reset state when modal closes
    setTimeout(() => {
      setEOverdrachtStep('services');
      setSelectedServiceGroup(null);
      setSelectedOrganization(null);
      setSelectedDepartment(null);
      setOrganizations([]);
      setOrganizationsError(null);
      setDepartments([]);
      setDepartmentsError(null);
      setTaskError(null);
      setTaskSuccess(false);
      setIsBgzReferral(false);
      setCreateWorkflowTask(true);
      setContextLaunchEndpoint(null);
      setWorkflowTaskId(null);
      setContextLaunchBSN(null);
    }, 300); // Delay to allow modal animation to complete
  };

  const handleBackToServices = () => {
    setEOverdrachtStep('services');
    setSelectedServiceGroup(null);
    setSelectedOrganization(null);
    setOrganizations([]);
    setOrganizationsError(null);
    setDepartments([]);
    setDepartmentsError(null);
  };

  const handleBackToOrganizations = () => {
    setEOverdrachtStep('organizations');
    setSelectedOrganization(null);
    setDepartments([]);
    setDepartmentsError(null);
    setTaskError(null);
    setTaskSuccess(false);
    setCreateWorkflowTask(true);
  };

  const handleDepartmentSelect = (department) => {
    setSelectedDepartment(department);
    setEOverdrachtStep('confirmation');
    setTaskError(null);
    setTaskSuccess(false);
    setCreateWorkflowTask(true);
  };

  const handleBackToDepartments = () => {
    setEOverdrachtStep('departments');
    setSelectedDepartment(null);
    setTaskError(null);
    setTaskSuccess(false);
    setCreateWorkflowTask(true);
  };

  const handleShowBgzConfirm = () => {
    setBgzError(null);
    setShowBgzConfirmModal(true);
  };

  const handleConfirmBGZ = async () => {
    setShowBgzConfirmModal(false);
    setBgzGenerating(true);
    setBgzError(null);
    setBgzSuccess(false);

    try {
      // Get patient's BSN
      const bsn = patientApi.getByBSN(patient);
      if (!bsn) {
        throw new Error('Patient does not have a BSN identifier');
      }

      console.log('Checking if patient exists on STU3 endpoint with BSN:', bsn);

      // Search for patient by BSN on STU3 endpoint
      let stu3Patients = await patientApi.searchByBSNOnStu3(bsn);
      let stu3PatientId;

      if (stu3Patients.length === 0) {
        console.log('Patient not found on STU3 endpoint, creating patient');

        // Patient doesn't exist on STU3, create it
        const patientFormData = patientApi.toForm(patient);
        const createdPatient = await patientApi.createOnStu3({
          bsn: patientFormData.bsn,
          given: patientFormData.given.split(' ').filter(n => n),
          family: patientFormData.family,
          prefix: patientFormData.prefix ? patientFormData.prefix.split(' ').filter(n => n) : [],
          birthDate: patientFormData.birthDate,
          gender: patientFormData.gender,
        });
        stu3PatientId = createdPatient.id;
        console.log('Patient created on STU3 endpoint with ID:', stu3PatientId);
      } else {
        stu3PatientId = stu3Patients[0].id;
        console.log('Patient found on STU3 endpoint with ID:', stu3PatientId);
      }

      console.log('Generating BGZ for patient:', stu3PatientId);
      await bgzApi.generateBGZ(stu3PatientId);
      console.log('BGZ generated successfully');
      setBgzSuccess(true);

      // Reload BGZ summary to update the display and hide the button
      await loadBGZSummary(patientId);

      // Reset success message after 3 seconds
      setTimeout(() => {
        setBgzSuccess(false);
      }, 3000);
    } catch (err) {
      console.error('Error generating BGZ:', err);
      setBgzError(err.message);
    } finally {
      setBgzGenerating(false);
    }
  };

  const handleShowBgzDelete = () => {
    setBgzDeleteError(null);
    setShowBgzDeleteModal(true);
  };

  const handleConfirmBgzDelete = async () => {
    setShowBgzDeleteModal(false);
    setBgzDeleting(true);
    setBgzDeleteError(null);

    try {
      console.log('Deleting BGZ data for patient:', patientId);

      // Get the patient's BSN to find the STU3 patient
      const bsn = patientApi.getByBSN(patient);
      if (!bsn) {
        throw new Error('Patient does not have a BSN identifier');
      }

      // Search for the STU3 patient by BSN
      const stu3Patients = await patientApi.searchByBSNOnStu3(bsn);
      let stu3PatientId = null;

      if (stu3Patients.length > 0) {
        stu3PatientId = stu3Patients[0].id;
        console.log('Found STU3 patient with ID:', stu3PatientId);
      }

      // Collect all resource IDs to delete (except patient)
      const resourcesToDelete = [];

      if (bgzSummary) {
        // Helper to add resources from an array
        const addResources = (resources) => {
          if (Array.isArray(resources)) {
            resources.forEach(resource => {
              if (resource.id && resource.resourceType && resource.resourceType !== 'Patient') {
                resourcesToDelete.push({ id: resource.id, type: resource.resourceType });
              }
            });
          }
        };

        // Add resources from all BGZ sections
        addResources(bgzSummary.paymentDetails);
        addResources(bgzSummary.treatmentDirectives);
        addResources(bgzSummary.advanceDirectives);
        addResources(bgzSummary.functionalStatus);
        addResources(bgzSummary.problems);
        addResources(bgzSummary.socialHistory?.livingSituation);
        addResources(bgzSummary.socialHistory?.drugUse);
        addResources(bgzSummary.socialHistory?.alcoholUse);
        addResources(bgzSummary.socialHistory?.tobaccoUse);
        addResources(bgzSummary.socialHistory?.nutritionAdvice);
        addResources(bgzSummary.alerts);
        addResources(bgzSummary.allergies);
        addResources(bgzSummary.medication?.medicationUse);
        addResources(bgzSummary.medication?.medicationAgreement);
        addResources(bgzSummary.medication?.administrationAgreement);
        addResources(bgzSummary.medicalAids);
        addResources(bgzSummary.vaccinations);
        addResources(bgzSummary.vitalSigns?.bloodPressure);
        addResources(bgzSummary.vitalSigns?.bodyWeight);
        addResources(bgzSummary.vitalSigns?.bodyHeight);
        addResources(bgzSummary.results);
        addResources(bgzSummary.procedures);
        addResources(bgzSummary.encounters);
        addResources(bgzSummary.plannedCare);
      }

      console.log(`Deleting ${resourcesToDelete.length} BGZ resources`);

      // Delete all resources
      const deletePromises = resourcesToDelete.map(async ({ id, type }) => {
        const url = `${require('../config').config.fhirStu3BaseURL}/${type}/${id}`;
        const response = await fetch(url, {
          method: 'DELETE',
          headers: {
            'Accept': 'application/fhir+json',
            'Content-Type': 'application/fhir+json',
          },
        });
        if (!response.ok) {
          console.warn(`Failed to delete ${type}/${id}: ${response.statusText}`);
        }
        return response;
      });

      await Promise.all(deletePromises);
      console.log('BGZ resources deleted successfully');

      // Delete the STU3 patient if it exists
      if (stu3PatientId) {
        console.log('Deleting STU3 patient:', stu3PatientId);
        const patientDeleteUrl = `${require('../config').config.fhirStu3BaseURL}/Patient/${stu3PatientId}`;
        const patientDeleteResponse = await fetch(patientDeleteUrl, {
          method: 'DELETE',
          headers: {
            'Accept': 'application/fhir+json',
            'Content-Type': 'application/fhir+json',
          },
        });

        if (!patientDeleteResponse.ok && patientDeleteResponse.status !== 204) {
          console.warn(`Failed to delete STU3 patient ${stu3PatientId}: ${patientDeleteResponse.statusText}`);
        } else {
          console.log('STU3 patient deleted successfully');
        }
      }

      // Reload BGZ summary to update the display
      await loadBGZSummary(patientId);

    } catch (err) {
      console.error('Error deleting BGZ data:', err);
      setBgzDeleteError(err.message);
    } finally {
      setBgzDeleting(false);
    }
  };

  const handleConfirmTask = async (isBgzReferralParam = false) => {
    setTaskCreating(true);
    setTaskError(null);
    setTaskSuccess(false);

    try {
      // Get practitioner ID from auth context
      if (!practitionerId) {
        throw new Error('Practitioner ID not found. Please log out and log back in.');
      }

      // Get user information for display
      const userName = user?.profile?.name || user?.profile?.email || 'Unknown User';
      const currentPatientBSN = patientApi.getByBSN(patient);

      let hasContextLaunchEndpoint = false;

      if(isBgzReferralParam) {
        const workflowTask = await handleConfirmBgZVerwijzing(userName, currentPatientBSN, createWorkflowTask);

        // Store workflow task ID and BSN for context launch
        if (workflowTask && workflowTask.id) {
          setWorkflowTaskId(workflowTask.id);
          console.log('Stored workflow task ID:', workflowTask.id);
        }
        setContextLaunchBSN(currentPatientBSN);

        // After successful BgZ Verwijzing, search for context-launch endpoint
        console.log('Searching for context-launch endpoint for organization:', selectedDepartment.id);
        try {
          const launchEndpoint = await organizationApi.getEndpoint(selectedDepartment.id, "context-launch");
          if (launchEndpoint) {
            console.log('Found context-launch endpoint:', launchEndpoint.address);
            setContextLaunchEndpoint(launchEndpoint);
            hasContextLaunchEndpoint = true;
          } else {
            console.log('No context-launch endpoint found');
            setContextLaunchEndpoint(null);
          }
        } catch (err) {
          console.warn('Error searching for context-launch endpoint:', err);
          setContextLaunchEndpoint(null);
        }
      } else {
        await handleConfirmEOverdracht(userName);
      }

      setTaskSuccess(true);

      // Reload tasks list
      loadEOverdrachtTasks(patientId);

      // Only auto-close modal if no context-launch endpoint for BgZ Verwijzing
      if (!isBgzReferralParam || !hasContextLaunchEndpoint) {
        // Show success message for 2 seconds, then close modal
        setTimeout(() => {
          handleCloseModal();
        }, 2000);
      }
    } catch (err) {
      console.error('Error creating Task:', err);
      setTaskError(err.message);
    } finally {
      setTaskCreating(false);
    }
  };

  const handleConfirmEOverdracht = async (userName) => {
    console.log('Creating eOverdracht Task...');
    const createdTask = await eOverdrachtApi.createEOverdrachtTask(
        patientId,
        patientName,
        practitionerId,
        userName,
        selectedDepartment.id,
        organizationApi.formatName(selectedDepartment)
    );
    console.log('Created Task:', createdTask);

    // Step 2: Find endpoint and notify receiving party
    console.log('Finding endpoint for organization:', selectedDepartment.id);
    const endpoint = await organizationApi.getEndpoint(selectedDepartment.id, "eOverdracht-notification");

    if (!endpoint) {
      // No endpoint found - delete task and fail
      console.error('No endpoint found for organization');
      await taskShared.deleteTask(createdTask.id);
      throw new Error('No relevant (eOverdracht-notification) endpoint found for the selected organization (or department)');
    }

    console.log('Found endpoint:', endpoint.address);
    console.log('Sending notification to receiving party...');

    try {
      await eOverdrachtApi.notifyReceivingParty(createdTask, endpoint);
      console.log('Notification sent successfully');
    } catch (notifyErr) {
      // Notification failed - delete task and fail
      console.error('Failed to notify receiving party:', notifyErr);
      await taskShared.deleteTask(createdTask.id);
      throw new Error(`Failed to notify receiving party: ${notifyErr.message}`);
    }
  }

  const handleContextLaunch = () => {
    if (!contextLaunchEndpoint || !contextLaunchEndpoint.address) {
      console.error('No context-launch endpoint available');
      return;
    }

    // Build navigation URL with query parameters
    const params = new URLSearchParams({
      launchUrl: contextLaunchEndpoint.address,
      ...(workflowTaskId && { workflow: workflowTaskId }),
      ...(contextLaunchBSN && { patient: contextLaunchBSN })
    });

    const navigationUrl = `/patients/${patientId}/context-launch?${params.toString()}`;
    console.log('Navigating to context launch page:', navigationUrl);

    // Navigate to context launch page
    navigate(navigationUrl);
  };

  const handleConfirmBgZVerwijzing = async (userName, bsn, createWorkflowTaskFlag) => {
    console.log('Creating BgZ Verwijzing Task...');
    console.log('Create workflow task selected:', !!createWorkflowTaskFlag);

    let createdWorkflowTask
    if(createWorkflowTaskFlag) {
      createdWorkflowTask = await bgzVerweijzingApi.createBgZWorkflowTask(
          bsn,
          patientName,
          practitionerId,
          userName,
          selectedDepartment.id,
          organizationApi.formatName(selectedDepartment)
      );
      console.log('Created Workflow Task:', createdWorkflowTask);
    }


    // Step 2: Find endpoint and notify receiving party
    console.log('Finding Twiin-TA-notification endpoint for organization:', selectedDepartment.id);
    const endpoint = await organizationApi.getEndpoint(selectedDepartment.id, "Twiin-TA-notification");

    if(!endpoint) {
      if (createdWorkflowTask != null) {
        // No endpoint found - delete task and fail
        console.error('No endpoint found for organization');
        await taskShared.deleteTask(createdWorkflowTask.id);
      }
      throw new Error('No relevant (Twiin-TA-notification) endpoint found for the selected organization (or department)');
    }


    console.log('Found endpoint:', endpoint.address);
    console.log('Sending notification to receiving party...');

    try {
      await bgzVerweijzingApi.createBgZNotificatonTask(bsn, createdWorkflowTask, '', user, selectedDepartment, endpoint.address, selectedOrganization);
      console.log('Notification sent successfully');
    } catch (notifyErr) {
      // Notification failed - delete task and fail
      console.error('Failed to notify receiving party:', notifyErr);
      if (createdWorkflowTask != null) {
        await taskShared.deleteTask(createdWorkflowTask.id);
      }
      throw new Error(`Failed to notify receiving party: ${notifyErr.message}`);
    }

    // Return the workflow task ID for context launch
    return createdWorkflowTask;
  }

  const formatDate = (dateString) => {
    if (!dateString) return '-';
    try {
      return new Date(dateString).toLocaleDateString('nl-NL');
    } catch {
      return dateString;
    }
  };

  const getGenderIcon = (gender) => {
    switch (gender?.toLowerCase()) {
      case 'male': return '♂️';
      case 'female': return '♀️';
      default: return '⚪';
    }
  };

  const calculateAge = (birthDate) => {
    if (!birthDate) return '-';
    try {
      const today = new Date();
      const birth = new Date(birthDate);
      const age = Math.floor((today - birth) / (365.25 * 24 * 60 * 60 * 1000));
      return age;
    } catch {
      return '-';
    }
  };

  if (!isAuthenticated) {
    return (
      <div className="app-container">
        <div className="loading">Please log in to view patient details.</div>
      </div>
    );
  }

  if (loading) {
    return (
      <div className="app-container">
        <header className="header">
          <div className="header-content">
            <div>
              <h1>🏥 Demo EHR - Patient Details</h1>
              <div className="header-subtitle">Loading patient information...</div>
            </div>
            <button onClick={logout} className="button button-secondary">
              Logout
            </button>
          </div>
        </header>
        <main className="main-content">
          <div className="loading-container">
            <div className="spinner"></div>
            <p>Loading patient data...</p>
          </div>
        </main>
      </div>
    );
  }

  if (error) {
    return (
      <div className="app-container">
        <header className="header">
          <div className="header-content">
            <div>
              <h1>🏥 Demo EHR - Patient Details</h1>
              <div className="header-subtitle">Error loading patient</div>
            </div>
            <button onClick={logout} className="button button-secondary">
              Logout
            </button>
          </div>
        </header>
        <main className="main-content">
          <div className="error-container">
            <div className="error-message">
              <strong>Error loading patient</strong>
              <p>{error}</p>
              <button onClick={() => navigate('/patients')} className="button" style={{ marginTop: '15px' }}>
                ← Back to Patients
              </button>
            </div>
          </div>
        </main>
      </div>
    );
  }

  if (!patient) {
    return (
      <div className="app-container">
        <header className="header">
          <div className="header-content">
            <div>
              <h1>🏥 Demo EHR - Patient Details</h1>
              <div className="header-subtitle">Patient not found</div>
            </div>
            <button onClick={logout} className="button button-secondary">
              Logout
            </button>
          </div>
        </header>
        <main className="main-content">
          <div className="error-container">
            <div className="error-message">
              <strong>Patient not found</strong>
              <p>The patient with ID "{patientId}" could not be found.</p>
              <button onClick={() => navigate('/patients')} className="button" style={{ marginTop: '15px' }}>
                ← Back to Patients
              </button>
            </div>
          </div>
        </main>
      </div>
    );
  }

  const patientName = patientApi.formatName(patient);
  const patientBSN = patientApi.getByBSN(patient);
  const patientGender = patientApi.formatGender(patient);
  const patientBirthDate = patientApi.formatBirthDate(patient);
  const patientAge = calculateAge(patientBirthDate);

  // Helper function to check if array has actual data (excluding OperationOutcome resources)
  const hasActualData = (arr) => {
    if (!arr || !Array.isArray(arr) || arr.length === 0) return false;
    // Filter out OperationOutcome resources - they're just metadata, not actual patient data
    const actualResources = arr.filter(item => item?.resourceType !== 'OperationOutcome');
    return actualResources.length > 0;
  };

  // Check if BGZ data exists (any section has data)
  // NOTE: Patient presence alone doesn't count as having BGZ data
  const bgzDataChecks = bgzSummary && !bgzSummaryLoading ? {
    paymentDetails: hasActualData(bgzSummary.paymentDetails),
    treatmentDirectives: hasActualData(bgzSummary.treatmentDirectives),
    advanceDirectives: hasActualData(bgzSummary.advanceDirectives),
    functionalStatus: hasActualData(bgzSummary.functionalStatus),
    problems: hasActualData(bgzSummary.problems),
    socialHistory: bgzSummary.socialHistory && (
      hasActualData(bgzSummary.socialHistory.livingSituation) ||
      hasActualData(bgzSummary.socialHistory.drugUse) ||
      hasActualData(bgzSummary.socialHistory.alcoholUse) ||
      hasActualData(bgzSummary.socialHistory.tobaccoUse) ||
      hasActualData(bgzSummary.socialHistory.nutritionAdvice)
    ),
    alerts: hasActualData(bgzSummary.alerts),
    allergies: hasActualData(bgzSummary.allergies),
    medication: bgzSummary.medication && (
      hasActualData(bgzSummary.medication.medicationUse) ||
      hasActualData(bgzSummary.medication.medicationAgreement) ||
      hasActualData(bgzSummary.medication.administrationAgreement)
    ),
    medicalAids: hasActualData(bgzSummary.medicalAids),
    vaccinations: hasActualData(bgzSummary.vaccinations),
    vitalSigns: bgzSummary.vitalSigns && (
      hasActualData(bgzSummary.vitalSigns.bloodPressure) ||
      hasActualData(bgzSummary.vitalSigns.bodyWeight) ||
      hasActualData(bgzSummary.vitalSigns.bodyHeight)
    ),
    results: hasActualData(bgzSummary.results),
    procedures: hasActualData(bgzSummary.procedures),
    encounters: hasActualData(bgzSummary.encounters),
    plannedCare: hasActualData(bgzSummary.plannedCare)
  } : {};

  const hasBgzData = Object.values(bgzDataChecks).some(check => check === true);

  // Debug logging
  console.log('BGZ Summary State:', {
    hasSummary: !!bgzSummary,
    isLoading: bgzSummaryLoading,
    hasError: !!bgzSummaryError,
    hasBgzData: hasBgzData,
    shouldShowGenerateButton: !hasBgzData && !bgzSummaryLoading,
    bgzDataChecks: bgzDataChecks
  });

  // Log sections that have data
  if (bgzSummary && !bgzSummaryLoading) {
    if (bgzSummary.paymentDetails?.length > 0) {
      console.log('paymentDetails data:', bgzSummary.paymentDetails);
    }
    if (bgzSummary.medicalAids?.length > 0) {
      console.log('medicalAids data:', bgzSummary.medicalAids);
    }
  }

  return (
    <div className="app-container">
      <header className="header">
        <div className="header-content">
          <div>
            <h1>🏥 Demo EHR - Patient Details</h1>
            <div className="header-subtitle">{patientName}</div>
          </div>
          <button onClick={logout} className="button button-secondary">
            Logout
          </button>
        </div>
      </header>

      <main className="main-content">
        <div style={{ marginBottom: '20px' }}>
          <button onClick={() => navigate('/patients')} className="button button-secondary">
            ← Back to Patients
          </button>
        </div>

        <div className="patient-split-layout">
          {/* Left side - Patient Details */}
          <div className="patient-details-section">
            <div className="card">
              <h2 style={{ marginTop: 0 }}>Patient Information</h2>

              <div className="patient-detail-grid">
                <div className="patient-detail-item">
                  <div className="patient-detail-label">Name</div>
                  <div className="patient-detail-value">{patientName}</div>
                </div>

                <div className="patient-detail-item">
                  <div className="patient-detail-label">BSN</div>
                  <div className="patient-detail-value">
                    {patientBSN ? (
                      <span className="bsn-badge">{patientBSN}</span>
                    ) : (
                      <span className="text-muted">-</span>
                    )}
                  </div>
                </div>

                <div className="patient-detail-item">
                  <div className="patient-detail-label">Gender</div>
                  <div className="patient-detail-value">
                    <span className="gender-badge">
                      {getGenderIcon(patientGender)} {patientGender}
                    </span>
                  </div>
                </div>

                <div className="patient-detail-item">
                  <div className="patient-detail-label">Birth Date</div>
                  <div className="patient-detail-value">{formatDate(patientBirthDate)}</div>
                </div>

                <div className="patient-detail-item">
                  <div className="patient-detail-label">Age</div>
                  <div className="patient-detail-value">{patientAge} years</div>
                </div>
              </div>

              {patient.address && patient.address.length > 0 && (
                <>
                  <h3 style={{ marginTop: '30px', marginBottom: '15px' }}>Address</h3>
                  {patient.address.map((addr, idx) => (
                    <div key={idx} className="patient-detail-grid">
                      {addr.line && (
                        <div className="patient-detail-item">
                          <div className="patient-detail-label">Street</div>
                          <div className="patient-detail-value">{addr.line.join(', ')}</div>
                        </div>
                      )}
                      {addr.city && (
                        <div className="patient-detail-item">
                          <div className="patient-detail-label">City</div>
                          <div className="patient-detail-value">{addr.city}</div>
                        </div>
                      )}
                      {addr.postalCode && (
                        <div className="patient-detail-item">
                          <div className="patient-detail-label">Postal Code</div>
                          <div className="patient-detail-value">{addr.postalCode}</div>
                        </div>
                      )}
                      {addr.country && (
                        <div className="patient-detail-item">
                          <div className="patient-detail-label">Country</div>
                          <div className="patient-detail-value">{addr.country}</div>
                        </div>
                      )}
                    </div>
                  ))}
                </>
              )}

              {patient.telecom && patient.telecom.length > 0 && (
                <>
                  <h3 style={{ marginTop: '30px', marginBottom: '15px' }}>Contact</h3>
                  <div className="patient-detail-grid">
                    {patient.telecom.map((contact, idx) => (
                      <div key={idx} className="patient-detail-item">
                        <div className="patient-detail-label">{contact.system || 'Contact'}</div>
                        <div className="patient-detail-value">{contact.value}</div>
                      </div>
                    ))}
                  </div>
                </>
              )}
            </div>

            {/* Care Network Card */}
            <div className="card" style={{ marginTop: '20px' }}>
              <h2 style={{ marginTop: 0 }}>🏥 Care Network</h2>
              <p style={{ color: '#666', fontSize: '14px', marginBottom: '15px' }}>
                Organizations that have data for this patient
              </p>

              {careNetworkLoading ? (
                <div className="loading-container" style={{ padding: '30px' }}>
                  <div className="spinner"></div>
                  <p>Loading care network...</p>
                </div>
              ) : careNetworkError ? (
                <div className="error-message">
                  <strong>Error loading care network</strong>
                  <p>{careNetworkError}</p>
                </div>
              ) : !patientBSN ? (
                <div className="empty-state" style={{ padding: '30px' }}>
                  <p>No BSN available for this patient</p>
                </div>
              ) : careNetworkOrganizations.length === 0 ? (
                <div className="empty-state" style={{ padding: '30px' }}>
                  <p>No organizations found in care network</p>
                </div>
              ) : (
                <div className="care-network-list">
                  {careNetworkOrganizations.map((org, idx) => (
                    <div key={org.ura || idx} className="care-network-item">
                      <div className="care-network-item-header">
                        <span className="care-network-org-name">{org.name}</span>
                        <span className="care-network-doc-count">
                          {org.documentCount} document{org.documentCount !== 1 ? 's' : ''}
                        </span>
                      </div>
                      <div className="care-network-ura">
                        <span className="care-network-ura-label">URA:</span>
                        <span className="care-network-ura-value">{nviApi.formatURA(org.ura)}</span>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>

            {/* eOvedracht Tasks Card */}
            <div className="card" style={{ marginTop: '20px' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '10px' }}>
                <h2 style={{ marginTop: 0, marginBottom: 0 }}>📋 eOvedracht Tasks</h2>
                <button onClick={() => { setIsBgzReferral(false); setShowEOverdrachtModal(true); }} className="button" style={{ fontSize: '14px', padding: '8px 16px' }}>
                  eOvedracht
                </button>
              </div>
              <p style={{ color: '#666', fontSize: '14px', marginBottom: '15px' }}>
                Care handover tasks for this patient
              </p>

              {eOverdrachtTasksLoading ? (
                <div className="loading-container" style={{ padding: '30px' }}>
                  <div className="spinner"></div>
                  <p>Loading eOvedracht tasks...</p>
                </div>
              ) : eOverdrachtTasksError ? (
                <div className="error-message">
                  <strong>Error loading eOvedracht tasks</strong>
                  <p>{eOverdrachtTasksError}</p>
                </div>
              ) : eOverdrachtTasks.length === 0 ? (
                <div className="empty-state" style={{ padding: '30px' }}>
                  <p>No eOvedracht tasks found</p>
                </div>
              ) : (
                <div className="care-network-list">
                  {eOverdrachtTasks.map((task, idx) => {
                    // Extract organization ID from task owner reference
                    const orgRef = task.owner?.reference;
                    const orgId = orgRef ? (orgRef.includes('/') ? orgRef.split('/').pop() : orgRef) : null;
                    const organization = orgId ? taskOrganizations[orgId] : null;

                    return (
                      <div key={task.id || idx} className="care-network-item">
                        <div className="care-network-item-header">
                          <span className="care-network-org-name">
                            {task.owner?.display || 'Unknown Organization'}
                          </span>
                          <span className={`status-badge ${task.status === 'in-progress' ? 'status-active' : task.status === 'completed' ? 'status-inactive' : ''}`}>
                            {task.status || 'unknown'}
                          </span>
                        </div>

                        {/* Organization Details */}
                        {organization && (
                          <div style={{ marginTop: '10px', fontSize: '13px', color: '#666' }}>
                            {organizationApi.getURA(organization) && (
                              <div style={{ marginBottom: '5px' }}>
                                <strong>URA:</strong> {organizationApi.getURA(organization)}
                              </div>
                            )}
                            {organizationApi.formatAddress(organization) !== '-' && (
                              <div style={{ marginBottom: '5px' }}>
                                <strong>Address:</strong> {organizationApi.formatAddress(organization)}
                              </div>
                            )}
                            {organizationApi.formatType(organization) !== '-' && (
                              <div style={{ marginBottom: '5px' }}>
                                <strong>Type:</strong> {organizationApi.formatType(organization)}
                              </div>
                            )}
                            {organizationApi.formatTelecomString(organization) !== '-' && (
                              <div style={{ marginBottom: '5px' }}>
                                <strong>Contact:</strong> {organizationApi.formatTelecomString(organization)}
                              </div>
                            )}
                          </div>
                        )}

                        {/* Requester Information */}
                        {task.requester?.agent?.display && (
                          <div style={{ fontSize: '13px', color: '#666', marginTop: '8px' }}>
                            <strong>Requested by:</strong> {task.requester.agent.display}
                          </div>
                        )}

                        {/* Created Date */}
                        {task.authoredOn && (
                          <div style={{ fontSize: '13px', color: '#666', marginTop: '5px' }}>
                            <strong>Created:</strong> {new Date(task.authoredOn).toLocaleDateString('nl-NL', {
                              year: 'numeric',
                              month: 'long',
                              day: 'numeric',
                              hour: '2-digit',
                              minute: '2-digit'
                            })}
                          </div>
                        )}

                        {/* Task ID */}
                        {task.id && (
                          <div
                            onClick={() => setExpandedTaskJson(expandedTaskJson === task.id ? null : task.id)}
                            style={{
                              fontSize: '12px',
                              color: '#2563eb',
                              marginTop: '5px',
                              cursor: 'pointer',
                              padding: '4px 8px',
                              backgroundColor: '#f0f4ff',
                              borderRadius: '4px',
                              display: 'inline-block',
                              transition: 'all 0.2s'
                            }}
                            onMouseEnter={(e) => {
                              e.currentTarget.style.backgroundColor = '#dbeafe';
                              e.currentTarget.style.textDecoration = 'underline';
                            }}
                            onMouseLeave={(e) => {
                              e.currentTarget.style.backgroundColor = '#f0f4ff';
                              e.currentTarget.style.textDecoration = 'none';
                            }}
                          >
                            Task ID: {task.id} {expandedTaskJson === task.id ? '▼' : '▶'}
                          </div>
                        )}

                        {/* Task JSON Display */}
                        {expandedTaskJson === task.id && (
                          <div style={{ marginTop: '10px', backgroundColor: '#f8f9fa', borderRadius: '4px', padding: '10px' }}>
                            <div style={{ fontSize: '13px', fontWeight: 'bold', marginBottom: '8px', color: '#333' }}>
                              Task JSON:
                            </div>
                            <pre style={{
                              backgroundColor: '#2d2d2d',
                              color: '#f8f8f2',
                              padding: '12px',
                              borderRadius: '4px',
                              fontSize: '12px',
                              overflow: 'auto',
                              maxHeight: '400px',
                              margin: 0,
                              fontFamily: 'Monaco, Consolas, "Courier New", monospace'
                            }}>
                              {JSON.stringify(task, null, 2)}
                            </pre>
                          </div>
                        )}
                      </div>
                    );
                  })}
                </div>
              )}
            </div>
          </div>

          {/* Right side - FHIR Resources */}
          <div className="patient-resources-section">
            {/* Medication Requests */}
            <div className="card" style={{ marginBottom: '20px' }}>
              <h2 style={{ marginTop: 0 }}>💊 Medication Requests</h2>

              {medicationRequestsLoading ? (
                <div className="loading-container">
                  <div className="spinner"></div>
                  <p>Loading medication requests...</p>
                </div>
              ) : medicationRequestsError ? (
                <div className="error-message">
                  <strong>Error loading medication requests</strong>
                  <p>{medicationRequestsError}</p>
                </div>
              ) : medicationRequests.length === 0 ? (
                <div className="empty-state" style={{ padding: '30px' }}>
                  <p>No medication requests found</p>
                </div>
              ) : (
                <div className="resource-table-container">
                  <table className="resource-table">
                    <thead>
                      <tr>
                        <th>Medication</th>
                        <th>Dosage</th>
                        <th>Status</th>
                        <th>Date</th>
                      </tr>
                    </thead>
                    <tbody>
                      {medicationRequests.map((req) => (
                        <tr key={req.id}>
                          <td className="medication-name">{medicationApi.formatMedication(req)}</td>
                          <td>{medicationApi.formatDosage(req)}</td>
                          <td>
                            <span className={`status-badge ${medicationApi.getStatusClass(req.status)}`}>
                              {req.status || 'unknown'}
                            </span>
                          </td>
                          <td>{medicationApi.formatDate(req.authoredOn)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </div>

            {/* Medication Dispenses */}
            <div className="card">
              <h2 style={{ marginTop: 0 }}>💊 Medication Dispenses</h2>

              {medicationDispensesLoading ? (
                <div className="loading-container">
                  <div className="spinner"></div>
                  <p>Loading medication dispenses...</p>
                </div>
              ) : medicationDispensesError ? (
                <div className="error-message">
                  <strong>Error loading medication dispenses</strong>
                  <p>{medicationDispensesError}</p>
                </div>
              ) : medicationDispenses.length === 0 ? (
                <div className="empty-state" style={{ padding: '30px' }}>
                  <p>No medication dispenses found</p>
                </div>
              ) : (
                <div className="resource-table-container">
                  <table className="resource-table">
                    <thead>
                      <tr>
                        <th>Medication</th>
                        <th>Dosage</th>
                        <th>Status</th>
                        <th>Date</th>
                      </tr>
                    </thead>
                    <tbody>
                      {medicationDispenses.map((disp) => (
                        <tr key={disp.id}>
                          <td className="medication-name">{medicationApi.formatMedication(disp)}</td>
                          <td>{medicationApi.formatDosage(disp)}</td>
                          <td>
                            <span className={`status-badge ${medicationApi.getStatusClass(disp.status)}`}>
                              {disp.status || 'unknown'}
                            </span>
                          </td>
                          <td>{medicationApi.formatDate(disp.whenHandedOver || disp.whenPrepared)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </div>

            {/* BGZ Patient Summary */}
            <div className="card" style={{ marginTop: '20px' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '10px' }}>
                <h2 style={{ marginTop: 0, marginBottom: 0 }}>📋 Patient Summary (BGZ)</h2>
                {hasBgzData && !bgzSummaryLoading && (
                  <div style={{ display: 'flex', gap: '8px' }}>
                    <button
                      onClick={() => { setIsBgzReferral(true); setShowEOverdrachtModal(true); }}
                      className="button"
                      style={{
                        fontSize: '13px',
                        padding: '6px 12px'
                      }}
                    >
                      BgZ verwijzing
                    </button>
                    <button
                      onClick={handleShowBgzDelete}
                      disabled={bgzDeleting}
                      title={bgzDeleting ? 'Deleting BGZ data...' : 'Delete BGZ data'}
                      className="button button-secondary"
                      style={{
                        fontSize: '16px',
                        padding: '4px 8px',
                        minWidth: 'auto',
                        backgroundColor: '#ef4444',
                        borderColor: '#ef4444',
                        color: 'white'
                      }}
                    >
                      {bgzDeleting ? '⏳' : '🗑️'}
                    </button>
                  </div>
                )}
              </div>

              {bgzSummaryLoading ? (
                <div className="loading-container">
                  <div className="spinner"></div>
                  <p>Loading patient summary...</p>
                </div>
              ) : bgzSummaryError ? (
                <div className="error-message">
                  <strong>Error loading patient summary</strong>
                  <p>{bgzSummaryError}</p>
                </div>
              ) : !hasBgzData ? (
                <div style={{ textAlign: 'center', padding: '40px 20px' }}>
                  <p style={{ color: '#666', marginBottom: '20px' }}>No BGZ data available for this patient.</p>
                  <button
                    onClick={handleShowBgzConfirm}
                    disabled={bgzGenerating}
                    className="button"
                  >
                    {bgzGenerating ? 'Generating...' : 'Generate BGZ'}
                  </button>
                  {bgzSuccess && (
                    <div style={{ color: '#10b981', fontSize: '14px', fontWeight: '500', marginTop: '15px' }}>
                      ✓ BGZ generated successfully
                    </div>
                  )}
                  {bgzError && (
                    <div style={{ color: '#ef4444', fontSize: '14px', fontWeight: '500', marginTop: '15px' }}>
                      ✗ {bgzError}
                    </div>
                  )}
                </div>
              ) : bgzSummary ? (
                <div style={{ fontSize: '14px' }}>
                  {/* 1. Patient information */}
                  <div style={{ marginBottom: '15px' }}>
                    <h3 style={{ fontSize: '14px', fontWeight: '600', marginBottom: '8px' }}>1. Patient Information</h3>
                    {bgzSummary.patient ? (
                      <div style={{ fontSize: '13px', lineHeight: '1.8' }}>
                        <div><strong>Name:</strong> {patientApi.formatName(bgzSummary.patient)}</div>
                        <div><strong>BSN:</strong> {patientApi.getByBSN(bgzSummary.patient) || 'Not available'}</div>
                        <div><strong>Birth Date:</strong> {patientApi.formatBirthDate(bgzSummary.patient)}</div>
                        <div><strong>Gender:</strong> {patientApi.formatGender(bgzSummary.patient)}</div>
                        {bgzSummary.patient.address && bgzSummary.patient.address[0] && (
                          <div>
                            <strong>Address:</strong> {' '}
                            {[
                              bgzSummary.patient.address[0].line?.join(', '),
                              bgzSummary.patient.address[0].postalCode,
                              bgzSummary.patient.address[0].city
                            ].filter(Boolean).join(', ')}
                          </div>
                        )}
                        {bgzSummary.patient.telecom && bgzSummary.patient.telecom.length > 0 && (
                          <div>
                            <strong>Contact:</strong> {' '}
                            {bgzSummary.patient.telecom
                              .filter(t => t.value)
                              .map(t => `${t.system || 'contact'}: ${t.value}`)
                              .join(', ')}
                          </div>
                        )}
                        {bgzSummary.patient.maritalStatus && (
                          <div>
                            <strong>Marital Status:</strong> {' '}
                            {bgzSummary.patient.maritalStatus.text ||
                             bgzSummary.patient.maritalStatus.coding?.[0]?.display ||
                             'Not specified'}
                          </div>
                        )}
                      </div>
                    ) : (
                      <div style={{ color: '#999' }}>No data</div>
                    )}
                  </div>

                  {/* 2. Payment details */}
                  <div style={{ marginBottom: '15px' }}>
                    <h3 style={{ fontSize: '14px', fontWeight: '600', marginBottom: '8px' }}>2. Payment Details</h3>
                    {bgzSummary.paymentDetails?.length > 0 ? (
                      <ul style={{ margin: 0, paddingLeft: '20px' }}>
                        {bgzSummary.paymentDetails.filter(pd => pd.resourceType === 'Coverage').map((coverage, idx) => (
                          <li key={coverage.id || idx}>
                            {coverage.type?.text || coverage.type?.coding?.[0]?.display || 'Insurance'}
                            {coverage.payor && coverage.payor[0]?.display && (
                              <span style={{ color: '#666', marginLeft: '5px' }}>- {coverage.payor[0].display}</span>
                            )}
                          </li>
                        ))}
                      </ul>
                    ) : (
                      <div style={{ color: '#999' }}>No data</div>
                    )}
                  </div>

                  {/* 3. Treatment directives */}
                  <div style={{ marginBottom: '15px' }}>
                    <h3 style={{ fontSize: '14px', fontWeight: '600', marginBottom: '8px' }}>3. Treatment Directives</h3>
                    {(bgzSummary.treatmentDirectives?.length > 0 || bgzSummary.advanceDirectives?.length > 0) ? (
                      <div>
                        {bgzSummary.treatmentDirectives?.length > 0 && (
                          <div style={{ marginBottom: '8px' }}>
                            <div style={{ fontWeight: '600', fontSize: '13px', marginBottom: '4px' }}>Treatment Directives:</div>
                            <ul style={{ margin: 0, paddingLeft: '20px' }}>
                              {bgzSummary.treatmentDirectives.map((consent, idx) => (
                                <li key={consent.id || idx}>
                                  {consent.status}: {consent.scope?.text || consent.scope?.coding?.[0]?.display || 'Treatment directive'}
                                  {consent.dateTime && <span style={{ color: '#666', marginLeft: '5px' }}>({new Date(consent.dateTime).toLocaleDateString()})</span>}
                                </li>
                              ))}
                            </ul>
                          </div>
                        )}
                        {bgzSummary.advanceDirectives?.length > 0 && (
                          <div>
                            <div style={{ fontWeight: '600', fontSize: '13px', marginBottom: '4px' }}>Advance Directives:</div>
                            <ul style={{ margin: 0, paddingLeft: '20px' }}>
                              {bgzSummary.advanceDirectives.map((consent, idx) => (
                                <li key={consent.id || idx}>
                                  {consent.status}: {consent.scope?.text || consent.scope?.coding?.[0]?.display || 'Advance directive'}
                                  {consent.dateTime && <span style={{ color: '#666', marginLeft: '5px' }}>({new Date(consent.dateTime).toLocaleDateString()})</span>}
                                </li>
                              ))}
                            </ul>
                          </div>
                        )}
                      </div>
                    ) : (
                      <div style={{ color: '#999' }}>No data</div>
                    )}
                  </div>

                  {/* 4. Contact persons */}
                  <div style={{ marginBottom: '15px' }}>
                    <h3 style={{ fontSize: '14px', fontWeight: '600', marginBottom: '8px' }}>4. Contact Persons</h3>
                    <div style={{ color: '#999' }}>See patient information</div>
                  </div>

                  {/* 5. Functional status */}
                  <div style={{ marginBottom: '15px' }}>
                    <h3 style={{ fontSize: '14px', fontWeight: '600', marginBottom: '8px' }}>5. Functional Status</h3>
                    {bgzSummary.functionalStatus?.length > 0 ? (
                      <ul style={{ margin: 0, paddingLeft: '20px' }}>
                        {bgzSummary.functionalStatus.map((obs, idx) => (
                          <li key={obs.id || idx}>
                            {obs.code?.text || obs.code?.coding?.[0]?.display || 'Functional status'}
                            {obs.valueCodeableConcept && (
                              <span style={{ color: '#666', marginLeft: '5px' }}>
                                : {obs.valueCodeableConcept.text || obs.valueCodeableConcept.coding?.[0]?.display}
                              </span>
                            )}
                            {obs.valueString && <span style={{ color: '#666', marginLeft: '5px' }}>: {obs.valueString}</span>}
                            {obs.effectiveDateTime && <span style={{ color: '#999', marginLeft: '5px', fontSize: '12px' }}>({new Date(obs.effectiveDateTime).toLocaleDateString()})</span>}
                          </li>
                        ))}
                      </ul>
                    ) : (
                      <div style={{ color: '#999' }}>No data</div>
                    )}
                  </div>

                  {/* 6. Problems */}
                  <div style={{ marginBottom: '15px' }}>
                    <h3 style={{ fontSize: '14px', fontWeight: '600', marginBottom: '8px' }}>6. Problems</h3>
                    {bgzSummary.problems?.length > 0 ? (
                      <ul style={{ margin: 0, paddingLeft: '20px' }}>
                        {bgzSummary.problems.map((problem, idx) => (
                          <li key={problem.id || idx}>
                            {problem.code?.text || problem.code?.coding?.[0]?.display || 'Unknown problem'}
                          </li>
                        ))}
                      </ul>
                    ) : (
                      <div style={{ color: '#999' }}>No data</div>
                    )}
                  </div>

                  {/* 7. Social history */}
                  <div style={{ marginBottom: '15px' }}>
                    <h3 style={{ fontSize: '14px', fontWeight: '600', marginBottom: '8px' }}>7. Social History</h3>
                    {(bgzSummary.socialHistory.livingSituation?.length > 0 || bgzSummary.socialHistory.drugUse?.length > 0 ||
                      bgzSummary.socialHistory.alcoholUse?.length > 0 || bgzSummary.socialHistory.tobaccoUse?.length > 0 ||
                      bgzSummary.socialHistory.nutritionAdvice?.length > 0) ? (
                      <div>
                        {bgzSummary.socialHistory.livingSituation?.length > 0 && (
                          <div style={{ marginBottom: '6px' }}>
                            <div style={{ fontWeight: '600', fontSize: '13px' }}>Living Situation:</div>
                            {bgzSummary.socialHistory.livingSituation.map((obs, idx) => (
                              <div key={obs.id || idx} style={{ paddingLeft: '10px', color: '#666', fontSize: '13px' }}>
                                {obs.valueCodeableConcept?.text || obs.valueCodeableConcept?.coding?.[0]?.display || obs.valueString || 'Living situation recorded'}
                              </div>
                            ))}
                          </div>
                        )}
                        {bgzSummary.socialHistory.drugUse?.length > 0 && (
                          <div style={{ marginBottom: '6px' }}>
                            <div style={{ fontWeight: '600', fontSize: '13px' }}>Drug Use:</div>
                            {bgzSummary.socialHistory.drugUse.map((obs, idx) => (
                              <div key={obs.id || idx} style={{ paddingLeft: '10px', color: '#666', fontSize: '13px' }}>
                                {obs.valueCodeableConcept?.text || obs.valueCodeableConcept?.coding?.[0]?.display || obs.valueString || 'Drug use recorded'}
                              </div>
                            ))}
                          </div>
                        )}
                        {bgzSummary.socialHistory.alcoholUse?.length > 0 && (
                          <div style={{ marginBottom: '6px' }}>
                            <div style={{ fontWeight: '600', fontSize: '13px' }}>Alcohol Use:</div>
                            {bgzSummary.socialHistory.alcoholUse.map((obs, idx) => (
                              <div key={obs.id || idx} style={{ paddingLeft: '10px', color: '#666', fontSize: '13px' }}>
                                {obs.valueCodeableConcept?.text || obs.valueCodeableConcept?.coding?.[0]?.display || obs.valueString || 'Alcohol use recorded'}
                              </div>
                            ))}
                          </div>
                        )}
                        {bgzSummary.socialHistory.tobaccoUse?.length > 0 && (
                          <div style={{ marginBottom: '6px' }}>
                            <div style={{ fontWeight: '600', fontSize: '13px' }}>Tobacco Use:</div>
                            {bgzSummary.socialHistory.tobaccoUse.map((obs, idx) => (
                              <div key={obs.id || idx} style={{ paddingLeft: '10px', color: '#666', fontSize: '13px' }}>
                                {obs.valueCodeableConcept?.text || obs.valueCodeableConcept?.coding?.[0]?.display || obs.valueString || 'Tobacco use recorded'}
                              </div>
                            ))}
                          </div>
                        )}
                        {bgzSummary.socialHistory.nutritionAdvice?.length > 0 && (
                          <div style={{ marginBottom: '6px' }}>
                            <div style={{ fontWeight: '600', fontSize: '13px' }}>Nutrition Advice:</div>
                            {bgzSummary.socialHistory.nutritionAdvice.map((order, idx) => (
                              <div key={order.id || idx} style={{ paddingLeft: '10px', color: '#666', fontSize: '13px' }}>
                                {order.oralDiet?.type?.[0]?.text || order.oralDiet?.type?.[0]?.coding?.[0]?.display || 'Nutrition advice provided'}
                              </div>
                            ))}
                          </div>
                        )}
                      </div>
                    ) : (
                      <div style={{ color: '#999' }}>No data</div>
                    )}
                  </div>

                  {/* 8. Alerts */}
                  <div style={{ marginBottom: '15px' }}>
                    <h3 style={{ fontSize: '14px', fontWeight: '600', marginBottom: '8px' }}>8. Alerts</h3>
                    {bgzSummary.alerts?.length > 0 ? (
                      <ul style={{ margin: 0, paddingLeft: '20px' }}>
                        {bgzSummary.alerts.map((alert, idx) => (
                          <li key={alert.id || idx} style={{ color: '#ef4444' }}>
                            {alert.code?.text || alert.code?.coding?.[0]?.display || 'Alert'}
                          </li>
                        ))}
                      </ul>
                    ) : (
                      <div style={{ color: '#999' }}>No data</div>
                    )}
                  </div>

                  {/* 9. Allergies */}
                  <div style={{ marginBottom: '15px' }}>
                    <h3 style={{ fontSize: '14px', fontWeight: '600', marginBottom: '8px' }}>9. Allergies</h3>
                    {bgzSummary.allergies?.length > 0 ? (
                      <ul style={{ margin: 0, paddingLeft: '20px' }}>
                        {bgzSummary.allergies.map((allergy, idx) => (
                          <li key={allergy.id || idx}>
                            {allergy.code?.text || allergy.code?.coding?.[0]?.display || 'Unknown allergy'}
                            {allergy.criticality && <span style={{ color: '#ef4444', marginLeft: '5px' }}>({allergy.criticality})</span>}
                          </li>
                        ))}
                      </ul>
                    ) : (
                      <div style={{ color: '#999' }}>No data</div>
                    )}
                  </div>

                  {/* 10. Medication */}
                  <div style={{ marginBottom: '15px' }}>
                    <h3 style={{ fontSize: '14px', fontWeight: '600', marginBottom: '8px' }}>10. Medication</h3>
                    {(bgzSummary.medication.medicationUse?.length > 0 || bgzSummary.medication.medicationAgreement?.length > 0 || bgzSummary.medication.administrationAgreement?.length > 0) ? (
                      <ul style={{ margin: 0, paddingLeft: '20px' }}>
                        {bgzSummary.medication.medicationUse.map((med, idx) => (
                          <li key={med.id || idx}>
                            {med.medicationCodeableConcept?.text || med.medicationCodeableConcept?.coding?.[0]?.display || 'Medication'}
                          </li>
                        ))}
                        {bgzSummary.medication.medicationAgreement.map((med, idx) => (
                          <li key={med.id || idx}>
                            {med.medicationCodeableConcept?.text || med.medicationCodeableConcept?.coding?.[0]?.display || 'Medication'}
                          </li>
                        ))}
                        {bgzSummary.medication.administrationAgreement.map((med, idx) => (
                          <li key={med.id || idx}>
                            {med.medicationCodeableConcept?.text || med.medicationCodeableConcept?.coding?.[0]?.display || 'Medication'}
                          </li>
                        ))}
                      </ul>
                    ) : (
                      <div style={{ color: '#999' }}>No data</div>
                    )}
                  </div>

                  {/* 11. Medical aids */}
                  <div style={{ marginBottom: '15px' }}>
                    <h3 style={{ fontSize: '14px', fontWeight: '600', marginBottom: '8px' }}>11. Medical Aids</h3>
                    {bgzSummary.medicalAids?.length > 0 ? (
                      <ul style={{ margin: 0, paddingLeft: '20px' }}>
                        {bgzSummary.medicalAids.filter(aid => aid.resourceType === 'DeviceUseStatement').map((deviceUse, idx) => {
                          // Try to find the referenced Device resource in the bundle
                          const deviceRef = deviceUse.device?.reference;
                          const device = bgzSummary.medicalAids.find(d => d.resourceType === 'Device' && deviceRef?.includes(d.id));

                          return (
                            <li key={deviceUse.id || idx}>
                              {device ? (
                                <>
                                  {device.type?.text || device.type?.coding?.[0]?.display || device.deviceName?.[0]?.name || 'Medical device'}
                                  {device.manufacturer && <span style={{ color: '#666', marginLeft: '5px' }}>({device.manufacturer})</span>}
                                </>
                              ) : (
                                deviceUse.device?.display || 'Medical device'
                              )}
                              {deviceUse.whenUsed && (
                                <span style={{ color: '#999', marginLeft: '5px', fontSize: '12px' }}>
                                  - Used: {deviceUse.whenUsed.start ? new Date(deviceUse.whenUsed.start).toLocaleDateString() : 'ongoing'}
                                </span>
                              )}
                            </li>
                          );
                        })}
                      </ul>
                    ) : (
                      <div style={{ color: '#999' }}>No data</div>
                    )}
                  </div>

                  {/* 12. Vaccinations */}
                  <div style={{ marginBottom: '15px' }}>
                    <h3 style={{ fontSize: '14px', fontWeight: '600', marginBottom: '8px' }}>12. Vaccinations</h3>
                    {bgzSummary.vaccinations?.length > 0 ? (
                      <ul style={{ margin: 0, paddingLeft: '20px' }}>
                        {bgzSummary.vaccinations.map((vacc, idx) => (
                          <li key={vacc.id || idx}>
                            {vacc.vaccineCode?.text || vacc.vaccineCode?.coding?.[0]?.display || 'Vaccine'}
                            {vacc.date && <span style={{ color: '#666', marginLeft: '5px' }}>({new Date(vacc.date).toLocaleDateString()})</span>}
                          </li>
                        ))}
                      </ul>
                    ) : (
                      <div style={{ color: '#999' }}>No data</div>
                    )}
                  </div>

                  {/* 13. Vital signs */}
                  <div style={{ marginBottom: '15px' }}>
                    <h3 style={{ fontSize: '14px', fontWeight: '600', marginBottom: '8px' }}>13. Vital Signs</h3>
                    {(bgzSummary.vitalSigns.bloodPressure?.length > 0 || bgzSummary.vitalSigns.bodyWeight?.length > 0 || bgzSummary.vitalSigns.bodyHeight?.length > 0) ? (
                      <div>
                        {bgzSummary.vitalSigns.bloodPressure?.length > 0 && (
                          <div style={{ marginBottom: '5px', color: '#666' }}>
                            <strong>Blood Pressure:</strong> {bgzSummary.vitalSigns.bloodPressure[0].valueQuantity?.value} {bgzSummary.vitalSigns.bloodPressure[0].valueQuantity?.unit}
                          </div>
                        )}
                        {bgzSummary.vitalSigns.bodyWeight?.length > 0 && (
                          <div style={{ marginBottom: '5px', color: '#666' }}>
                            <strong>Weight:</strong> {bgzSummary.vitalSigns.bodyWeight[0].valueQuantity?.value} {bgzSummary.vitalSigns.bodyWeight[0].valueQuantity?.unit}
                          </div>
                        )}
                        {bgzSummary.vitalSigns.bodyHeight?.length > 0 && (
                          <div style={{ marginBottom: '5px', color: '#666' }}>
                            <strong>Height:</strong> {bgzSummary.vitalSigns.bodyHeight[0].valueQuantity?.value} {bgzSummary.vitalSigns.bodyHeight[0].valueQuantity?.unit}
                          </div>
                        )}
                      </div>
                    ) : (
                      <div style={{ color: '#999' }}>No data</div>
                    )}
                  </div>

                  {/* 14. Results */}
                  <div style={{ marginBottom: '15px' }}>
                    <h3 style={{ fontSize: '14px', fontWeight: '600', marginBottom: '8px' }}>14. Results</h3>
                    {bgzSummary.results?.length > 0 ? (
                      <ul style={{ margin: 0, paddingLeft: '20px' }}>
                        {bgzSummary.results.map((result, idx) => (
                          <li key={result.id || idx}>
                            {result.code?.text || result.code?.coding?.[0]?.display || 'Lab test'}
                            {result.valueQuantity && (
                              <span style={{ color: '#666', marginLeft: '5px' }}>
                                : {result.valueQuantity.value} {result.valueQuantity.unit || result.valueQuantity.code}
                              </span>
                            )}
                            {result.valueString && <span style={{ color: '#666', marginLeft: '5px' }}>: {result.valueString}</span>}
                            {result.valueCodeableConcept && (
                              <span style={{ color: '#666', marginLeft: '5px' }}>
                                : {result.valueCodeableConcept.text || result.valueCodeableConcept.coding?.[0]?.display}
                              </span>
                            )}
                            {result.effectiveDateTime && (
                              <span style={{ color: '#999', marginLeft: '5px', fontSize: '12px' }}>
                                ({new Date(result.effectiveDateTime).toLocaleDateString()})
                              </span>
                            )}
                            {result.interpretation && result.interpretation[0] && (
                              <span style={{
                                marginLeft: '5px',
                                fontSize: '12px',
                                color: result.interpretation[0].coding?.[0]?.code === 'H' ? '#ef4444' :
                                       result.interpretation[0].coding?.[0]?.code === 'L' ? '#f59e0b' : '#666'
                              }}>
                                [{result.interpretation[0].text || result.interpretation[0].coding?.[0]?.display || result.interpretation[0].coding?.[0]?.code}]
                              </span>
                            )}
                          </li>
                        ))}
                      </ul>
                    ) : (
                      <div style={{ color: '#999' }}>No data</div>
                    )}
                  </div>

                  {/* 15. Procedures */}
                  <div style={{ marginBottom: '15px' }}>
                    <h3 style={{ fontSize: '14px', fontWeight: '600', marginBottom: '8px' }}>15. Procedures</h3>
                    {bgzSummary.procedures?.length > 0 ? (
                      <ul style={{ margin: 0, paddingLeft: '20px' }}>
                        {bgzSummary.procedures.map((proc, idx) => (
                          <li key={proc.id || idx}>
                            {proc.code?.text || proc.code?.coding?.[0]?.display || 'Procedure'}
                            {proc.performedDateTime && <span style={{ color: '#666', marginLeft: '5px' }}>({new Date(proc.performedDateTime).toLocaleDateString()})</span>}
                          </li>
                        ))}
                      </ul>
                    ) : (
                      <div style={{ color: '#999' }}>No data</div>
                    )}
                  </div>

                  {/* 16. Encounters */}
                  <div style={{ marginBottom: '15px' }}>
                    <h3 style={{ fontSize: '14px', fontWeight: '600', marginBottom: '8px' }}>16. Encounters</h3>
                    {bgzSummary.encounters?.length > 0 ? (
                      <ul style={{ margin: 0, paddingLeft: '20px' }}>
                        {bgzSummary.encounters.map((encounter, idx) => (
                          <li key={encounter.id || idx}>
                            {encounter.type?.[0]?.text || encounter.type?.[0]?.coding?.[0]?.display || encounter.class?.display || 'Hospital admission'}
                            {encounter.period && (encounter.period.start || encounter.period.end) && (
                              <span style={{ color: '#666', marginLeft: '5px' }}>
                                - {encounter.period.start ? new Date(encounter.period.start).toLocaleDateString() : '?'}
                                {encounter.period.end && ` to ${new Date(encounter.period.end).toLocaleDateString()}`}
                              </span>
                            )}
                            {encounter.serviceProvider?.display && (
                              <span style={{ color: '#999', marginLeft: '5px', fontSize: '12px' }}>
                                ({encounter.serviceProvider.display})
                              </span>
                            )}
                          </li>
                        ))}
                      </ul>
                    ) : (
                      <div style={{ color: '#999' }}>No data</div>
                    )}
                  </div>

                  {/* 17. Planned care */}
                  <div style={{ marginBottom: '15px' }}>
                    <h3 style={{ fontSize: '14px', fontWeight: '600', marginBottom: '8px' }}>17. Planned Care</h3>
                    {bgzSummary.plannedCare?.length > 0 ? (
                      <ul style={{ margin: 0, paddingLeft: '20px' }}>
                        {bgzSummary.plannedCare.map((item, idx) => {
                          // Handle different resource types
                          if (item.resourceType === 'ImmunizationRecommendation') {
                            return (
                              <li key={item.id || idx}>
                                Immunization recommendation
                                {item.recommendation?.[0]?.vaccineCode?.[0]?.text || item.recommendation?.[0]?.vaccineCode?.[0]?.coding?.[0]?.display ? (
                                  <span style={{ color: '#666', marginLeft: '5px' }}>
                                    : {item.recommendation[0].vaccineCode[0].text || item.recommendation[0].vaccineCode[0].coding[0].display}
                                  </span>
                                ) : null}
                              </li>
                            );
                          } else if (item.resourceType === 'DeviceRequest') {
                            return (
                              <li key={item.id || idx}>
                                Device request: {item.codeCodeableConcept?.text || item.codeCodeableConcept?.coding?.[0]?.display || 'Device'}
                                {item.authoredOn && (
                                  <span style={{ color: '#999', marginLeft: '5px', fontSize: '12px' }}>
                                    ({new Date(item.authoredOn).toLocaleDateString()})
                                  </span>
                                )}
                              </li>
                            );
                          } else if (item.resourceType === 'Appointment') {
                            return (
                              <li key={item.id || idx}>
                                Appointment
                                {item.serviceType?.[0]?.text || item.serviceType?.[0]?.coding?.[0]?.display ? (
                                  <span style={{ color: '#666', marginLeft: '5px' }}>
                                    : {item.serviceType[0].text || item.serviceType[0].coding[0].display}
                                  </span>
                                ) : null}
                                {item.start && (
                                  <span style={{ color: '#999', marginLeft: '5px', fontSize: '12px' }}>
                                    ({new Date(item.start).toLocaleDateString()})
                                  </span>
                                )}
                              </li>
                            );
                          } else {
                            return (
                              <li key={item.id || idx}>
                                Planned care activity ({item.resourceType})
                              </li>
                            );
                          }
                        })}
                      </ul>
                    ) : (
                      <div style={{ color: '#999' }}>No data</div>
                    )}
                  </div>

                  {/* 18. General practitioner */}
                  <div style={{ marginBottom: '15px' }}>
                    <h3 style={{ fontSize: '14px', fontWeight: '600', marginBottom: '8px' }}>18. General Practitioner</h3>
                    <div style={{ color: '#999' }}>See patient information</div>
                  </div>
                </div>
              ) : null}
            </div>
          </div>
        </div>

        {/* BGZ Confirmation Modal */}
        {showBgzConfirmModal && (
          <div className="modal-overlay" onClick={() => setShowBgzConfirmModal(false)}>
            <div className="modal" style={{ maxWidth: '500px' }} onClick={(e) => e.stopPropagation()}>
              <h3 style={{ marginTop: 0, color: '#ef4444' }}>⚠️ Generate BGZ</h3>
              <p style={{ fontSize: '14px', lineHeight: '1.6', color: '#666' }}>
                This will create <strong>{bgzApi.getBGZResourceCount()} new resources</strong> in the FHIR server for patient <strong>{patientName}</strong>.
              </p>
              <p style={{ fontSize: '14px', lineHeight: '1.6', color: '#666' }}>
                Are you sure you want to proceed?
              </p>
              <div style={{ display: 'flex', gap: '10px', marginTop: '20px' }}>
                <button
                  onClick={() => setShowBgzConfirmModal(false)}
                  className="button button-secondary"
                  style={{ flex: 1 }}
                >
                  Cancel
                </button>
                <button
                  onClick={handleConfirmBGZ}
                  className="button"
                  style={{ flex: 1 }}
                >
                  Generate BGZ
                </button>
              </div>
            </div>
          </div>
        )}

        {/* BGZ Delete Confirmation Modal */}
        {showBgzDeleteModal && (
          <div className="modal-overlay" onClick={() => setShowBgzDeleteModal(false)}>
            <div className="modal" style={{ maxWidth: '500px' }} onClick={(e) => e.stopPropagation()}>
              <h3 style={{ marginTop: 0, color: '#ef4444' }}>⚠️ Delete BGZ Data</h3>
              <p style={{ fontSize: '14px', lineHeight: '1.6', color: '#666' }}>
                This will <strong>permanently delete all BGZ resources</strong> for patient <strong>{patientName}</strong> from the FHIR server.
              </p>
              <p style={{ fontSize: '14px', lineHeight: '1.6', color: '#666' }}>
                This action cannot be undone. Are you sure you want to proceed?
              </p>
              {bgzDeleteError && (
                <div className="error-message" style={{ marginTop: '15px' }}>
                  <strong>Error deleting BGZ data</strong>
                  <p>{bgzDeleteError}</p>
                </div>
              )}
              <div style={{ display: 'flex', gap: '10px', marginTop: '20px' }}>
                <button
                  onClick={() => setShowBgzDeleteModal(false)}
                  className="button button-secondary"
                  style={{ flex: 1 }}
                >
                  Cancel
                </button>
                <button
                  onClick={handleConfirmBgzDelete}
                  className="button"
                  style={{
                    flex: 1,
                    backgroundColor: '#ef4444',
                    borderColor: '#ef4444'
                  }}
                >
                  Delete BGZ Data
                </button>
              </div>
            </div>
          </div>
        )}

        {/* eOvedracht Modal */}
        {showEOverdrachtModal && (
          <div className="modal-overlay" onClick={handleCloseModal}>
            <div className="modal" style={{ maxWidth: '1000px' }} onClick={(e) => e.stopPropagation()}>
              {eOverdrachtStep === 'services' ? (
                <>
                  <h3 style={{ marginTop: 0 }}>Step 1: Select Healthcare Service</h3>

                  <div style={{ marginTop: '20px' }}>
                    {healthcareServicesLoading ? (
                      <div className="loading-container" style={{ padding: '30px' }}>
                        <div className="spinner"></div>
                        <p>Loading healthcare services...</p>
                      </div>
                    ) : healthcareServicesError ? (
                      <div className="error-message">
                        <strong>Error loading healthcare services</strong>
                        <p>{healthcareServicesError}</p>
                      </div>
                    ) : groupedHealthcareServices.length === 0 ? (
                      <div className="empty-state" style={{ padding: '30px' }}>
                        <p>No healthcare services found</p>
                      </div>
                    ) : (
                      <div style={{ maxHeight: '400px', overflowY: 'auto' }}>
                        <table className="resource-table">
                          <thead>
                            <tr>
                              <th>Service Name</th>
                              <th>Count</th>
                              <th>Type(s)</th>
                              <th>Status</th>
                              <th>Action</th>
                            </tr>
                          </thead>
                          <tbody>
                            {groupedHealthcareServices.map((group, idx) => {
                              const types = group.types !== '-' ? group.types.split(', ') : ['-'];

                              return (
                                <tr key={`${group.name}-${idx}`}>
                                  <td>
                                    <strong>{group.name}</strong>
                                  </td>
                                  <td>
                                    <span style={{ color: '#666', fontSize: '14px' }}>
                                      {group.count} {group.count === 1 ? 'service' : 'services'}
                                    </span>
                                  </td>
                                  <td style={{ fontSize: '13px' }}>
                                    {types.length > 1 ? (
                                      <ul style={{ margin: 0, paddingLeft: '20px', listStyle: 'disc' }}>
                                        {types.map((t, tidx) => (
                                          <li key={tidx}>{t}</li>
                                        ))}
                                      </ul>
                                    ) : (
                                      types[0]
                                    )}
                                  </td>
                                  <td>
                                    <span className={`status-badge ${group.hasActive ? 'status-active' : 'status-inactive'}`}>
                                      {group.hasActive ? 'Active' : 'Inactive'}
                                    </span>
                                  </td>
                                  <td>
                                    <button
                                      className="button"
                                      style={{ fontSize: '13px', padding: '6px 12px' }}
                                      onClick={() => handleServiceSelect(group)}
                                    >
                                      Select
                                    </button>
                                  </td>
                                </tr>
                              );
                            })}
                          </tbody>
                        </table>
                      </div>
                    )}
                  </div>

                  <div style={{ marginTop: '24px', display: 'flex', justifyContent: 'flex-end', gap: '10px' }}>
                    <button
                      type="button"
                      className="button button-secondary"
                      onClick={handleCloseModal}
                    >
                      Close
                    </button>
                  </div>
                </>
              ) : eOverdrachtStep === 'organizations' ? (
                <>
                  <h3 style={{ marginTop: 0 }}>
                    Step 2: Select Organization
                    {selectedServiceGroup && (
                      <div style={{ fontSize: '14px', fontWeight: 'normal', color: '#666', marginTop: '8px' }}>
                        Service: {selectedServiceGroup.name}
                      </div>
                    )}
                  </h3>

                  <div style={{ marginTop: '20px' }}>
                    {organizationsLoading ? (
                      <div className="loading-container" style={{ padding: '30px' }}>
                        <div className="spinner"></div>
                        <p>Loading organizations...</p>
                      </div>
                    ) : organizationsError ? (
                      <div className="error-message">
                        <strong>Error loading organizations</strong>
                        <p>{organizationsError}</p>
                      </div>
                    ) : organizations.length === 0 ? (
                      <div className="empty-state" style={{ padding: '30px' }}>
                        <p>No organizations found for this service</p>
                      </div>
                    ) : (
                      <div style={{ maxHeight: '400px', overflowY: 'auto' }}>
                        <table className="resource-table">
                          <thead>
                            <tr>
                              <th>Name</th>
                              <th>URA</th>
                              <th>Type</th>
                              <th>Address</th>
                              <th>Contact</th>
                              <th>Status</th>
                              <th>Action</th>
                            </tr>
                          </thead>
                          <tbody>
                            {organizations.map((org) => {
                              const ura = organizationApi.getURA(org);
                              const address = organizationApi.formatAddress(org);
                              const type = organizationApi.formatType(org);
                              const types = type !== '-' ? type.split(', ') : ['-'];
                              const telecom = organizationApi.formatTelecomString(org);

                              return (
                                <tr key={org.id}>
                                  <td>
                                    <strong>{organizationApi.formatName(org)}</strong>
                                  </td>
                                  <td>
                                    {ura ? (
                                      <span className="bsn-badge">{ura}</span>
                                    ) : (
                                      <span style={{ color: '#999' }}>-</span>
                                    )}
                                  </td>
                                  <td style={{ fontSize: '13px' }}>
                                    {types.length > 1 ? (
                                      <ul style={{ margin: 0, paddingLeft: '20px', listStyle: 'disc' }}>
                                        {types.map((t, idx) => (
                                          <li key={idx}>{t}</li>
                                        ))}
                                      </ul>
                                    ) : (
                                      types[0]
                                    )}
                                  </td>
                                  <td style={{ fontSize: '13px' }}>{address}</td>
                                  <td style={{ fontSize: '13px' }}>{telecom}</td>
                                  <td>
                                    <span className={`status-badge ${organizationApi.isActive(org) ? 'status-active' : 'status-inactive'}`}>
                                      {organizationApi.isActive(org) ? 'Active' : 'Inactive'}
                                    </span>
                                  </td>
                                  <td>
                                    <button
                                      className="button"
                                      style={{ fontSize: '13px', padding: '6px 12px' }}
                                      onClick={() => handleOrganizationSelect(org)}
                                    >
                                      Select
                                    </button>
                                  </td>
                                </tr>
                              );
                            })}
                          </tbody>
                        </table>
                      </div>
                    )}
                  </div>

                  <div style={{ marginTop: '24px', display: 'flex', justifyContent: 'space-between', gap: '10px' }}>
                    <button
                      type="button"
                      className="button button-secondary"
                      onClick={handleBackToServices}
                    >
                      ← Back to Services
                    </button>
                    <button
                      type="button"
                      className="button button-secondary"
                      onClick={handleCloseModal}
                    >
                      Close
                    </button>
                  </div>
                </>
              ) : eOverdrachtStep === 'departments' ? (
                <>
                  <h3 style={{ marginTop: 0 }}>
                    Step 3: Select Department
                    {selectedOrganization && (
                      <div style={{ fontSize: '14px', fontWeight: 'normal', color: '#666', marginTop: '8px' }}>
                        Organization: {organizationApi.formatName(selectedOrganization)}
                      </div>
                    )}
                  </h3>

                  <div style={{ marginTop: '20px' }}>
                    {departmentsLoading ? (
                      <div className="loading-container" style={{ padding: '30px' }}>
                        <div className="spinner"></div>
                        <p>Loading departments...</p>
                      </div>
                    ) : departmentsError ? (
                      <div className="error-message">
                        <strong>Error loading departments</strong>
                        <p>{departmentsError}</p>
                      </div>
                    ) : departments.length === 0 ? (
                      <div className="empty-state" style={{ padding: '30px' }}>
                        <p>No departments found for this organization</p>
                      </div>
                    ) : (
                      <div style={{ maxHeight: '400px', overflowY: 'auto' }}>
                        <table className="resource-table">
                          <thead>
                            <tr>
                              <th>Department Name</th>
                              <th>URA</th>
                              <th>Type</th>
                              <th>Address</th>
                              <th>Contact</th>
                              <th>Status</th>
                              <th>Action</th>
                            </tr>
                          </thead>
                          <tbody>
                            {departments.map((dept) => {
                              const ura = organizationApi.getURA(dept);
                              const address = organizationApi.formatAddress(dept);
                              const type = organizationApi.formatType(dept);
                              const types = type !== '-' ? type.split(', ') : ['-'];
                              const telecom = organizationApi.formatTelecomString(dept);

                              return (
                                <tr key={dept.id}>
                                  <td>
                                    <strong>{organizationApi.formatName(dept)}</strong>
                                  </td>
                                  <td>
                                    {ura ? (
                                      <span className="bsn-badge">{ura}</span>
                                    ) : (
                                      <span style={{ color: '#999' }}>-</span>
                                    )}
                                  </td>
                                  <td style={{ fontSize: '13px' }}>
                                    {types.length > 1 ? (
                                      <ul style={{ margin: 0, paddingLeft: '20px', listStyle: 'disc' }}>
                                        {types.map((t, idx) => (
                                          <li key={idx}>{t}</li>
                                        ))}
                                      </ul>
                                    ) : (
                                      types[0]
                                    )}
                                  </td>
                                  <td style={{ fontSize: '13px' }}>{address}</td>
                                  <td style={{ fontSize: '13px' }}>{telecom}</td>
                                  <td>
                                    <span className={`status-badge ${organizationApi.isActive(dept) ? 'status-active' : 'status-inactive'}`}>
                                      {organizationApi.isActive(dept) ? 'Active' : 'Inactive'}
                                    </span>
                                  </td>
                                  <td>
                                    <button
                                      className="button"
                                      style={{ fontSize: '13px', padding: '6px 12px' }}
                                      onClick={() => handleDepartmentSelect(dept)}
                                    >
                                      Select
                                    </button>
                                  </td>
                                </tr>
                              );
                            })}
                          </tbody>
                        </table>
                      </div>
                    )}
                  </div>

                  <div style={{ marginTop: '24px', display: 'flex', justifyContent: 'space-between', gap: '10px' }}>
                    <button
                      type="button"
                      className="button button-secondary"
                      onClick={handleBackToOrganizations}
                    >
                      ← Back to Organizations
                    </button>
                    <button
                      type="button"
                      className="button button-secondary"
                      onClick={handleCloseModal}
                    >
                      Close
                    </button>
                  </div>
                </>
              ) : (
                <>
                  <h3 style={{ marginTop: 0 }}>
                    {isBgzReferral ? 'Confirm BGZ verwijzing' : 'Confirm eOvedracht Task Creation'}
                  </h3>

                  <div style={{ marginTop: '20px' }}>
                    <div style={{ padding: '20px', backgroundColor: '#f8f9fa', borderRadius: '8px', border: '1px solid #dee2e6' }}>
                      <h4 style={{ marginTop: 0, marginBottom: '20px', color: '#495057' }}>Summary</h4>

                      <div style={{ display: 'grid', gap: '15px' }}>
                        <div>
                          <div style={{ fontSize: '12px', fontWeight: '600', color: '#6c757d', textTransform: 'uppercase', letterSpacing: '0.5px', marginBottom: '5px' }}>
                            Patient
                          </div>
                          <div style={{ fontSize: '15px', fontWeight: '500' }}>
                            {patientName}
                          </div>
                          <div style={{ fontSize: '13px', color: '#6c757d' }}>
                            ID: {patientId}
                          </div>
                        </div>

                        <div style={{ borderTop: '1px solid #dee2e6', paddingTop: '15px' }}>
                          <div style={{ fontSize: '12px', fontWeight: '600', color: '#6c757d', textTransform: 'uppercase', letterSpacing: '0.5px', marginBottom: '5px' }}>
                            Healthcare Service
                          </div>
                          <div style={{ fontSize: '15px', fontWeight: '500' }}>
                            {selectedServiceGroup?.name}
                          </div>
                        </div>

                        <div style={{ borderTop: '1px solid #dee2e6', paddingTop: '15px' }}>
                          <div style={{ fontSize: '12px', fontWeight: '600', color: '#6c757d', textTransform: 'uppercase', letterSpacing: '0.5px', marginBottom: '5px' }}>
                            Organization
                          </div>
                          <div style={{ fontSize: '15px', fontWeight: '500' }}>
                            {selectedOrganization && organizationApi.formatName(selectedOrganization)}
                          </div>
                          {selectedOrganization && organizationApi.getURA(selectedOrganization) && (
                            <div style={{ fontSize: '13px', color: '#6c757d' }}>
                              URA: {organizationApi.getURA(selectedOrganization)}
                            </div>
                          )}
                        </div>

                        <div style={{ borderTop: '1px solid #dee2e6', paddingTop: '15px' }}>
                          <div style={{ fontSize: '12px', fontWeight: '600', color: '#6c757d', textTransform: 'uppercase', letterSpacing: '0.5px', marginBottom: '5px' }}>
                            Department
                          </div>
                          <div style={{ fontSize: '15px', fontWeight: '500' }}>
                            {selectedDepartment && organizationApi.formatName(selectedDepartment)}
                          </div>
                          {selectedDepartment && organizationApi.getURA(selectedDepartment) && (
                            <div style={{ fontSize: '13px', color: '#6c757d' }}>
                              URA: {organizationApi.getURA(selectedDepartment)}
                            </div>
                          )}
                          {selectedDepartment && (
                            <div style={{ fontSize: '13px', color: '#6c757d', marginTop: '5px' }}>
                              {organizationApi.formatAddress(selectedDepartment)}
                            </div>
                          )}
                        </div>

                        <div style={{ borderTop: '1px solid #dee2e6', paddingTop: '15px' }}>
                          <div style={{ fontSize: '12px', fontWeight: '600', color: '#6c757d', textTransform: 'uppercase', letterSpacing: '0.5px', marginBottom: '5px' }}>
                            Requested By
                          </div>
                          <div style={{ fontSize: '15px', fontWeight: '500' }}>
                            {user?.profile?.name || user?.profile?.email || 'Unknown User'}
                          </div>
                        </div>
                      </div>

                      <div style={{ marginTop: '20px', padding: '15px', backgroundColor: '#fff3cd', border: '1px solid #ffc107', borderRadius: '6px' }}>
                        <div style={{ fontSize: '14px', color: '#856404' }}>
                          {isBgzReferral ? (
                            <>
                              <strong>Note:</strong> By confirming, you will confirm the BGZ verwijzing. A notification (initiating a <i>notified-pull</i>) will be sent to the receiving party.
                            </>
                          ) : (
                            <>
                              <strong>Note:</strong> By confirming, an eOvedracht Task will be created with status "in-progress" for the care handover process.
                            </>
                          )}
                        </div>
                      </div>
                    </div>

                    {isBgzReferral && (
                      <div style={{
                        marginTop: '20px',
                        padding: '15px',
                        backgroundColor: '#f8f9fa',
                        border: '1px solid #dee2e6',
                        borderRadius: '6px'
                      }}>
                        <div style={{ fontSize: '12px', fontWeight: '600', color: '#6c757d', textTransform: 'uppercase', letterSpacing: '0.5px', marginBottom: '10px' }}>
                          Workflow Options
                        </div>
                        <label style={{
                          display: 'flex',
                          alignItems: 'center',
                          gap: '10px',
                          cursor: 'pointer',
                          fontSize: '14px'
                        }}>
                          <input
                            type="checkbox"
                            checked={createWorkflowTask}
                            onChange={(e) => setCreateWorkflowTask(e.target.checked)}
                            style={{ width: '16px', height: '16px', cursor: 'pointer' }}
                          />
                          <span style={{ fontWeight: '500' }}>Create a workflow task</span>
                        </label>
                        <div style={{ fontSize: '12px', color: '#6c757d', marginTop: '8px', marginLeft: '26px' }}>
                          Creates a Workflow FHIR Task resource. Unchecked means only a notification will be sent (and notification will include all required inputs).
                        </div>
                      </div>
                    )}

                    {/* Task creation feedback */}
                    {taskSuccess && (
                      <div style={{ marginTop: '20px', padding: '15px', backgroundColor: '#d4edda', border: '1px solid #c3e6cb', borderRadius: '8px', color: '#155724' }}>
                        <strong>Success!</strong>
                        {isBgzReferral ? (
                            contextLaunchEndpoint ? (
                                " BgZ Referral flow has been finalized successfully."
                            ) : (
                                " BgZ Referral flow has been finalized successfully. Closing..."
                            )
                        ) : (
                            " eOvedracht flow has been finalized successfully. Closing..."
                        )}

                        {/* Context launch button */}
                        {isBgzReferral && contextLaunchEndpoint && (
                          <div style={{ marginTop: '15px' }}>
                            <button
                              type="button"
                              className="button"
                              onClick={() => handleContextLaunch()}
                              style={{ width: '100%' }}
                            >
                              🚀 Context Launch Receiving Application
                            </button>
                          </div>
                        )}
                      </div>
                    )}

                    {taskError && (
                      <div className="error-message" style={{ marginTop: '20px' }}>
                        <strong>Error creating Task</strong>
                        <p>{taskError}</p>
                      </div>
                    )}
                  </div>

                  <div style={{ marginTop: '24px', display: 'flex', justifyContent: 'space-between', gap: '10px' }}>
                    <button
                      type="button"
                      className="button button-secondary"
                      onClick={handleBackToDepartments}
                      disabled={taskCreating}
                    >
                      ← Back to Departments
                    </button>
                    <div style={{ display: 'flex', gap: '10px' }}>
                      <button
                        type="button"
                        className="button button-secondary"
                        onClick={handleCloseModal}
                        disabled={taskCreating}
                      >
                        Cancel
                      </button>
                      <button
                        type="button"
                        className="button"
                        onClick={() => handleConfirmTask(isBgzReferral)}
                        disabled={taskCreating || (taskSuccess && contextLaunchEndpoint)}
                        style={{
                          backgroundColor: (taskCreating || (taskSuccess && contextLaunchEndpoint)) ? '#6c757d' : '#28a745',
                          borderColor: (taskCreating || (taskSuccess && contextLaunchEndpoint)) ? '#6c757d' : '#28a745'
                        }}
                      >
                        {taskCreating ? 'Creating Task...' : 'Confirm & Create'}
                      </button>
                    </div>
                  </div>
                </>
              )}
            </div>
          </div>
        )}
      </main>
    </div>
  );
}

export default PatientPage;

