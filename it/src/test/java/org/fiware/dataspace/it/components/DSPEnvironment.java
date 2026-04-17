package org.fiware.dataspace.it.components;

/**
 * Environment constants for the Dataspace Protocol (DSP) deployment profile.
 * These endpoints are available when deploying with {@code mvn clean deploy -Plocal,dsp}.
 *
 * @see <a href="../../../../../../doc/DSP_INTEGRATION.md">DSP Integration Guide</a>
 */
public abstract class DSPEnvironment {

    // --- DSP Management API endpoints ---

    /**
     * Base URL of the DSP DCP management API.
     * Used for catalog requests, contract negotiations, and transfer processes via DCP authentication.
     */
    public static final String DCP_MANAGEMENT_API_ADDRESS = "http://dsp-dcp-management.127.0.0.1.nip.io";

    /**
     * Base URL of the DSP OID4VC management API.
     * Used for catalog requests, contract negotiations, and transfer processes via OID4VC authentication.
     */
    public static final String OID4VC_MANAGEMENT_API_ADDRESS = "http://dsp-oid4vc-management.127.0.0.1.nip.io";

    // --- DSP Provider endpoints ---

    /**
     * Base URL of the DCP-authenticated provider data service.
     * The counter-party address for DCP-based DSP protocol interactions.
     */
    public static final String DCP_PROVIDER_ADDRESS = "http://dcp-mp-operations.127.0.0.1.nip.io";

    /**
     * Base URL of the OID4VC-authenticated provider data service.
     * The counter-party address for OID4VC-based DSP protocol interactions.
     */
    public static final String OID4VC_PROVIDER_ADDRESS = "http://dsp-mp-operations.127.0.0.1.nip.io";

    // --- IdentityHub Management endpoints ---

    /**
     * Base URL of the consumer's IdentityHub management API.
     * Used for participant registration and credential management for the consumer.
     */
    public static final String IDENTITYHUB_MANAGEMENT_CONSUMER_ADDRESS = "http://identityhub-management-fancy-marketplace.127.0.0.1.nip.io";

    /**
     * Base URL of the provider's IdentityHub management API.
     * Used for participant registration and credential management for the provider.
     */
    public static final String IDENTITYHUB_MANAGEMENT_PROVIDER_ADDRESS = "http://identityhub-management-mp-operations.127.0.0.1.nip.io";

    // --- IdentityHub DID endpoints ---

    /**
     * Base URL of the consumer's IdentityHub (public DID endpoint).
     * Used to verify DID document availability at {@code .well-known/did.json}.
     */
    public static final String IDENTITYHUB_CONSUMER_ADDRESS = "http://identityhub-fancy-marketplace.127.0.0.1.nip.io";

    /**
     * Base URL of the provider's IdentityHub (public DID endpoint).
     * Used to verify DID document availability at {@code .well-known/did.json}.
     */
    public static final String IDENTITYHUB_PROVIDER_ADDRESS = "http://identityhub-mp-operations.127.0.0.1.nip.io";

    // --- Kubernetes secret configuration ---

    /**
     * Name of the cert-manager TLS secret containing the signing key for mp-operations.org
     */
    public static final String PROVIDER_SIGNING_KEY_SECRET_NAME = "mp-operations.org-tls";
    /**
     * Name of the cert-manager TLS secret containing the signing key for fancy-marketplace.biz
     */
    public static final String CONSUMER_SIGNING_KEY_SECRET_NAME = "fancy-marketplace.biz-tls";

    /**
     * Kubernetes namespace for the consumer deployment.
     */
    public static final String CONSUMER_NAMESPACE = "consumer";

    /**
     * Kubernetes namespace for the provider deployment.
     */
    public static final String PROVIDER_NAMESPACE = "provider";

    // --- Vault endpoints ---

    /**
     * Base URL of the consumer's Vault instance.
     * Used for inserting private keys needed by the IdentityHub.
     */
    public static final String VAULT_CONSUMER_ADDRESS = "https://vault-fancy-marketplace.127.0.0.1.nip.io";

    /**
     * Base URL of the provider's Vault instance.
     * Used for inserting private keys needed by the IdentityHub.
     */
    public static final String VAULT_PROVIDER_ADDRESS = "https://vault-mp-operations.127.0.0.1.nip.io";
}
