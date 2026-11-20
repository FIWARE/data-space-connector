package org.fiware.dataspace.it.components;

import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.JsonNode;
import io.cucumber.java.After;
import io.cucumber.java.Before;
import io.cucumber.java.en.Given;
import io.cucumber.java.en.Then;
import io.cucumber.java.en.When;
import lombok.extern.slf4j.Slf4j;
import okhttp3.Request;
import okhttp3.Response;
import org.apache.http.HttpStatus;
import org.awaitility.Awaitility;

import java.time.Instant;
import java.util.ArrayList;
import java.util.HashSet;
import java.util.List;
import java.util.Objects;
import java.util.Optional;
import java.util.Set;
import java.util.UUID;
import java.util.stream.Collectors;

import static org.fiware.dataspace.it.components.DataSpaceEventingEnvironment.*;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertTrue;
import static org.junit.jupiter.api.Assertions.fail;

/**
 * Data space eventing (doc/EVENTING.md): a participant removed at the trust anchor is announced by the publisher and
 * acted on by the provider's contract-management. The trust state the deployment set up is checked, never written,
 * except where the {@code @After} restores what a scenario removed.
 */
@Slf4j
public class DataSpaceEventingStepDefinitions extends StepDefintions {

    private static final String ORDERS_PATH = "/tmf-api/productOrderingManagement/v4/productOrder";
    private static final String JOB_SUCCEEDED = "1";
    private static final String PUBLISHER_REPLICAS = "--replicas=1";

    /** The registration Job's membership grant of the consumer, read before any fixture of this run wrote trust. */
    private static ProviderTilClient.Grant jobGrant;
    private static List<ProviderTilClient.Credential> jobGrantCredentials;
    private static boolean jobGrantVerified;

    private final ContractManagementManagementClient publisher =
            new ContractManagementManagementClient(PUBLISHER_NAMESPACE, CM_DEPLOYMENT);
    private final ContractManagementManagementClient providerCm =
            new ContractManagementManagementClient(PROVIDER_NAMESPACE, CM_DEPLOYMENT);
    private final ContractManagementManagementClient consumerCm =
            new ContractManagementManagementClient(CONSUMER_NAMESPACE, CM_DEPLOYMENT);
    private final ProviderTilClient providerTil = new ProviderTilClient(PROVIDER_TIL_ADDRESS);
    private final TrustAnchorClient trustAnchor =
            new TrustAnchorClient(TRUST_ANCHOR_TIL_ADDRESS, TrustAnchorEnvironment.TIR_ADDRESS, publisher);

    private List<ProviderTilClient.Grant> grantsBeforeRemoval;
    private Instant removedSince;
    private ContractManagementManagementClient.ParticipantReport outcome;

    /** Checks the deployment's starting state; it fails instead of repairing, so a broken deploy is not masked. */
    @Before("@eventing")
    public void checkStartingState() throws Exception {
        assertTrue(trustAnchor.isListed(CONSUMER_DID), "The consumer should be registered at the trust anchor.");
        List<ProviderTilClient.Grant> grants = providerTil.scopes(CONSUMER_DID);
        Optional<ProviderTilClient.Grant> membership = membershipGrant(grants);
        assertTrue(membership.isPresent(), "registration Job grant missing: " + grants);
        if (!jobGrantVerified) {
            recordJobGrant(grants, membership.get());
        }
        assertEquals(Optional.of(MODE_REPORT),
                providerCm.publisherReport(PUBLISHER_ID).map(ContractManagementManagementClient.PublisherReport::mode),
                "The provider should start in report mode.");
    }

    // the Job's grant is only isolable while the issuer holds nothing else, i.e. before any order of this run
    private static void recordJobGrant(List<ProviderTilClient.Grant> grants, ProviderTilClient.Grant membership)
            throws Exception {
        // under helm-install the Job is a hook-succeeded hook: Helm deletes it on success and keeps it on failure,
        // so absent means succeeded. Presence is read by name, as .status.succeeded is empty on a failed Job too.
        boolean jobPresent = !KubernetesHelper.kubectl("get", "job", "-n", PROVIDER_NAMESPACE,
                PROVIDER_REGISTRATION_JOB, "--ignore-not-found", "-o", "name").isEmpty();
        if (jobPresent) {
            assertEquals(JOB_SUCCEEDED, KubernetesHelper.kubectl("get", "job", "-n", PROVIDER_NAMESPACE,
                    PROVIDER_REGISTRATION_JOB, "-o", "jsonpath={.status.succeeded}"),
                    "The provider's registration Job should have succeeded.");
        }
        List<ProviderTilClient.Credential> credentials =
                new ProviderTilClient(PROVIDER_TIL_ADDRESS).credentials(CONSUMER_DID);
        if (grants.size() != 1 || membership.credentialCount() != credentials.size()) {
            fail(String.format("cannot isolate the registration Job grant: grants %s, %d credentials.",
                    grants, credentials.size()));
        }
        jobGrant = membership;
        jobGrantCredentials = credentials;
        jobGrantVerified = true;
    }

