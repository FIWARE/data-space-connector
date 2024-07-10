package org.fiware.dataspace.it.components.model;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;
import lombok.AllArgsConstructor;
import lombok.Data;
import lombok.NoArgsConstructor;

import java.util.List;
import java.util.Map;

/**
 * @author <a href="https://github.com/wistefan">Stefan Wiedemann</a>
 */
@NoArgsConstructor
@AllArgsConstructor
@Data
@JsonIgnoreProperties(ignoreUnknown = true)
public class CredentialOffer {

    @JsonProperty("grants")
    private Map<String, Grant> grants;

    @JsonProperty("credential_issuer")
    private String credentialIssuer;

    @JsonProperty("credential_configuration_ids")
    private List<String> credentialConfigurationIds;
}
