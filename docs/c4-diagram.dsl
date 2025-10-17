workspace "Knooppunt" "Description" {
    !identifiers hierarchical

    model {
        archetypes {
            fhirServer = container {
                tags "FHIR Server"
                technology "HAPI FHIR"
            }
        }
        properties {
            "structurizr.groupSeparator" "/"
        }

        group "External Systems" {
            group "National Generic Function Systems" {
                lrza = softwareSystem "LRZa mCSD Administration Directory" "Authority of combination URA and the mCSD Directory" {
                    tags "External System,addressing"
                }
                nvi = softwareSystem "NVI" "Nationale Verwijs Index, contains entries with URA and patient" {
                    tags "External System,localization"
                }
                otv = softwareSystem "OTV" "Nation Online Consent System, contains patient consents" "External System"
            }


            group "External XIS" {
                remoteXIS = softwareSystem "Remote XIS\nimplementing Generic Functions" {
                    tags "External System" "addressing"
                    mcsdUpdateClient = container "mCSD Update Client" "Syncing data from mCSD directory" {
                        tags "External System,addressing"
                    }

                    mcsdDirectory = container "Organization mCSD Administration Directory" "Authority of Organization Endpoints, HealthcareServices and PractitionerRoles" {
                        tags "External System,addressing"
                    }

                    viewer = container "Viewer" "Request healthcare data from other Care Providers" {
                        tags "External System"
                    }
                }
            }

        }

        group "Local Systems" {


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

                fhirQueryDir = fhirServer "mCSD Query Directory" "Stores mCSD resources for querying" {
                    tags "addressing"
                }

                fhirAdminDir = fhirServer "mCSD Administration Directory" "Stores mCSD resources for synchronization" {
                    tags "addressing"
                }
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
        xis.ehr.localizationClient -> xis.kp.nviGateway "Publish and find localization data" FHIR {
            tags "localization"
            url "http://knooppunt:8081/nvi"
        }
        xis.kp.nviGateway -> nvi "Publish and find localization data\n(pseudonymized)" FHIR {
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
        properties {
            c4plantuml.tags true
        }

        # Overall
        systemContext xis "GF_SystemContext" {
            title "Systems involved in a Generic Functions implementation"
            include *
        }

        # GF Adressing
        container xis "GF_Addressing_ContainerDiagram" {
            title "XIS Perspective: containers, systems and databases involved in GF Addressing"
            include "element.tag==addressing || relationship.tag==addressing"
            exclude "relationship.tag==localization"
        }
        component xis.kp "GF_Addressing_ComponentDiagram" {
            title "Knooppunt perspective: component diagram of systems and transactions involved in GF Addressing"
            include "element.tag==addressing || relationship.tag==addressing"
        }

        # GF Localization
        container xis "GF_Localization_ContainerDiagram" {
            title "XIS Perspective: containers, systems and databases involved in GF Localization"
            include "element.tag==localization || relationship.tag==localization"
        }
        component xis.kp "GF_Localization_ComponentDiagram" {
            title "Knooppunt perspective: component diagram of systems and transactions involved in GF Localization"
            include "element.tag==localization || relationship.tag==localization"
        }

        styles {
            element "Element" {
                background #bddcf2
                color #3e4d57
                stroke #257bb8
                strokeWidth 2
            }
            element "FHIR Server" {
                shape cylinder
            }

            element "External System" {
                background #eeeeee
            }
        }
    }
}