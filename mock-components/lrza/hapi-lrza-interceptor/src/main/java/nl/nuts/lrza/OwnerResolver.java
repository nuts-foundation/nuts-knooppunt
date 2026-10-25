package nl.nuts.lrza;

import ca.uhn.fhir.rest.server.exceptions.ForbiddenOperationException;
import java.util.List;
import org.hl7.fhir.r4.model.Endpoint;
import org.hl7.fhir.r4.model.HealthcareService;
import org.hl7.fhir.r4.model.Identifier;
import org.hl7.fhir.r4.model.Location;
import org.hl7.fhir.r4.model.Organization;
import org.hl7.fhir.r4.model.Reference;
import org.hl7.fhir.r4.model.Resource;

/**
 * Finds the care provider responsible for a resource: the Organization carrying a URA. An Organization
 * is responsible for itself when it carries a URA, else its partOf chain leads to one; Location and
 * Endpoint belong to their managingOrganization, HealthcareService to its providedBy. These are the
 * rules the Knooppunt applies to its tenants (component/lrza/ownership.go).
 */
final class OwnerResolver {

    /** How far up a partOf chain is followed, so a cycle can't make a write read without end. */
    static final int MAX_PART_OF_DEPTH = 10;

    /**
     * The care provider: its URA and the id of its Organization. The id is null when a reference names
     * the care provider by URA only and the directory holds no Organization with that URA.
     */
    record Owner(String ura, String organizationId) {
    }

    private final ResourceLookup lookup;

    OwnerResolver(final ResourceLookup lookup) {
        this.lookup = lookup;
    }

    /**
     * The care provider responsible for an Organization, Location, HealthcareService or Endpoint.
     *
     * @throws ForbiddenOperationException when there is none, so the write can't be authorized
     */
    Owner ownerOf(final Resource resource) {
        if (resource instanceof final Organization organization) {
            return ownerOfOrganization(organization, 0);
        }
        final String element;
        final Reference reference;
        if (resource instanceof final Location location) {
            element = "Location.managingOrganization";
            reference = location.getManagingOrganization();
        } else if (resource instanceof final Endpoint endpoint) {
            element = "Endpoint.managingOrganization";
            reference = endpoint.getManagingOrganization();
        } else if (resource instanceof final HealthcareService service) {
            element = "HealthcareService.providedBy";
            reference = service.getProvidedBy();
        } else {
            throw new ForbiddenOperationException(resource.fhirType() + " has no owning care provider");
        }
        return ownerOfOrganizationReference(reference, element);
    }

    /** The care provider an Organization reference leads to; element names it in error messages. */
    Owner ownerOfOrganizationReference(final Reference reference, final String element) {
        if (reference == null || reference.isEmpty()) {
            throw new ForbiddenOperationException(element + " is missing, so the care provider it belongs to is unknown");
        }
        final Identifier identifier = reference.getIdentifier();
        if (!reference.hasReference() && identifier != null && Caller.URA_SYSTEM.equals(identifier.getSystem())
                && identifier.hasValue()) {
            final List<Organization> organizations = lookup.organizationsByIdentifier(Caller.URA_SYSTEM, identifier.getValue());
            return new Owner(identifier.getValue(), organizations.isEmpty() ? null : organizations.get(0).getIdPart());
        }
        final Organization organization = readOrganization(reference);
        if (organization == null) {
            throw new ForbiddenOperationException(String.format("%s (%s) does not refer to an Organization in the LRZA",
                    element, describe(reference)));
        }
        return ownerOfOrganization(organization, 0);
    }

    private Owner ownerOfOrganization(final Organization organization, final int depth) {
        for (final Identifier identifier : organization.getIdentifier()) {
            if (Caller.URA_SYSTEM.equals(identifier.getSystem()) && identifier.hasValue()) {
                return new Owner(identifier.getValue(), organization.getIdElement().getIdPart());
            }
        }
        if (!organization.hasPartOf()) {
            throw new ForbiddenOperationException("Organization has neither a URA nor a partOf leading to one");
        }
        if (depth >= MAX_PART_OF_DEPTH) {
            throw new ForbiddenOperationException("Organization's partOf chain is longer than " + MAX_PART_OF_DEPTH);
        }
        final Organization parent = readOrganization(organization.getPartOf());
        if (parent == null) {
            throw new ForbiddenOperationException(String.format("Organization.partOf (%s) does not refer to an Organization in the LRZA",
                    describe(organization.getPartOf())));
        }
        return ownerOfOrganization(parent, depth + 1);
    }

    /** Reads the Organization a literal reference points to, or null. */
    Organization readOrganization(final Reference reference) {
        final References.Target target = References.parse(reference);
        if (target == null || !"Organization".equals(target.resourceType())) {
            return null;
        }
        return lookup.read("Organization", target.id()) instanceof final Organization organization ? organization : null;
    }

    static String describe(final Reference reference) {
        if (reference.hasReference()) {
            return reference.getReference();
        }
        if (reference.hasIdentifier()) {
            return reference.getIdentifier().getSystem() + "|" + reference.getIdentifier().getValue();
        }
        return "empty reference";
    }
}
