package io.aplfintech.luna.config;

import org.junit.jupiter.api.Test;

import java.util.List;

import static io.aplfintech.luna.config.ConfigTestHelper.createFeatureConfig;
import static org.assertj.core.api.Assertions.assertThat;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
class FeatureConfigTest {
    @Test
    void config_with_int_properties() {
        var gf = createFeatureConfig("gf-bls12381.json");

        //WHEN
        var config = gf.getProperties();
        //THEN
        assertNotNull(config);
        assertThat(config).hasSize(1);
        //WHEN
        var values = config.get("blsDiscount");
        //THEN
        var expected = List.of(-1, 1200, 888, 764, 175, 175, 174);
        assertThat(values)
            .isNotNull()
            .returns(expected, PropertyItem::getIntValues);
        Integer[] rc = values.getIntValues().toArray(new Integer[0]);
        assertEquals(-1, rc[0]);
        assertEquals(1200, rc[1]);
        assertEquals(174, rc[6]);
    }

    @Test
    void config_without_int_properties() {
        var gf = createFeatureConfig("gf-wa.json");

        //WHEN
        var config = gf.getProperties();
        //THEN
        assertNotNull(config);
        assertThat(config).hasSize(1);
        //WHEN
        var prop1 = config.get("prop1");
        //THEN
        assertThat(prop1)
            .isNotNull()
            .returns("2", PropertyItem::getValue)
            .returns(null, PropertyItem::getIntValues);
    }

}