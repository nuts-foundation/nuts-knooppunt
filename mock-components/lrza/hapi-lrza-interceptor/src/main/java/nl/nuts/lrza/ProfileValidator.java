package nl.nuts.lrza;

import ca.uhn.fhir.context.FhirContext;
import ca.uhn.fhir.context.support.DefaultProfileValidationSupport;
import ca.uhn.fhir.context.support.IValidationSupport;
import ca.uhn.fhir.rest.server.exceptions.UnprocessableEntityException;
import ca.uhn.fhir.validation.FhirValidator;
import ca.uhn.fhir.validation.ResultSeverityEnum;
import ca.uhn.fhir.validation.SingleValidationMessage;
import ca.uhn.fhir.validation.ValidationOptions;
import ca.uhn.fhir.validation.ValidationResult;
import java.io.IOException;
import java.io.InputStream;
import java.nio.charset.StandardCharsets;
import java.nio.file.DirectoryStream;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import org.hl7.fhir.common.hapi.validation.support.CommonCodeSystemsTerminologyService;
import org.hl7.fhir.common.hapi.validation.support.InMemoryTerminologyServerValidationSupport;
import org.hl7.fhir.common.hapi.validation.support.PrePopulatedValidationSupport;
import org.hl7.fhir.common.hapi.validation.support.SnapshotGeneratingValidationSupport;
import org.hl7.fhir.common.hapi.validation.support.UnknownCodeSystemWarningValidationSupport;
import org.hl7.fhir.common.hapi.validation.support.ValidationSupportChain;
import org.hl7.fhir.common.hapi.validation.validator.FhirInstanceValidator;
import org.hl7.fhir.instance.model.api.IBaseResource;
import org.hl7.fhir.r4.model.OperationOutcome;
import org.hl7.fhir.utilities.npm.NpmPackage;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/**
 * Validates a write against the GF Care Services Directory profiles, as the LRZA does, answering 422
 * with every error found. The profile is the one for the resource type, whatever meta.profile claims.
 * The IG package and its dependencies are read from a directory of package tarballs, which the image
 * downloads at build time (mock-components/hapi/Dockerfile).
 *
 * Terminology is in-memory: value sets over code systems in the packages are checked, codes from a code
 * system none of them defines only give a warning.
 */
final class ProfileValidator {

    private static final Logger log = LoggerFactory.getLogger(ProfileValidator.class);

    static final String PROFILE_BASE = "http://fhir.generiekefuncties.nl/csd/StructureDefinition/";

    /** The profile each LRZA resource type is validated against. Provenance has none in the IG. */
    static final Map<String, String> PROFILES = Map.of(
            "Organization", PROFILE_BASE + "nl-gf-organization",
            "Location", PROFILE_BASE + "nl-gf-location",
            "HealthcareService", PROFILE_BASE + "nl-gf-healthcareservice",
            "Endpoint", PROFILE_BASE + "nl-gf-endpoint",
            "OrganizationAffiliation", PROFILE_BASE + "nl-gf-organizationaffiliation");

    private final FhirContext fhirContext;
    private final FhirValidator validator;

    private ProfileValidator(final FhirContext fhirContext, final FhirValidator validator) {
        this.fhirContext = fhirContext;
        this.validator = validator;
    }

    /** A validator for the packages (*.tgz) in a directory. */
    static ProfileValidator fromPackageDirectory(final FhirContext fhirContext, final Path directory) throws IOException {
        final PrePopulatedValidationSupport packages = new PrePopulatedValidationSupport(fhirContext);
        int count = 0;
        try (DirectoryStream<Path> tarballs = Files.newDirectoryStream(directory, "*.tgz")) {
            for (final Path tarball : tarballs) {
                count += load(fhirContext, packages, tarball);
            }
        }
        if (count == 0) {
            throw new IOException("no FHIR packages (*.tgz) in " + directory);
        }
        final UnknownCodeSystemWarningValidationSupport unknownCodeSystems = new UnknownCodeSystemWarningValidationSupport(fhirContext);
        unknownCodeSystems.setNonExistentCodeSystemSeverity(IValidationSupport.IssueSeverity.WARNING);
        final DefaultProfileValidationSupport core = new DefaultProfileValidationSupport(fhirContext);
        final ValidationSupportChain chain = new ValidationSupportChain(
                packages,
                core,
                new VersionedCanonicalValidationSupport(fhirContext, List.of(packages, core)),
                new CommonCodeSystemsTerminologyService(fhirContext),
                new InMemoryTerminologyServerValidationSupport(fhirContext),
                new SnapshotGeneratingValidationSupport(fhirContext),
                unknownCodeSystems);
        final FhirInstanceValidator instanceValidator = new FhirInstanceValidator(chain);
        instanceValidator.setAnyExtensionsAllowed(true);
        instanceValidator.setErrorForUnknownProfiles(true);
        final FhirValidator validator = fhirContext.newValidator();
        validator.registerValidatorModule(instanceValidator);
        log.info("LRZA profile validation: loaded {} conformance resources from {}", count, directory);
        return new ProfileValidator(fhirContext, validator);
    }

    /** Loads a package's conformance resources; returns how many. */
    private static int load(final FhirContext fhirContext, final PrePopulatedValidationSupport target, final Path tarball) throws IOException {
        final NpmPackage npmPackage;
        try (InputStream in = Files.newInputStream(tarball)) {
            npmPackage = NpmPackage.fromPackage(in);
        }
        final NpmPackage.NpmPackageFolder folder = npmPackage.getFolders().get("package");
        if (folder == null) {
            return 0;
        }
        int count = 0;
        for (final String file : folder.listFiles()) {
            final String name = file.toLowerCase(Locale.ROOT);
            if (!name.endsWith(".json") || name.equals("package.json") || name.startsWith(".")
                    || !(name.startsWith("structuredefinition-") || name.startsWith("valueset-") || name.startsWith("codesystem-"))) {
                continue;
            }
            final IBaseResource resource;
            try {
                resource = fhirContext.newJsonParser().parseResource(new String(folder.fetchFile(file), StandardCharsets.UTF_8));
            } catch (RuntimeException e) {
                log.warn("Skipping {} in {}: {}", file, tarball.getFileName(), e.getMessage());
                continue;
            }
            target.addResource(resource);
            count++;
        }
        log.info("Loaded {} conformance resources from {} {}", count, npmPackage.name(), npmPackage.version());
        return count;
    }

    /**
     * @throws UnprocessableEntityException with every error, when the resource does not conform
     */
    void validate(final IBaseResource resource) {
        final String type = fhirContext.getResourceType(resource);
        final ValidationOptions options = new ValidationOptions();
        final String profile = PROFILES.get(type);
        if (profile != null) {
            options.addProfile(profile);
        }
        final ValidationResult result = validator.validateWithResult(resource, options);
        final List<SingleValidationMessage> errors = new ArrayList<>();
        for (final SingleValidationMessage message : result.getMessages()) {
            if (message.getSeverity() == ResultSeverityEnum.ERROR || message.getSeverity() == ResultSeverityEnum.FATAL) {
                errors.add(message);
            }
        }
        if (errors.isEmpty()) {
            return;
        }
        final OperationOutcome outcome = new OperationOutcome();
        for (final SingleValidationMessage error : errors) {
            Outcomes.add(outcome, "processing", error.getLocationString(), error.getMessage());
        }
        throw new UnprocessableEntityException(fhirContext, outcome);
    }
}
