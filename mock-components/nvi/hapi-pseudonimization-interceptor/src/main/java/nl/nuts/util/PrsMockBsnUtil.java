package nl.nuts.util;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ObjectNode;
import java.io.IOException;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;
import nl.nuts.PseudonimizationExecutionException;

/**
 * Token handling backed by the mock PRS's sandbox helper (mock-components/prs): the tokens the
 * Knooppunt registers are real blinded values under a JWE, which this HAPI-based NVI cannot
 * de-blind itself, so it asks the mock, which holds the recipient key, to do it. The real NVI
 * does this itself; the routes used here exist only on the mock.
 */
public class PrsMockBsnUtil extends BsnUtil {

    private static final ObjectMapper MAPPER = new ObjectMapper();
    private static final Duration TIMEOUT = Duration.ofSeconds(10);

    private final HttpClient client;
    private final URI finalizeUri;
    private final URI tokenizeUri;

    public PrsMockBsnUtil(final String baseUrl) {
        this(baseUrl, HttpClient.newBuilder().connectTimeout(TIMEOUT).build());
    }

    PrsMockBsnUtil(final String baseUrl, final HttpClient client) {
        final String base = baseUrl.endsWith("/") ? baseUrl.substring(0, baseUrl.length() - 1) : baseUrl;
        this.client = client;
        this.finalizeUri = URI.create(base + "/sandbox/finalize");
        this.tokenizeUri = URI.create(base + "/sandbox/tokenize");
    }

    /**
     * Turns a Knooppunt identifier value into the stable pseudonym it de-blinds to, as hex.
     */
    @Override
    public String transportTokenToPseudonym(final String token) throws PseudonimizationExecutionException {
        final ObjectNode request = MAPPER.createObjectNode();
        request.put("token", token);
        return post(finalizeUri, request, "pseudonym");
    }

    /**
     * Turns a pseudonym back into a fresh identifier value for the audience, so what this NVI
     * hands out on read has the shape the Knooppunt produces.
     */
    @Override
    public String pseudonymToTransportToken(final String pseudonym, final String audience)
            throws PseudonimizationExecutionException {
        final ObjectNode request = MAPPER.createObjectNode();
        request.put("pseudonym", pseudonym);
        request.put("audience", audience);
        return post(tokenizeUri, request, "token");
    }

    private String post(final URI uri, final ObjectNode body, final String member)
            throws PseudonimizationExecutionException {
        final HttpResponse<String> response;
        try {
            final HttpRequest request = HttpRequest.newBuilder(uri)
                    .timeout(TIMEOUT)
                    .header("Content-Type", "application/json")
                    .POST(HttpRequest.BodyPublishers.ofString(MAPPER.writeValueAsString(body)))
                    .build();
            response = client.send(request, HttpResponse.BodyHandlers.ofString());
        } catch (IOException e) {
            throw new PseudonimizationExecutionException(
                    String.format("mock PRS %s unreachable: %s", uri.getPath(), e.getMessage()));
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
            throw new PseudonimizationExecutionException("interrupted while calling the mock PRS");
        }
        if (response.statusCode() != 200) {
            throw new PseudonimizationExecutionException(String.format("mock PRS %s answered %d: %s",
                    uri.getPath(), response.statusCode(), errorText(response.body())));
        }
        final JsonNode answer = parseJson(response.body());
        final JsonNode value = answer == null ? null : answer.get(member);
        if (value == null || !value.isTextual() || value.asText().isEmpty()) {
            throw new PseudonimizationExecutionException(
                    String.format("mock PRS %s answered without %s", uri.getPath(), member));
        }
        return value.asText();
    }

    /**
     * The "error" member of a JSON error body, or the body itself when it is not JSON, as a
     * proxy in front of the mock would produce; bounded so a log line stays readable.
     */
    private static String errorText(final String body) {
        final JsonNode parsed = parseJson(body);
        final String text = parsed != null && parsed.path("error").isTextual()
                ? parsed.path("error").asText() : body == null ? "" : body.trim();
        return text.length() > 200 ? text.substring(0, 200) : text;
    }

    private static JsonNode parseJson(final String body) {
        if (body == null || body.isBlank()) {
            return null;
        }
        try {
            final JsonNode parsed = MAPPER.readTree(body);
            return parsed == null || parsed.isMissingNode() ? null : parsed;
        } catch (JsonProcessingException e) {
            return null;
        }
    }
}