    @Given("The data space publisher delivers events to both participants.")
    public void publisherDeliversToBothParticipants() throws Exception {
        String probeDid = String.format(PROBE_DID_TEMPLATE, UUID.randomUUID());
        Instant since = Instant.now();
        publisher.awaitSubscriptions(probeDid, PARTICIPANT_SUBSCRIPTIONS, SUBSCRIPTION_RECOVERY_TIMEOUT);
        // subscriptions only counts registrations; the report entries prove both gateways accepted the delivery
        providerCm.awaitOutcome(PUBLISHER_ID, probeDid, TRIGGER_EVENT, since, DELIVERY_TIMEOUT);
        consumerCm.awaitOutcome(PUBLISHER_ID, probeDid, TRIGGER_EVENT, since, DELIVERY_TIMEOUT);
    }

    @Then("the provider's registration Job wrote the membership grant for Fancy Marketplace.")
    public void registrationJobWroteMembershipGrant() throws Exception {
        assertTrue(jobGrantVerified, "The Job grant should have been read before any fixture wrote trust.");
        assertEquals(PUBLISHER_ID, jobGrant.scope(), "The membership grant should be scoped to the publisher.");
        assertEquals(SCOPE_TYPE_MEMBERSHIP, jobGrant.scopeType(), "The grant should be a membership grant.");
        Set<String> types = jobGrantCredentials.stream()
                .map(ProviderTilClient.Credential::credentialsType)
                .collect(Collectors.toSet());
        assertEquals(CONSUMER_MEMBERSHIP_CREDENTIAL_TYPES, types,
                "The grant should hold the credentials of the consumer's registration.issuer entry.");
        assertTrue(membershipGrant(providerTil.scopes(CONSUMER_DID)).isPresent(),
                "The membership grant should still be at the provider.");
    }

    @Then("the provider's TIL holds an order-scoped grant for Fancy Marketplace.")
    public void providerHoldsOrderGrant() throws Exception {
        String orderId = singleOrderId();
        // CM writes order grants untyped, and only after the order call has returned
        Awaitility.await(String.format("The provider should hold an untyped grant scoped to order %s.", orderId))
                .atMost(ORDER_GRANT_TIMEOUT)
                .pollInterval(POLL_INTERVAL)
                .ignoreExceptions()
                .until(() -> providerTil.scopes(CONSUMER_DID).stream()
                        .anyMatch(g -> orderId.equals(g.scope()) && g.scopeType() == null));
    }

    @When("the operator removes Fancy Marketplace from the trust anchor.")
    public void removeConsumer() throws Exception {
        grantsBeforeRemoval = providerTil.scopes(CONSUMER_DID);
        removedSince = trustAnchor.removeParticipant(CONSUMER_DID);
    }

    @Then("the provider reports the removal of Fancy Marketplace in report mode without changes.")
    public void providerReportsRemoval() {
        outcome = providerCm.awaitOutcome(PUBLISHER_ID, CONSUMER_DID, TRIGGER_EVENT, removedSince, REMOVAL_TIMEOUT);
        assertEquals(MODE_REPORT, outcome.mode(), "The removal should be handled in report mode.");
        assertEquals(ACTION_REPORTED, outcome.action(), "Report mode only reports.");
        assertFalse(outcome.applied(), "Report mode writes nothing.");
    }

    @Then("the report lists the membership and the order grant Fancy Marketplace still holds.")
    public void reportListsHeldGrants() throws Exception {
        assertNotNull(outcome, "The removal should have been reported first.");
        assertEquals(Boolean.TRUE, outcome.attributable(), "Every held grant should be attributable.");
        assertNotNull(outcome.held(), "The report should have read what the participant holds.");
        String orderId = singleOrderId();
        assertTrue(outcome.held().stream().anyMatch(g -> PUBLISHER_ID.equals(g.scope())
                        && SCOPE_TYPE_MEMBERSHIP.equals(g.scopeType())),
                "The membership grant should be held: " + outcome.held());
        assertTrue(outcome.held().stream().anyMatch(g -> orderId.equals(g.scope()) && g.scopeType() == null),
                "The order grant should be held: " + outcome.held());
    }

