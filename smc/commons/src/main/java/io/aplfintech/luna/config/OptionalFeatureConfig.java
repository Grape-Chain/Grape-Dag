package io.aplfintech.luna.config;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.annotation.JsonPropertyOrder;
import lombok.Getter;

import java.util.List;

@JsonPropertyOrder({"enable", "disable"})
@JsonInclude(JsonInclude.Include.NON_NULL)
@Getter
public class OptionalFeatureConfig {
    @JsonProperty("enable")
    //Enabled features list
    private final List<String> enableFeatures;

    @JsonProperty("disable")
    //Disabled features list
    private final List<String> disableFeatures;

    @JsonCreator
    public OptionalFeatureConfig(@JsonProperty("enable") List<String> enable,
                                 @JsonProperty("disable") List<String> disable) {
        this.enableFeatures = enable;
        this.disableFeatures = disable;
    }
}
