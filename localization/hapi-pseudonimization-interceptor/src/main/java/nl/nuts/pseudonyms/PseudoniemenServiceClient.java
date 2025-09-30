package nl.nuts.pseudonyms;

import com.fasterxml.jackson.databind.ObjectMapper;
import lombok.extern.slf4j.Slf4j;

import java.io.IOException;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;

@Slf4j
public class PseudoniemenServiceClient {
    private static final String CONTENT_TYPE_JSON = "application/json";
    private static final String ENDPOINT_GET_TOKEN = "/getToken";
    private static final String ENDPOINT_EXCHANGE_TOKEN = "/exchangeToken";
    private static final String ENDPOINT_EXCHANGE_IDENTIFIER = "/exchangeIdentifier";

    private final String baseUrl;
    private final HttpClient httpClient;
    private final ObjectMapper objectMapper;

    public PseudoniemenServiceClient(final String baseUrl) {
        this.baseUrl = baseUrl.endsWith("/") ? baseUrl.substring(0, baseUrl.length() - 1) : baseUrl;
        this.httpClient = HttpClient.newHttpClient();
        this.objectMapper = new ObjectMapper();
    }

    public GetTokenResponse getToken(final GetTokenRequest request) throws IOException, InterruptedException {
        log.debug("Requesting token for identifier: {}", request.getIdentifier().getValue());
        return post(ENDPOINT_GET_TOKEN, request, GetTokenResponse.class);
    }

    public ExchangeTokenResponse exchangeToken(final ExchangeTokenRequest request) throws IOException, InterruptedException {
        log.debug("Exchanging token");
        return post(ENDPOINT_EXCHANGE_TOKEN, request, ExchangeTokenResponse.class);
    }

    public String exchangeIdentifier(final ExchangeIdentifierRequest request) throws IOException, InterruptedException {
        log.debug("Exchanging identifier: {}", request.getIdentifier().getValue());
        final String requestBody = objectMapper.writeValueAsString(request);
        final HttpResponse<String> response = sendRequest(ENDPOINT_EXCHANGE_IDENTIFIER, requestBody);
        return response.body();
    }

    private <T, R> R post(final String endpoint, final T request, final Class<R> responseType) throws IOException, InterruptedException {
        final String requestBody = objectMapper.writeValueAsString(request);
        final HttpResponse<String> response = sendRequest(endpoint, requestBody);
        return objectMapper.readValue(response.body(), responseType);
    }

    private HttpResponse<String> sendRequest(final String endpoint, final String requestBody) throws IOException, InterruptedException {
        final HttpRequest httpRequest = HttpRequest.newBuilder()
                .uri(URI.create(baseUrl + endpoint))
                .header("Content-Type", CONTENT_TYPE_JSON)
                .POST(HttpRequest.BodyPublishers.ofString(requestBody))
                .build();

        return httpClient.send(httpRequest, HttpResponse.BodyHandlers.ofString());
    }
}
