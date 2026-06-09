package io.aplfintech.grape.grap3.ether.precompile;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
class EcRecoverTest extends PrecompiledContractTestBase {

    @BeforeEach
    void setUp() {
        setUpInitialConfig();
    }

    @Test
    void runContract() {
        assertTrue(testJson("ecrecover", new EcRecover(price, crypto)));
    }
}