package org.fiware.dataspace.it.components;

/**
 * Environment constants for the consent-management deployment profile.
 *
 * <p>These endpoints exist when the data space is deployed with consent management enabled
 * ({@code mvn clean deploy -Pconsent}). The consent components are split over two namespaces: the
 * consent-manager and the authority APISIX facade run at the authority ({@code trust-anchor}), while
 * the consent-filter plugin, the OwnerResolver and a provider-local consent-facade run at the
 * provider.
 *
 * @see <a href="../../../../../../doc/CONSENT_MANAGEMENT.md">Consent Management Guide</a>
 */
public abstract class ConsentEnvironment {

    // --- authority endpoints, reached through the ingress + squid proxy ------------------------

    /**
     * Participant-authenticated consent-manager API, behind the authority APISIX facade.
     *
     * <p>The routes under {@code /consent-manager/*} require a participant access token; the facade
     * validates it and injects the shared consent key server-side. The routes are host-scoped to the
     * ingress name, so they are only reachable under this host - not through a port-forward.
     */
    public static final String CONSENT_MANAGER_FACADE_ADDRESS =
            "https://consent-manager.dataspace-authority.org/consent-manager/v1";

    /**
     * Subject routes of the consent-manager, behind the same facade.
     *
     * <p>Allow-listed: no participant authentication and no consent-key injection, so a data subject
     * can sign up and grant with its own OID4VP token.
     */
    public static final String CONSENT_USER_ADDRESS =
            "https://consent-manager.dataspace-authority.org/consent-user/v1";

    /**
     * The authority verifier service a data subject authenticates against to obtain the access token
     * the consent-manager verifies (audience {@code consent-manager}).
     */
    public static final String CONSENT_MANAGER_VERIFIER_ADDRESS =
            "https://verifier.dataspace-authority.org/services/consent-manager";

    /**
     * Public base URL of the authority consent-facade - the id space participant self-descriptions
     * and catalog data resources are built from. It MUST equal the facade's own {@code selfUrl}: the
     * consent-manager matches participants on that exact string.
     */
    public static final String CONSENT_FACADE_ADDRESS = "https://consent-facade.dataspace-authority.org";

    // --- endpoints that are not published and are reached via kubectl port-forward -------------

    /**
     * Namespace of the authority deployment.
     */
    public static final String TRUST_ANCHOR_NAMESPACE = "trust-anchor";

    /**
     * Namespace of the provider deployment.
     */
    public static final String PROVIDER_NAMESPACE = "provider";

    /**
     * Service and port of the consent-manager itself. Participant onboarding
     * ({@code POST /participants}) is an unauthenticated authority action and therefore not exposed
     * through the participant-authenticated facade.
     */
    public static final String CONSENT_MANAGER_SERVICE = "consent-manager";
    public static final int CONSENT_MANAGER_PORT = 3000;
    public static final int CONSENT_MANAGER_LOCAL_PORT = 13000;

    /**
     * Service and port of the provider-local consent-facade. Its {@code /internal/tokens} endpoint
     * mints the provider's participant token over OID4VP - the same way the consent-filter plugin
     * obtains one. Its NetworkPolicy admits only the APISIX pods and the OwnerResolver, so it is
     * reached through a port-forward (which originates from the node, not from a pod).
     */
    public static final String CONSENT_FACADE_SERVICE = "consent-facade";
    public static final int CONSENT_FACADE_PORT = 8080;
    public static final int CONSENT_FACADE_LOCAL_PORT = 18081;

    /**
     * Path of the facade's internal token service.
     */
    public static final String INTERNAL_TOKENS_PATH = "/internal/tokens";

    // --- the consent-enforced data endpoint ---------------------------------------------------

    /**
     * The provider data service behind the consent-filter plugin. A request here passes two gates:
     * OPA authorizes it on the presented credential, then the plugin gates it on the data subject's
     * consent.
     */
    public static final String CONSENT_ENFORCED_DATA_ADDRESS = "https://mp-data-service-consent.127.0.0.1.nip.io";

    // --- fixtures ----------------------------------------------------------------------------

    /**
     * The audience the participant token is minted for.
     */
    public static final String CONSENT_MANAGER_AUDIENCE = "consent-manager";

    /**
     * Name of the TM Forum organization backing the provider participant.
     */
    public static final String PROVIDER_ORGANIZATION_NAME = "Consent IT Provider";

    /**
     * Name of the TM Forum organization backing the consumer participant.
     */
    public static final String CONSUMER_ORGANIZATION_NAME = "Consent IT Consumer";

    /**
     * The personal data entity the consent is exercised on.
     */
    public static final String PERSONAL_PROFILE_ENTITY_ID = "urn:ngsi-ld:PersonalProfile:consent-it";

    /**
     * Type of that entity - the type the OPA policy permits reading.
     */
    public static final String PERSONAL_PROFILE_ENTITY_TYPE = "PersonalProfile";
}
