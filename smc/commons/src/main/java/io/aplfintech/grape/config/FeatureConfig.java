package io.aplfintech.grape.config;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.annotation.JsonPropertyOrder;
import lombok.EqualsAndHashCode;
import lombok.Getter;
import lombok.ToString;

import java.util.Map;

/**
 * The chain config for hard forks
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@JsonPropertyOrder({"name", "config", "gasPrice"})
@JsonInclude(JsonInclude.Include.NON_NULL)
@Getter
@ToString
@EqualsAndHashCode
public class FeatureConfig {
    @JsonProperty("name")
    private final String name;
    @JsonProperty("config")
    private final Map<String, PropertyItem> properties;
    @JsonProperty("gasPrice")
    private final GasPriceMap gasPrice;

    @JsonCreator
    public FeatureConfig(@JsonProperty("name") String name,
                         @JsonProperty("config") Map<String, PropertyItem> properties,
                         @JsonProperty("gasPrice") GasPriceMap gasPrice) {
        this.name = name;
        this.properties = properties;
        this.gasPrice = gasPrice;
    }

}
