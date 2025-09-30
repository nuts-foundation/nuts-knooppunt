package nl.nuts.pseudonyms;

import lombok.AllArgsConstructor;
import lombok.Data;
import lombok.NoArgsConstructor;

@Data
@NoArgsConstructor
@AllArgsConstructor
public class Identifier {
    private String value;
    private String type;
}

