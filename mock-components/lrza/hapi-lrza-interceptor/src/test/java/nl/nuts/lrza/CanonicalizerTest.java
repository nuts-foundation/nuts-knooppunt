package nl.nuts.lrza;

import static org.junit.jupiter.api.Assertions.assertEquals;

import java.nio.charset.StandardCharsets;
import org.junit.jupiter.api.Test;

class CanonicalizerTest {

    /**
     * The expected output is what the Knooppunt's canonicalize (component/lrza/provenance.go) makes of the
     * same input: meta and narratives gone, a plain-string text kept, keys sorted, numbers and strings
     * serialized per RFC 8785.
     */
    @Test
    void matchesTheKnooppuntsCanonicalForm() throws Exception {
        final String input = "{\"resourceType\":\"Location\",\"id\":\"x\",\"meta\":{\"versionId\":\"2\",\"source\":\"s\"},"
                + "\"text\":{\"status\":\"generated\",\"div\":\"<div/>\"},\"name\":\"Zonnebloem \\u00e9 \\\"q\\\"\","
                + "\"type\":[{\"text\":\"plain text kept\",\"coding\":[{\"system\":\"http://x\",\"code\":\"A\"}]}],"
                + "\"position\":{\"longitude\":4.50,\"latitude\":-0.000001,\"altitude\":1e21},"
                + "\"contained\":[{\"resourceType\":\"Endpoint\",\"meta\":{\"x\":1},\"z\":true,\"a\":null}]}";

        final String expected = "{\"contained\":[{\"a\":null,\"resourceType\":\"Endpoint\",\"z\":true}],\"id\":\"x\","
                + "\"name\":\"Zonnebloem é \\\"q\\\"\",\"position\":{\"altitude\":1e+21,\"latitude\":-0.000001,\"longitude\":4.5},"
                + "\"resourceType\":\"Location\",\"type\":[{\"coding\":[{\"code\":\"A\",\"system\":\"http://x\"}],\"text\":\"plain text kept\"}]}";

        assertEquals(expected, new String(Canonicalizer.canonicalize(input), StandardCharsets.UTF_8));
    }
}
