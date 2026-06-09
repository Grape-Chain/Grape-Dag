package io.aplfintech.grape.config;

import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.annotation.JsonPropertyOrder;
import lombok.AllArgsConstructor;
import lombok.Data;
import lombok.NoArgsConstructor;

import java.util.List;

/**
 * Simple map item for any options
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@JsonPropertyOrder({"d", "value", "intValues"})
@JsonInclude(JsonInclude.Include.NON_NULL)
@AllArgsConstructor
@NoArgsConstructor
@Data
public class PropertyItem {
    @JsonProperty("value")
    private String value;
    @JsonProperty("d")
    private String description;
    @JsonProperty("intValues")
    private List<Integer> intValues;
}
