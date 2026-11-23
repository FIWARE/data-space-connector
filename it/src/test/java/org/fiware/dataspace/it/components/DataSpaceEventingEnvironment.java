package org.fiware.dataspace.it.components;

import java.time.Duration;
import java.util.Set;

/**
 * Names, addresses and timeouts of the {@code eventing} deployment (k3s/eventing-*.yaml).
 * The timeouts follow the k3s values: watcher 10 s × 2 confirmations, lease PT2M, retry backoff ≤ PT20S.
 */
public abstract class DataSpaceEventingEnvironment {

    /** The publisher: the contract-management of the authority release. */
    public static final String PUBLISHER_ID = "did:web:dataspace-authority.org";
    public static final String CONSUMER_DID = FancyMarketplaceEnvironment.CONSUMER_DID;
    /** A simulated participant the provider's keep list protects (scenario 5); written and removed by the IT. */
    public static final String KEPT_MEMBER_DID = "did:web:kept-member.org";
    /** A second data space's publisher (scenario 7); only simulated by a grant the IT writes at the provider. */
    public static final String OTHER_DATASPACE_ID = "did:web:other-dataspace.org";

    public static final String PARTICIPANT_REMOVED_TYPE = "org.fiware.dataspace.participant.removed";

    /** The trust anchor's writable TIL; the TIR at {@link TrustAnchorEnvironment#TIR_ADDRESS} is read-only. */
    public static final String TRUST_ANCHOR_TIL_ADDRESS = "https://til.127.0.0.1.nip.io";
    public static final String PROVIDER_TIL_ADDRESS = MPOperationsEnvironment.TIL_DIRECT_ADDRESS;

    public static final String PUBLISHER_NAMESPACE = "trust-anchor";
    public static final String PROVIDER_NAMESPACE = "provider";
    public static final String CONSUMER_NAMESPACE = "consumer";
    /** The Deployment name of contract-management in all three namespaces. */
    public static final String CM_DEPLOYMENT = "contract-management";
    /** contract-management's management port; its Service does not expose it. */
    public static final int CM_MANAGEMENT_PORT = 9090;
    public static final String EVENTS_CONFIG_MAP = "contract-management-dataspace-events";
    public static final String PROVIDER_REGISTRATION_JOB = "provider-til-registration-job";

    public static final String MODE_REPORT = "report";
    public static final String MODE_DATASPACE = "dataspace";
    /** A participant entry's mode for a DID on keep, whatever the publisher's mode (CM RemovalMode.KEEP_TAG_VALUE). */
    public static final String MODE_KEEP = "keep";
    public static final String TRIGGER_EVENT = "event";
    public static final String TRIGGER_RECONCILIATION = "reconciliation";
    public static final String ACTION_REPORTED = "reported";
    public static final String ACTION_REVOKED_MEMBERSHIP = "revoked_membership";
    public static final String ACTION_KEPT = "kept";
    public static final String MEMBERSHIP_CREDENTIAL_TYPE = "MembershipCredential";
    public static final String SCOPE_TYPE_MEMBERSHIP = "dataspace-membership";
    /** Credential types of the consumer's entry in the provider's registration.issuer. */
    public static final Set<String> CONSUMER_MEMBERSHIP_CREDENTIAL_TYPES =
            Set.of("LegalPersonCredential", "UserCredential", "MembershipCredential");

    /** Distinct watcher cycles that must end after an instant before one of them surely read the TIR later. */
    public static final int WATCHER_CYCLES_TO_SEE = 2;
    public static final Duration WATCHER_SEEN_TIMEOUT = Duration.ofSeconds(90);
    /** A removal reaches the participants after confirmations × interval (~30 s). */
    public static final Duration REMOVAL_TIMEOUT = Duration.ofMinutes(3);
    /** kubelet ConfigMap sync plus CM's file watch. */
    public static final Duration MODE_SWITCH_TIMEOUT = Duration.ofMinutes(3);
    /** One lease renewal (≤ 60 s) plus one retry backoff (≤ 20 s) plus a publisher start-up. */
    public static final Duration SUBSCRIPTION_RECOVERY_TIMEOUT = Duration.ofMinutes(3);
    public static final Duration DELIVERY_TIMEOUT = Duration.ofSeconds(30);
    /** The pod's termination grace period plus margin. */
    public static final Duration PUBLISHER_STOP_TIMEOUT = Duration.ofMinutes(2);
    /** A new CM pod passes its readiness probe after 31 s at the earliest. */
    public static final Duration CM_RESTART_TIMEOUT = Duration.ofMinutes(5);
    /** {@code kubectl rollout status} gives up after this and is retried, as kubectl calls time out after 30 s. */
    public static final String ROLLOUT_STATUS_TIMEOUT = "--timeout=20s";
    /** The start-up sweep runs reconcile.initial-delay (PT20S) after start-up; counted from Ready, so a margin only. */
    public static final Duration RECONCILIATION_TIMEOUT = Duration.ofMinutes(2);
    /** CM writes the order grant to the TIL asynchronously, after the order call has returned. */
    public static final Duration ORDER_GRANT_TIMEOUT = Duration.ofSeconds(30);
    /** The TIR reads the trust anchor's TIL; a fresh registration is listed almost at once. */
    public static final Duration REGISTRATION_TIMEOUT = Duration.ofSeconds(30);
    public static final Duration POLL_INTERVAL = Duration.ofSeconds(2);
    /** Both participants subscribe to the publisher. */
    public static final int PARTICIPANT_SUBSCRIPTIONS = 2;
    public static final String PROBE_DID_TEMPLATE = "did:web:probe-%s.example";

    /**
     * {@code @After} orders (Cucumber runs higher first): the TMF/PAP cleanup deletes the order, so CM's own
     * revocation of the order grant starts before the restore reads the TIL.
     */
    public static final int FIXTURE_CLEANUP_HOOK_ORDER = 20000;
    public static final int RESTORE_HOOK_ORDER = 10000;

    private DataSpaceEventingEnvironment() {
        // constants only
    }
}
