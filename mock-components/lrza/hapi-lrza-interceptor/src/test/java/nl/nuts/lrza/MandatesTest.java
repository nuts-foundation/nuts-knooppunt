package nl.nuts.lrza;

import static nl.nuts.lrza.Directory.VENDOR;
import static nl.nuts.lrza.Directory.careProvider;
import static nl.nuts.lrza.Directory.mandate;
import static org.junit.jupiter.api.Assertions.assertEquals;

import java.util.Date;
import org.hl7.fhir.r4.model.Identifier;
import org.hl7.fhir.r4.model.Organization;
import org.hl7.fhir.r4.model.OrganizationAffiliation;
import org.hl7.fhir.r4.model.Period;
import org.hl7.fhir.r4.model.Reference;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

class MandatesTest {

    private static final Date NOW = new Date(1_800_000_000_000L);
    private static final OwnerResolver.Owner SUNFLOWER = new OwnerResolver.Owner("00000020", "sunflower");

    private final Directory directory = new Directory();
    private final Mandates mandates = new Mandates(directory, new OwnerResolver(directory), () -> NOW);

    @BeforeEach
    void setUp() {
        directory.put(careProvider("sunflower", "00000020"));
    }

    @Test
    void noAffiliationIsNoMandate() {
        assertEquals(Mandates.Mandate.NONE, mandates.granted(SUNFLOWER, VENDOR));
    }

    @Test
    void anActiveAffiliationGrantsItsCode() {
        directory.put(mandate("m1", "sunflower", VENDOR, "lrza-careprovider-admin"));

        assertEquals(Mandates.Mandate.CAREPROVIDER_ADMIN, mandates.granted(SUNFLOWER, VENDOR));
    }

    @Test
    void theWidestMandateWins() {
        directory.put(mandate("m1", "sunflower", VENDOR, "lrza-endpoint-admin"));
        assertEquals(Mandates.Mandate.ENDPOINT_ADMIN, mandates.granted(SUNFLOWER, VENDOR));

        directory.put(mandate("m2", "sunflower", VENDOR, "lrza-careprovider-admin"));
        assertEquals(Mandates.Mandate.CAREPROVIDER_ADMIN, mandates.granted(SUNFLOWER, VENDOR));
    }

    @Test
    void anotherVendorsMandateDoesNotCount() {
        directory.put(mandate("m1", "sunflower", new Caller("http://kvk.nl", "11111111"), "lrza-careprovider-admin"));

        assertEquals(Mandates.Mandate.NONE, mandates.granted(SUNFLOWER, VENDOR));
    }

    @Test
    void anotherCareProvidersMandateDoesNotCount() {
        directory.put(careProvider("plataan", "00000010"));
        directory.put(mandate("m1", "plataan", VENDOR, "lrza-careprovider-admin"));

        assertEquals(Mandates.Mandate.NONE, mandates.granted(SUNFLOWER, VENDOR));
    }

    @Test
    void aRevokedMandateDoesNotCount() {
        directory.put(mandate("m1", "sunflower", VENDOR, "lrza-careprovider-admin").setActive(false));

        assertEquals(Mandates.Mandate.NONE, mandates.granted(SUNFLOWER, VENDOR));
    }

    @Test
    void aMandateCountsWithinItsPeriodOnly() {
        final OrganizationAffiliation affiliation = directory.put(mandate("m1", "sunflower", VENDOR, "lrza-careprovider-admin"));

        affiliation.setPeriod(new Period().setEnd(new Date(NOW.getTime() - 1000)));
        assertEquals(Mandates.Mandate.NONE, mandates.granted(SUNFLOWER, VENDOR), "expired");

        affiliation.setPeriod(new Period().setStart(new Date(NOW.getTime() + 1000)));
        assertEquals(Mandates.Mandate.NONE, mandates.granted(SUNFLOWER, VENDOR), "not started");

        affiliation.setPeriod(new Period().setStart(new Date(NOW.getTime() - 1000)).setEnd(new Date(NOW.getTime() + 1000)));
        assertEquals(Mandates.Mandate.CAREPROVIDER_ADMIN, mandates.granted(SUNFLOWER, VENDOR), "current");
    }

    @Test
    void otherCodesGrantNothing() {
        directory.put(mandate("m1", "sunflower", VENDOR, "something-else"));

        assertEquals(Mandates.Mandate.NONE, mandates.granted(SUNFLOWER, VENDOR));
    }

    @Test
    void theVendorCanBeNamedByItsOrganization() {
        final Organization vendor = new Organization();
        vendor.setId("examplesoft");
        vendor.addIdentifier(new Identifier().setSystem(VENDOR.system()).setValue(VENDOR.value()));
        directory.put(vendor);
        directory.put(mandate("m1", "sunflower", VENDOR, "lrza-careprovider-admin")
                .setParticipatingOrganization(new Reference("Organization/examplesoft")));

        assertEquals(Mandates.Mandate.CAREPROVIDER_ADMIN, mandates.granted(SUNFLOWER, VENDOR));
    }

    @Test
    void aCareProviderWithoutOrganizationGrantedNothing() {
        assertEquals(Mandates.Mandate.NONE, mandates.granted(new OwnerResolver.Owner("00000020", null), VENDOR));
    }
}
