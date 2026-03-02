package nl.nuts.interceptor;

import ca.uhn.fhir.context.FhirContext;
import ca.uhn.fhir.interceptor.api.Hook;
import ca.uhn.fhir.interceptor.api.Interceptor;
import ca.uhn.fhir.interceptor.api.Pointcut;
import ca.uhn.fhir.jpa.searchparam.SearchParameterMap;
import ca.uhn.fhir.model.api.IQueryParameterType;
import ca.uhn.fhir.rest.api.server.IPreResourceShowDetails;
import ca.uhn.fhir.rest.param.ReferenceParam;
import ca.uhn.fhir.rest.server.servlet.ServletRequestDetails;
import ca.uhn.fhir.rest.server.util.ICachedSearchDetails;
import java.util.List;
import lombok.extern.slf4j.Slf4j;
import nl.nuts.util.BsnUtil;
import org.apache.commons.lang3.StringUtils;
import org.hl7.fhir.instance.model.api.IBaseResource;
import org.hl7.fhir.instance.model.api.IIdType;
import org.hl7.fhir.r4.model.DocumentReference;
import org.hl7.fhir.r4.model.Extension;
import org.hl7.fhir.r4.model.IdType;
import org.hl7.fhir.r4.model.Identifier;
import org.hl7.fhir.r4.model.ListResource;
import org.hl7.fhir.r4.model.Reference;
import org.hl7.fhir.r4.model.ResourceType;
import org.springframework.stereotype.Component;

@Component
@Interceptor
@Slf4j
public class PseudonymInterceptor {

    private static final String PSEUDO_BSN_SYSTEM = System.getenv().getOrDefault(
            "PSEUDO_BSN_SYSTEM", "http://fhir.nl/fhir/NamingSystem/bsn-pseudonym");
    private static final String BSN_TOKEN_SYSTEM = System.getenv().getOrDefault(
            "BSN_TOKEN_SYSTEM", "http://fhir.nl/fhir/NamingSystem/bsn-transport-token");
    private static final String NVI_AUDIENCE = System.getenv().getOrDefault(
            "NVI_AUDIENCE", "nvi-1");
    private static final String INTERCEPTOR_ENABLED_FOR_TENANT = System.getenv().getOrDefault(
            "NVI_TENANT", "nvi");
    private static final String REQUESTER_URA_HEADER = "X-Requester-URA";

    private final BsnUtil bsnUtil;

    public PseudonymInterceptor() {
        this.bsnUtil = new BsnUtil();
    }

    private boolean isEnabled(final ServletRequestDetails servletRequestDetails) {
        log.info("Tenant: {}", servletRequestDetails.getTenantId());
        return INTERCEPTOR_ENABLED_FOR_TENANT.equals(servletRequestDetails.getTenantId());
    }

    @Hook(Pointcut.STORAGE_PRESEARCH_REGISTERED)
    public void preSearch(final ICachedSearchDetails searchDetails,
                          final SearchParameterMap searchParameterMap,
                          final ServletRequestDetails servletRequestDetails) {
        if (!isEnabled(servletRequestDetails)) {
            return;
        }

        final String resourceType = searchDetails.getResourceType();

        if (ResourceType.DocumentReference.name().equals(resourceType)) {
            preSearchDocumentReference(searchParameterMap);
        } else if (ResourceType.List.name().equals(resourceType)) {
            preSearchList(searchParameterMap);
        }
    }

