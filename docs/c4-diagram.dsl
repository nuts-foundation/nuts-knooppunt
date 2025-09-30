workspace "Nuts Knooppunt" "Description"

!identifiers hierarchical

model {

    group "Remote CareProvider Systems" {
        mcsdUpdateClient = softwareSystem "mCSD Update Client" "Syncing data from mCSD directory" {
            tags "external"
        }

        mcsdDirectory = softwareSystem "Organization mCSD Administration Directory" "Authority of Organization Endpoints, HealthcareServices and PractitionerRoles" "external"

        externalViewer = softwareSystem "External Viewer" "Request healthcare data from other Care Providers" "external"
    }


    group "National Generic Function Systems" {
        lrza = softwareSystem "LRZa mCSD Administration Directory" "Authority of combination URA and the mCSD Directory" "external"
        nvi = softwareSystem "NVI" "Nationale Verwijs Index, contains entries with URA and patient" "external"
        otv = softwareSystem "OTV" "Nation Online Consent System, contains patient consents" "external"
    }

    group "Local Care Provider Systems" {
        xis = softwareSystem "XIS" "The XIS integrating the Knooppunt" {
            viewer = container "Viewer" {
                mcsdQueryClient = component "mCSD Query Client" "Queries the mcsd Directory"
                localisationClient = component "Localisation Client" "Localise patient data"
            }

            ehr = container "EHR" {
                localisationPublisher = component "Localisation Publisher" "Publish patient localisation data"
            }

            fhirAdminDir = container "mCSD Administration Directory" "Exposes mCSD resources for synchronization" "FHIR API" {
                tags "existing-fhir-admin-directory"
            }

            nuts = container "Nuts Node" "Provides authentication services" {
                tags "existing-nuts-node"
            }
        }

        kpSystem = softwareSystem "Nuts Knooppunt" {
            kp = container "Knooppunt Container" {
                group "Authentication" {
                    nuts = component "Nuts Node" {
                        tags "new-nuts-node"
                    }
                }
                group "Addressing" {
                    mcsdSyncer = component "mCSD Update client" "Syncing data from remote mCSD directory and consolidate into a Query Directory"
                    mcsdDataEntry = component "Addressing Admin" "Administering Organization mCSD resources" {
                        tags "webapp"
                    }
                }

                group "Localisation" {
                    localisationClient = component "NVI Client" "Administer NVI entries and search NVI"
                    lmr = component "Localisation Metadata Registry" "FHIR Server which can be used to search records by predefined meta data"
                }

                group "Consent" {
                    otvClient = component "OTV Client" "Request consent information from the Mitz OTV"
                }
            }

            admin = container "Nuts admin" {
                tags "new-nuts-node"
            }

            db = container "Database" {
                tags "database"
            }

            fhirQueryDir = container "mCSD Query Directory" "Stores mCSD resources for querying" {
                tags "database"
                technology "HAPI FHIR"
            }
            fhirAdminDir = container "mCSD Administration Directory" "Stores mCSD resources for synchronization" {
                tags "database"
                tags "new-fhir-admin-directory"
                technology "HAPI FHIR"
            }

            keyStore = container "Secure Key storage" {
                tags "database"
            }

            kp.nuts -> keyStore "Creates and uses keys"
            kp.nuts -> db "Store credentials, dids etc."

            kp.mcsdSyncer -> fhirQueryDir "Update mCSD Resources from remote Administration Directories" FHIR
            kp.mcsdSyncer -> lrza "Fetch Organizations with their URA and mCSD Directory endpoints" FHIR
            kp.mcsdSyncer -> mcsdDirectory "Fetch Organization resources" FHIR
            # For 'new' mCSD Administration Directory (e.g. HAPI FHIR):
            kp.mcsdSyncer -> fhirAdminDir "Fetch Organizations resources" FHIR {
                tags "existing-fhir-admin-directory"
            }
            kp.mcsdDataEntry -> fhirAdminDir "CRUD on organization resources" {
                tags "existing-fhir-admin-directory"
            }


            kp.localisationClient -> nvi "Register Patients, Query for URAs per patient" FHIR
            kp.localisationClient -> kp.lmr "Search for FHIR resources by metadata"

            kp.otvClient -> otv "Perform the 'gesloten-vraag'"

            admin -> kp.nuts "Manage Nuts node"
        }

        xis -> kpSystem "Queries addressing data"

        xis.viewer.mcsdQueryClient -> kpSystem.fhirQueryDir "Query the mCSD addressing directory"
        xis.ehr.localisationPublisher -> kpSystem.kp.localisationClient "Publish localisation metadata" "FHIR"
        # For 'existing' mCSD Administration Directory, typically a facade on the XIS:
        mcsdUpdateClient -> xis.fhirAdminDir "Query updated mCSD resources" "FHIR" {
            tags "existing-fhir-admin-directory"
        }
        # For 'new' mCSD Administration Directory (e.g. HAPI FHIR):
        mcsdUpdateClient -> kpSystem.fhirAdminDir "Query updated mCSD resources" "FHIR" {
            tags "new-fhir-admin-directory"
        }

        # For 'new' Nuts node (embedded):
        externalViewer -> kpSystem.kp.nuts "Request AccessToken" {
            tags "new-nuts-node"
        }
        # For 'existing' Nuts node:
        externalViewer -> xis.nuts "Request AccessToken" {
            tags "existing-nuts-node"
        }
    }
}


