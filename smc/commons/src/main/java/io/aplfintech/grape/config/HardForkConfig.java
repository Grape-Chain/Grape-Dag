package io.aplfintech.grape.config;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonFormat;
import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.annotation.JsonPropertyOrder;
import lombok.EqualsAndHashCode;
import lombok.Getter;
import lombok.NonNull;
import lombok.ToString;

import java.util.Date;

/**
 * General hard fork config
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@JsonPropertyOrder({"name", "timestamp", "gfs", "chainConfig"})
@JsonInclude(JsonInclude.Include.NON_NULL)
@Getter
@ToString
@EqualsAndHashCode
public class HardForkConfig {
    @JsonProperty("name")
    private final String name;
    @JsonFormat(pattern = "yyyy-MM-dd'T'HH:mm:ss")
    @JsonProperty("timestamp")
    private final Date timestamp;
    @JsonProperty("gfs")
    private final OptionalFeatureConfig gfs;
    @JsonProperty("chainConfig")
    private final BaseChainConfig chainConfig;

    @JsonCreator
    public HardForkConfig(@JsonProperty("name") String name,
                          @JsonProperty("timestamp") Date timestamp,
                          @JsonProperty("gfs") OptionalFeatureConfig gfs,
                          @JsonProperty("chainConfig") BaseChainConfig chainConfig) {
        this.name = name;
        this.timestamp = timestamp;
        this.gfs = gfs;
        this.chainConfig = chainConfig;
    }

    public boolean isEnabledAt(@NonNull Date timestamp) {
        return isEnabledAt(timestamp.getTime() / 1000);
    }

    /**
     * Returns true if the given timestamp is after the hard fork timestamp
     *
     * @param timestamp the timestamp in seconds
     * @return true if the given timestamp is after the hard fork timestamp, otherwise - false
     */
    public boolean isEnabledAt(long timestamp) {
        return this.timestamp.getTime() / 1000 < timestamp;
    }
}