    private void preSearchDocumentReference(final SearchParameterMap searchParameterMap) {
        final List<List<IQueryParameterType>> patient = searchParameterMap.get(DocumentReference.SP_PATIENT);
        final List<List<IQueryParameterType>> subject = searchParameterMap.get(DocumentReference.SP_SUBJECT);

        final ReferenceParam modifiedPatient = modifySearchParameter(patient, false);
        final ReferenceParam modifiedSubject = modifySearchParameter(subject, false);

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

    private void preSearchList(final SearchParameterMap searchParameterMap) {
        final List<List<IQueryParameterType>> patient = searchParameterMap.get(ListResource.SP_PATIENT);
        final List<List<IQueryParameterType>> subject = searchParameterMap.get(ListResource.SP_SUBJECT);
        final List<List<IQueryParameterType>> source = searchParameterMap.get(ListResource.SP_SOURCE);

        final ReferenceParam modifiedPatient = modifySearchParameter(patient, false);
        final ReferenceParam modifiedSubject = modifySearchParameter(subject, false);
        final ReferenceParam modifiedSource = modifySearchParameter(source, true);

        if (modifiedPatient != null) {
            searchParameterMap.remove(ListResource.SP_PATIENT);
            searchParameterMap.add(ListResource.SP_PATIENT, modifiedPatient);
        }

        if (modifiedSubject != null) {
            searchParameterMap.remove(ListResource.SP_SUBJECT);
            searchParameterMap.add(ListResource.SP_SUBJECT, modifiedSubject);
        }

        if (modifiedSource != null) {
            searchParameterMap.remove(ListResource.SP_SOURCE);
            searchParameterMap.add(ListResource.SP_SOURCE, modifiedSource);
        }

        log.info("{}", searchParameterMap);

        if ((patient == null || patient.isEmpty()) && (subject == null || subject.isEmpty()) && (source == null
                || source.isEmpty())) {
            throw new IllegalArgumentException("You have to search by 'patient' or 'subject' (patient) or 'source'.");
        }
    }

    private ReferenceParam modifySearchParameter(final List<List<IQueryParameterType>> params,
                                                 final boolean source) {
        if (params == null) {
            return null;
        }
        for (final List<IQueryParameterType> orList : params) {
            for (final IQueryParameterType param : orList) {
                if (param instanceof ReferenceParam) {
                    final ReferenceParam modifiedSearchParam = getModifiedSearchParam((ReferenceParam) param, source);
                    if (modifiedSearchParam != null) {
                        return modifiedSearchParam;
                    }
                }
            }
        }
        return null;
    }

    /**
     * Modifies a ReferenceParam whose value is in {@code system|value} format.
     * For subject/patient params, requires the BSN token system and converts the token to a pseudonym.
     * For source params, passes the identifier through as-is, mapped to a Device reference.
     */
    private ReferenceParam getModifiedSearchParam(final ReferenceParam param, final boolean source) {
        final String value = param.getValue();
        if (value == null || !value.contains("|")) {
            return null;
        }

        final String[] parts = value.split("\\|", 2);
        final String system = parts[0];
        final String identifierValue = parts[1];

        if (source) {
            return new ReferenceParam(String.format("%s/%s/%s", system, ResourceType.Device.name(), identifierValue));
        }

        if (!BSN_TOKEN_SYSTEM.equals(system)) {
            return null;
        }
        log.info("Converting token to pseudonym in search parameter: {}", identifierValue);
        return new ReferenceParam(String.format("%s/%s/%s", PSEUDO_BSN_SYSTEM, ResourceType.Patient.name(), tokenToPseudonym(identifierValue)));
    }

    @Hook(Pointcut.STORAGE_PRESTORAGE_RESOURCE_CREATED)
    public void resourceCreated(final IBaseResource newResource,
                                final ServletRequestDetails servletRequestDetails) {
        if (!isEnabled(servletRequestDetails)) {
            return;
        }

        validateUraHeaderPresence(servletRequestDetails);

        if (newResource instanceof final DocumentReference documentReference) {
            validateCustodian(documentReference, servletRequestDetails);
            modifyDocumentFromTokenToPseudonym(documentReference);
            log.info("{}", FhirContext.forR4Cached().newJsonParser().encodeResourceToString(documentReference));
        } else if (newResource instanceof final ListResource list) {
            validateCustodian(list, servletRequestDetails);
            modifyListSubjectFromTokenToPseudonym(list);
            modifyListSourceFromTokenToPseudonym(list);
            log.info("{}", FhirContext.forR4Cached().newJsonParser().encodeResourceToString(list));
        }
    }

    /**
     * Triggers before resources are shown. Converts pseudonyms back to audience-specific tokens.
     */
    @Hook(Pointcut.STORAGE_PRESHOW_RESOURCES)
    public void handlePreShowResources(final IPreResourceShowDetails requestDetails,
                                       final ServletRequestDetails servletRequestDetails) {
        if (!isEnabled(servletRequestDetails)) {
            return;
        }

        validateUraHeaderPresence(servletRequestDetails);

        final String audience = getUraHeader(servletRequestDetails);
        final List<IBaseResource> allResources = requestDetails.getAllResources();
        allResources.forEach(resource -> {
            if (resource instanceof final DocumentReference documentReference) {
                modifyDocumentReferenceFromPseudonymToToken(documentReference, audience);
            } else if (resource instanceof final ListResource list) {
                modifyListFromPseudonymToToken(list, audience);
            }
        });
    }

    private void validateUraHeaderPresence(final ServletRequestDetails servletRequestDetails) {
        final String audience = getUraHeader(servletRequestDetails);
        if (StringUtils.isBlank(audience)) {
            throw new IllegalArgumentException(
                    String.format("'%s' header is mandatory.", REQUESTER_URA_HEADER));
        }
    }

    private String getUraHeader(final ServletRequestDetails servletRequestDetails) {
        return servletRequestDetails.getHeader(REQUESTER_URA_HEADER);
    }

    private void modifyDocumentReferenceFromPseudonymToToken(final DocumentReference resource, final String audience) {
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

    private void modifyListFromPseudonymToToken(final ListResource resource, final String audience) {
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
        final Identifier identifier = docRef.getSubject().getIdentifier();
        if (identifier == null || !BSN_TOKEN_SYSTEM.equals(identifier.getSystem())) {
            return;
        }
        log.trace("Found identifier: system={}, value={}", identifier.getSystem(), identifier.getValue());
        final String pseudonym = tokenToPseudonym(identifier.getValue());
        docRef.setSubject(identifierToReference(PSEUDO_BSN_SYSTEM, ResourceType.Patient.name(), pseudonym));
    }

    private void modifyListSubjectFromTokenToPseudonym(final ListResource docRef) {
        final Identifier identifier = docRef.getSubject().getIdentifier();
        if (identifier == null || !BSN_TOKEN_SYSTEM.equals(identifier.getSystem())) {
            return;
        }
        log.trace("Found identifier: system={}, value={}", identifier.getSystem(), identifier.getValue());
        final String pseudonym = tokenToPseudonym(identifier.getValue());
        docRef.setSubject(identifierToReference(PSEUDO_BSN_SYSTEM, ResourceType.Patient.name(), pseudonym));
    }

    private void modifyListSourceFromTokenToPseudonym(final ListResource docRef) {
        final Identifier identifier = docRef.getSource().getIdentifier();
        if (identifier == null) {
            return;
        }
        log.trace("Found identifier: system={}, value={}", identifier.getSystem(), identifier.getValue());
        docRef.setSource(identifierToReference(identifier.getSystem(), ResourceType.Device.name(), identifier.getValue()));
    }

    /**
     * Converts an identifier to a Reference using a system/resourceType/value path.
     * Unfortunately we need to do this, because HAPI doesn't support the :identifier modifier and so we need to
     * translate this to a reference instead, so we can search it by this reference. We'll also handle :identifier
     * modifier ourselves, as if we handle it, but in reality, we'll just modify :identifier modifier to a reference
     * param search — all this without letting 'the client' know of this workaround. Client acts as if it's storing
     * and searching by subject.as(Identifier).
     */
    private static Reference identifierToReference(final String system, final String resourceType, final String value) {
        return new Reference(new IdType(system, resourceType, value, null));
    }

    private String tokenToPseudonym(final String token) {
        final String pseudonym = bsnUtil.transportTokenToPseudonym(token);
        log.trace("Converted token to pseudonym: {}", pseudonym);
        return pseudonym;
    }

    private String pseudonymToToken(final String pseudonym, final String audience) {
        final String token = bsnUtil.pseudonymToTransportToken(pseudonym, audience);
        log.trace("Converted pseudonym to token: {}", token);
        return token;
    }

    /**
     * Validates that DocumentReference.custodian matches the X-Requester-URA header.
     */
    private void validateCustodian(final DocumentReference documentReference,
                                   final ServletRequestDetails servletRequestDetails) {
        final String requesterURA = servletRequestDetails.getHeader(REQUESTER_URA_HEADER);
        if (StringUtils.isEmpty(requesterURA)) {
            return;
        }

        final Reference custodian = documentReference.getCustodian();

        if (custodian.getIdentifier().isEmpty() && !custodian.isEmpty()) {
            // means that it's actually a reference to the Organization
            // meaning we'd ideally then need to fetch it and check of Organization.identifier matches
            // our header, but we won't do that... so just let it pass in this case
            return;
        }

        final String custodianURA = extractURAFromCustodian(custodian);
        if (!requesterURA.equals(custodianURA)) {
            throw new IllegalArgumentException(
                    String.format("DocumentReference.custodian URA (%s) does not match %s header (%s)",
                                  custodianURA, REQUESTER_URA_HEADER, requesterURA));
        }

        log.debug("Custodian URA validation successful: {}", custodianURA);
    }

    /**
     * Validates that ListResource custodian extension matches the X-Requester-URA header.
     */
    private void validateCustodian(final ListResource list,
                                   final ServletRequestDetails servletRequestDetails) {
        final String requesterURA = servletRequestDetails.getHeader(REQUESTER_URA_HEADER);
        if (StringUtils.isEmpty(requesterURA)) {
            return;
        }

        final String LIST_CUSTODIAN_URL = "http://minvws.github.io/generiekefuncties-docs/StructureDefinition/nl-gf-localization-custodian";
        final Extension custodianExtension = list.getExtensionByUrl(LIST_CUSTODIAN_URL);
        if (custodianExtension == null) {
            throw new IllegalArgumentException(
                    String.format("List.extension with url %s not present", LIST_CUSTODIAN_URL));
        }

        final Reference custodianReference = (Reference) custodianExtension.getValue();

        if (custodianReference.getIdentifier().isEmpty() && !custodianReference.isEmpty()) {
            // means that it's actually a reference to the Organization
            // meaning we'd ideally then need to fetch it and check of Organization.identifier matches
            // our header, but we won't do that... so just let it pass in this case
            return;
        }

        final String custodianURA = extractURAFromCustodian(custodianReference);
        if (!requesterURA.equals(custodianURA)) {
            throw new IllegalArgumentException(
                    String.format("List.custodian URA (%s) does not match %s header (%s)",
                                  custodianURA, REQUESTER_URA_HEADER, requesterURA));
        }

        log.debug("Custodian URA validation successful: {}", custodianURA);
    }

    /**
     * Extracts the URA value from a custodian Reference.
     * Expects the custodian to contain an identifier with the URA naming system.
     *
     * @param custodian the custodian Reference
     * @return the URA value, or null if not found
     */
    private String extractURAFromCustodian(final Reference custodian) {
        final Identifier identifier = custodian.getIdentifier();
        if (identifier == null) {
            return null;
        }

        if ("http://fhir.nl/fhir/NamingSystem/ura".equals(identifier.getSystem())) {
            return identifier.getValue();
        }
        return null;
    }
}
