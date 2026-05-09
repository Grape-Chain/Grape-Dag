package io.aplfintech.luna.config;

import lombok.EqualsAndHashCode;
import lombok.Getter;
import lombok.NonNull;
import lombok.ToString;

import java.util.Date;
import java.util.HashMap;
import java.util.HashSet;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.Set;

/**
 * The chain config for hard forks
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */

@Getter
public class ChainConfigFactory {
    private final GrapeChainConfig grapeChainConfig;

    public ChainConfigFactory(@NonNull GrapeChainConfig grapeChainConfig) {
        this.grapeChainConfig = grapeChainConfig;
    }

    public ChainConfig configAt(Date timestamp) {
        return configAt(timestamp.getTime() / 1000);
    }

    /**
     * Returns the actual chain config at the given timestamp
     *
     * @param timestamp timestamp in seconds
     * @return the actual chain config at the given timestamp
     */
    public ChainConfig configAt(long timestamp) {
        var chainConfig = mergeConfig(timestamp);
        if (!chainConfig.isValid()) {
            chainConfig.validate();
        }
        return chainConfig;
    }

    /**
     * @param timestamp timestamp in seconds
     */
    private ChainConfigImpl mergeConfig(final long timestamp) {
        GasConfig gasConfig = grapeChainConfig.getGasConfig();
        VmConfig vmConfig = grapeChainConfig.getVmConfig();
        GasPriceMap gasPriceMap = grapeChainConfig.getGasPriceMap();

        Set<HardForkConfig> hardForks = new LinkedHashSet<>();
        Set<String> enabledFeatures = new LinkedHashSet<>();
        var chainConfig = new ChainConfigImpl(timestamp, gasConfig, vmConfig, gasPriceMap);

        if (grapeChainConfig.getHardForks() != null) {
            //merge gas price for all hard forks
            for (HardForkConfig hfConfig : grapeChainConfig.getHardForks()) {
                if (hfConfig.isEnabledAt(timestamp)) {
                    hardForks.add(hfConfig);
                    if (hfConfig.getGfs() != null) {
                        if (hfConfig.getGfs().getEnableFeatures() != null) {
                            enabledFeatures.addAll(hfConfig.getGfs().getEnableFeatures());
                        }
                        if (hfConfig.getGfs().getDisableFeatures() != null) {
                            hfConfig.getGfs().getDisableFeatures().forEach(enabledFeatures::remove);
                        }
                    }
                }
            }
        }

        hardForks.forEach(chainConfig::addHardFork);

        for (String featureName : enabledFeatures) {
            var featureOpt = grapeChainConfig.getGfs().stream().filter(featureConfig -> featureConfig.getName().equals(featureName)).findFirst();
            if (featureOpt.isEmpty()) {
                throw new ConfigurationException("Unknown feature: " + featureName);
            }
            chainConfig.addFeature(featureOpt.get());
        }

        return chainConfig;
    }

    @ToString(callSuper = true)
    @EqualsAndHashCode(callSuper = true)
    private static final class ChainConfigImpl extends BaseChainConfig implements ChainConfig {
        @Getter
        private final long timestamp;
        private final Set<String> hardForks;
        private final Set<String> features;
        private final Map<String, Map<String, PropertyItem>> featureProperties;

        private ChainConfigImpl(long timestamp, @NonNull GasConfig gasConfig, @NonNull VmConfig vmConfig,
                                @NonNull GasPriceMap gasPriceMap) {
            super(GasConfig.from(gasConfig), VmConfig.from(vmConfig), GasPriceMap.from(gasPriceMap));
            this.timestamp = timestamp;
            this.hardForks = new HashSet<>();
            this.features = new HashSet<>();
            this.featureProperties = new HashMap<>();
        }

        @Override
        public boolean isFeatureEnabled(@NonNull String featureName) {
            return features.contains(featureName);
        }

        @Override
        public boolean isForkEnabled(@NonNull String forkName) {
            return hardForks.contains(forkName);
        }

        @Override
        public Optional<String> getProperty(@NonNull String feature, @NonNull String property) {
            if (featureProperties.containsKey(feature)) {
                if (featureProperties.get(feature).containsKey(property)) {
                    return Optional.of(featureProperties.get(feature).get(property).getValue());
                }
            }
            return Optional.empty();
        }

        @Override
        public Optional<Integer> getIntProperty(String feature, String property) {
            if (featureProperties.containsKey(feature)) {
                if (featureProperties.get(feature).containsKey(property)) {
                    var value = featureProperties.get(feature).get(property).getValue();
                    if (value != null) {
                        return Optional.of(Integer.parseInt(value));
                    }
                }
            }
            return Optional.empty();
        }

        @Override
        public Optional<List<Integer>> getIntProperties(String feature, String property) {
            if (featureProperties.containsKey(feature)) {
                if (featureProperties.get(feature).containsKey(property)) {
                    return Optional.ofNullable(featureProperties.get(feature).get(property).getIntValues());
                }
            }
            return Optional.empty();
        }

        @Override
        public GasConfig gasConfig() {
            return gasConfig;
        }

        @Override
        public VmConfig vmConfig() {
            return vmConfig;
        }

        @Override
        public GasPriceMap gasPriceMap() {
            return gasPriceMap;
        }

        public void validate() {
            if (!gasConfig.isValid()) {
                throw new ConfigurationException("Wrong gas config: " + gasConfig);
            }
            if (!vmConfig.isValid()) {
                throw new ConfigurationException("Wrong vm config:" + vmConfig);
            }
            if (!gasPriceMap.isValid()) {
                throw new ConfigurationException("Wrong gas price map:" + gasPriceMap);
            }
        }

        private void addHardFork(@NonNull HardForkConfig hardFork) {
            hardForks.add(hardFork.getName());
            BaseChainConfig mergedConfig = hardFork.getChainConfig();
            if (mergedConfig != null) {
                gasConfig.merge(mergedConfig.getGasConfig());
                vmConfig.merge(mergedConfig.getVmConfig());
                gasPriceMap.merge(mergedConfig.getGasPriceMap());
            }
        }

        private void addFeature(@NonNull FeatureConfig feature) {
            features.add(feature.getName());
            gasPriceMap.merge(feature.getGasPrice());
            if (feature.getProperties() != null) {
                featureProperties.put(feature.getName(), new HashMap<>(feature.getProperties()));
            }
        }

    }

}
