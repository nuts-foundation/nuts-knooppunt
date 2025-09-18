package com.example.fhirserver.pseudonyms;

public class GetTokenRequest {
    public Identifier identifier;
    public String receiver;
    public String scope;
    public String sender;

    public GetTokenRequest(Identifier identifier, String receiver, String scope, String sender) {
        this.identifier = identifier;
        this.receiver = receiver;
        this.scope = scope;
        this.sender = sender;
    }

    public String toJson() {
        return String.format(
            "{\"identifier\":%s,\"receiver\":\"%s\",\"scope\":\"%s\",\"sender\":\"%s\"}",
            identifier.toJson(), receiver, scope, sender
        );
    }
}

