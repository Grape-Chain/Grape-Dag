package io.aplfintech.grape.config;

import lombok.extern.slf4j.Slf4j;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import java.util.Map;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.catchException;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
class GasPriceMapTest {
    private GasPriceMap gasPriceMap;
    private PriceItem priceItem;

    @BeforeEach
    void setUp() {
        priceItem = new PriceItem(2, "Gas base cost");
        gasPriceMap = new GasPriceMap(Map.of(
            "base", priceItem
            , "tierStep", new PriceItem(2, "Once per operation, for a selection of them")
            , "exp", new PriceItem(10, "Base fee of the EXP opcode")
        ));
    }

    @Test
    void mergeConfig() {
        //GIVEN
        var rGasPrice = new GasPriceMap(Map.of(
            "tierStep", new PriceItem(20, "Once per operation, for a selection of them")
            , "sha3", new PriceItem(15, "Base fee of the SHA3 opcode")
        ));
        //WHEN
        gasPriceMap.merge(rGasPrice);
        //THEN
        assertEquals(4, gasPriceMap.size());
        assertEquals(20, gasPriceMap.get("tierStep").getValue());
        assertEquals(2, gasPriceMap.get("base").getValue());
        assertEquals(10, gasPriceMap.get("exp").getValue());
        assertEquals(15, gasPriceMap.get("sha3").getValue());
    }

    @Test
    void lookFor() {
        //WHEN
        var rc = gasPriceMap.lookForGasPrice("base");
        //THEN
        assertEquals(priceItem.getValue(), rc);

        //GIVEN
        var itemKey = "UnknownPriceItem";
        // WHEN THEN
        var ex = catchException(() -> gasPriceMap.lookForGasPrice(itemKey));
        assertThat(ex)
            .isInstanceOf(IllegalStateException.class)
            .hasMessage("Can't locate the gas price for item=" + itemKey, ex.getMessage());
    }

    @Test
    void from() {
        //WHEN
        var gpm = GasPriceMap.from(gasPriceMap);
        //THEN
        assertTrue(gpm != gasPriceMap);
        assertThat(gpm)
            .containsExactlyEntriesOf(gasPriceMap);
    }

    @Test
    void tostring() {
        var rc = gasPriceMap.toString();
        assertThat(rc)
            .contains("gasPriceMap=")
            .contains("base")
            .contains("exp")
            .contains("tierStep");
    }
}