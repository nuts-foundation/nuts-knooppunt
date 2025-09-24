package com.example.fhirserver.pseudonyms;
import com.fasterxml.jackson.annotation.JsonProperty;

public class Identifier {
    public String value;
    public String type; // BSN or ORGANISATION_PSEUDO

    public Identifier() {}

    public Identifier(String value, String type) {
        this.value = value;
        this.type = type;
    }
}

