package io.aplfintech.grape.l1vm.precompiled;

import io.aplfintech.grape.config.ChainConfigFactory;
import io.aplfintech.grape.config.ChainConfigLoader;
import io.aplfintech.grape.config.CryptoConfig;
import io.aplfintech.grape.config.GrapeChainConfig;
import io.aplfintech.grape.crypto.CryptoLib;
import io.aplfintech.grape.exception.VmException;
import io.aplfintech.grape.l1vm.VmAddress;
import io.aplfintech.grape.l1vm.VmImpl;
import io.aplfintech.grape.l1vm.contract.GasPool;
import io.aplfintech.grape.vm.PrecompiledFn;
import io.aplfintech.grape.utils.HexUtils;
import lombok.extern.slf4j.Slf4j;
import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.catchException;
import static org.junit.jupiter.api.Assertions.*;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
class PrecompiledFactoryTest {
    static GrapeChainConfig grapeChainConfig;
    static CryptoLib crypto;
    static PrecompiledFn[] precompiledFns;

    @BeforeAll
    static void beforeAll() {
        crypto = CryptoConfig.crypto();
        var chainConfigLoader = new ChainConfigLoader("test-chain.json");
        grapeChainConfig = chainConfigLoader.load();
        var configFactory = new ChainConfigFactory(grapeChainConfig);
        var chainConfig = configFactory.configAt(0);
        precompiledFns = PrecompiledContracts.createPrecompiledContracts(chainConfig, crypto);
    }

    @Test
    void factory() {
        assertEquals(19, precompiledFns.length, "There are 18 precompiled contracts, 0x00 address is not used.");
        assertNull(precompiledFns[0], "The address 0x00 is reserved");
        for (byte i = 1; i < precompiledFns.length; i++) {
            var fn = precompiledFns[i];
            assertNotNull(fn, "The precompiled contract must be defined for address " + HexUtils.toHex(i, true));
        }
    }

    @Test
    void testInitialUnsupported() {
        checkUnsupported(new int[]{0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x11, 0x12});
    }

    private static void checkUnsupported(int[] contracts) {
        for (int idx : contracts) {
            var fn = precompiledFns[idx];
            String addr = HexUtils.toHex(idx);
            var ex = catchException(() -> VmImpl.runPrecompiledContract(VmAddress.from(addr), fn, null, new GasPool(0), addr));
            assertThat(ex)
                    .isInstanceOf(VmException.class)
                    .hasMessageContaining("not implemented yet");
        }
    }

}