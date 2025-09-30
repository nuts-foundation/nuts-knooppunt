package nl.nuts.pseudonyms;

import lombok.AllArgsConstructor;
import lombok.Data;

@Data
@AllArgsConstructor
public class ExchangeTokenRequest {
    private String token;
    private String identifierType;
    private String scope;
    private String organisation;
}

