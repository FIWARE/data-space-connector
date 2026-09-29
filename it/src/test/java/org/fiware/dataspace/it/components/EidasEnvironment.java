package org.fiware.dataspace.it.components;

import java.security.KeyManagementException;
import java.security.NoSuchAlgorithmException;

/**
 * The consumer of the eIDAS deployment ({@code mvn clean deploy -Plocal,eidas}).
 *
 * <p>It is the same Keycloak as in the standard local deployment, but issuing under a
 * {@code did:elsi} identity with an eIDAS certificate: credentials carry the certificate chain in
 * the {@code x5c} JOSE header, and the provider's VCVerifier validates that chain against a trust
 * list instead of trusting the issuer DID alone.
 *
 * @author <a href="https://github.com/wistefan">Stefan Wiedemann</a>
 */
public abstract class EidasEnvironment {

	/**
	 * The DID of the issuing organization. The identifier after {@code did:elsi:} has to match the
	 * {@code organizationIdentifier} (OID 2.5.4.97) of the certificate the credential is signed with -
	 * that binding is what the verifier checks in addition to the PKIX chain.
	 */
	public static final String CONSUMER_DID = "did:elsi:VATDE-1234567";

	/** The organization identifier carried by the signing certificate's subject DN. */
	public static final String ORGANIZATION_IDENTIFIER = "VATDE-1234567";

	/** The base URL of the consumer's Keycloak identity provider. */
	public static final String CONSUMER_KEYCLOAK_ADDRESS = "https://keycloak-consumer.127.0.0.1.nip.io";

	private static final String TEST_REALM = "test-realm";

	/** The username of the test user the eIDAS realm declares. */
	public static final String TEST_USER_NAME = "test-user";

	private static final String TEST_USER_PASSWORD = "test";

	/**
	 * Returns an access token to be used with the consumer's Keycloak.
	 *
	 * @param user the username to log in as
	 * @return the access token of that user
	 */
	public static String loginToConsumerKeycloak(String user) throws NoSuchAlgorithmException, KeyManagementException {
		KeycloakHelper consumerKeycloak = new KeycloakHelper(TEST_REALM, CONSUMER_KEYCLOAK_ADDRESS);
		return consumerKeycloak.getUserToken(user, TEST_USER_PASSWORD);
	}
}
