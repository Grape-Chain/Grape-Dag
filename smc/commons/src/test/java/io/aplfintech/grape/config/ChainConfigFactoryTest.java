package io.aplfintech.grape.config;

import org.junit.jupiter.api.Test;

import java.util.Date;
import java.util.HashMap;

import static io.aplfintech.grape.config.ConfigTestHelper.createBaseConfig;
import static io.aplfintech.grape.config.ConfigTestHelper.createFeatureConfig;
import static io.aplfintech.grape.config.ConfigTestHelper.createHardForkConfig;
import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.catchException;
import static org.junit.jupiter.api.Assertions.assertNotNull;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
class ChainConfigFactoryTest {

    @Test
    void configFrom() {
        var bcc = createBaseConfig("base-cfg-0.json");
        var rc = GrapeChainConfig.from(bcc);
        assertNotNull(rc, "It's valid config");
    }

    @Test
    void configFromWrong() {
        var bcc = createBaseConfig("base-cfg-1.json");
        var rc = catchException(() -> GrapeChainConfig.from(bcc));
        assertThat(rc)
            .isInstanceOf(ConfigurationException.class)
            .hasMessageContaining("Wrong config:");
    }

    @Test
    void config_before_hf() {
        var bcc = createBaseConfig("base-cfg-0.json");
        var hf = createHardForkConfig("hf-2.json");
        var beforeHfTimestamp = new Date(hf.getTimestamp().toInstant().minusSeconds(1).toEpochMilli());
        //WHEN
        var config = GrapeChainConfig.from(bcc);
        //THEN
        assertNotNull(config);
        //WHEN
        config.addHardFork(hf);
        var ccf = new ChainConfigFactory(config);
        var cfg = ccf.configAt(beforeHfTimestamp);
        //THEN
        assertThat(cfg)
            .isNotNull()
            .extracting("gasConfig", "vmConfig")
            .contains(bcc.gasConfig, bcc.vmConfig);
        assertThat(cfg.gasPriceMap())
            .containsExactlyEntriesOf(bcc.gasPriceMap);

    }

    @Test
    void config_after_hf_without_prices() {
        var bcc = createBaseConfig("base-cfg-0.json");
        var hf = createHardForkConfig("hf-0.json");
        var afterHfTimestamp = new Date(hf.getTimestamp().toInstant().plusSeconds(1).toEpochMilli());
        //WHEN
        var config = GrapeChainConfig.from(bcc);
        //THEN
        assertNotNull(config);
        //WHEN
        config.addHardFork(hf);
        var ccf = new ChainConfigFactory(config);
        var cfg = ccf.configAt(afterHfTimestamp);
        //THEN
        assertThat(cfg)
            .isNotNull()
            .returns(hf.getChainConfig().getVmConfig().getMaxCodeSize(), (chainConfig) -> chainConfig.vmConfig().getMaxCodeSize());

        assertThat(cfg.gasPriceMap())
            .containsExactlyEntriesOf(bcc.gasPriceMap);

    }

    @Test
    void config_after_hf_with_new_prices() {
        var bcc = createBaseConfig("base-cfg-0.json");
        var hf = createHardForkConfig("hf-1.json");
        var afterHfTimestamp = new Date(hf.getTimestamp().toInstant().plusSeconds(1).toEpochMilli());
        //WHEN
        var config = GrapeChainConfig.from(bcc);
        //THEN
        assertNotNull(config);
        //WHEN
        config.addHardFork(hf);
        var ccf = new ChainConfigFactory(config);
        var cfg = ccf.configAt(afterHfTimestamp);
        //THEN
        assertThat(cfg)
            .isNotNull()
            .returns(hf.getChainConfig().getVmConfig().getMaxCodeSize(), (chainConfig) -> chainConfig.vmConfig().getMaxCodeSize());

        assertThat(cfg.gasPriceMap())
            .containsAllEntriesOf(bcc.gasPriceMap)
            .containsAllEntriesOf(hf.getChainConfig().gasPriceMap);
    }

