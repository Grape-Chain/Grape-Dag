package io.aplfintech.luna.grap3.crypto.utils;

import org.junit.jupiter.api.Test;

import java.nio.charset.StandardCharsets;
import java.util.Arrays;

import static org.junit.jupiter.api.Assertions.assertArrayEquals;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;

/**
 * @author andrew.zinchenko@gmail.com
 */
class ScryptUtilsTest {

    @Test
    void encrypt() {
        //GIVEN
        var data = "test data 1234567890".getBytes(StandardCharsets.UTF_8);
        var password = "qweasdzxc".getBytes(StandardCharsets.UTF_8);
        //WHEN
        var spec = ScryptUtils.encryptData(data, password);
        //THEN
        assertFalse(Arrays.equals(data, spec.getCipherText()), "Encrypted data doesn't equal input data");
        assertEquals(data.length + 16, spec.getCipherText().length);
        //WHEN
        var decrypted = ScryptUtils.decryptData(spec, password);
        //THEN
        assertArrayEquals(data, decrypted);
    }

}