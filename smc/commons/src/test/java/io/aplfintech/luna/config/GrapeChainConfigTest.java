package io.aplfintech.luna.config;

import lombok.SneakyThrows;
import org.junit.jupiter.api.Test;

import java.sql.Date;
import java.time.LocalDateTime;
import java.util.List;
import java.util.Map;

import static java.time.ZoneOffset.UTC;
import static org.assertj.core.api.Assertions.assertThat;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
class GrapeChainConfigTest {
    @SneakyThrows
    @Test
    void checkConfigSerializeDeserialize() {
        //GIVEN
        var gasConfig = new GasConfig(5000, 1024, 2);
        var vmConfig = new VmConfig(1024, 24576);
        var gasPrice = new GasPriceMap(Map.of(
            "base", new PriceItem(2, "Gas base cost")
            , "tierStep", new PriceItem(2, "Once per operation, for a selection of them")
            , "exp", new PriceItem(10, "Base fee of the EXP opcode")
        ));
        var dt = Date.from(LocalDateTime.of(2023, 4, 1, 15, 0, 0).atZone(UTC).toInstant());
        var hardForks = List.of(
            new HardForkConfig("firstFork", dt, null, null)
        );
        var chainCfg = new GrapeChainConfig(gasConfig, vmConfig, gasPrice, hardForks, null);

        var json = ConfigHelper.writeValueAsString(chainCfg);

        var cfg = ConfigHelper.readValue(json, GrapeChainConfig.class);

        assertThat(cfg)
            .usingRecursiveComparison()
            .isEqualTo(chainCfg);
    }
}