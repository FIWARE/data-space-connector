package org.fiware.dataspace.it.components.model;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;
import lombok.AllArgsConstructor;
import lombok.Data;
import lombok.NoArgsConstructor;

import java.util.List;

/**
 * OID Configuration, that maps only those fields we need for the pre-authorized flow
 *
 * @author <a href="https://github.com/wistefan">Stefan Wiedemann</a>
 */
@NoArgsConstructor
@AllArgsConstructor
@Data
@JsonIgnoreProperties(ignoreUnknown = true)
public class OpenIdConfiguration {

    @JsonProperty("token_endpoint")
    private String tokenEndpoint;

    @JsonProperty("authorization_endpoint")
    private String authorizationEndpoint;

    @JsonProperty("jwks_uri")
    private String jwksUri;

    @JsonProperty("scopes_supported")
    private List<String> scopesSupported;

    @JsonProperty("response_mode_supported")
    private List<String> responseModeSupported;

    @JsonProperty("grant_types_supported")
    private List<String> grantTypesSupported;

}
