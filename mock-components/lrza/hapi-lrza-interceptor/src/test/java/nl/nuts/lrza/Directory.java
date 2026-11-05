package nl.nuts.lrza;

import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import org.hl7.fhir.r4.model.CodeableConcept;
import org.hl7.fhir.r4.model.Coding;
import org.hl7.fhir.r4.model.Endpoint;
import org.hl7.fhir.r4.model.Identifier;
import org.hl7.fhir.r4.model.Location;
import org.hl7.fhir.r4.model.Organization;
import org.hl7.fhir.r4.model.OrganizationAffiliation;
import org.hl7.fhir.r4.model.Reference;
import org.hl7.fhir.r4.model.Resource;

/** An in-memory LRZA partition for the rules to read, with builders for its resources. */
final class Directory implements ResourceLookup {

    static final String VENDOR_KVK = "90001234";
    static final Caller VENDOR = new Caller("http://kvk.nl", VENDOR_KVK);

    private final Map<String, Resource> current = new HashMap<>();
    private final Map<String, Resource> versions = new HashMap<>();

    <T extends Resource> T put(final T resource) {
        final String key = resource.fhirType() + "/" + resource.getIdElement().getIdPart();
        current.put(key, resource);
        if (resource.getIdElement().hasVersionIdPart()) {
            versions.put(key + "/_history/" + resource.getIdElement().getVersionIdPart(), resource);
        }
        return resource;
    }

    @Override
    public Resource read(final String resourceType, final String id) {
        return current.get(resourceType + "/" + id);
    }

    @Override
    public Resource readVersion(final String resourceType, final String id, final String versionId) {
        return versions.get(resourceType + "/" + id + "/_history/" + versionId);
    }

    @Override
    public List<Organization> organizationsByIdentifier(final String system, final String value) {
        final List<Organization> result = new ArrayList<>();
        for (final Resource resource : current.values()) {
            if (resource instanceof final Organization organization && organization.getIdentifier().stream()
                    .anyMatch(identifier -> system.equals(identifier.getSystem()) && value.equals(identifier.getValue()))) {
                result.add(organization);
            }
        }
        return result;
    }

    @Override
    public List<OrganizationAffiliation> affiliationsOf(final String organizationId) {
        final List<OrganizationAffiliation> result = new ArrayList<>();
        for (final Resource resource : current.values()) {
            if (resource instanceof final OrganizationAffiliation affiliation
                    && ("Organization/" + organizationId).equals(affiliation.getOrganization().getReference())) {
                result.add(affiliation);
            }
        }
        return result;
    }

    static Organization careProvider(final String id, final String ura) {
        final Organization organization = new Organization();
        organization.setId(id);
        organization.setName("Care provider " + ura);
        organization.addIdentifier(new Identifier().setSystem(Caller.URA_SYSTEM).setValue(ura));
        return organization;
    }

    static Organization department(final String id, final String parentId) {
        final Organization organization = new Organization();
        organization.setId(id);
        organization.setName("Department " + id);
        organization.setPartOf(new Reference("Organization/" + parentId));
        return organization;
    }

    static Location location(final String id, final String organizationId) {
        final Location location = new Location();
        location.setId(id);
        location.setName("Location " + id);
        location.setManagingOrganization(new Reference("Organization/" + organizationId));
        return location;
    }

    static Endpoint endpoint(final String id, final String organizationId) {
        final Endpoint endpoint = new Endpoint();
        endpoint.setId(id);
        endpoint.setAddress("https://example.org/fhir/" + id);
        endpoint.setManagingOrganization(new Reference("Organization/" + organizationId));
        return endpoint;
    }

    static OrganizationAffiliation mandate(final String id, final String organizationId, final Caller vendor, final String code) {
        final OrganizationAffiliation affiliation = new OrganizationAffiliation();
        affiliation.setId(id);
        affiliation.setActive(true);
        affiliation.setOrganization(new Reference("Organization/" + organizationId));
        affiliation.setParticipatingOrganization(new Reference()
                .setIdentifier(new Identifier().setSystem(vendor.system()).setValue(vendor.value())));
        affiliation.addCode(new CodeableConcept(new Coding(Mandates.AUTHORIZATION_TYPE_SYSTEM, code, null)));
        return affiliation;
    }
}
