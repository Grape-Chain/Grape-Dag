package io.aplfintech.luna.config;

import com.github.fge.jsonpatch.diff.JsonDiff;
import io.aplfintech.luna.utils.FileUtils;
import lombok.SneakyThrows;
import org.junit.jupiter.api.Test;

import java.io.File;
import java.util.Map;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.catchException;
import static org.junit.jupiter.api.Assertions.assertDoesNotThrow;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
class GrapeChainConfigLoaderTest {
    GasConfig gasConfig = new GasConfig(21000, 1024, 2);
    VmConfig vmConfig = new VmConfig(1024, 24000);
    GrapeChainConfig chainCfg = new GrapeChainConfig(gasConfig, vmConfig, null, null, null);

    @Test
    void load() {
        var loader = new ChainConfigLoader("test-chain.json");
        //WHEN
        var cfg = loader.load();
        //THEN
        assertThat(cfg)
            .usingRecursiveComparison()
            .ignoringFieldsMatchingRegexes("gasPriceMap", "hardForks", "gfs")
            .isEqualTo(chainCfg);
        var gasPrice = cfg.getGasPriceMap();
        assertNotNull(gasPrice, "Gas price is present");
        assertEquals(6, gasPrice.size());
    }

    @Test
    void loadMissedResource() {
        String resourceName = "wrong_resource_name-chain.json";
        var loader = new ChainConfigLoader(resourceName);
        //WHEN
        var ex = catchException(loader::load);
        //THEN
        assertThat(ex)
            .isInstanceOf(ConfigurationException.class)
            .hasMessage("Can't load config, path=config" + File.separator + resourceName);
    }

    @SneakyThrows
    @Test
    void toJson() {
        String resourceName = "test-chain.json";
        var loader = new ChainConfigLoader(resourceName);
        var chainJson = FileUtils.readResourceContent(loader.getResourceName());
        var expected = ConfigHelper.MAPPER.readTree(chainJson);
        //WHEN
        var cfg = loader.load();
        //THEN
        var rc = assertDoesNotThrow(() -> ConfigHelper.writeValueAsString(cfg));
        var jsonTree = ConfigHelper.MAPPER.readTree(rc);
        var diff = JsonDiff.asJson(jsonTree, expected);
        assertNotNull(diff);
        assertTrue(diff.isEmpty(), "The JSON trees must don't have differs");
    }

    @SneakyThrows
    @Test
    void checkConfigSerializeDeserialize() {
        //GIVEN
        var gasPrice = new GasPriceMap(Map.of(
            "base", new PriceItem(2, "Gas base cost")
            , "tierStep", new PriceItem(2, "Once per operation, for a selection of them")
            , "exp", new PriceItem(10, "Base fee of the EXP opcode")
        ));
        chainCfg.setGasPriceMap(gasPrice);

        var json = ConfigHelper.writeValueAsString(chainCfg);

        var cfg = ConfigHelper.readValue(json, GrapeChainConfig.class);

        assertThat(cfg)
            .usingRecursiveComparison()
            .isEqualTo(chainCfg);
    }

}