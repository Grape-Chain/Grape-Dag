package io.aplfintech.luna.utils;

import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.Arguments;
import org.junit.jupiter.params.provider.MethodSource;
import org.junit.jupiter.params.provider.ValueSource;

import java.math.BigInteger;
import java.util.stream.Stream;

import static org.assertj.core.api.Assertions.assertThat;
import static org.junit.jupiter.api.Assertions.assertArrayEquals;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.params.provider.Arguments.arguments;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
class HexUtilsTest {
    @ParameterizedTest
    @ValueSource(strings = {"d6407697be282ced", "0xd6407697be282ced", "D6407697BE282CED", "0xD6407697BE282CED"})
    void getLongId(String value) {
        //GIVEN
        //WHEN
        byte[] bytes = HexUtils.parseHex(value);

        //THEN
        assertEquals("d6407697be282ced", HexUtils.toHex(bytes));
        assertEquals(-3008274156981048083L, new BigInteger(bytes).longValue());
    }

    @ParameterizedTest
    @ValueSource(strings = {"0", "0x0", "00", "0x00"})
    void getZero(String value) {
        //GIVEN
        //WHEN
        byte[] bytes = HexUtils.parseHex(value);

        //THEN
        assertArrayEquals(new byte[]{0}, bytes);
    }

    @MethodSource("bytesAndHexProvider")
    @ParameterizedTest
    void toHex(byte[] bytes, String expected) {
        //WHEN
        var rc = HexUtils.toHex(bytes, false);
        assertThat(rc)
            .isEqualTo(expected);
    }

    @MethodSource("bytesAndHexProvider")
    @ParameterizedTest
    void fromHex(byte[] expected, String hex) {
        //WHEN prefixed
        var rc = HexUtils.fromHex(hex);
        //THEN
        assertThat(rc)
            .isEqualTo(expected);
    }

    @MethodSource("bytesAndHexNotEvenProvider")
    @ParameterizedTest
    void fromNotEvenHex(byte[] expected, String notEvenHex) {
        //WHEN
        var rc = HexUtils.fromHex(notEvenHex);
        //THEN
        assertThat(rc)
            .isEqualTo(expected);

    }

    static Stream<Arguments> bytesAndHexProvider() {
        return Stream.of(
            arguments(new byte[]{}, ""),
            arguments(new byte[]{0}, "00"),
            arguments(new byte[]{1}, "01"),
            arguments(new byte[]{1, 2, 3}, "010203"),
            arguments(new byte[]{0x1a, 0x2b, 0x3c, 0x4d, 0x5e, 0x6f}, "1a2b3c4d5e6f")
        );
    }

    static Stream<Arguments> bytesAndHexNotEvenProvider() {
        return Stream.of(
            arguments(new byte[]{}, ""),
            arguments(new byte[]{0}, "0"),
            arguments(new byte[]{1}, "1"),
            arguments(new byte[]{1, 0x23}, "123"),
            arguments(new byte[]{1, 0x23, 0x45}, "12345")
        );
    }

}