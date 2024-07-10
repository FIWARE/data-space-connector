package org.fiware.dataspace.it.components;

/**
 * @author <a href="https://github.com/wistefan">Stefan Wiedemann</a>
 */
public abstract class FancyMarketplaceEnvironment {
    public static final String DID_CONSUMER_ADDRESS = "http://did-consumer.127.0.0.1.nip.io:8080";
    public static final String CONSUMER_KEYCLOAK_ADDRESS = "http://keycloak-consumer.127.0.0.1.nip.io:8080";

    public static final String OIDC_WELL_KNOWN_PATH = "/.well-known/openid-configuration";
    private static final String TEST_REALM = "test-realm";
    private static final String TEST_USER_NAME = "test-user";
    private static final String TEST_USER_PASSWORD = "test";

    /**
     * Returns an access token to be used with Keycloak.
     */
    public static String loginToConsumerKeycloak() {
        KeycloakHelper consumerKeycloak = new KeycloakHelper(TEST_REALM, CONSUMER_KEYCLOAK_ADDRESS);
        return consumerKeycloak.getUserToken(TEST_USER_NAME, TEST_USER_PASSWORD);
    }
}
