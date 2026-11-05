package nl.nuts.lrza;

import ca.uhn.fhir.context.FhirContext;
import ca.uhn.fhir.rest.server.exceptions.ForbiddenOperationException;
import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ObjectNode;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Set;
import org.hl7.fhir.r4.model.OrganizationAffiliation;
import org.hl7.fhir.r4.model.Provenance;
import org.hl7.fhir.r4.model.Reference;
import org.hl7.fhir.r4.model.Resource;

/**
 * Who may write what, as the LRZA authorizes it:
 * <ul>
 *   <li>a care provider writes its own resources, and is the only one to grant (or revoke) a mandate;</li>
 *   <li>an IT vendor writes a care provider's resources under the mandate the care provider granted it:
 *       lrza-careprovider-admin for all of them, lrza-endpoint-admin for Endpoints and the links to
 *       them only;</li>
 *   <li>Practitioner and PractitionerRole are out of scope, as is every type the LRZA doesn't hold.</li>
 * </ul>
 * An update is checked against both versions, so a write can't move a resource from a care provider
 * to another one either.
 */
final class WriteRules {

    private static final ObjectMapper MAPPER = new ObjectMapper();

    private final FhirContext fhirContext;
    private final ResourceLookup lookup;
    private final OwnerResolver owners;
    private final Mandates mandates;

    WriteRules(final FhirContext fhirContext, final ResourceLookup lookup, final OwnerResolver owners, final Mandates mandates) {
        this.fhirContext = fhirContext;
        this.lookup = lookup;
        this.owners = owners;
        this.mandates = mandates;
    }

    /**
     * Authorizes a write of newVersion by caller; oldVersion is the version it replaces, null for a create.
     *
     * @throws ForbiddenOperationException when the caller may not write it
     */
    void authorize(final Caller caller, final Resource newVersion, final Resource oldVersion) {
        if (caller == null) {
            throw new ForbiddenOperationException("Unknown caller: the mock LRZA has no LRZA_CALLER configured and the request has no X-Mock-Caller");
        }
        switch (newVersion.fhirType()) {
            case "Organization", "Location", "HealthcareService", "Endpoint" -> {
                final Set<OwnerResolver.Owner> careProviders = new LinkedHashSet<>();
                careProviders.add(owners.ownerOf(newVersion));
                if (oldVersion != null) {
                    careProviders.add(owners.ownerOf(oldVersion));
                }
                for (final OwnerResolver.Owner careProvider : careProviders) {
                    authorizeFor(caller, careProvider, newVersion, oldVersion);
                }
            }
            case "OrganizationAffiliation" -> {
                authorizeMandate(caller, (OrganizationAffiliation) newVersion);
                if (oldVersion != null) {
                    authorizeMandate(caller, (OrganizationAffiliation) oldVersion);
                }
            }
            case "Provenance" -> authorizeFor(caller, ownerOfTarget((Provenance) newVersion), newVersion, null);
            case "Practitioner", "PractitionerRole" ->
                    throw new ForbiddenOperationException(newVersion.fhirType() + " is out of scope of the LRZA");
            default -> throw new ForbiddenOperationException(newVersion.fhirType() + " is not a resource type the LRZA holds");
        }
    }

    /** A care provider writes its own resources; a vendor needs the care provider's mandate. */
    private void authorizeFor(final Caller caller, final OwnerResolver.Owner careProvider, final Resource newVersion,
                              final Resource oldVersion) {
        if (caller.isCareProvider()) {
            if (!caller.value().equals(careProvider.ura())) {
                throw new ForbiddenOperationException(String.format("%s belongs to the care provider with URA %s, not to the caller (%s)",
                        newVersion.fhirType(), careProvider.ura(), caller));
            }
            return;
        }
        final Mandates.Mandate mandate = mandates.granted(careProvider, caller);
        switch (mandate) {
            case CAREPROVIDER_ADMIN -> {
            }
            case ENDPOINT_ADMIN -> {
                if (!"Endpoint".equals(newVersion.fhirType()) && !"Provenance".equals(newVersion.fhirType())
                        && !onlyEndpointsChanged(oldVersion, newVersion)) {
                    throw new ForbiddenOperationException(String.format(
                            "The care provider with URA %s granted %s only %s: it may write Endpoints and change %s.endpoint, nothing else",
                            careProvider.ura(), caller, mandate.code, newVersion.fhirType()));
                }
            }
            default -> throw new ForbiddenOperationException(String.format(
                    "The care provider with URA %s has not granted %s a mandate (an active OrganizationAffiliation with code %s|%s)",
                    careProvider.ura(), caller, Mandates.AUTHORIZATION_TYPE_SYSTEM, Mandates.Mandate.CAREPROVIDER_ADMIN.code));
        }
    }

    /** Only the care provider named as organization writes a mandate. */
    private void authorizeMandate(final Caller caller, final OrganizationAffiliation affiliation) {
        if (!caller.isCareProvider()) {
            throw new ForbiddenOperationException(String.format(
                    "Only a care provider grants a mandate; the caller (%s) is not identified by a URA", caller));
        }
        final OwnerResolver.Owner careProvider = owners.ownerOfOrganizationReference(affiliation.getOrganization(),
                "OrganizationAffiliation.organization");
        if (!caller.value().equals(careProvider.ura())) {
            throw new ForbiddenOperationException(String.format(
                    "OrganizationAffiliation.organization is the care provider with URA %s, not the caller (%s)", careProvider.ura(), caller));
        }
    }

    /** The care provider responsible for the resource a Provenance is about. */
    OwnerResolver.Owner ownerOfTarget(final Provenance provenance) {
        return owners.ownerOf(target(provenance));
    }

    /** The version of the resource a Provenance is about. */
    Resource target(final Provenance provenance) {
        if (provenance.getTarget().size() != 1) {
            throw new ForbiddenOperationException("A Provenance in the LRZA has exactly one target");
        }
        final Reference reference = provenance.getTargetFirstRep();
        final References.Target target = References.parse(reference);
        if (target == null) {
            throw new ForbiddenOperationException("Provenance.target (" + OwnerResolver.describe(reference) + ") is not a literal reference");
        }
        final Resource resource = target.versionId() == null
                ? lookup.read(target.resourceType(), target.id())
                : lookup.readVersion(target.resourceType(), target.id(), target.versionId());
        if (resource == null) {
            throw new ForbiddenOperationException("Provenance.target (" + reference.getReference() + ") does not exist in the LRZA");
        }
        return resource;
    }

    /** Whether an update changes nothing but the resource's endpoint references. */
    boolean onlyEndpointsChanged(final Resource oldVersion, final Resource newVersion) {
        if (oldVersion == null) {
            return false;
        }
        return withoutEndpoints(oldVersion).equals(withoutEndpoints(newVersion));
    }

    private ObjectNode withoutEndpoints(final Resource resource) {
        try {
            final ObjectNode json = (ObjectNode) MAPPER.readTree(fhirContext.newJsonParser().encodeResourceToString(resource));
            json.remove(List.of("id", "meta", "text", "endpoint"));
            return json;
        } catch (JsonProcessingException e) {
            throw new IllegalStateException("HAPI encoded invalid JSON", e);
        }
    }
}
