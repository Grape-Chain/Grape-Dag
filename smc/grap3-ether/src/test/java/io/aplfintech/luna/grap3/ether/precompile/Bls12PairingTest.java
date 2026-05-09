package io.aplfintech.luna.grap3.ether.precompile;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
class Bls12PairingTest extends PrecompiledContractTestBase {
    @BeforeEach
    void setUp() {
        setUpFuelOptimizationConfig();
    }

    @Test
    void runSuccess() {
        assertTrue(testJson("bls12pairing", new Bls12Pairing(chainConfig, crypto)));
    }

    @Test
    void runInvalid() {
        assertTrue(testJson("bls12pairing_invalid", new Bls12Pairing(chainConfig, crypto)));
    }

}