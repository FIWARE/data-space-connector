package org.fiware.dataspace.it.components;

import java.security.KeyManagementException;
import java.security.NoSuchAlgorithmException;

/**
 * @author <a href="https://github.com/wistefan">Stefan Wiedemann</a>
 */
public abstract class FancyMarketplaceEnvironment {

	/** The DID of the consumer organization (Fancy Marketplace Co.). */
	public static final String CONSUMER_DID = "did:web:fancy-marketplace.biz";

	/** The base URL of the consumer's Keycloak identity provider. */
	public static final String CONSUMER_KEYCLOAK_ADDRESS = "https://keycloak-consumer.127.0.0.1.nip.io";

	public static final String CONSUMER_TPP_ADDRESS = "http://rainbow-consumer.127.0.0.1.nip.io:8080/api/v1/callbacks";

	private static final String TEST_REALM = "test-realm";

	/** The username for the standard employee test user at the consumer organization. */
	public static final String TEST_USER_NAME = "employee";

	/** The username for the operator test user at the consumer organization. */
	public static final String OPERATOR_USER_NAME = "operator";

	/** The username for the representative test user at the consumer organization, used in marketplace flows. */
	public static final String REPRESENTATIVE_USER_NAME = "representative";

	private static final String TEST_USER_PASSWORD = "test";

	/**
	 * Returns an access token to be used with Keycloak.
	 */
	public static String loginToConsumerKeycloak(String user) throws NoSuchAlgorithmException, KeyManagementException {
		KeycloakHelper consumerKeycloak = new KeycloakHelper(TEST_REALM, CONSUMER_KEYCLOAK_ADDRESS);
		return consumerKeycloak.getUserToken(user, TEST_USER_PASSWORD);
	}

}
