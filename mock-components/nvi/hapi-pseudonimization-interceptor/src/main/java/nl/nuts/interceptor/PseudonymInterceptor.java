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
import lombok.extern.slf4j.Slf4j;
import nl.nuts.util.BsnUtil;
import org.hl7.fhir.instance.model.api.IBaseResource;
import org.hl7.fhir.instance.model.api.IIdType;
import org.hl7.fhir.r4.model.*;
import org.springframework.stereotype.Component;

import java.util.List;

@Component
@Interceptor
@Slf4j
public class PseudonymInterceptor {

    private static final String PSEUDO_BSN_SYSTEM = System.getenv().getOrDefault(
            "PSEUDO_BSN_SYSTEM", "http://fhir.nl/fhir/NamingSystem/bsn-pseudonym");
    /**
     * @deprecated use BSN_TOKEN_SYSTEM_NEW
     */
    @Deprecated
    private static final String BSN_TOKEN_SYSTEM = System.getenv().getOrDefault(
            "BSN_TOKEN_SYSTEM", "http://fhir.nl/fhir/NamingSystem/bsn-transport-token");
    private static final String BSN_TOKEN_SYSTEM_NEW = "http://minvws.github.io/generiekefuncties-docs/NamingSystem/nvi-identifier"; // for fake nvi to be hackwards compatible, both of these are checked
    private static final String INTERCEPTOR_ENABLED_FOR_TENANT = System.getenv().getOrDefault(
            "NVI_TENANT", "nvi");
    private static final String LIST_EXTENSION_CUSTODIAN_URL = "http://minvws.github.io/generiekefuncties-docs/StructureDefinition/nl-gf-localization-custodian";

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
        if (!ResourceType.List.name().equals(searchDetails.getResourceType())) {
            return;
        }
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

        if (!BSN_TOKEN_SYSTEM.equals(system) && !BSN_TOKEN_SYSTEM_NEW.equals(system)) {
            return null;
        }
        log.info("Converting token to pseudonym in search parameter: {}", identifierValue);
        return new ReferenceParam(String.format("%s/%s/%s", PSEUDO_BSN_SYSTEM, ResourceType.Patient.name(), tokenToPseudonym(identifierValue)));
    }

    /**
     * Triggers before a Resource (in our case a ListResource) is created. We take currently set
     * ListResource.subject and create pseudonym from a token set on there
     */
    @Hook(Pointcut.STORAGE_PRESTORAGE_RESOURCE_CREATED)
    public void resourceCreated(final IBaseResource newResource,
                                final ServletRequestDetails servletRequestDetails) {
        if (!isEnabled(servletRequestDetails)) {
            return;
        }
        if (!(newResource instanceof final ListResource list)) {
            return;
        }

        modifyListSubjectFromTokenToPseudonym(list);
        modifyListSourceFromTokenToPseudonym(list);
        log.info("{}", FhirContext.forR4Cached().newJsonParser().encodeResourceToString(list));
    }


    /**
     * Triggers before a Resource (in our case a ListResource) is read (also invoked when a Resource is created,
     * but is returned in a response to creation. We replace currently set ListResource.subject (which is a
     * pseudonym) with an audience-specific token
     */
    @Hook(Pointcut.STORAGE_PRESHOW_RESOURCES)
    public void handlePreShowResources(final IPreResourceShowDetails requestDetails,
                                       final ServletRequestDetails servletRequestDetails) {
        if (!isEnabled(servletRequestDetails)) {
            return;
        }

        final List<IBaseResource> allResources = requestDetails.getAllResources();
        allResources.stream()
                .filter(aResource -> aResource instanceof ListResource)
                .forEach(list -> modifyDocumentFromPseudonymToToken((ListResource) list,
                        getListCustodianUra((ListResource) list)));
    }

    private String getListCustodianUra(final ListResource listResource) {
        final Extension custodianExtension = listResource.getExtensionByUrl(LIST_EXTENSION_CUSTODIAN_URL);
        if (custodianExtension == null) {
            throw new IllegalArgumentException(
                    String.format("List.extension with url %s not present", LIST_EXTENSION_CUSTODIAN_URL));
        }

        final Reference custodianReference = (Reference) custodianExtension.getValue();

        if (custodianReference.getIdentifier().isEmpty() && !custodianReference.isEmpty()) {
            throw new IllegalArgumentException(
                    String.format("List.extension.url(%s).identifier is empty", LIST_EXTENSION_CUSTODIAN_URL));
        }

        // Extract URA from custodian
        return extractURAFromCustodian(custodianReference);
    }

    private void modifyDocumentFromPseudonymToToken(final ListResource resource, final String audience) {
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
        identifier.setSystem(BSN_TOKEN_SYSTEM_NEW);
        identifier.setValue(token);
        resource.setSubject(new Reference().setIdentifier(identifier));
    }

    private void modifyListSubjectFromTokenToPseudonym(final ListResource docRef) {
        final Identifier identifier = docRef.getSubject().getIdentifier();
        if (identifier == null || (!BSN_TOKEN_SYSTEM.equals(identifier.getSystem())
                && !BSN_TOKEN_SYSTEM_NEW.equals(identifier.getSystem()))) {
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
        docRef.setSource(
                identifierToReference(identifier.getSystem(), ResourceType.Device.name(), identifier.getValue()));
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



