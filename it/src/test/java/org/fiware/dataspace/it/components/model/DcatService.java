package org.fiware.dataspace.it.components.model;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;
import lombok.AllArgsConstructor;
import lombok.Data;
import lombok.NoArgsConstructor;

@NoArgsConstructor
@AllArgsConstructor
@Data
@JsonIgnoreProperties(ignoreUnknown = true)
public class DcatService {

	@JsonProperty("@type")
	private String type;

	@JsonProperty("dcat:endpointDescription")
	private String endpointDescription;

	@JsonProperty("dcat:endpointURL")
	private String endpointUrl;

}