    @Test
    void config_after_hf_with_updated_prices() {
        var bcc = createBaseConfig("base-cfg-0.json");
        var hf = createHardForkConfig("hf-1-u.json");
        var afterHfTimestamp = new Date(hf.getTimestamp().toInstant().plusSeconds(1).toEpochMilli());
        var resultPrice = new HashMap<>(bcc.gasPriceMap);
        resultPrice.putAll(hf.getChainConfig().gasPriceMap);

        //WHEN
        var config = GrapeChainConfig.from(bcc);
        //THEN
        assertNotNull(config);
        //WHEN
        config.addHardFork(hf);
        var ccf = new ChainConfigFactory(config);
        var cfg = ccf.configAt(afterHfTimestamp);
        //THEN
        assertThat(cfg)
            .isNotNull()
            .returns(hf.getChainConfig().getVmConfig().getMaxCodeSize(), (chainConfig) -> chainConfig.vmConfig().getMaxCodeSize());
        assertThat(cfg.gasPriceMap())
            .containsAllEntriesOf(resultPrice);
    }

    @Test
    void config_after_hf_with_feature_and_updated_prices() {
        var bcc = createBaseConfig("base-cfg-0.json");
        var hf = createHardForkConfig("hf-2.json");
        var gf = createFeatureConfig("gf-wa.json");
        var afterHfTimestamp = new Date(hf.getTimestamp().toInstant().plusSeconds(1).toEpochMilli());
        var resultPrice = new HashMap<>(bcc.gasPriceMap);
        resultPrice.putAll(hf.getChainConfig().gasPriceMap);

        //WHEN
        var config = GrapeChainConfig.from(bcc);
        //THEN
        assertNotNull(config);
        //WHEN
        config.addHardFork(hf);
        config.addFeature(gf);
        var ccf = new ChainConfigFactory(config);
        var cfg = ccf.configAt(afterHfTimestamp);
        //THEN
        assertThat(cfg)
            .isNotNull()
            .returns(hf.getChainConfig().getVmConfig().getMaxCodeSize(), (chainConfig) -> chainConfig.vmConfig().getMaxCodeSize());
        assertThat(cfg.gasPriceMap())
            .containsAllEntriesOf(resultPrice);

        //feature is enabled
        assertThat(cfg.isFeatureEnabled(gf.getName()))
            .isEqualTo(true);

        //feature brings the new price item
        assertThat(cfg.gasPriceMap().lookForGasPrice("coldaccountaccess"))
            .isEqualTo(2600);
        //feature brings the property
        assertThat(cfg.getProperty(gf.getName(), "prop1"))
            .hasValue("2");
        assertThat(cfg.getIntProperty(gf.getName(), "prop1"))
            .hasValue(2);
        //
        assertThat(cfg.getProperty(gf.getName(), "unknown-Property"))
            .isEmpty();

    }

    @Test
    void config_enable_disable_feature() {
        var bcc = createBaseConfig("base-cfg-0.json");
        var hf = createHardForkConfig("hf-e-d.json");
        var afterHfTimestamp = new Date(hf.getTimestamp().toInstant().plusSeconds(1).toEpochMilli());
        //WHEN
        var config = GrapeChainConfig.from(bcc);
        //THEN
        assertNotNull(config);
        //WHEN
        config.addHardFork(hf);
        var ccf = new ChainConfigFactory(config);
        var cfg = ccf.configAt(afterHfTimestamp);
        //THEN
        assertThat(cfg)
            .isNotNull()
            .extracting("gasConfig", "vmConfig")
            .contains(bcc.gasConfig, bcc.vmConfig);
        assertThat(cfg.gasPriceMap())
            .containsExactlyEntriesOf(bcc.gasPriceMap);

    }

    @Test
    void enable_nonExist_feature() {
        var bcc = createBaseConfig("base-cfg-0.json");
        var hf = createHardForkConfig("hf-2.json");
        var afterHfTimestamp = new Date(hf.getTimestamp().toInstant().plusSeconds(1).toEpochMilli());
        //WHEN
        var config = GrapeChainConfig.from(bcc);
        //THEN
        assertNotNull(config);
        //WHEN
        config.addHardFork(hf);
        var ccf = new ChainConfigFactory(config);
        var ex = catchException(() -> ccf.configAt(afterHfTimestamp));
        //THEN
        assertThat(ex)
            .isInstanceOf(ConfigurationException.class)
            .hasMessage("Unknown feature: " + hf.getGfs().getEnableFeatures().get(0));
    }

    //test data
}