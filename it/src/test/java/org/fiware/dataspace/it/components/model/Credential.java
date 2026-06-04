package org.fiware.dataspace.it.components.model;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import lombok.AllArgsConstructor;
import lombok.Data;
import lombok.NoArgsConstructor;

import java.util.List;
import java.util.Map;

/**
 * Response wrapper for the OID4VCI /credential endpoint.
 *
 * <p>Pre-KC 26.4: a flat {@code {"credential": "<jwt>"}} object.
 * <br>KC 26.4+ (OID4VCI draft 14+): {@code {"credentials": [{"credential": "<jwt>"}]}}.
 *
 * <p>This bean accepts both shapes and exposes a single accessor {@link #getCredential()}.
 *
 * @author <a href="https://github.com/wistefan">Stefan Wiedemann</a>
 */
@NoArgsConstructor
@AllArgsConstructor
@Data
@JsonIgnoreProperties(ignoreUnknown = true)
public class Credential {

    /** Legacy single-credential field (KC < 26.4). */
    private String credential;

    /** KC 26.4+ array. Each entry is expected to contain a {@code credential} string. */
    private List<Map<String, Object>> credentials;

    /** Returns the (first) credential string, regardless of which response shape was received. */
    public String resolveCredential() {
        if (credential != null) {
            return credential;
        }
        if (credentials != null && !credentials.isEmpty()) {
            Object first = credentials.get(0).get("credential");
            return first != null ? first.toString() : null;
        }
        return null;
    }
}
