package io.aplfintech.luna.config;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonIgnore;
import com.fasterxml.jackson.annotation.JsonInclude;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.annotation.JsonPropertyOrder;
import lombok.EqualsAndHashCode;
import lombok.Getter;
import lombok.NonNull;
import lombok.ToString;

import java.util.ArrayList;
import java.util.List;
import java.util.Optional;

/**
 * General chain config
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@JsonPropertyOrder({"gasConfig", "vm", "gasPrice", "hardForks", "gfs"})
@JsonInclude(JsonInclude.Include.NON_NULL)
@Getter
@ToString
@EqualsAndHashCode
public class GrapeChainConfig {
    @JsonProperty("gasConfig")
    private final GasConfig gasConfig;
    @JsonProperty("vm")
    private final VmConfig vmConfig;
    @JsonProperty("gasPrice")
    private GasPriceMap gasPriceMap;
    @JsonProperty("hardForks")
    private final List<HardForkConfig> hardForks;
    @JsonProperty("gfs")
    private final List<FeatureConfig> gfs;

    @JsonCreator
    public GrapeChainConfig(@JsonProperty("gasConfig") GasConfig gasConfig,
                            @JsonProperty("vm") VmConfig vmConfig,
                            @JsonProperty("gasPrice") GasPriceMap gasPriceMap,
                            @JsonProperty("hardForks") List<HardForkConfig> hardForks,
                            @JsonProperty("gfs") List<FeatureConfig> gfs
    ) {
        this.gasConfig = gasConfig;
        this.gasPriceMap = gasPriceMap;
        this.vmConfig = vmConfig;
        this.hardForks = hardForks;
        this.gfs = gfs;
    }

    static GrapeChainConfig from(@NonNull BaseChainConfig chainConfig) {
        var cfg = new GrapeChainConfig(chainConfig.getGasConfig(), chainConfig.getVmConfig(), chainConfig.getGasPriceMap(),
            new ArrayList<>(), new ArrayList<>());
        if (!cfg.isValid()) {

            throw new ConfigurationException("Wrong config:" + cfg);
        }
        return cfg;
    }

    @JsonIgnore
    public boolean isValid() {
        return gasConfig != null && gasConfig.isValid()
            && vmConfig != null && vmConfig.isValid()
            && gasPriceMap != null && gasPriceMap.isValid();
    }

    void addHardFork(@NonNull HardForkConfig hardForkConfig) {
        hardForks.add(hardForkConfig);
    }

    void addFeature(@NonNull FeatureConfig featureConfig) {
        gfs.add(featureConfig);
    }

    void setGasPriceMap(@NonNull GasPriceMap gasPriceMap) {
        this.gasPriceMap = gasPriceMap;
    }

    public Optional<HardForkConfig> locateHardFork(@NonNull String forkName) {
        return hardForks.stream().filter(forkConfig -> forkName.equals(forkConfig.getName())).findFirst();
    }
}
