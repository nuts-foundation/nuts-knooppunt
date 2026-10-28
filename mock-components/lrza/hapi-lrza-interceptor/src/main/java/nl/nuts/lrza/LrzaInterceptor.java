package nl.nuts.lrza;

import ca.uhn.fhir.context.FhirContext;
import ca.uhn.fhir.interceptor.api.Hook;
import ca.uhn.fhir.interceptor.api.Interceptor;
import ca.uhn.fhir.interceptor.api.Pointcut;
import ca.uhn.fhir.jpa.api.dao.DaoRegistry;
import ca.uhn.fhir.rest.api.RestOperationTypeEnum;
import ca.uhn.fhir.rest.api.server.RequestDetails;
import ca.uhn.fhir.rest.server.exceptions.BaseServerResponseException;
import ca.uhn.fhir.rest.server.exceptions.PreconditionFailedException;
import ca.uhn.fhir.rest.server.exceptions.ResourceVersionConflictException;
import ca.uhn.fhir.util.HapiExtensions;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ObjectNode;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.Date;
import java.util.Optional;
import java.util.concurrent.CompletableFuture;
import org.hl7.fhir.instance.model.api.IBaseBundle;
import org.hl7.fhir.instance.model.api.IBaseResource;
import org.hl7.fhir.r4.model.Bundle;
import org.hl7.fhir.r4.model.DomainResource;
import org.hl7.fhir.r4.model.OperationOutcome;
import org.hl7.fhir.r4.model.Provenance;
import org.hl7.fhir.r4.model.Resource;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.stereotype.Component;

/**
 * Makes the LRZA partition of this HAPI server behave like the LRZA (Landelijk Register Zorgaanbieders)
 * towards its writers, so the Knooppunt's registration proxy is tested against the rules it meets in
 * production:
 * <ul>
 *   <li>writes are authorized by the care provider's mandate ({@link WriteRules});</li>
 *   <li>a DELETE is refused (withdraw by status) and an update needs If-Match ({@link RequestRules}),
 *       and a stale If-Match is answered with 412 rather than HAPI's 409;</li>
 *   <li>resources are validated against the GF Care Services Directory profiles ({@link ProfileValidator});</li>
 *   <li>Provenance signatures are verified ({@link ProvenanceVerifier});</li>
 *   <li>a test can arm 429 (with Retry-After) or 5xx answers ({@link FaultInjector}).</li>
 * </ul>
 * The caller is not identified by mTLS but by configuration ({@link Caller}). A request with
 * {@code X-Mock-Operator: true} skips every rule: seeding, and tests that set up or inspect the
 * directory directly. Other partitions are left alone.
 */
@Component
@Interceptor
public class LrzaInterceptor {

    private static final Logger log = LoggerFactory.getLogger(LrzaInterceptor.class);

    static final String CALLER_HEADER = "X-Mock-Caller";
    static final String OPERATOR_HEADER = "X-Mock-Operator";
    static final String MOCK_FAIL_OPERATION = "$mock-fail";

    private static final String TENANT = env("LRZA_TENANT", "lrza-root-directory");
    private static final String DEFAULT_CALLER = env("LRZA_CALLER", null);
    private static final String PACKAGES_DIR = env("LRZA_PACKAGES_DIR", "/app/lrza-packages");
    private static final boolean PROFILE_VALIDATION = !"false".equalsIgnoreCase(env("LRZA_PROFILE_VALIDATION", "true"));

    private static final ObjectMapper MAPPER = new ObjectMapper();

    private final DaoRegistry daoRegistry;
    private final FhirContext fhirContext;
    private final FaultInjector faults = new FaultInjector();
    /** Loading the packages takes a while, so it runs in the background while HAPI starts. */
    private final CompletableFuture<Optional<ProfileValidator>> validator;

    public LrzaInterceptor(final DaoRegistry daoRegistry, final FhirContext fhirContext) {
        this.daoRegistry = daoRegistry;
        this.fhirContext = fhirContext;
        this.validator = CompletableFuture.supplyAsync(this::loadValidator);
        log.info("Mock LRZA rules apply to partition {}; default caller: {}", TENANT,
                DEFAULT_CALLER == null ? "none (set LRZA_CALLER)" : DEFAULT_CALLER);
    }

