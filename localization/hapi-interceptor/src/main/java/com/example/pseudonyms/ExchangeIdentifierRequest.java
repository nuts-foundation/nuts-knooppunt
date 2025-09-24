package com.example.fhirserver.pseudonyms;

public class ExchangeIdentifierRequest {
    public Identifier identifier;
    public String recipientIdentifierType;
    public String scope;
    public String organisation;

    public ExchangeIdentifierRequest(Identifier identifier, String recipientIdentifierType, String scope, String organisation) {
        this.identifier = identifier;
        this.recipientIdentifierType = recipientIdentifierType;
        this.scope = scope;
        this.organisation = organisation;
    }
}

