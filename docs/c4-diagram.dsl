workspace "Knooppunt" "Description"

!identifiers hierarchical

model {
    group "National Generic Function Systems" {
        lrza = softwareSystem "LRZa mCSD Administration Directory" "Authority of combination URA and the mCSD Directory" {
            tags "external,addressing"
        }
        nvi = softwareSystem "NVI" "Nationale Verwijs Index, contains entries with URA and patient" {
            tags "external,localization"
        }
        otv = softwareSystem "OTV" "Nation Online Consent System, contains patient consents" "external"
    }


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


    xis = softwareSystem "XIS" "Local XIS integrating the Knooppunt" {
        ehr = container "EHR" {
            tags "addressing,localization"
            localizationClient = component "Localization Client" "Publishing and localizing patient localization data" {
                tags "localization"
            }
        }

        kp = container "Knooppunt" {
            tags "addressing,localization,consent"

            mcsdSyncer = component "mCSD Update client" "Syncing data from remote mCSD directory and consolidate into a Query Directory" {
                tags "addressing"
            }
            mcsdAdminApp = component "mCSD Administration Application" "Administering Organization mCSD resources" {
                tags "addressing,webapp"
                technology "HTMX"
            }

            nviGateway = component "NVI Gateway" "Administer NVI entries and search NVI" {
                tags "localization"
            }

            otvClient = component "OTV Client" "Request consent information from the Mitz OTV" {
                tags "consent"
            }
        }

        fhirQueryDir = container "mCSD Query Directory" "Stores mCSD resources for querying" {
            tags "database,addressing"
            technology "HAPI FHIR"
        }

        fhirAdminDir = container "mCSD Administration Directory" "Stores mCSD resources for synchronization" {
            tags "database,addressing"
            technology "HAPI FHIR"
        }
    }

    #
    # GF Addressing transactions
    #
    xis.kp.mcsdSyncer -> xis.fhirQueryDir "Update mCSD Resources from remote Administration Directories" FHIR {
        tags "addressing"
    }
    xis.kp.mcsdAdminApp -> xis.fhirAdminDir "Manage mCSD resources" {
        tags "addressing"
    }
    xis.ehr -> xis.fhirQueryDir "Query the mCSD directory" "FHIR" {
        tags "addressing"
    }
    remoteXIS.mcsdUpdateClient -> xis.fhirAdminDir "Query updated mCSD resources" "FHIR" {
        tags "addressing"
    }
    xis.kp.mcsdSyncer -> lrza "Fetch Organizations with their URA and mCSD Directory endpoints" FHIR {
        tags "addressing"
    }
    xis.kp.mcsdSyncer -> remoteXIS.mcsdDirectory "Query updated mCSD resources" FHIR {
        tags "addressing"
    }

    #
    # GF Localization transactions
    #
//    xis.ehr -> xis.kp "Publish patient localization data" FHIR {
//        tags "localization"
//    }
//    xis.ehr.localizationClient -> xis.kp.nviGateway "Publish patient localization data" FHIR {
//        tags "localization"
//    }
//    xis.kp.nviGateway -> nvi "Publish patient localization data\n(pseudonymized)" FHIR {
//        tags "localization"
//    }
//    xis.kp.nviGateway -> nvi "Localize patient data\n(pseudonymized)" FHIR {
//        tags "localization"
//    }
//    xis.ehr -> xis.kp.nviGateway "Localize patient data" FHIR {
//        tags "localization"
//    }

    #
    # GF Consent transactions
    #
    xis.kp.otvClient -> otv "Perform the 'gesloten-vraag'" {
        tags "consent"
    }
}

views {
    # GF Adressing
    container xis "GF_Addressing_ContainerDiagram" {
        title "Container diagram of systems and transactions involved in GF Addressing"
        include "element.tag==addressing || relationship.tag==addressing"
//        autolayout lr
    }
    component xis.kp "GF_Addressing_ComponentDiagram" {
        title "Component diagram of systems and transactions involved in GF Addressing"
        include "element.tag==addressing || relationship.tag==addressing"
        //        autolayout lr
    }

    # GF Localization
    container xis "GF_Localization_ContainerDiagram" {
        title "Container diagram of systems and transactions involved in GF Localization"
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