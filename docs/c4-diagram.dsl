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
            group "National Systems" {
                group "Generic Functions" {
                    lrza = softwareSystem "LRZa mCSD Administration Directory" "Authority of combination URA and the mCSD Directory" {
                        tags "External System,addressing"
                    }
                    nvi = softwareSystem "NVI" "Nationale Verwijs Index, contains entries with URA and patient" {
                        tags "External System,localization"
                    }
                    otv = softwareSystem "OTV" "Mitz national system, containing patient consents" "External System" {
                        tags "External System,consent"
                    }
                }
                dezi = softwareSystem "Dezi" "National Care Giver authentication service ('DE ZorgIdentiteit')" {
                    tags "External System,authentication"
                }
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
                        tags "External System,dataexchange"
                    }
                }
            }

        }

        group "Local Systems" {


            xis = softwareSystem "XIS" "Local XIS consisting of EHR and Knooppunt services" {
                ehr = container "EHR" {
                    tags "addressing,localization,dataexchange,authentication"
                    localizationClient = component "Localization Client" "Publishing and localizing patient localization data" {
                        tags "localization"
                    }
                }

                pep = container "Policy Enforcement Point" "Proxy that enforces access policies on data exchanges." "NGINX" {
                    tags "dataexchange"
                }

                kp = container "Knooppunt" {
                    tags "addressing,localization,consent,dataexchange,authentication"

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

                    mitzClient = component "Mitz Client" "Request consent information from the Mitz OTV" {
                        tags "consent"
                    }

                    pdp = component "Policy Decision Point" "Makes authorization decisions for data exchange requests" {
                        tags "dataexchange"
                    }

                    oidcProvider = component "OIDC Provider" "Authenticate users against Dezi" {
                        tags "authentication"
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
        xis.kp.mcsdSyncer -> xis.fhirQueryDir "ITI-130: Update mCSD Resources from remote Administration Directories" FHIR {
            tags "addressing"
            url "https://profiles.ihe.net/ITI/mCSD/ITI-130.html"
        }
        xis.kp.mcsdAdminApp -> xis.fhirAdminDir "Manage mCSD resources" {
            tags "addressing"
        }
        xis.ehr -> xis.fhirQueryDir "ITI-90: Query the mCSD directory" "FHIR" {
            tags "addressing"
            url "https://profiles.ihe.net/ITI/mCSD/ITI-90.html"
        }
        remoteXIS.mcsdUpdateClient -> xis.fhirAdminDir "ITI-91: Query mCSD resources" "FHIR" {
            tags "addressing"
            url "https://profiles.ihe.net/ITI/mCSD/ITI-91.html"

        }
        //  After we introduce a PEP:
        //    remoteXIS.mcsdUpdateClient -> xis.fhirAdminDir "Query mCSD resources" "FHIR" {
        //        tags "addressing"
        //    }
        //    xis.pep -> xis.fhirAdminDir "Query mCSD resources" "FHIR" {
        //        tags "addressing"
        //    }
        xis.kp.mcsdSyncer -> lrza "ITI-91: Query Organizations with their URA and mCSD Directory endpoints" FHIR {
            tags "addressing"
            url "https://profiles.ihe.net/ITI/mCSD/ITI-91.html"
        }
        xis.kp.mcsdSyncer -> remoteXIS.mcsdDirectory "ITI-91: Query mCSD resources" FHIR {
            tags "addressing"
            url "https://profiles.ihe.net/ITI/mCSD/ITI-91.html"
        }

        #
        # GF Localization transactions
        #
        xis.ehr.localizationClient -> xis.kp.nviGateway "Publish and find localization data\nhttp://knooppunt:8081/nvi" FHIR {
            tags "localization"
        }
        xis.kp.nviGateway -> nvi "Publish and find localization data\n(pseudonymized)" FHIR {
            tags "localization"
        }

        #
        # GF Consent transactions
        #
        xis.kp.mitzClient -> otv "Perform the 'gesloten-vraag'" SOAP {
            tags "consent"
        }

        #
        # Data Exchange transactions
        #
        remoteXIS.viewer -> xis.pep "Request patient healthcare data" "FHIR" {
            tags "dataexchange"
        }
        xis.pep -> xis.kp.pdp "Authorize data exchange request" "OPA / AuthzAPI" {
            tags "dataexchange"
        }
        xis.kp.pdp -> xis.kp.mitzClient "Check patient consent" {
            tags "dataexchange,consent"
        }

        xis.pep -> xis.ehr "Forward authorized request" "FHIR" {
            tags "dataexchange"
        }

        #
        # Authentication transactions
        #
        xis.ehr -> xis.kp "Log in user" "OIDC" {
            tags "authentication"
        }
        xis.ehr -> xis.kp.oidcProvider "Log in user" "OIDC AuthZ Code" {
            tags "authentication"
        }
        xis.kp.oidcProvider -> dezi "Authenticate user" "OIDC AuthZ Code" {
            tags "authentication"
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

        # GF Addressing
        container xis "GF_Addressing_ContainerDiagram" {
            title "XIS Perspective: containers, systems and databases involved in GF Addressing"
            include "element.tag==addressing || relationship.tag==addressing"
            exclude "relationship.tag==localization"
            exclude "relationship.tag==authentication"
        }
        component xis.kp "GF_Addressing_ComponentDiagram" {
            title "Knooppunt perspective: component diagram of systems and transactions involved in GF Addressing"
            include "element.tag==addressing || relationship.tag==addressing"
            exclude "relationship.tag==localization"
            exclude "relationship.tag==authentication"
        }

        # GF Localization
        container xis "GF_Localization_ContainerDiagram" {
            title "XIS Perspective: containers, systems and databases involved in GF Localization"
            include "element.tag==localization || relationship.tag==localization"
            exclude "relationship.tag==authentication"
        }
        component xis.kp "GF_Localization_ComponentDiagram" {
            title "Knooppunt perspective: component diagram of systems and transactions involved in GF Localization"
            include "element.tag==localization || relationship.tag==localization"
            exclude "relationship.tag==authentication"
        }

        # Data exchange
        container xis "DataExchange_ContainerDiagram" {
            title "XIS Perspective: containers, systems and databases involved in Data Exchange"
            include "element.tag==dataexchange || relationship.tag==dataexchange"
            include "element.tag==consent || relationship.tag==consent"
            exclude "relationship.tag==localization"
            exclude "relationship.tag==authentication"
        }
        component xis.kp "DataExchange_ComponentDiagram" {
            title "Knooppunt perspective: component diagram of systems and transactions involved in Data Exchange"
            include "element.tag==dataexchange || relationship.tag==dataexchange"
            include "element.tag==consent || relationship.tag==consent"
            exclude "relationship.tag==localization"
            exclude "relationship.tag==authentication"
        }

        # Authentication
        container xis "Authentication_ContainerDiagram" {
            title "XIS Perspective: containers, systems and databases involved in Authentication"
            include "element.tag==authentication || relationship.tag==authentication"
            exclude "relationship.tag==localization"
        }
        component xis.kp "Authentication_ComponentDiagram" {
            title "Knooppunt perspective: component diagram of systems and transactions involved in Authentication"
            include "element.tag==authentication || relationship.tag==authentication"
            exclude "relationship.tag==localization"
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