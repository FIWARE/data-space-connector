package org.fiware.dataspace.it.components;

/**
 * The deployment of the reference profiles (charts/*&#47;examples), filled in with the values of
 * k3s/reference/*.env. The public endpoints are reached through the squid proxy; the internal
 * ones, which the profiles do not publish, through {@code kubectl port-forward}.
 */
public abstract class ReferenceEnvironment {

    public static final String PROVIDER_DID = "did:web:did-provider.127.0.0.1.nip.io";
    public static final String CONSUMER_DID = "did:web:did-consumer.127.0.0.1.nip.io";

    public static final String TIR_ADDRESS = "https://tir.127.0.0.1.nip.io";
    public static final String CONSUMER_KEYCLOAK_ADDRESS = "https://keycloak-consumer.127.0.0.1.nip.io";
    public static final String DATA_SERVICE_ADDRESS = "https://data-provider.127.0.0.1.nip.io";
    public static final String TMF_ADDRESS = "https://tmf-provider.127.0.0.1.nip.io";
    public static final String MARKETPLACE_ADDRESS = "https://marketplace-provider.127.0.0.1.nip.io";

    /** The realm both profiles are filled in with: the one {@link Wallet} requests credentials from. */
    public static final String REALM = "test-realm";
    /** The test user of k3s/reference/consumer-local.yaml. */
    public static final String CONSUMER_USER = "employee";
    public static final String CONSUMER_USER_PASSWORD = "test";

    public static final String TRUST_ANCHOR_NAMESPACE = "trust-anchor";
    public static final String PROVIDER_NAMESPACE = "provider";
    /** The trust anchor's trusted issuers list, whose list API is not published. */
    public static final String TRUST_ANCHOR_TIL_SERVICE = "tir";
    public static final int TRUST_ANCHOR_TIL_PORT = 8080;
    public static final String PAP_SERVICE = "odrl-pap";
    public static final int PAP_PORT = 8080;
    public static final String SCORPIO_SERVICE = "data-service-scorpio";
    public static final int SCORPIO_PORT = 9090;

    /** Local ports of the port-forwards, apart from those of the other scenarios. */
    public static final int TRUST_ANCHOR_TIL_LOCAL_PORT = 18091;
    public static final int PAP_LOCAL_PORT = 18092;
    public static final int SCORPIO_LOCAL_PORT = 18093;
}
