package nl.nuts.lrza;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ArrayNode;
import com.fasterxml.jackson.databind.node.ObjectNode;
import java.io.IOException;
import java.util.ArrayList;
import java.util.Iterator;
import java.util.List;
import java.util.Map;
import org.erdtman.jcs.JsonCanonicalizer;

/**
 * The canonical form a resource signature is computed over, {@code json#static}: every Resource.meta and
 * every narrative (an object-valued text) removed, recursively, then serialized per RFC 8785 (JCS). An
 * implementation of its own, so verifying a Knooppunt signature tests the two agree
 * (component/lrza/provenance.go).
 */
final class Canonicalizer {

    static final String STATIC_CANONICALIZATION = "http://hl7.org/fhir/canonicalization/json#static";

    private static final ObjectMapper MAPPER = new ObjectMapper();

    private Canonicalizer() {
    }

    static byte[] canonicalize(final String resourceJson) throws IOException {
        final JsonNode tree = MAPPER.readTree(resourceJson);
        strip(tree);
        return new JsonCanonicalizer(MAPPER.writeValueAsString(tree)).getEncodedUTF8();
    }

    private static void strip(final JsonNode node) {
        if (node instanceof final ObjectNode object) {
            final List<String> remove = new ArrayList<>();
            final Iterator<Map.Entry<String, JsonNode>> fields = object.fields();
            while (fields.hasNext()) {
                final Map.Entry<String, JsonNode> field = fields.next();
                if (("meta".equals(field.getKey()) || "text".equals(field.getKey())) && field.getValue().isObject()) {
                    remove.add(field.getKey());
                } else {
                    strip(field.getValue());
                }
            }
            object.remove(remove);
        } else if (node instanceof final ArrayNode array) {
            array.forEach(Canonicalizer::strip);
        }
    }
}
