package org.fiware.dataspace.it.components.model;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;
import lombok.AllArgsConstructor;
import lombok.Data;
import lombok.NoArgsConstructor;

import java.util.List;

/**
 * @author <a href="https://github.com/wistefan">Stefan Wiedemann</a>
 */
@NoArgsConstructor
@AllArgsConstructor
@Data
@JsonIgnoreProperties(ignoreUnknown = true)
public class VerifiablePresentation {

    private List<String> type = List.of("VerifiablePresentation");
    private List<String> verifiableCredential;
    private String holder;
    @JsonProperty("@context")
    private List<String> context = List.of("https://www.w3.org/2018/credentials/v1");

}
