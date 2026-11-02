package org.fiware.dataspace.it.components;

/**
 * Environment constants for the consent-management deployment profile.
 *
 * <p>These endpoints exist when the data space is deployed with consent management enabled
 * ({@code mvn clean deploy -Pconsent}). The consent components are split over two namespaces: the
 * consent-manager and its verifier run at the authority ({@code trust-anchor}), while the
 * consent-filter plugin, the OwnerResolver and a provider-local consent-facade run at the provider.
 *
 * @see <a href="../../../../../../doc/CONSENT_MANAGEMENT.md">Consent Management Guide</a>
 */
public abstract class ConsentEnvironment {

    // --- authority endpoints, reached through the ingress + squid proxy ------------------------

    /**
     * The consent-manager's {@code /v1} API; every call carries a token of the verifier below.
     */
    public static final String CONSENT_MANAGER_API = "https://consent-manager.dataspace-authority.org/v1";

    /**
     * The authority verifier service that issues the consent-manager's tokens (audience
     * {@code consent-manager}): scope {@code openid} for data subjects, {@code participant} for
     * participants.
     */
    public static final String CONSENT_MANAGER_VERIFIER_ADDRESS =
            "https://verifier.dataspace-authority.org/services/consent-manager";

    /**
     * Public base URL of the authority consent-facade, which serves the participant self-descriptions.
     */
    public static final String CONSENT_FACADE_ADDRESS = "https://consent-facade.dataspace-authority.org";

    /**
     * Template of a bilateral contract URI: the contract service the consent-manager projects its
     * privacy notices from, plus the TM Forum agreement the contract stands for.
     *
     * <p>The base has to be the one the deployment configures
     * ({@code consent-manager.config.contractServiceUrl} in
     * {@code k3s/consent-trust-anchor-components.yaml}): the consent-manager fetches the contract
     * from there under the URI's last path segment and stores the URI verbatim on the notice.
     */
    public static final String CONTRACT_URI_TEMPLATE =
            "http://consent-facade.trust-anchor.svc.cluster.local:8080/bilaterals/default~%s";

    // --- endpoints that are not published and are reached via kubectl port-forward -------------

    /**
     * Namespace of the provider deployment.
     */
    public static final String PROVIDER_NAMESPACE = "provider";

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
     * The audience of every consent-manager token.
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
    public static final String OPERATOR_PROFILE_ENTITY_ID = "urn:ngsi-ld:OperatorProfile:consent-it";

    /**
     * Type of that entity - the type the OPA policy permits reading.
     */
    public static final String OPERATOR_PROFILE_ENTITY_TYPE = "OperatorProfile";
}
