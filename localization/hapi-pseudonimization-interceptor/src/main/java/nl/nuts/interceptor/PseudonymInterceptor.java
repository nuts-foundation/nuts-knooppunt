package nl.nuts.interceptor;

import ca.uhn.fhir.context.FhirContext;
import ca.uhn.fhir.interceptor.api.Hook;
import ca.uhn.fhir.interceptor.api.Interceptor;
import ca.uhn.fhir.interceptor.api.Pointcut;
import ca.uhn.fhir.jpa.searchparam.SearchParameterMap;
import ca.uhn.fhir.model.api.IQueryParameterType;
import ca.uhn.fhir.rest.api.RequestTypeEnum;
import ca.uhn.fhir.rest.api.server.IPreResourceShowDetails;
import ca.uhn.fhir.rest.api.server.ResponseDetails;
import ca.uhn.fhir.rest.param.ReferenceParam;
import ca.uhn.fhir.rest.server.servlet.ServletRequestDetails;
import ca.uhn.fhir.rest.server.util.ICachedSearchDetails;
import jakarta.servlet.http.HttpServletResponse;
import java.util.List;
import lombok.extern.slf4j.Slf4j;
import nl.nuts.util.BsnUtil;
import org.apache.commons.lang3.StringUtils;
import org.hl7.fhir.instance.model.api.IBaseResource;
import org.hl7.fhir.instance.model.api.IIdType;
import org.hl7.fhir.r4.model.CodeableConcept;
import org.hl7.fhir.r4.model.Coding;
import org.hl7.fhir.r4.model.DocumentReference;
import org.hl7.fhir.r4.model.IdType;
import org.hl7.fhir.r4.model.Identifier;
import org.hl7.fhir.r4.model.OperationOutcome;
import org.hl7.fhir.r4.model.OperationOutcome.IssueSeverity;
import org.hl7.fhir.r4.model.OperationOutcome.IssueType;
import org.hl7.fhir.r4.model.OperationOutcome.OperationOutcomeIssueComponent;
import org.hl7.fhir.r4.model.Reference;
import org.hl7.fhir.r4.model.ResourceType;
import org.springframework.stereotype.Component;

@Component
@Interceptor
@Slf4j
public class PseudonymInterceptor {

    private static final String PSEUDO_BSN_SYSTEM = System.getenv().getOrDefault(
            "PSEUDO_BSN_SYSTEM", "http://fhir.nl/fhir/NamingSystem/pseudo-bsn");
    private static final String BSN_TOKEN_SYSTEM = System.getenv().getOrDefault(
            "BSN_TOKEN_SYSTEM", "http://fhir.nl/fhir/NamingSystem/bsn-transport-token");
    private static final String NVI_AUDIENCE = System.getenv().getOrDefault(
            "NVI_AUDIENCE", "nvi-1");
    private static final String REQUESTER_URA_HEADER = "X-Requester-URA";

    private final BsnUtil bsnUtil;

    public PseudonymInterceptor() {
        this.bsnUtil = new BsnUtil();
    }

    @Hook(Pointcut.STORAGE_PRESEARCH_REGISTERED)
    public void preSearch(final ICachedSearchDetails searchDetails,
                          final SearchParameterMap searchParameterMap) {
        if (!ResourceType.DocumentReference.name().equals(searchDetails.getResourceType())) {
            return;
        }
        final List<List<IQueryParameterType>> patient = searchParameterMap.get(DocumentReference.SP_PATIENT);
        final List<List<IQueryParameterType>> subject = searchParameterMap.get(DocumentReference.SP_SUBJECT);

        final ReferenceParam modifiedPatient = modifySearchParameter(patient);
        final ReferenceParam modifiedSubject = modifySearchParameter(subject);

        if (modifiedPatient != null) {
            searchParameterMap.remove(DocumentReference.SP_PATIENT);
            searchParameterMap.add(DocumentReference.SP_PATIENT, modifiedPatient);
        }

        if (modifiedSubject != null) {
            searchParameterMap.remove(DocumentReference.SP_SUBJECT);
            searchParameterMap.add(DocumentReference.SP_SUBJECT, modifiedSubject);
        }

        log.info("{}", searchParameterMap);

        if ((patient == null || patient.isEmpty()) && (subject == null || subject.isEmpty())) {
            throw new IllegalArgumentException("You have to search by 'patient' or 'subject' (patient).");
        }
    }

