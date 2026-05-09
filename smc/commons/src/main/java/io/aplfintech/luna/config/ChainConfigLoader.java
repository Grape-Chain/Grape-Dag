package io.aplfintech.luna.config;

import java.io.IOException;
import java.io.InputStream;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public class ChainConfigLoader extends AbstractConfigLoader<GrapeChainConfig> {
    private static final String DEFAULT_CHAIN_FILENAME = "chain.json";

    public ChainConfigLoader() {
        this(DEFAULT_CHAIN_FILENAME);
    }

    public ChainConfigLoader(String resourceName) {
        super(resourceName, false);
    }

    public ChainConfigLoader(String configDir, String resourceName, boolean ignoreResources) {
        super(configDir, resourceName, ignoreResources);
    }

    @Override
    protected GrapeChainConfig read(InputStream is) throws IOException {
        return ConfigHelper.readValue(is, GrapeChainConfig.class);
    }
}
