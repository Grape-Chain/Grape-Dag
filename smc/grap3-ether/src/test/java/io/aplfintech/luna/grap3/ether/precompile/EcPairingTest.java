package io.aplfintech.luna.grap3.ether.precompile;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
class EcPairingTest extends PrecompiledContractTestBase {
    @BeforeEach
    void setUp() {
        setUpFuelOptimizationConfig();
    }

    @Test
    void run() {
        assertTrue(testJson("ecpairing", new EcPairing(price, crypto)));
    }

}