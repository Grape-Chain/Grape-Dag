package io.aplfintech.grape.config;

import java.util.List;
import java.util.Optional;

/**
 * The chain config
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */

public interface ChainConfig {
    GasConfig gasConfig();

    VmConfig vmConfig();

    GasPriceMap gasPriceMap();

    boolean isFeatureEnabled(String featureName);

    boolean isForkEnabled(String forkName);

    Optional<String> getProperty(String feature, String property);

    Optional<Integer> getIntProperty(String feature, String property);

    Optional<List<Integer>> getIntProperties(String feature, String property);

    boolean isValid();
}
