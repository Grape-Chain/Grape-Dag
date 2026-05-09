package io.aplfintech.luna.grap3.ether.precompile;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
class Sha256HashTest extends PrecompiledContractTestBase {

    @BeforeEach
    void setUp() {
        setUpInitialConfig();
    }

    @Test
    void run() {
        assertTrue(testJson("sha256", new Sha256Hash(price, crypto)));
    }
}