    private Optional<ProfileValidator> loadValidator() {
        if (!PROFILE_VALIDATION) {
            log.info("LRZA profile validation is off (LRZA_PROFILE_VALIDATION=false)");
            return Optional.empty();
        }
        final Path directory = Path.of(PACKAGES_DIR);
        if (!Files.isDirectory(directory)) {
            log.warn("LRZA profile validation is off: no package directory at {} (LRZA_PACKAGES_DIR)", directory);
            return Optional.empty();
        }
        try {
            return Optional.of(ProfileValidator.fromPackageDirectory(fhirContext, directory));
        } catch (IOException | RuntimeException e) {
            log.error("LRZA profile validation is off: loading the packages in {} failed", directory, e);
            return Optional.empty();
        }
    }

    /**
     * The mock's own operation ($mock-fail) and armed failures, answered before HAPI looks at the request:
     * a failure stands in for the whole request, as a rate limiter or load balancer in front of the LRZA
     * would.
     */
    @Hook(Pointcut.SERVER_INCOMING_REQUEST_PRE_PROCESSED)
    public boolean preProcessed(final HttpServletRequest request, final HttpServletResponse response) throws IOException {
        final String[] path = path(request);
        if (path.length == 0 || !TENANT.equals(path[0])) {
            return true;
        }
        final boolean operator = isOperator(request.getHeader(OPERATOR_HEADER));
        if (path.length == 2 && MOCK_FAIL_OPERATION.equals(path[1])) {
            mockFail(request, response, operator);
            return false;
        }
        if (operator) {
            return true;
        }
        final Optional<FaultInjector.Fault> fault = faults.take(request.getMethod());
        if (fault.isEmpty()) {
            return true;
        }
        log.info("Mock LRZA: failing {} {} with armed status {}", request.getMethod(), request.getRequestURI(), fault.get().status());
        if (fault.get().retryAfter() != null) {
            response.setHeader("Retry-After", String.valueOf(fault.get().retryAfter()));
        }
        writeOutcome(response, fault.get().status(), fault.get().status() == 429 ? "throttled" : "transient",
                "Mock LRZA: failure armed through " + MOCK_FAIL_OPERATION);
        return false;
    }

    /**
     * {@code POST [base]/$mock-fail?status=429&count=2&retryAfter=1&method=PUT} arms a failure (method
     * optional: every method), {@code GET} shows what is armed, {@code DELETE} disarms. Operator only.
     */
    private void mockFail(final HttpServletRequest request, final HttpServletResponse response, final boolean operator) throws IOException {
        if (!operator) {
            writeOutcome(response, 403, "forbidden", MOCK_FAIL_OPERATION + " needs " + OPERATOR_HEADER + ": true");
            return;
        }
        switch (request.getMethod()) {
            case "POST" -> {
                try {
                    faults.arm(Integer.parseInt(required(request, "status")), Integer.parseInt(required(request, "count")),
                            request.getParameter("retryAfter") == null ? null : Integer.parseInt(request.getParameter("retryAfter")),
                            request.getParameter("method"));
                } catch (IllegalArgumentException e) {
                    writeOutcome(response, 400, "invalid", e.getMessage());
                    return;
                }
            }
            case "DELETE" -> faults.disarm();
            case "GET" -> {
            }
            default -> {
                writeOutcome(response, 405, "not-supported", MOCK_FAIL_OPERATION + " supports GET, POST and DELETE");
                return;
            }
        }
        final ObjectNode state = MAPPER.createObjectNode();
        faults.current().ifPresentOrElse(fault -> {
            state.put("status", fault.status());
            state.put("remaining", fault.remaining());
            if (fault.retryAfter() != null) {
                state.put("retryAfter", fault.retryAfter());
            }
            if (fault.method() != null) {
                state.put("method", fault.method());
            }
        }, () -> state.put("remaining", 0));
        response.setStatus(200);
        response.setContentType("application/json");
        response.setCharacterEncoding(StandardCharsets.UTF_8.name());
        response.getWriter().write(MAPPER.writeValueAsString(state));
    }

    private static String required(final HttpServletRequest request, final String parameter) {
        final String value = request.getParameter(parameter);
        if (value == null || value.isBlank()) {
            throw new IllegalArgumentException("query parameter " + parameter + " is required");
        }
        return value;
    }

    /** A DELETE, or an update without If-Match. */
    @Hook(Pointcut.SERVER_INCOMING_REQUEST_PRE_HANDLED)
    public void preHandled(final RequestDetails requestDetails, final RestOperationTypeEnum operation) {
        if (applies(requestDetails)) {
            RequestRules.check(operation, requestDetails.getHeader("If-Match"));
        }
    }

