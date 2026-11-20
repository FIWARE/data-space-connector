package org.fiware.dataspace.it.components;

import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import lombok.extern.slf4j.Slf4j;
import okhttp3.MediaType;
import okhttp3.OkHttpClient;
import okhttp3.Request;
import okhttp3.RequestBody;
import okhttp3.Response;
import org.awaitility.Awaitility;

import java.io.IOException;
import java.time.Duration;
import java.time.Instant;
import java.util.ArrayList;
import java.util.HashSet;
import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.Set;
import java.util.concurrent.atomic.AtomicReference;

import static org.fiware.dataspace.it.components.DataSpaceEventingEnvironment.CM_MANAGEMENT_PORT;
import static org.fiware.dataspace.it.components.DataSpaceEventingEnvironment.POLL_INTERVAL;
import static org.fiware.dataspace.it.components.DataSpaceEventingEnvironment.WATCHER_CYCLES_TO_SEE;
import static org.fiware.dataspace.it.components.DataSpaceEventingEnvironment.WATCHER_SEEN_TIMEOUT;

/**
 * Client for the management port of one contract-management Deployment: the removal report, manual publish and health.
 * Every request opens its own port-forward, so it survives pod restarts; the {@code await*} methods retry until timeout.
 */
@Slf4j
public class ContractManagementManagementClient implements AutoCloseable {

    private static final OkHttpClient HTTP_CLIENT = TestUtils.DIRECT_HTTP_CLIENT;
    private static final ObjectMapper OBJECT_MAPPER = TestUtils.OBJECT_MAPPER;
    private static final MediaType JSON = MediaType.parse("application/json");
    private static final String REPORT_PATH = "/participants-removed";
    private static final String EVENTS_PATH = "/events";
    private static final String HEALTH_PATH = "/health";
    private static final String TIR_WATCHER_INDICATOR = "dataspace-tir-watcher";
    private static final String SUBSCRIPTIONS_INDICATOR = "dataspace-subscriptions";
    private static final String LAST_SUCCESSFUL_CYCLE = "lastSuccessfulCycle";

    private final String namespace;
    private final String deployment;
    private final List<KubernetesHelper.PortForward> openForwards = new ArrayList<>();

    public ContractManagementManagementClient(String namespace, String deployment) {
        this.namespace = namespace;
        this.deployment = deployment;
    }

    /** One publisher's section of {@code GET /participants-removed}. */
    public record PublisherReport(String publisher, String mode, String til, List<ParticipantReport> participants) {
    }

    /** The latest outcome for one removed participant; held, attributable and unattributedCount are null if unread. */
    public record ParticipantReport(String id, String trigger, String lastSeen, String mode, String action,
                                    boolean applied, Boolean attributable, List<HeldGrant> held,
                                    Integer unattributedCount) {

        /** {@link #lastSeen} as an instant; CM writes it in ISO-8601. */
        public Instant lastSeenInstant() {
            return Instant.parse(lastSeen);
        }
    }

    /** One grant a removed participant still held; {@code scopeType} null is an untyped (order) grant. */
    public record HeldGrant(String scope, String scopeType, int credentialCount) {
    }

    /** Response of a manual publish: the event id and how many subscriptions it went to. */
    public record PublishedEvent(String id, int subscriptions) {
    }

    /** Reads the removal report. */
    public List<PublisherReport> report() throws Exception {
        String body = send(new Request.Builder().get(), REPORT_PATH, true);
        return OBJECT_MAPPER.readValue(body, new TypeReference<>() {
        });
    }

    /** Publishes an event through the publisher's management endpoint. */
    public PublishedEvent publish(String type, String subject) throws Exception {
        RequestBody requestBody = RequestBody.create(
                OBJECT_MAPPER.writeValueAsString(Map.of("type", type, "subject", subject)), JSON);
        String body = send(new Request.Builder().post(requestBody), EVENTS_PATH, true);
        return OBJECT_MAPPER.readValue(body, PublishedEvent.class);
    }

    /** The report section of {@code publisher}, if CM knows that publisher. */
    public Optional<PublisherReport> publisherReport(String publisher) throws Exception {
        return report().stream().filter(r -> publisher.equals(r.publisher())).findFirst();
    }

    /** Waits until the report shows {@code mode} for {@code publisher}, i.e. a ConfigMap change has been reloaded. */
    public void awaitMode(String publisher, String mode, Duration timeout) {
        Awaitility.await(String.format("%s/%s should report mode %s for %s.", namespace, deployment, mode, publisher))
                .atMost(timeout)
                .pollInterval(POLL_INTERVAL)
                .ignoreExceptions()
                .until(() -> publisherReport(publisher).map(PublisherReport::mode).filter(mode::equals).isPresent());
    }

