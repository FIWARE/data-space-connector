package org.fiware.dataspace.it.components;

/**
 * Environment constants for the Central Marketplace deployment profile.
 * These endpoints are available when deploying with {@code mvn clean deploy -Plocal,central}.
 *
 * @see <a href="../../../../../../doc/CENTRAL_MARKETPLACE.md">Central Marketplace Integration Guide</a>
 */
public abstract class CentralMarketplaceEnvironment {

    /**
     * Base URL of the central marketplace TMForum API.
     * Used for product offering management, ordering, and organization registration.
     */
    public static final String MARKETPLACE_API_ADDRESS = "https://fancy-marketplace.127.0.0.1.nip.io";

    /**
     * Base URL of the contract management service.
     * Handles contract lifecycle and notification flows between marketplace and provider.
     */
    public static final String CONTRACT_MANAGEMENT_ADDRESS = "https://contract-management.127.0.0.1.nip.io:443";

    /**
     * Base URL of the consumer's PAP (Policy Administration Point) for the central marketplace.
     * Used to register TMForum access policies that restrict marketplace interactions to authorized roles.
     */
    public static final String CONSUMER_PAP_ADDRESS = "https://pap-consumer.127.0.0.1.nip.io";

    /** Credential configuration ID for the SD-JWT format UserCredential used in central marketplace flows. */
    public static final String USER_SD_CREDENTIAL = "user-sd";

    /** The SD-JWT credential format string (KC 26.4+ value, see Format enum). */
    public static final String SD_JWT_FORMAT = "dc+sd-jwt";

    /** The credential configuration ID for the provider's LegalPersonCredential. */
    public static final String PROVIDER_USER_CREDENTIAL = "user-credential";

    /** The provider employee username for credential issuance. */
    public static final String PROVIDER_EMPLOYEE_USER = "employee";

    /** The consumer employee username for SD-JWT credential issuance. */
    public static final String CONSUMER_SD_EMPLOYEE_USER = "employee@consumer.org";
}
