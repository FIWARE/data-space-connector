package org.fiware.dataspace.it.components;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;

import java.util.ArrayList;
import java.util.List;
import java.util.Map;

/**
 * The {@code dataspace-events.yaml} key of a contract-management's events ConfigMap, which CM reloads through its file
 * watch. Edits are line-based, so every line other than the edited ones stays byte-equal.
 */
public class DataSpaceEventsConfigMap {

    private static final ObjectMapper OBJECT_MAPPER = TestUtils.OBJECT_MAPPER;
    private static final String CONFIG_KEY = "dataspace-events.yaml";
    private static final String LINE_SEPARATOR = "\n";
    private static final String LIST_ITEM = "- ";
    private static final String PUBLISHERS_KEY = "publishers:";
    private static final String REMOVAL_KEY = "on-participant-removed:";
    private static final String MODE_KEY = "mode:";
    private static final String KEEP_KEY = "keep:";

    private final String namespace;
    private final String name;

    public DataSpaceEventsConfigMap(String namespace, String name) {
        this.namespace = namespace;
        this.name = name;
    }

    /** The current configuration file, exactly as stored. */
    public String read() throws Exception {
        JsonNode configMap = OBJECT_MAPPER.readTree(
                KubernetesHelper.kubectl("get", "configmap", name, "-n", namespace, "-o", "json"));
        JsonNode config = configMap.path("data").path(CONFIG_KEY);
        if (!config.isTextual()) {
            throw new IllegalStateException(String.format("%s/%s has no %s.", namespace, name, CONFIG_KEY));
        }
        return config.asText();
    }

    /** Replaces the configuration file; the kubelet syncs it into the pod, where CM's file watch reloads it. */
    public void write(String config) throws Exception {
        String patch = OBJECT_MAPPER.writeValueAsString(Map.of("data", Map.of(CONFIG_KEY, config)));
        KubernetesHelper.kubectl("patch", "configmap", name, "-n", namespace, "--type", "merge", "-p", patch);
    }

    /**
     * Sets {@code publishers[0].on-participant-removed.mode} and appends {@code additionalKeep} to its {@code keep}.
     * Fails if the mode does not change: the report only shows {@code mode}, so an unchanged mode hides the reload.
     */
    public static String withRemovalPolicy(String config, String mode, List<String> additionalKeep) {
        List<String> lines = new ArrayList<>(List.of(config.split(LINE_SEPARATOR, -1)));
        int publishers = findKey(lines, 0, lines.size(), -1, PUBLISHERS_KEY);
        int item = firstListItem(lines, publishers);
        // the item's keys sit two columns right of its dash, the first of them on the dash's line
        int itemKeyIndent = indent(lines.get(item)) + LIST_ITEM.length();
        int itemEnd = blockEnd(lines, item, indent(lines.get(item)));
        int removal = findKey(lines, item, itemEnd, itemKeyIndent, REMOVAL_KEY);
        int removalEnd = blockEnd(lines, removal, itemKeyIndent);
        int childIndent = indent(lines.get(removal + 1));

        int modeLine = findKey(lines, removal + 1, removalEnd, childIndent, MODE_KEY);
        String currentMode = lines.get(modeLine).substring(childIndent + MODE_KEY.length()).trim();
        if (currentMode.equals(mode)) {
            throw new IllegalArgumentException(String.format(
                    "The mode is already %s: a reload would not be observable.", mode));
        }
        lines.set(modeLine, " ".repeat(childIndent) + MODE_KEY + " " + mode);

        int keepLine = findKey(lines, removal + 1, removalEnd, childIndent, KEEP_KEY);
        int keepEnd = keepLine + 1;
        // a block sequence under a key may start at the key's own column (toYaml) or further right
        while (keepEnd < removalEnd && isKeepEntry(lines.get(keepEnd), childIndent)) {
            keepEnd++;
        }
        if (keepEnd == keepLine + 1) {
            throw new IllegalStateException("on-participant-removed.keep has no block-sequence entries.");
        }
        String entryPrefix = lines.get(keepLine + 1).substring(0, lines.get(keepLine + 1).indexOf(LIST_ITEM));
        List<String> added = additionalKeep.stream().map(did -> entryPrefix + LIST_ITEM + did).toList();
        lines.addAll(keepEnd, added);
        return String.join(LINE_SEPARATOR, lines);
    }

    private static boolean isKeepEntry(String line, int keyIndent) {
        if (line.isBlank()) {
            return false;
        }
        return indent(line) > keyIndent || indent(line) == keyIndent && line.trim().startsWith(LIST_ITEM);
    }

    private static int firstListItem(List<String> lines, int parent) {
        int parentIndent = indent(lines.get(parent));
        for (int i = parent + 1; i < lines.size(); i++) {
            if (lines.get(i).isBlank()) {
                continue;
            }
            if (indent(lines.get(i)) >= parentIndent && lines.get(i).trim().startsWith(LIST_ITEM)) {
                return i;
            }
            break;
        }
        throw new IllegalStateException("publishers has no entry.");
    }

    // the first line after start that is not blank and sits at or left of the given indent
    private static int blockEnd(List<String> lines, int start, int ownIndent) {
        for (int i = start + 1; i < lines.size(); i++) {
            if (!lines.get(i).isBlank() && indent(lines.get(i)) <= ownIndent) {
                return i;
            }
        }
        return lines.size();
    }

    // a key at the given indent (any indent if negative); a list item's first key counts at its content column
    private static int findKey(List<String> lines, int from, int to, int keyIndent, String key) {
        for (int i = from; i < to; i++) {
            String line = lines.get(i);
            String content = line.trim().startsWith(LIST_ITEM)
                    ? " ".repeat(indent(line) + LIST_ITEM.length()) + line.trim().substring(LIST_ITEM.length())
                    : line;
            if (content.trim().startsWith(key) && (keyIndent < 0 || indent(content) == keyIndent)) {
                return i;
            }
        }
        throw new IllegalStateException(String.format("No %s found in the events configuration.", key));
    }

    private static int indent(String line) {
        int indent = 0;
        while (indent < line.length() && line.charAt(indent) == ' ') {
            indent++;
        }
        return indent;
    }
}
