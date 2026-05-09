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
class GasConfigTest {
    @CsvSource(value = {
        "   ,  ,  ,     ,  ,  ,    , ,   "
        , "1,  ,  ,     ,  , 2,   1,  , 2"
        , "1,  ,  ,     , 1, 2,   1, 1, 2"
        , "1, 2,  ,     , 1, 2,   1, 1, 2"
        , "1, 2, 3,     , 5, 6,   1, 5, 6"
        , "1, 2, 3,    4, 5, 6,   4, 5, 6"
    })
    @ParameterizedTest
    void mergeConfig(Integer l1, Integer l2, Integer l3, Integer r1, Integer r2, Integer r3, Integer t1, Integer t2, Integer t3) {
        //GIVEN
        var lCfg = new GasConfig(l1, l2, l3);
        var rCfg = new GasConfig(r1, r2, r3);
        var tCfg = new GasConfig(t1, t2, t3);
        //WHEN
        lCfg.merge(rCfg);
        //THEN
        assertThat(lCfg)
            .isEqualTo(tCfg);

    }

    @CsvSource(value = {
        "   ,  ,  ,  false"
        , "1,  ,  ,  false"
        , "1, 2,  ,  false"
        , "1, 2, 3,  true"
        , "0, 2, 3,  false"
        , "1, 0, 3,  false"
        , "1, 2, 0,  false"
        , "-1, 2, 3,  false"
        , "1, -1, 3,  false"
        , "1, 2, -1,  false"
    })
    @ParameterizedTest
    void isValid(Integer l1, Integer l2, Integer l3, boolean expected) {
        //GIVEN
        var lCfg = new GasConfig(l1, l2, l3);
        //WHEN
        var rc = lCfg.isValid();
        //THEN
        assertEquals(expected, rc);
    }

    @Test
    void from() {
        //GIVEN
        var cfg1 = new GasConfig(1, 2, 3);
        //WHEN
        var cfg2 = GasConfig.from(cfg1);
        //THEN
        assertTrue(cfg1 != cfg2, "Different instance");
        assertEquals(cfg1, cfg2, "The same object");
    }
}