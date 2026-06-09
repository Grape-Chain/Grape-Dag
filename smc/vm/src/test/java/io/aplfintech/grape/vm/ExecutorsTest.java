package io.aplfintech.grape.vm;

import io.aplfintech.grape.bcei.DEI;
import io.aplfintech.grape.config.ChainConfig;
import io.aplfintech.grape.config.GasPriceMap;
import io.aplfintech.grape.config.PriceItem;
import io.aplfintech.grape.utils.TracerUtils;
import io.aplfintech.grape.vm.opcode.OpTable;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
class ExecutorsTest {
    ChainConfig chainConfig;
    OpTable opTable;

    @BeforeEach
    void setUp() {
        chainConfig = mock(ChainConfig.class);
        var priceMap = new GasPriceMap(Map.of("base", new PriceItem(2, "base"),
            "callStipend", new PriceItem(3, "callStipend")));
        when(chainConfig.gasPriceMap()).thenReturn(priceMap);
    }

    @Test
    void createEstimator() {
        //GIVEN
        var dei = mock(DEI.class);
        //WHEN
        var txExecutor = Executors.createEstimator(chainConfig, dei, TracerUtils.stdOutWriter());
        //THEN
        assertNotNull(txExecutor);
    }

}