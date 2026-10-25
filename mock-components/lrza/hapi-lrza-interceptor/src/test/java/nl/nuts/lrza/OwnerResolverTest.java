package nl.nuts.lrza;

import static nl.nuts.lrza.Directory.careProvider;
import static nl.nuts.lrza.Directory.department;
import static nl.nuts.lrza.Directory.endpoint;
import static nl.nuts.lrza.Directory.location;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import ca.uhn.fhir.rest.server.exceptions.ForbiddenOperationException;
import org.hl7.fhir.r4.model.HealthcareService;
import org.hl7.fhir.r4.model.Identifier;
import org.hl7.fhir.r4.model.Location;
import org.hl7.fhir.r4.model.Organization;
import org.hl7.fhir.r4.model.Practitioner;
import org.hl7.fhir.r4.model.Reference;
import org.junit.jupiter.api.Test;

class OwnerResolverTest {

    private final Directory directory = new Directory();
    private final OwnerResolver owners = new OwnerResolver(directory);

    @Test
    void anOrganizationWithAUraOwnsItself() {
        assertEquals(new OwnerResolver.Owner("00000020", "sunflower"), owners.ownerOf(careProvider("sunflower", "00000020")));
    }

    @Test
    void aDepartmentBelongsToTheCareProviderItsPartOfChainLeadsTo() {
        directory.put(careProvider("sunflower", "00000020"));
        directory.put(department("ward", "sunflower"));

        assertEquals(new OwnerResolver.Owner("00000020", "sunflower"), owners.ownerOf(department("bed", "ward")));
    }

    @Test
    void aCyclicPartOfChainEnds() {
        directory.put(department("a", "b"));
        directory.put(department("b", "a"));

        final ForbiddenOperationException exception = assertThrows(ForbiddenOperationException.class,
                () -> owners.ownerOf(department("c", "a")));
        assertTrue(exception.getMessage().contains("longer than"), exception.getMessage());
    }

    @Test
    void anOrganizationWithoutUraOrPartOfHasNoOwner() {
        final Organization organization = new Organization();
        organization.setId("anonymous");

        assertThrows(ForbiddenOperationException.class, () -> owners.ownerOf(organization));
    }

    @Test
    void locationAndEndpointBelongToTheirManagingOrganization() {
        directory.put(careProvider("sunflower", "00000020"));

        assertEquals("00000020", owners.ownerOf(location("north", "sunflower")).ura());
        assertEquals("00000020", owners.ownerOf(endpoint("fhir", "sunflower")).ura());
    }

    @Test
    void aHealthcareServiceBelongsToItsProvider() {
        directory.put(careProvider("sunflower", "00000020"));
        final HealthcareService service = new HealthcareService();
        service.setProvidedBy(new Reference("Organization/sunflower"));

        assertEquals("00000020", owners.ownerOf(service).ura());
    }

    @Test
    void aReferenceByUraNeedsNoOrganization() {
        final Location location = location("north", "unused");
        location.setManagingOrganization(new Reference().setIdentifier(new Identifier().setSystem(Caller.URA_SYSTEM).setValue("00000020")));

        final OwnerResolver.Owner owner = owners.ownerOf(location);

        assertEquals("00000020", owner.ura());
        assertNull(owner.organizationId(), "no Organization to grant a mandate from");

        directory.put(careProvider("sunflower", "00000020"));
        assertEquals("sunflower", owners.ownerOf(location).organizationId());
    }

    @Test
    void aMissingOrUnknownOrganizationHasNoOwner() {
        final Location withoutOrganization = location("north", "unused");
        withoutOrganization.setManagingOrganization(null);

        assertThrows(ForbiddenOperationException.class, () -> owners.ownerOf(withoutOrganization));
        assertThrows(ForbiddenOperationException.class, () -> owners.ownerOf(location("north", "nobody")));
    }

    @Test
    void otherTypesHaveNoOwner() {
        assertThrows(ForbiddenOperationException.class, () -> owners.ownerOf(new Practitioner()));
    }

    @Test
    void referencesParse() {
        assertEquals(new References.Target("Organization", "1", null), References.parse("Organization/1"));
        assertEquals(new References.Target("Endpoint", "2", "3"), References.parse("Endpoint/2/_history/3"));
        assertEquals(new References.Target("Endpoint", "2", "3"), References.parse("http://lrza/fhir/Endpoint/2/_history/3"));
        assertNull(References.parse("urn:uuid:5d8a2f61-0c1e-4d0b-8a7e-0f1d3c4b5a69"));
        assertNull(References.parse("#contained"));
        assertNull(References.parse("organization"));
    }
}
