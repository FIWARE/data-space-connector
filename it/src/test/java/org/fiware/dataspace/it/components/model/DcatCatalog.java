package org.fiware.dataspace.it.components.model;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;
import lombok.AllArgsConstructor;
import lombok.Data;
import lombok.NoArgsConstructor;
import org.checkerframework.checker.index.qual.HasSubsequence;

import java.util.ArrayList;
import java.util.List;

@NoArgsConstructor
@AllArgsConstructor
@Data
@JsonIgnoreProperties(ignoreUnknown = true)
public class DcatCatalog {

	@JsonProperty("@type")
	private String type;

	@JsonProperty("@id")
	private String id;

	@JsonProperty("dcat:service")
	private List<DcatService> service = new ArrayList<>();
}
