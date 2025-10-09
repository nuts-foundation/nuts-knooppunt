workspace "Nuts Knooppunt" "Description"

!identifiers hierarchical

model {

    group "Remote CareProvider Systems" {
        remoteXIS = softwareSystem "Remote XIS\nimplementing Generic Functions" {
            tags "external,addressing"
            mcsdUpdateClient = container "mCSD Update Client" "Syncing data from mCSD directory" {
                tags "external,addressing"
            }

            mcsdDirectory = container "Organization mCSD Administration Directory" "Authority of Organization Endpoints, HealthcareServices and PractitionerRoles" {
                tags "external,addressing"
            }

            viewer = container "Viewer" "Request healthcare data from other Care Providers" {
                tags "external"
            }
        }
    }


    group "National Generic Function Systems" {
        lrza = softwareSystem "LRZa mCSD Administration Directory" "Authority of combination URA and the mCSD Directory" {
            tags "external,addressing"
        }
        nvi = softwareSystem "NVI" "Nationale Verwijs Index, contains entries with URA and patient" {
            tags "external,localization"
        }
        otv = softwareSystem "OTV" "Nation Online Consent System, contains patient consents" "external"
    }

    group "Local Care Provider Systems" {
        xis = softwareSystem "Local XIS" "The XIS integrating the Knooppunt" {
            tags "addressing,localization"
            viewer = container "Viewer" {
                tags "addressing,localization"
                mcsdQueryClient = component "mCSD Query Client" "Queries the mcsd Directory"
                localizationClient = component "Localization Client" "Localize patient data"
            }

            ehr = container "EHR" {
                tags "localization"
            }

            fhirAdminDir = container "mCSD Administration Directory" "Exposes mCSD resources for synchronization" "FHIR API" {
                tags "addressing,existing-fhir-admin-directory"
            }

            nuts = container "Nuts Node" "Provides authentication services" {
                tags "existing-nuts-node"
            }
        }

        kpSystem = softwareSystem "Local Knooppunt" {
            kp = container "Knooppunt Container" {
                tags "addressing,localization"
                group "Authentication" {
                    nuts = component "Nuts Node" {
                        tags "new-nuts-node"
                    }
                }
                group "Addressing" {
                    mcsdSyncer = component "mCSD Update client" "Syncing data from remote mCSD directory and consolidate into a Query Directory" {
                        tags "addressing"
                    }
                    mcsdDataEntry = component "Addressing Admin" "Administering Organization mCSD resources" {
                        tags "addressing,webapp"
                        technology "HTMX"
                    }
                }

                group "Localization" {
                    nviGateway = component "NVI Gateway" "Administer NVI entries and search NVI" {
                        tags "localization"
                    }
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
                tags "database,addressing"
                technology "HAPI FHIR"
            }

            fhirAdminDir = container "mCSD Administration Directory" "Stores mCSD resources for synchronization" {
                tags "database,addressing"
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
            kp.mcsdSyncer -> remoteXIS.mcsdDirectory "Query updated mCSD resources" FHIR
            # For 'new' mCSD Administration Directory (e.g. HAPI FHIR):
            kp.mcsdSyncer -> fhirAdminDir "Query updated mCSD resources" FHIR {
                tags "addressing,existing-fhir-admin-directory"
            }
            kp.mcsdDataEntry -> fhirAdminDir "CRUD on organization resources" {
                tags "addressing,existing-fhir-admin-directory"
            }


            kp.otvClient -> otv "Perform the 'gesloten-vraag'"

            admin -> kp.nuts "Manage Nuts node"

            # GF Localization transactions
            xis.ehr -> kp.nviGateway "Publish patient localization data" FHIR {
                tags "localization"
            }
            kp.nviGateway -> nvi "Publish patient localization data\n(pseudonymized)" FHIR {
                tags "localization"
            }
            kp -> nvi "Localize patient data\n(pseudonymized)" FHIR {
                tags "localization"
            }
            xis.viewer -> kp.nviGateway "Localize patient data" FHIR {
                tags "localization"
            }

        }

        xis -> kpSystem "Queries addressing data"

        xis.viewer.mcsdQueryClient -> kpSystem.fhirQueryDir "Query the mCSD addressing directory" "FHIR"

        # For 'existing' mCSD Administration Directory, typically a facade on the XIS:
        remoteXIS.mcsdUpdateClient -> xis.fhirAdminDir "Query updated mCSD resources" "FHIR" {
            tags "existing-fhir-admin-directory"
        }
        # For 'new' mCSD Administration Directory (e.g. HAPI FHIR):
        remoteXIS.mcsdUpdateClient -> kpSystem.fhirAdminDir "Query updated mCSD resources" "FHIR" {
            tags "new-fhir-admin-directory"
        }

        # For 'new' Nuts node (embedded):
//        remoteXIS.viewer -> kpSystem.kp.nuts "Request AccessToken" {
//            tags "new-nuts-node"
//        }
        # For 'existing' Nuts node:
//        remoteXIS.viewer -> xis.nuts "Request AccessToken" {
//            tags "existing-nuts-node"
//        }
    }
}


views {
    # GF Adressing
    container kpSystem "GF_Addressing_ContainerDiagram" {
        title "Container diagram of systems and transactions involved in GF Addressing"
        include "element.tag==addressing || relationship.tag==addressing"
        exclude "element.tag==existing-fhir-admin-directory || relationship.tag==existing-fhir-admin-directory"
        autolayout lr
    }

    # GF Localization
    container kpSystem "GF_Localization_ContainerDiagram" {
        title "Container diagram of systems and transactions involved in GF Localization"
        exclude "element.tag==existing-fhir-admin-directory || relationship.tag==existing-fhir-admin-directory"
        exclude "element.tag==new-nuts-node || relationship.tag==new-nuts-node"
        include "element.tag==localization || relationship.tag==localization"
        autolayout lr
    }

    # Deployment A: new (embedded) Nuts node, new FHIR mCSD Admin Directory
    //    systemContext kpSystem "A1_SystemContext" {
    //        title "Deployment A: System diagram of Knooppunt deployment,\nwith embedded Nuts node and new mCSD Administration Directory"
    //        include *
    //        exclude "element.tag==existing-fhir-admin-directory || relationship.tag==existing-fhir-admin-directory"
    //        exclude "element.tag==existing-nuts-node || relationship.tag==existing-nuts-node"
    //        autolayout lr
    //    }
    //    container kpSystem "A2_ContainerDiagram" {
    //        title "Deployment A: System diagram of Knooppunt deployment,\nwith embedded Nuts node and new mCSD Administration Directory"
    //        include *
    //        exclude "element.tag==existing-fhir-admin-directory || relationship.tag==existing-fhir-admin-directory"
    //        exclude "element.tag==existing-nuts-node || relationship.tag==existing-nuts-node"
    //        autolayout lr
    //    }
    //
    //    # Deployment B: new (embedded) Nuts node, existing FHIR mCSD Admin Directory
    //    container kpSystem "B2_ContainerDiagram" {
    //        title "Deployment B: Container diagram of Knooppunt deployment,\nwith embedded Nuts node and existing mCSD Administration Directory"
    //        include *
    //        exclude "element.tag==new-fhir-admin-directory || relationship.tag==new-fhir-admin-directory"
    //        exclude "element.tag==existing-nuts-node || relationship.tag==existing-nuts-node"
    //        autolayout lr
    //    }
    //    container xis "B2_XIS_ContainerDiagram" {
    //        title "Deployment B: Container diagram of Knooppunt deployment,\nwith existing Nuts node and existing mCSD Administration Directory\n(XIS perspective)"
    //        include *
    //        exclude "element.tag==new-fhir-admin-directory || relationship.tag==new-fhir-admin-directory"
    //        exclude "element.tag==existing-nuts-node || relationship.tag==existing-nuts-node"
    //        autolayout lr
    //    }
    //
    //    # Deployment C: existing Nuts node, new FHIR mCSD Admin Directory
    //    container kpSystem "C2_ContainerDiagram" {
    //        title "Deployment C: Container diagram of Knooppunt deployment,\nwith existing Nuts node and new mCSD Administration Directory"
    //        include *
    //        exclude "element.tag==new-nuts-node || relationship.tag==new-nuts-node"
    //        exclude "element.tag==existing-fhir-admin-directory || relationship.tag==existing-fhir-admin-directory"
    //        autolayout lr
    //    }
    //    container xis "C2_XIS_ContainerDiagram" {
    //        title "Deployment C: Container diagram of Knooppunt deployment,\nwith existing Nuts node and new mCSD Administration Directory\n(XIS perspective)"
    //        include *
    //        exclude "element.tag==new-nuts-node || relationship.tag==new-nuts-node"
    //        exclude "element.tag==existing-fhir-admin-directory || relationship.tag==existing-fhir-admin-directory"
    //        autolayout lr
    //    }

    styles {
        element "database" {
            shape cylinder
        }
        element "Boundary" {
            strokeWidth 5
        }
        element "external" {
            background #1168bd
            color #ffffff
            shape RoundedBox
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
