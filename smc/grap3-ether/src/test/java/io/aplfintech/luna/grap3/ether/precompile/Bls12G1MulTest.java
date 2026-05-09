package io.aplfintech.luna.grap3.ether.precompile;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
class Bls12G1MulTest extends PrecompiledContractTestBase {
    @BeforeEach
    void setUp() {
        setUpFuelOptimizationConfig();
    }

    @Test
    void run() {
        assertTrue(testJson("bls12g1_mul", new Bls12G1Mul(chainConfig, crypto)));
    }

}