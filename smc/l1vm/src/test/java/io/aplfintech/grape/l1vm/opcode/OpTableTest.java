package io.aplfintech.grape.l1vm.opcode;

import io.aplfintech.grape.config.ChainConfig;
import io.aplfintech.grape.config.ChainConfigFactory;
import io.aplfintech.grape.config.ChainConfigLoader;
import io.aplfintech.grape.config.CryptoConfig;
import io.aplfintech.grape.config.GrapeChainConfig;
import io.aplfintech.grape.crypto.CryptoLib;
import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertDoesNotThrow;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
class OpTableTest {

    static GrapeChainConfig grapeChainConfig;
    static ChainConfig chainConfig;
    static CryptoLib crypto;


    @BeforeAll
    static void beforeAll() {
        var loader = new ChainConfigLoader("test-chain.json");
        grapeChainConfig = loader.load();
        var configFactory = new ChainConfigFactory(grapeChainConfig);
        chainConfig = configFactory.configAt(0);
        crypto = CryptoConfig.crypto();

    }

    @Test
    void create() {
        //GIVEN

        //WHEN
        var factory = OpTableFactory.newFactory(chainConfig, crypto);
        var table = factory.createTable();
        //THEN
        assertNotNull(table);
        assertDoesNotThrow(() -> OpCodes.validate(table.opCodes()));
        var gasPrice = chainConfig.gasPriceMap();
        assertNotNull(gasPrice, "Gas price must be set");
        assertEquals(92, gasPrice.size());
    }
}