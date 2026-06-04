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
public class TokenResponse {

    @JsonProperty("access_token")
    private String accessToken;
    @JsonProperty("token_type")
    private String tokenType;
    @JsonProperty("expires_in")
    private long expiresIn;
    @JsonProperty("authorization_details")
    private List<AuthorizationDetail> authorizationDetails;

    @NoArgsConstructor
    @AllArgsConstructor
    @Data
    @JsonIgnoreProperties(ignoreUnknown = true)
    public static class AuthorizationDetail {
        @JsonProperty("type")
        private String type;
        @JsonProperty("credential_configuration_id")
        private String credentialConfigurationId;
        @JsonProperty("credential_identifiers")
        private List<String> credentialIdentifiers;
    }
}
