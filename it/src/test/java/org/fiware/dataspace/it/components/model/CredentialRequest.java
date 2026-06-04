package org.fiware.dataspace.it.components.model;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;
import lombok.AllArgsConstructor;
import lombok.Data;
import lombok.NoArgsConstructor;

/**
 * @author <a href="https://github.com/wistefan">Stefan Wiedemann</a>
 */
@NoArgsConstructor
@AllArgsConstructor
@Data
@JsonIgnoreProperties(ignoreUnknown = true)
@com.fasterxml.jackson.annotation.JsonInclude(com.fasterxml.jackson.annotation.JsonInclude.Include.NON_NULL)
public class CredentialRequest {

    /**
     * KC 26.4+ (OID4VCI draft 14+) requires either `credential_configuration_id`
     * or `credential_identifier` in the body of /credential. The legacy `format`
     * field is no longer accepted as input; the format is implied by the chosen
     * configuration id.
     */
    @JsonProperty("credential_configuration_id")
    private String credentialConfigurationId;

    @JsonProperty("credential_identifier")
    private String credentialIdentifier;
}
