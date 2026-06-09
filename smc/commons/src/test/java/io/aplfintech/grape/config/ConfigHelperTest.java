package io.aplfintech.grape.config;

import lombok.SneakyThrows;
import org.junit.jupiter.api.Test;

import java.util.Date;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNull;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
class ConfigHelperTest {

    @Test
    void writeValueAsString() {
        //WHEN
        var rc = ConfigHelper.writeValueAsString(null);
        //THEN
        assertEquals("null", rc);
        //WHEN
        rc = ConfigHelper.writeValueAsString("abc");
        //THEN
        assertEquals("\"abc\"", rc);
    }

    @Test
    void readValue() {
        //WHEN
        var rc = ConfigHelper.readValue("null", String.class);
        //THEN
        assertNull(rc);
        //WHEN
        rc = ConfigHelper.readValue("\"abc\"", String.class);
        //THEN
        assertEquals("abc", rc);
    }

    @SneakyThrows
    @Test
    void timestampParser() {
        //GIVEN
        String ts1 = "2023-05-01T13:00:00";
        var timestamp = ConfigHelper.TIMESTAMP_FORMAT.parse(ts1);
        //WHEN
        var rc = ConfigHelper.writeValueAsString(timestamp);
        //THEN
        assertEquals("\"" + ts1 + "\"", rc);

        //WHEN
        var d = ConfigHelper.readValue(rc, Date.class);
        //THEN
        assertEquals(timestamp, d);
    }
}