    /**
     * Modifies search parameters by converting BSN tokens to pseudonyms.
     * Handles both ReferenceParam (e.g., Patient/identifier) and TokenParam (e.g., identifier=system|value).
     */
    private ReferenceParam modifySearchParameter(final List<List<IQueryParameterType>> params) {
        if (params == null) {
            return null;
        }
        for (final List<IQueryParameterType> orList : params) {
            for (final IQueryParameterType param : orList) {
                if (param instanceof ReferenceParam) {
                    final ReferenceParam modifiedSearchParam = getModifiedSearchParam((ReferenceParam) param);
                    if (modifiedSearchParam != null) {
                        return modifiedSearchParam;
                    }
                }
            }
        }
        return null;
    }

    /**
     * Modifies a ReferenceParam if it contains an identifier with BSN token system.
     * Example: patient.identifier=http://example.com/BSNToken|token-hospital-abc123-def456
     */
    private ReferenceParam getModifiedSearchParam(final ReferenceParam param) {
        // Get the identifier value which should be in format: system|value
        final String value = param.getValue();
        if (value == null || !value.contains("|")) {
            return null;
        }

        final String[] parts = value.split("\\|", 2);
        if (parts.length != 2) {
            return null;
        }

        final String system = parts[0];
        final String identifierValue = parts[1];

        // Convert token to pseudonym if it's a BSN token
        if (!BSN_TOKEN_SYSTEM.equals(system)) {
            return null;
        }

        log.info("Converting token to pseudonym in search parameter: {}", identifierValue);
        final String pseudonym = tokenToPseudonym(identifierValue);

        return new ReferenceParam(String.format("%s/%s/%s", PSEUDO_BSN_SYSTEM, ResourceType.Patient.name(), pseudonym));
    }

    /**
     * Triggers before a Resource (in our case a DocumentReference) is created. We take currently set
     * DocumentReference.subject and create pseudonym from a token set on there
     */
    @Hook(Pointcut.STORAGE_PRESTORAGE_RESOURCE_CREATED)
    public void resourceCreated(final IBaseResource newResource) {
        if (!(newResource instanceof final DocumentReference documentReference)) {
            return;
        }
        log.trace("Intercepting DocumentReference resource creation");
        modifyDocumentFromTokenToPseudonym(documentReference);
        log.info("{}", FhirContext.forR4Cached().newJsonParser().encodeResourceToString(documentReference));
    }

    /**
     * If it is a POST (Resource has been created), but there is no REQUESTER_URA_HEADER present, we must prevent
     * presentation of the created Resource as a response. In this scenario, this hook replaces actual DocumentReference
     * response with an OperationOutcome containing the warning and description.
     * <p>
     * This can not be done in @Hook(Pointcut.STORAGE_PRESHOW_RESOURCES) as you can not modify response Resources
     * in that flow.
     */
    @Hook(Pointcut.SERVER_OUTGOING_RESPONSE)
    public void handleServerResponse(final ResponseDetails responseDetails,
                                     final ServletRequestDetails servletRequestDetails,
                                     final HttpServletResponse response) {
        if (servletRequestDetails.getRequestType() != RequestTypeEnum.POST
                || !StringUtils.isEmpty(servletRequestDetails.getHeader(REQUESTER_URA_HEADER))) {
            return;
        }
        responseDetails.setResponseResource(
                createWarningOutcome(responseDetails.getResponseResource().getIdElement().getValue()));
    }


