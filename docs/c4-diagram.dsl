workspace "Knooppunt" "Description" {
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
            tags "external" "addressing"
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
            //  After we introduce a PEP:
            //        pep = container "Policy Enforcement Point" "Proxy that enforces access policies on data exchanges." "NGINX" {
            //            tags "addressing,localization"
            //        }

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
        remoteXIS.mcsdUpdateClient -> xis.fhirAdminDir "Query mCSD resources" "FHIR" {
            tags "addressing"
        }
        //  After we introduce a PEP:
        //    remoteXIS.mcsdUpdateClient -> xis.fhirAdminDir "Query mCSD resources" "FHIR" {
        //        tags "addressing"
        //    }
        //    xis.pep -> xis.fhirAdminDir "Query mCSD resources" "FHIR" {
        //        tags "addressing"
        //    }
        xis.kp.mcsdSyncer -> lrza "Fetch Organizations with their URA and mCSD Directory endpoints" FHIR {
            tags "addressing"
        }
        xis.kp.mcsdSyncer -> remoteXIS.mcsdDirectory "Query mCSD resources" FHIR {
            tags "addressing"
        }

        #
        # GF Localization transactions
        #
        xis.ehr.localizationClient -> xis.kp.nviGateway "Publish patient localization data,\nlocalize patient data" FHIR {
            tags "localization"
        }
        xis.kp.nviGateway -> nvi "Publish patient localization data,\nlocalize patient data\n(pseudonymized)" FHIR {
            tags "localization"
        }

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
            exclude "relationship.tag==localization"
            //        autolayout lr
        }
        component xis.kp "GF_Addressing_ComponentDiagram" {
            title "Component diagram of systems and transactions involved in GF Addressing"
            include "element.tag==addressing || relationship.tag==addressing"
            autolayout lr
        }

        # GF Localization
        container xis "GF_Localization_ContainerDiagram" {
            title "Container diagram of systems and transactions involved in GF Localization"
            include "element.tag==localization || relationship.tag==localization"

            autolayout lr
        }

//        styles {
////            element "database" {
////                shape cylinder
////            }
////            element "Boundary" {
////                strokeWidth 5
////            }
//            element "external" {
//                background #999999
//                color #990099
//                shape cylinder
//            }
//            element Container {
//                background #999999
//                color #990099
//                shape cylinder
//            }
////            element "webapp" {
////                shape WebBrowser
////            }
////            relationship "Relationship" {
////                thickness 4
////            }
//        }
    }
}