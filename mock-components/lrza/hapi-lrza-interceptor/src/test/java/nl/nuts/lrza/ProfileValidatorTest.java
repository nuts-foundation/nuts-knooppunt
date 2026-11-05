package nl.nuts.lrza;

import static org.junit.jupiter.api.Assertions.assertDoesNotThrow;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;
import static org.junit.jupiter.api.Assumptions.assumeTrue;

import ca.uhn.fhir.context.FhirContext;
import ca.uhn.fhir.rest.server.exceptions.UnprocessableEntityException;
import java.io.InputStream;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import org.hl7.fhir.r4.model.CodeableConcept;
import org.hl7.fhir.r4.model.Coding;
import org.hl7.fhir.r4.model.Endpoint;
import org.hl7.fhir.r4.model.Resource;
import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.ValueSource;
import org.junit.jupiter.api.Test;

/**
 * Needs the IG package and its dependencies, which the image downloads at build time: run with
 * LRZA_PACKAGES_DIR pointing at a directory of them (see the README); skipped otherwise.
 */
class ProfileValidatorTest {

    private static final FhirContext FHIR = FhirContext.forR4Cached();
    private static ProfileValidator validator;

    @BeforeAll
    static void load() throws Exception {
        final String directory = System.getenv("LRZA_PACKAGES_DIR");
        assumeTrue(directory != null && Files.isDirectory(Path.of(directory)), "LRZA_PACKAGES_DIR is not set");
        validator = ProfileValidator.fromPackageDirectory(FHIR, Path.of(directory));
    }

    /** The seeded directory (test/testdata/vectors/lrza) conforms. */
    @ParameterizedTest
    @ValueSource(strings = {
            "Organization-e5909595-767e-41c1-9b00-a23ddf33e5d1.json",
            "Endpoint-f8a9c2d1-4567-489a-bcde-123456789abc.json",
            "Endpoint-3c1f0b6e-2d84-4a15-9f77-6b0e2a5c8d31.json",
            "HealthcareService-6d2b9e41-0c7a-4f38-b5e2-8a14c3f7d920.json",
            "OrganizationAffiliation-2b7e3f0a-5c4d-4e8f-9a1b-6c3d2e1f0a9b.json",
    })
    void theSeededResourcesConform(final String file) throws Exception {
        assertDoesNotThrow(() -> validator.validate(vector(file)));
    }

    @Test
    void anUnknownDataCategoryDoesNotConform() throws Exception {
        final Endpoint endpoint = (Endpoint) vector("Endpoint-f8a9c2d1-4567-489a-bcde-123456789abc.json");
        endpoint.getPayloadType().clear();
        endpoint.addPayloadType(new CodeableConcept(new Coding(
                "http://fhir.generiekefuncties.nl/csd/CodeSystem/nl-gf-data-categories-cs", "NoSuchCategory", null)));

        final UnprocessableEntityException exception = assertThrows(UnprocessableEntityException.class, () -> validator.validate(endpoint));
        assertTrue(exception.getMessage().contains("payloadType") || exception.getMessage().contains("nl-gf-payloadtype-req"),
                exception.getMessage());
    }

    @Test
    void aMissingRequiredElementDoesNotConform() throws Exception {
        final Endpoint endpoint = (Endpoint) vector("Endpoint-f8a9c2d1-4567-489a-bcde-123456789abc.json");
        endpoint.setManagingOrganization(null);

        assertThrows(UnprocessableEntityException.class, () -> validator.validate(endpoint));
    }

    private static Resource vector(final String file) throws Exception {
        try (InputStream in = ProfileValidatorTest.class.getResourceAsStream("/vectors/" + file)) {
            return (Resource) FHIR.newJsonParser().parseResource(new String(in.readAllBytes(), StandardCharsets.UTF_8));
        }
    }
}
