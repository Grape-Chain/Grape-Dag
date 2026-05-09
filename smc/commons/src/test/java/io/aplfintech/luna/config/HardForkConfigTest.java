package io.aplfintech.luna.config;

import lombok.SneakyThrows;
import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
class HardForkConfigTest {
    @SneakyThrows
    @Test
    void isEnableAt() {
        //GIVEN
        var forkDate = ConfigHelper.parseTimestamp("2023-01-01T13:00:00");
        var dateBefore = ConfigHelper.parseTimestamp("2023-01-01T10:00:00");
        var dateAfter = ConfigHelper.parseTimestamp("2023-01-01T13:00:01");
        var cfg = new HardForkConfig("fork1", forkDate, null, null);
        //WHEN
        //THEN
        assertFalse(cfg.isEnabledAt(dateBefore), "Fork disabled");
        assertFalse(cfg.isEnabledAt(forkDate), "Fork disabled");
        assertTrue(cfg.isEnabledAt(dateAfter), "Fork enabled");
    }
}