    /**
     * Triggers before a Resource (in our case a DocumentReference) is read (also invoked when a Resource is created,
     * but is returned in a response to creation. We replace currently set DocumentReference.subject (which is a
     * pseudonym) with an audience-specific token (audience information is obtained from @see REQUESTER_URA_HEADER).
     */
    @Hook(Pointcut.STORAGE_PRESHOW_RESOURCES)
    public void handlePreShowResources(final IPreResourceShowDetails requestDetails,
                                       final ServletRequestDetails servletRequestDetails) {
        final String audience = servletRequestDetails.getHeader(REQUESTER_URA_HEADER);

        if (StringUtils.isEmpty(audience) && servletRequestDetails.getRequestType() == RequestTypeEnum.POST) {
            return;
        } else if (StringUtils.isEmpty(audience)) {
            throw new IllegalArgumentException(
                    String.format("Resource can not be retrieved due to the fact there is no %s header present.",
                                  REQUESTER_URA_HEADER));
        }

        final List<IBaseResource> allResources = requestDetails.getAllResources();
        allResources.stream()
                .filter(aResource -> aResource instanceof DocumentReference)
                .forEach(documentReference -> modifyDocumentFromPseudonymToToken((DocumentReference) documentReference,
                                                                                 audience));

    }

    private OperationOutcome createWarningOutcome(final String createdResourceId) {
        final OperationOutcome warningOutcome = new OperationOutcome();
        final OperationOutcomeIssueComponent component = new OperationOutcomeIssueComponent();
        component.setSeverity(IssueSeverity.WARNING);
        component.setCode(IssueType.SECURITY);
        component.setDetails(new CodeableConcept(new Coding()).setText(String.format(
                "Resource was created (%s, see Location header), but can not be presented as no audience has been supplied. Do a GET with %s header to retrieve the Resource.",
                createdResourceId,
                REQUESTER_URA_HEADER)));
        warningOutcome.addIssue(component);
        return warningOutcome;
    }

    private void modifyDocumentFromPseudonymToToken(final DocumentReference resource, final String audience) {
        final Reference subject = resource.getSubject();
        if (subject == null) {
            return;
        }
        final IIdType referenceElement = subject.getReferenceElement();
        if (!PSEUDO_BSN_SYSTEM.equals(referenceElement.getBaseUrl())) {
            return;
        }

        log.trace("Found identifier: system={}, value={}", referenceElement.getBaseUrl(), referenceElement.getIdPart());
        final String token = pseudonymToToken(referenceElement.getIdPart(), audience);
        final Identifier identifier = new Identifier();
        identifier.setSystem(BSN_TOKEN_SYSTEM);
        identifier.setValue(token);
        resource.setSubject(new Reference().setIdentifier(identifier));
    }

    private void modifyDocumentFromTokenToPseudonym(final DocumentReference docRef) {
        final org.hl7.fhir.r4.model.Identifier identifier = docRef.getSubject().getIdentifier();

        if (identifier == null || !BSN_TOKEN_SYSTEM.equals(identifier.getSystem())) {
            return;
        }

        log.trace("Found identifier: system={}, value={}", identifier.getSystem(), identifier.getValue());
        final String pseudonym = tokenToPseudonym(identifier.getValue());

        // unfortunately we need to do this, because HAPI doesn't support :identifier modifier and so we need to translate
        // this to a reference instead, so we can search it by this reference. We'll also handle :identifier modifier
        // ourselves, as if we handle it, but in reality, we'll just modify :identifier modifier to a reference param search
        // all this without letting 'the client' know of this workaround. Client acts as if it's storing and searching by
        // subject.as(Identifier)
        docRef.setSubject(new Reference(new IdType(PSEUDO_BSN_SYSTEM, ResourceType.Patient.name(), pseudonym, null)));
    }

    private String tokenToPseudonym(final String token) {
        final String pseudonym = bsnUtil.transportTokenToPseudonym(token, NVI_AUDIENCE);
        log.trace("Converted token to pseudonym: {}", pseudonym);
        return pseudonym;
    }

    private String pseudonymToToken(final String pseudonym, final String audience) {
        final String token = bsnUtil.pseudonymToTransportToken(pseudonym, audience);
        log.trace("Converted pseudonym to token: {}", token);
        return token;
    }
}



