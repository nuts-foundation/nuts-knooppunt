package nl.nuts.util;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import com.sun.net.httpserver.HttpExchange;
import com.sun.net.httpserver.HttpServer;
import java.io.IOException;
import java.net.InetSocketAddress;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.List;
import nl.nuts.PseudonimizationExecutionException;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

class PrsMockBsnUtilTest {

    private HttpServer server;
    private final List<String> requests = new ArrayList<>();
    private int status = 200;
    private String answer = "{}";

    @BeforeEach
    void start() throws IOException {
        server = HttpServer.create(new InetSocketAddress("127.0.0.1", 0), 0);
        server.createContext("/sandbox/", this::handle);
        server.start();
    }

    @AfterEach
    void stop() {
        server.stop(0);
    }

    private void handle(final HttpExchange exchange) throws IOException {
        requests.add(exchange.getRequestMethod() + " " + exchange.getRequestURI().getPath() + " "
                + new String(exchange.getRequestBody().readAllBytes(), StandardCharsets.UTF_8));
        final byte[] body = answer.getBytes(StandardCharsets.UTF_8);
        exchange.getResponseHeaders().add("Content-Type", "application/json");
        exchange.sendResponseHeaders(status, body.length);
        exchange.getResponseBody().write(body);
        exchange.close();
    }

    private PrsMockBsnUtil util() {
        return new PrsMockBsnUtil("http://127.0.0.1:" + server.getAddress().getPort() + "/");
    }

    @Test
    void transportTokenToPseudonym_postsTheTokenToFinalize() {
        answer = "{\"pseudonym\":\"4cfe4459f0cc8417\"}";

        assertEquals("4cfe4459f0cc8417", util().transportTokenToPseudonym("eyJibGluZF9mYWN0b3IiOiIuLi4ifQ"));

        assertEquals(1, requests.size());
        assertEquals("POST /sandbox/finalize {\"token\":\"eyJibGluZF9mYWN0b3IiOiIuLi4ifQ\"}", requests.get(0));
    }

    @Test
    void pseudonymToTransportToken_postsPseudonymAndAudienceToTokenize() {
        answer = "{\"token\":\"eyJ0b2tlbiI6Ii4uLiJ9\"}";

        assertEquals("eyJ0b2tlbiI6Ii4uLiJ9", util().pseudonymToTransportToken("4cfe4459f0cc8417", "90000901"));

        assertEquals(1, requests.size());
        assertEquals("POST /sandbox/tokenize {\"pseudonym\":\"4cfe4459f0cc8417\",\"audience\":\"90000901\"}",
                requests.get(0));
    }

    @Test
    void errorAnswersBecomeExceptions() {
        status = 400;
        answer = "{\"error\":\"token is not base64url\"}";

        final PseudonimizationExecutionException exception = assertThrows(
                PseudonimizationExecutionException.class, () -> util().transportTokenToPseudonym("!!!"));

        assertTrue(exception.getMessage().contains("400"), exception.getMessage());
        assertTrue(exception.getMessage().contains("token is not base64url"), exception.getMessage());
    }

    @Test
    void plainTextErrorAnswersReportStatusAndBody() {
        status = 502;
        answer = "<html>502 Bad Gateway</html>";

        final PseudonimizationExecutionException exception = assertThrows(
                PseudonimizationExecutionException.class, () -> util().transportTokenToPseudonym("token"));

        assertTrue(exception.getMessage().contains("502"), exception.getMessage());
        assertTrue(exception.getMessage().contains("502 Bad Gateway"), exception.getMessage());
    }

    @Test
    void emptySuccessAnswersBecomeExceptions() {
        answer = "";

        final PseudonimizationExecutionException exception = assertThrows(
                PseudonimizationExecutionException.class, () -> util().transportTokenToPseudonym("token"));

        assertTrue(exception.getMessage().contains("without pseudonym"), exception.getMessage());
    }

    @Test
    void nonJsonSuccessAnswersBecomeExceptions() {
        answer = "not json";

        assertThrows(PseudonimizationExecutionException.class,
                () -> util().pseudonymToTransportToken("4cfe4459f0cc8417", "90000901"));
    }

    @Test
    void answersWithoutTheMemberBecomeExceptions() {
        answer = "{\"unexpected\":true}";

        assertThrows(PseudonimizationExecutionException.class,
                () -> util().pseudonymToTransportToken("4cfe4459f0cc8417", "90000901"));
    }

    @Test
    void unreachableMockBecomesException() {
        final PrsMockBsnUtil unreachable = new PrsMockBsnUtil("http://127.0.0.1:1");

        assertThrows(PseudonimizationExecutionException.class,
                () -> unreachable.transportTokenToPseudonym("token"));
    }
}
