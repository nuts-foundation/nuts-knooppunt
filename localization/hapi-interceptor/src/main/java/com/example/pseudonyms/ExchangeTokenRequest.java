package com.example.fhirserver.pseudonyms;

public class ExchangeTokenRequest {
    public String token;
    public String identifierType;
    public String scope;
    public String organisation;

    public ExchangeTokenRequest(String token, String identifierType, String scope, String organisation) {
        this.token = token;
        this.identifierType = identifierType;
        this.scope = scope;
        this.organisation = organisation;
    }

    public String toJson() {
        return String.format(
            "{\"token\":\"%s\",\"identifierType\":\"%s\",\"scope\":\"%s\",\"organisation\":\"%s\"}",
            token, identifierType, scope, organisation
        );
    }
}