    @Then("the provider's TIL still holds the same grants for Fancy Marketplace.")
    public void providerTilUnchanged() throws Exception {
        assertEquals(new HashSet<>(grantsBeforeRemoval), new HashSet<>(providerTil.scopes(CONSUMER_DID)),
                "Report mode should not change the provider's TIL.");
    }

    /**
     * Restores everything a scenario may have changed; every restore runs even if an earlier one fails, and the hook
     * fails at the end with all errors.
     */
    @After(value = "@eventing", order = RESTORE_HOOK_ORDER)
    public void restore() {
        List<String> errors = new ArrayList<>();
        attempt(errors, "restore the consumer at the trust anchor", () -> trustAnchor.restoreParticipant(CONSUMER_DID));
        attempt(errors, "scale the publisher back up", this::ensurePublisherRunning);
        attempt(errors, "restore the membership grant", this::restoreMembershipGrant);
        attempt(errors, "remove test grants of the consumer", this::removeTestGrants);
        attempt(errors, "wait for both subscriptions", () -> publisher.awaitSubscriptions(
                String.format(PROBE_DID_TEMPLATE, UUID.randomUUID()), PARTICIPANT_SUBSCRIPTIONS,
                SUBSCRIPTION_RECOVERY_TIMEOUT));
        publisher.close();
        providerCm.close();
        consumerCm.close();
        assertTrue(errors.isEmpty(), "Restoring the eventing deployment failed: " + errors);
    }

    // a no-op unless a scenario scaled it down; the subscription wait below covers its start-up
    private void ensurePublisherRunning() throws Exception {
        KubernetesHelper.kubectl("scale", "deploy/" + CM_DEPLOYMENT, "-n", PUBLISHER_NAMESPACE, PUBLISHER_REPLICAS);
    }

    // the only place that writes the Job's grant; dataspace mode may have removed it or the whole issuer
    private void restoreMembershipGrant() throws Exception {
        if (jobGrantVerified && membershipGrant(providerTil.scopes(CONSUMER_DID)).isEmpty()) {
            providerTil.putGrant(CONSUMER_DID, jobGrant.scope(), jobGrant.scopeType(), jobGrantCredentials);
        }
    }

    // each grant is deleted with its own scopeType: none for the untyped order grants
    private void removeTestGrants() throws Exception {
        for (ProviderTilClient.Grant grant : providerTil.scopes(CONSUMER_DID)) {
            if (!isJobGrant(grant)) {
                providerTil.removeGrant(CONSUMER_DID, grant.scope(), grant.scopeType());
            }
        }
    }

    private static boolean isJobGrant(ProviderTilClient.Grant grant) {
        return PUBLISHER_ID.equals(grant.scope()) && SCOPE_TYPE_MEMBERSHIP.equals(grant.scopeType());
    }

    private static Optional<ProviderTilClient.Grant> membershipGrant(List<ProviderTilClient.Grant> grants) {
        return grants.stream().filter(DataSpaceEventingStepDefinitions::isJobGrant).findFirst();
    }

    // the @After of every scenario deletes the TMF orders, so the one order is this scenario's
    private static String singleOrderId() throws Exception {
        Request request = new Request.Builder().get().url(MPOperationsEnvironment.TMF_DIRECT_ADDRESS + ORDERS_PATH).build();
        try (Response response = HTTP_CLIENT.newCall(request).execute()) {
            assertEquals(HttpStatus.SC_OK, response.code(), "The orders should have been returned.");
            List<JsonNode> orders = OBJECT_MAPPER.readValue(Objects.requireNonNull(response.body()).string(),
                    new TypeReference<>() {
                    });
            assertEquals(1, orders.size(), "Exactly one order should exist.");
            String id = orders.get(0).path("id").asText(null);
            assertNotNull(id, "The order should have an id.");
            return id;
        }
    }

    private static void attempt(List<String> errors, String what, ThrowingRunnable action) {
        try {
            action.run();
        } catch (Throwable e) {
            log.warn("Failed to {}.", what, e);
            errors.add(what + ": " + e.getMessage());
        }
    }

    @FunctionalInterface
    private interface ThrowingRunnable {
        void run() throws Exception;
    }
}
