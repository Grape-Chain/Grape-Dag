package io.aplfintech.luna.config;

import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.annotation.JsonPropertyOrder;
import lombok.AllArgsConstructor;
import lombok.Data;
import lombok.NoArgsConstructor;

/**
 * Simple map item for any options
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@JsonPropertyOrder({"v", "d"})
@AllArgsConstructor
@NoArgsConstructor
@Data
public class PriceItem {
    @JsonProperty("v")
    private int value;
    @JsonProperty("d")
    private String description;
}
