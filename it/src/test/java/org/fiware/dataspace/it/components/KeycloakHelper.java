package org.fiware.dataspace.it.components;

import lombok.RequiredArgsConstructor;
import org.keycloak.admin.client.Keycloak;
import org.keycloak.admin.client.KeycloakBuilder;
import org.keycloak.admin.client.token.TokenManager;
import org.keycloak.representations.idm.ClientRepresentation;

import java.util.List;

/**
 * @author <a href="https://github.com/wistefan">Stefan Wiedemann</a>
 */
@RequiredArgsConstructor
public class KeycloakHelper {
    private final String realm;
    private final String address;

    public String getUserToken(String username, String password) {

        TokenManager tokenManager = KeycloakBuilder.builder()
                .username(username)
                .password(password)
                .realm(realm)
                .grantType("password")
                .clientId("admin-cli")
                .serverUrl(address)
                .build()
                .tokenManager();
        return tokenManager.getAccessToken().getToken();
    }
}
