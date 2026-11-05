package nl.nuts.lrza;

import static nl.nuts.lrza.Directory.VENDOR;
import static nl.nuts.lrza.Directory.careProvider;
import static nl.nuts.lrza.Directory.endpoint;
import static nl.nuts.lrza.Directory.location;
import static nl.nuts.lrza.Directory.mandate;
import static org.junit.jupiter.api.Assertions.assertDoesNotThrow;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import ca.uhn.fhir.context.FhirContext;
import ca.uhn.fhir.rest.server.exceptions.ForbiddenOperationException;
import java.util.Date;
import org.hl7.fhir.r4.model.Location;
import org.hl7.fhir.r4.model.Organization;
import org.hl7.fhir.r4.model.OrganizationAffiliation;
import org.hl7.fhir.r4.model.Patient;
import org.hl7.fhir.r4.model.Practitioner;
import org.hl7.fhir.r4.model.PractitionerRole;
import org.hl7.fhir.r4.model.Provenance;
import org.hl7.fhir.r4.model.Reference;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

class WriteRulesTest {

    private static final Caller SUNFLOWER = new Caller(Caller.URA_SYSTEM, "00000020");
    private static final Caller CARE2CURE = new Caller(Caller.URA_SYSTEM, "00000030");

    private final Directory directory = new Directory();
    private final OwnerResolver owners = new OwnerResolver(directory);
    private final WriteRules rules = new WriteRules(FhirContext.forR4Cached(), directory, owners, new Mandates(directory, owners, Date::new));

    @BeforeEach
    void setUp() {
        directory.put(careProvider("sunflower", "00000020"));
        directory.put(careProvider("care2cure", "00000030"));
        directory.put(mandate("sunflower-mandate", "sunflower", VENDOR, "lrza-careprovider-admin"));
    }

    @Test
    void aVendorWritesUnderTheCareProvidersMandate() {
        assertDoesNotThrow(() -> rules.authorize(VENDOR, location("north", "sunflower"), null));
        assertDoesNotThrow(() -> rules.authorize(VENDOR, endpoint("fhir", "sunflower"), null));
        assertDoesNotThrow(() -> rules.authorize(VENDOR, careProvider("sunflower", "00000020").setName("Renamed"),
                careProvider("sunflower", "00000020")));
    }

    @Test
    void aVendorWithoutMandateIsForbidden() {
        final ForbiddenOperationException exception = assertThrows(ForbiddenOperationException.class,
                () -> rules.authorize(VENDOR, location("north", "care2cure"), null));

        assertTrue(exception.getMessage().contains("has not granted"), exception.getMessage());
        assertTrue(exception.getMessage().contains("00000030"), exception.getMessage());
    }

    @Test
    void anUpdateCannotMoveAResourceToACareProviderThatDidNotGrantTheMandate() {
        assertThrows(ForbiddenOperationException.class,
                () -> rules.authorize(VENDOR, location("north", "care2cure"), location("north", "sunflower")));
        assertThrows(ForbiddenOperationException.class,
                () -> rules.authorize(VENDOR, location("north", "sunflower"), location("north", "care2cure")));
    }

    @Test
    void aVendorCannotRegisterANewCareProvider() {
        assertThrows(ForbiddenOperationException.class,
                () -> rules.authorize(VENDOR, careProvider("new", "00000099"), null));
    }

    @Test
    void aCareProviderWritesItsOwnResourcesOnly() {
        assertDoesNotThrow(() -> rules.authorize(CARE2CURE, location("north", "care2cure"), null));
        assertDoesNotThrow(() -> rules.authorize(CARE2CURE, careProvider("new-care2cure", "00000030"), null));
        assertThrows(ForbiddenOperationException.class, () -> rules.authorize(CARE2CURE, location("north", "sunflower"), null));
    }

    @Test
    void onlyTheCareProviderGrantsAMandate() {
        final OrganizationAffiliation grant = mandate("care2cure-mandate", "care2cure", VENDOR, "lrza-careprovider-admin");

        assertDoesNotThrow(() -> rules.authorize(CARE2CURE, grant, null));
        assertThrows(ForbiddenOperationException.class, () -> rules.authorize(VENDOR, grant, null), "the vendor");
        assertThrows(ForbiddenOperationException.class, () -> rules.authorize(SUNFLOWER, grant, null), "another care provider");
    }

    @Test
    void aCareProviderCannotTakeOverAnotherCareProvidersMandate() {
        final OrganizationAffiliation existing = mandate("care2cure-mandate", "care2cure", VENDOR, "lrza-careprovider-admin");
        final OrganizationAffiliation takenOver = mandate("care2cure-mandate", "sunflower", VENDOR, "lrza-careprovider-admin");

        assertThrows(ForbiddenOperationException.class, () -> rules.authorize(SUNFLOWER, takenOver, existing));
    }

    @Test
    void practitionersAndOtherTypesAreNotInTheLrza() {
        assertThrows(ForbiddenOperationException.class, () -> rules.authorize(VENDOR, new Practitioner(), null));
        assertThrows(ForbiddenOperationException.class, () -> rules.authorize(VENDOR, new PractitionerRole(), null));
        assertThrows(ForbiddenOperationException.class, () -> rules.authorize(SUNFLOWER, new Patient(), null));
    }

    @Test
    void anUnknownCallerIsForbidden() {
        assertThrows(ForbiddenOperationException.class, () -> rules.authorize(null, location("north", "sunflower"), null));
    }

    @Test
    void anEndpointAdminMandateCoversEndpointsAndTheLinksToThem() {
        directory.put(mandate("care2cure-mandate", "care2cure", VENDOR, "lrza-endpoint-admin"));
        final Organization current = careProvider("care2cure", "00000030");

        assertDoesNotThrow(() -> rules.authorize(VENDOR, endpoint("fhir", "care2cure"), null), "an Endpoint");
        final Organization linked = careProvider("care2cure", "00000030");
        linked.addEndpoint(new Reference("Endpoint/fhir"));
        assertDoesNotThrow(() -> rules.authorize(VENDOR, linked, current), "a link to it");

        final Organization renamed = careProvider("care2cure", "00000030").setName("Renamed");
        assertThrows(ForbiddenOperationException.class, () -> rules.authorize(VENDOR, renamed, current), "anything else");
        assertThrows(ForbiddenOperationException.class, () -> rules.authorize(VENDOR, location("north", "care2cure"), null),
                "a new Location");
    }

    @Test
    void aProvenanceIsWrittenForTheCareProviderOfItsTarget() {
        final Location target = directory.put(location("north", "sunflower"));
        target.setId("Location/north/_history/1");
        directory.put(target);
        final Provenance provenance = new Provenance();
        provenance.addTarget(new Reference("Location/north/_history/1"));

        assertDoesNotThrow(() -> rules.authorize(VENDOR, provenance, null));

        final Location other = directory.put(location("south", "care2cure"));
        final Provenance forOther = new Provenance();
        forOther.addTarget(new Reference("Location/" + other.getIdElement().getIdPart()));
        assertThrows(ForbiddenOperationException.class, () -> rules.authorize(VENDOR, forOther, null));

        final Provenance forNothing = new Provenance();
        forNothing.addTarget(new Reference("Location/missing"));
        assertThrows(ForbiddenOperationException.class, () -> rules.authorize(VENDOR, forNothing, null));
    }
}