    /** The same, per entry of a transaction. */
    @Hook(Pointcut.STORAGE_TRANSACTION_PROCESSING)
    public void transaction(final IBaseBundle bundle, final RequestDetails requestDetails) {
        if (applies(requestDetails) && bundle instanceof final Bundle r4Bundle) {
            RequestRules.check(r4Bundle);
        }
    }

    @Hook(Pointcut.STORAGE_PRESTORAGE_RESOURCE_CREATED)
    public void created(final IBaseResource resource, final RequestDetails requestDetails) {
        checkWrite(requestDetails, resource, null);
    }

    @Hook(Pointcut.STORAGE_PRESTORAGE_RESOURCE_UPDATED)
    public void updated(final IBaseResource oldResource, final IBaseResource newResource, final RequestDetails requestDetails) {
        checkWrite(requestDetails, newResource, oldResource);
    }

    private void checkWrite(final RequestDetails requestDetails, final IBaseResource newVersion, final IBaseResource oldVersion) {
        if (!applies(requestDetails) || isPlaceholder(newVersion) || !(newVersion instanceof final Resource resource)) {
            return;
        }
        final Caller caller = Caller.resolve(requestDetails.getHeader(CALLER_HEADER), DEFAULT_CALLER);
        final ResourceLookup lookup = new DaoResourceLookup(daoRegistry, requestDetails);
        final OwnerResolver owners = new OwnerResolver(lookup);
        final WriteRules rules = new WriteRules(fhirContext, lookup, owners, new Mandates(lookup, owners, Date::new));
        rules.authorize(caller, resource, isPlaceholder(oldVersion) ? null : (Resource) oldVersion);
        if (resource instanceof final Provenance provenance) {
            new ProvenanceVerifier(fhirContext, rules, owners).verify(provenance);
        }
        validator.join().ifPresent(profiles -> profiles.validate(resource));
    }

    /**
     * HAPI answers a stale If-Match with 409; the LRZA, as the IG prescribes, with 412.
     */
    @Hook(Pointcut.SERVER_PRE_PROCESS_OUTGOING_EXCEPTION)
    public BaseServerResponseException outgoingException(final RequestDetails requestDetails, final Throwable exception) {
        if (!applies(requestDetails)) {
            return null;
        }
        final boolean withIfMatch = requestDetails.getHeader("If-Match") != null
                || requestDetails.getRestOperationType() == RestOperationTypeEnum.TRANSACTION;
        for (Throwable cause = exception; cause != null; cause = cause.getCause()) {
            if (cause instanceof final ResourceVersionConflictException conflict && withIfMatch) {
                return new PreconditionFailedException(conflict.getMessage(), conflict.getOperationOutcome());
            }
        }
        return null;
    }

    /** A request to the LRZA partition by someone other than the operator. */
    private static boolean applies(final RequestDetails requestDetails) {
        return requestDetails != null && TENANT.equals(requestDetails.getTenantId())
                && !isOperator(requestDetails.getHeader(OPERATOR_HEADER));
    }

    static boolean isOperator(final String header) {
        return "true".equalsIgnoreCase(header);
    }

    /** A resource HAPI created as the target of a reference that didn't resolve yet. */
    private static boolean isPlaceholder(final IBaseResource resource) {
        return resource instanceof final DomainResource domainResource
                && domainResource.hasExtension(HapiExtensions.EXT_RESOURCE_PLACEHOLDER);
    }

    /** The path below the FHIR servlet, e.g. [lrza-root-directory, Endpoint, 123]. */
    private static String[] path(final HttpServletRequest request) {
        String path = request.getPathInfo();
        if (path == null) {
            final String uri = request.getRequestURI();
            final String prefix = request.getContextPath() + request.getServletPath();
            path = uri.startsWith(prefix) ? uri.substring(prefix.length()) : uri;
        }
        final String trimmed = path.replaceAll("^/+|/+$", "");
        return trimmed.isEmpty() ? new String[0] : trimmed.split("/");
    }

    private void writeOutcome(final HttpServletResponse response, final int status, final String code, final String message) throws IOException {
        final OperationOutcome outcome = Outcomes.error(code, null, message);
        response.setStatus(status);
        response.setContentType("application/fhir+json");
        response.setCharacterEncoding(StandardCharsets.UTF_8.name());
        response.getWriter().write(fhirContext.newJsonParser().encodeResourceToString(outcome));
    }

    private static String env(final String name, final String defaultValue) {
        final String value = System.getenv(name);
        return value == null || value.isBlank() ? defaultValue : value;
    }
}
