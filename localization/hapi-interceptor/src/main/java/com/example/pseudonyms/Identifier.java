package com.example.fhirserver.pseudonyms;
import com.fasterxml.jackson.annotation.JsonProperty;

public class Identifier {
    @JsonProperty("value")
    public String value;
    @JsonProperty("type")
    public String type; // BSN or ORGANISATION_PSEUDO

    public Identifier() {}

    public Identifier(String value, String type) {
        this.value = value;
        this.type = type;
    }

  
    @JsonProperty("value")
    public String getValue() {
      return value;
    }

    @JsonProperty("value")
    public void setValue(String value) {
      this.value = value;
    }

    @JsonProperty("type")
    public String getType() {
      return type;
    }

    @JsonProperty("type")
    public void setType(String type) {
      this.type = type;
    }

    public String toJson() {
        return String.format("{\"value\":\"%s\",\"type\":\"%s\"}", value, type);
    }
}