    /** Publishes {@code probeDid} until it reaches {@code subscriptions} subscriptions; one DID keeps one report entry. */
    public void awaitSubscriptions(String probeDid, int subscriptions, Duration timeout) {
        Awaitility.await(String.format("The publisher should deliver to %d subscriptions.", subscriptions))
                .atMost(timeout)
                .pollInterval(POLL_INTERVAL)
                .ignoreExceptions()
                .until(() -> publish(DataSpaceEventingEnvironment.PARTICIPANT_REMOVED_TYPE, probeDid).subscriptions()
                        == subscriptions);
    }

    /**
     * Waits for a report entry of {@code did} written after {@code since} by {@code trigger}, and returns it.
     * Older entries are outcomes of earlier removals of the same DID and never count.
     */
    public ParticipantReport awaitOutcome(String publisher, String did, String trigger, Instant since, Duration timeout) {
        AtomicReference<ParticipantReport> found = new AtomicReference<>();
        Awaitility.await(String.format("%s/%s should report a %s outcome for %s after %s.",
                        namespace, deployment, trigger, did, since))
                .atMost(timeout)
                .pollInterval(POLL_INTERVAL)
                .ignoreExceptions()
                .until(() -> {
                    Optional<ParticipantReport> entry = publisherReport(publisher).stream()
                            .flatMap(r -> Optional.ofNullable(r.participants()).orElse(List.of()).stream())
                            .filter(p -> did.equals(p.id()))
                            .filter(p -> trigger.equals(p.trigger()))
                            .filter(p -> p.lastSeenInstant().isAfter(since))
                            .findFirst();
                    entry.ifPresent(found::set);
                    return entry.isPresent();
                });
        log.debug("{}/{} reports {} for {}.", namespace, deployment, found.get(), did);
        return found.get();
    }

    /**
     * Waits until the TIR watcher finished {@link DataSpaceEventingEnvironment#WATCHER_CYCLES_TO_SEE} distinct cycles
     * after {@code since}: the last of them started after the first ended, so it read the TIR after {@code since}.
     */
    public void awaitWatcherSaw(Instant since) {
        Set<Instant> cycles = new HashSet<>();
        Awaitility.await(String.format("The TIR watcher should complete %d cycles after %s.", WATCHER_CYCLES_TO_SEE, since))
                .atMost(WATCHER_SEEN_TIMEOUT)
                .pollInterval(POLL_INTERVAL)
                .ignoreExceptions()
                .until(() -> {
                    lastSuccessfulCycle().filter(cycle -> cycle.isAfter(since)).ifPresent(cycles::add);
                    return cycles.size() >= WATCHER_CYCLES_TO_SEE;
                });
    }

    /** The Deployment's pods, terminating ones included. */
    public List<String> pods() throws Exception {
        return KubernetesHelper.deploymentPods(namespace, deployment);
    }

    /** The subscriber's registration state per publisher, for failure messages; never throws. */
    public String subscriptionHealth() {
        try {
            return namespace + "/" + deployment + " " + healthIndicator(SUBSCRIPTIONS_INDICATOR).path("details");
        } catch (Exception e) {
            return namespace + "/" + deployment + " health unreadable: " + e.getMessage();
        }
    }

    private Optional<Instant> lastSuccessfulCycle() throws Exception {
        JsonNode cycle = healthIndicator(TIR_WATCHER_INDICATOR).path("details").path(LAST_SUCCESSFUL_CYCLE);
        return cycle.isTextual() ? Optional.of(Instant.parse(cycle.asText())) : Optional.empty();
    }

    private JsonNode healthIndicator(String indicator) throws Exception {
        // a DOWN health answers 503 with the same body
        JsonNode health = OBJECT_MAPPER.readTree(send(new Request.Builder().get(), HEALTH_PATH, false));
        return health.path("details").path(indicator);
    }

    private String send(Request.Builder request, String path, boolean requireSuccess) throws Exception {
        KubernetesHelper.PortForward forward = open();
        try (forward; Response response = HTTP_CLIENT.newCall(
                request.url("http://localhost:" + forward.localPort() + path).build()).execute()) {
            String body = response.body() == null ? "" : response.body().string();
            if (requireSuccess && !response.isSuccessful()) {
                throw new IOException(String.format("%s %s on %s/%s answered %d: %s",
                        response.request().method(), path, namespace, deployment, response.code(), body));
            }
            return body;
        } finally {
            synchronized (openForwards) {
                openForwards.remove(forward);
            }
        }
    }

    private KubernetesHelper.PortForward open() throws IOException, InterruptedException {
        KubernetesHelper.PortForward forward =
                KubernetesHelper.portForwardToDeployment(namespace, deployment, CM_MANAGEMENT_PORT);
        synchronized (openForwards) {
            openForwards.add(forward);
        }
        return forward;
    }

    /** Closes every forward a failed request left behind. */
    @Override
    public void close() {
        synchronized (openForwards) {
            openForwards.forEach(KubernetesHelper.PortForward::close);
            openForwards.clear();
        }
    }
}
