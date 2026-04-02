package org.fiware.dataspace.it.components.model;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import lombok.AllArgsConstructor;
import lombok.Data;
import lombok.NoArgsConstructor;

/**
 * Response from EDR (Endpoint Data Reference) data address queries on the EDC Management API.
 * Contains the provisioned endpoint URL and access token for data retrieval.
 */
@NoArgsConstructor
@AllArgsConstructor
@Data
@JsonIgnoreProperties(ignoreUnknown = true)
public class DataAddress {

    /** The data service endpoint URL where data can be retrieved. */
    private String endpoint;

    /** The access token to authenticate against the data service endpoint. */
    private String token;
}