views {
    # Deployment A: new (embedded) Nuts node, new FHIR mCSD Admin Directory
    systemContext kpSystem "A1_SystemContext" {
        title "Deployment A: System diagram of Knooppunt deployment,\nwith embedded Nuts node and new mCSD Administration Directory"
        include *
        exclude "element.tag==existing-fhir-admin-directory || relationship.tag==existing-fhir-admin-directory"
        exclude "element.tag==existing-nuts-node || relationship.tag==existing-nuts-node"
        autolayout lr
    }
    container kpSystem "A2_ContainerDiagram" {
        title "Deployment A: System diagram of Knooppunt deployment,\nwith embedded Nuts node and new mCSD Administration Directory"
        include *
        exclude "element.tag==existing-fhir-admin-directory || relationship.tag==existing-fhir-admin-directory"
        exclude "element.tag==existing-nuts-node || relationship.tag==existing-nuts-node"
        autolayout lr
    }

    # Deployment B: new (embedded) Nuts node, existing FHIR mCSD Admin Directory
    container kpSystem "B2_ContainerDiagram" {
        title "Deployment B: Container diagram of Knooppunt deployment,\nwith embedded Nuts node and existing mCSD Administration Directory"
        include *
        exclude "element.tag==new-fhir-admin-directory || relationship.tag==new-fhir-admin-directory"
        exclude "element.tag==existing-nuts-node || relationship.tag==existing-nuts-node"
        autolayout lr
    }
    container xis "B2_XIS_ContainerDiagram" {
        title "Deployment B: Container diagram of Knooppunt deployment,\nwith existing Nuts node and existing mCSD Administration Directory\n(XIS perspective)"
        include *
        exclude "element.tag==new-fhir-admin-directory || relationship.tag==new-fhir-admin-directory"
        exclude "element.tag==existing-nuts-node || relationship.tag==existing-nuts-node"
        autolayout lr
    }

    # Deployment C: existing Nuts node, new FHIR mCSD Admin Directory
    container kpSystem "C2_ContainerDiagram" {
        title "Deployment C: Container diagram of Knooppunt deployment,\nwith existing Nuts node and new mCSD Administration Directory"
        include *
        exclude "element.tag==new-nuts-node || relationship.tag==new-nuts-node"
        exclude "element.tag==existing-fhir-admin-directory || relationship.tag==existing-fhir-admin-directory"
        autolayout lr
    }
    container xis "C2_XIS_ContainerDiagram" {
        title "Deployment C: Container diagram of Knooppunt deployment,\nwith existing Nuts node and new mCSD Administration Directory\n(XIS perspective)"
        include *
        exclude "element.tag==new-nuts-node || relationship.tag==new-nuts-node"
        exclude "element.tag==existing-fhir-admin-directory || relationship.tag==existing-fhir-admin-directory"
        autolayout lr
    }

    styles {
        element "Element" {
            color #0773af
            stroke #0773af
            strokeWidth 7
            shape roundedbox
        }

        element "demo" {
            stroke "#cccccc"
        }

        element "Group" {
            stroke "#0773af"
            color #0773af
            strokeWidth 5
        }


        element "Person" {
            shape person
        }
        element "database" {
            shape cylinder
        }
        element "Boundary" {
            strokeWidth 5
        }
        element "external" {
            border dashed
        }
        element "webapp" {
            shape WebBrowser
        }
        relationship "Relationship" {
            thickness 4
        }
    }
}
}
