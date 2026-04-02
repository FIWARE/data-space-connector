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
    public static final String MARKETPLACE_API_ADDRESS = "http://fancy-marketplace.127.0.0.1.nip.io";

    /**
     * Base URL of the contract management service.
     * Handles contract lifecycle and notification flows between marketplace and provider.
     */
    public static final String CONTRACT_MANAGEMENT_ADDRESS = "http://contract-management.127.0.0.1.nip.io";
}
