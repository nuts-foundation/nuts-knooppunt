package nl.nuts.pseudonyms;

import lombok.AllArgsConstructor;
import lombok.Data;

@Data
@AllArgsConstructor
public class ExchangeIdentifierRequest {
    private Identifier identifier;
    private String recipientIdentifierType;
    private String scope;
    private String organisation;
}

