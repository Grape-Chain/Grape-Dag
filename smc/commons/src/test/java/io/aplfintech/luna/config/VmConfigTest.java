package io.aplfintech.luna.config;

import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.CsvSource;

import static org.assertj.core.api.Assertions.assertThat;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
class VmConfigTest {

    @CsvSource(value = {
        ", , , , , "
        , "1, , , 2, 1, 2"
        , "1, 2, , , 1, 2"
        , "1, 2, , 3, 1, 3"
        , "1, 2, 3, 4, 3, 4"
    })
    @ParameterizedTest
    void mergeConfig(Integer l1, Integer l2, Integer r1, Integer r2, Integer t1, Integer t2) {
        //GIVEN
        var lCfg = new VmConfig(l1, l2);
        var rCfg = new VmConfig(r1, r2);
        var tCfg = new VmConfig(t1, t2);
        //WHEN
        lCfg.merge(rCfg);
        //THEN
        assertThat(lCfg)
            .isEqualTo(tCfg);
    }

    @CsvSource(value = {
        "   ,  ,  false"
        , "1,  ,  false"
        , " , 2,  false"
        , "1, 2,  true"
        , "0, 2,  false"
        , "1, 0,  false"
        , "-1, 2,  false"
        , "1, -1,  false"
    })
    @ParameterizedTest
    void isValid(Integer l1, Integer l2, boolean expected) {
        //GIVEN
        var lCfg = new VmConfig(l1, l2);
        //WHEN
        var rc = lCfg.isValid();
        //THEN
        assertEquals(expected, rc);
    }

    @Test
    void from() {
        //GIVEN
        var cfg1 = new VmConfig(1, 2);
        //WHEN
        var cfg2 = VmConfig.from(cfg1);
        //THEN
        assertTrue(cfg1 != cfg2, "Different instance");
        assertEquals(cfg1, cfg2, "The same object");
    }
}
