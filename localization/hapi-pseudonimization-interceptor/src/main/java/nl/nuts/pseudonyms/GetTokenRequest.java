package nl.nuts.pseudonyms;

import lombok.AllArgsConstructor;
import lombok.Data;

@Data
@AllArgsConstructor
public class GetTokenRequest {
    private Identifier identifier;
    private String receiver;
    private String scope;
    private String sender;
}

