package io.aplfintech.grape.utils;

import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.Arguments;
import org.junit.jupiter.params.provider.MethodSource;
import org.junit.jupiter.params.provider.ValueSource;

import java.math.BigInteger;
import java.util.stream.Stream;

import static org.assertj.core.api.Assertions.assertThat;
import static org.junit.jupiter.api.Assertions.assertArrayEquals;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.params.provider.Arguments.arguments;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
class BytesTest {

    @MethodSource("leftPaddedBytes")
    @ParameterizedTest
    void leftPadding(int length, byte[] bytes, byte[] expected) {
        //WHEN
        var rc = Bytes.leftPadBytes(bytes, length);
        //THEN
        assertThat(rc)
            .isEqualTo(expected);
    }

    @MethodSource("leftPaddedFFBytes")
    @ParameterizedTest
    void leftPadding_FF(int length, byte[] bytes, byte[] expected) {
        //WHEN
        var rc = Bytes.leftPadBytes(bytes, length, (byte) 0xff);
        //THEN
        assertThat(rc)
            .isEqualTo(expected);
    }

    @MethodSource("rightPaddedBytes")
    @ParameterizedTest
    void rightPadding(int length, byte[] bytes, byte[] expected) {
        //WHEN
        var rc = Bytes.rightPadBytes(bytes, length);
        //THEN
        assertThat(rc)
            .isEqualTo(expected);
    }

    @MethodSource("leftZerosBytes")
    @ParameterizedTest
    void trimLeftZeros(byte[] bytes, byte[] expected) {
        //WHEN
        var rc = Bytes.trimLeftZeros(bytes);
        //THEN
        assertThat(rc)
            .isEqualTo(expected);
    }

    @MethodSource("rightZerosBytes")
    @ParameterizedTest
    void trimRightZeros(byte[] bytes, byte[] expected) {
        //WHEN
        var rc = Bytes.trimRightZeros(bytes);
        //THEN
        assertThat(rc)
            .isEqualTo(expected);
    }

    @ParameterizedTest
    @ValueSource(strings = {"123456789", "d6407697beced", "41d6e1396b8ebe2", "b5a1ea4dfa666", "2", "0", "00"})
    void toLong(String value) {
        //GIVEN
        var bi = new BigInteger(value, 16);
        byte[] bytes = bi.toByteArray();
        //WHEN
        var rc = Bytes.toLong(bytes);
        //THEN
        assertEquals(bi.longValueExact(), rc);
    }

    @Test
    void slice() {
        //GIVEN
        byte[] data = HexUtils.fromHex("0102030405060708090a0b0c0d0e0f");
        byte[] slice0 = new byte[0];
        byte[] slice1 = HexUtils.fromHex("0102030405");
        byte[] slice2 = HexUtils.fromHex("0405060708090a0b0c");
        byte[] slice3 = HexUtils.fromHex("08090a0b0c0d0e0f");
        //WHEN //THEN
        assertArrayEquals(new byte[]{2}, Bytes.slice(data, 1, 2));
        assertArrayEquals(slice1, Bytes.slice(data, 0, 5));
        assertArrayEquals(slice2, Bytes.slice(data, 3, 12));
        assertArrayEquals(slice3, Bytes.slice(data, 7, 15));
        assertArrayEquals(slice3, Bytes.slice(data, 7));
        assertArrayEquals(slice0, Bytes.slice(data, 1, 1));
        assertArrayEquals(slice0, Bytes.slice(data, 15, 15));
        assertArrayEquals(slice0, Bytes.slice(slice0, 0, 0));
        var ex0 = assertThrows(IndexOutOfBoundsException.class, () -> Bytes.slice(data, 16, 16));
        assertThat(ex0)
            .hasMessage("index (16) must not be greater than size (15)");
        var ex1 = assertThrows(IndexOutOfBoundsException.class, () -> Bytes.slice(data, 1, 16));
        assertThat(ex1)
            .hasMessage("index (16) must not be greater than size (15)");
        var ex2 = assertThrows(IndexOutOfBoundsException.class, () -> Bytes.slice(data, -1, 2));
        assertThat(ex2)
            .hasMessage("index (-1) must not be negative");
        var ex3 = assertThrows(IndexOutOfBoundsException.class, () -> Bytes.slice(data, 16));
        assertThat(ex3)
            .hasMessage("index (16) must be less than size (15)");
        var ex4 = assertThrows(NullPointerException.class, () -> Bytes.slice(null, 5, 1));
        assertThat(ex4)
            .hasMessage("input is marked non-null but is null");
    }

    @Test
    void slicePadded() {
        //GIVEN
        byte[] data = HexUtils.fromHex("0102030405060708090a0b0c0d0e0f");
        byte[] slice0 = new byte[0];
        byte[] slice1 = HexUtils.fromHex("0102030405");
        byte[] slice2 = HexUtils.fromHex("0405060708090a0b0c");
        byte[] slice3 = HexUtils.fromHex("08090a0b0c0d0e0f");
        byte[] slice4 = HexUtils.fromHex("00000000");
        //WHEN //THEN
        assertArrayEquals(slice0, Bytes.slicePadded(data, 0, 0));
        assertArrayEquals(slice0, Bytes.slicePadded(data, 15, 0));
        assertArrayEquals(slice0, Bytes.slicePadded(data, 32, 0));

        assertArrayEquals(slice4, Bytes.slicePadded(data, 15, 4));
        assertArrayEquals(slice4, Bytes.slicePadded(data, 32, 4));

        assertArrayEquals(slice1, Bytes.slicePadded(data, 0, 5));
        assertArrayEquals(slice2, Bytes.slicePadded(data, 3, 9));
        assertArrayEquals(slice3, Bytes.slicePadded(data, 7, 8));
    }

    @Test
    void concat() {
        //GIVEN
        byte[] expected = HexUtils.fromHex("0102030405060708090a0b0c0d0e0f");
        byte[][] input = new byte[][]{
            new byte[0],
            HexUtils.fromHex("0102030405"),
            HexUtils.fromHex("06070809"),
            new byte[0],
            HexUtils.fromHex("0a0b0c0d0e"),
            new byte[]{0x0f}
        };
        //WHEN
        var rc = Bytes.concat(input);
        // THEN
        assertThat(rc)
            .isEqualTo(expected);
    }

    public static Stream<Arguments> leftZerosBytes() {
        return Stream.of(
            arguments(new byte[]{}, new byte[]{}),
            arguments(new byte[]{0x01}, new byte[]{0x01}),
            arguments(new byte[]{0x0, 0xa}, new byte[]{0xa}),
            arguments(new byte[]{0xf, 0x0, 0x0, 0x0, (byte) 0xf0}, new byte[]{0xf, 0x0, 0x0, 0x0, (byte) 0xf0}),
            arguments(new byte[]{0x0, 0x0, 0x0}, new byte[]{}),
            arguments(new byte[]{0x0, 0x0, 0x0, 0xf}, new byte[]{0xf})
        );
    }

    public static Stream<Arguments> rightZerosBytes() {
        return Stream.of(
            arguments(new byte[]{}, new byte[]{}),
            arguments(new byte[]{0x10}, new byte[]{0x10}),
            arguments(new byte[]{(byte) 0xa0, 0x0}, new byte[]{(byte) 0xa0}),
            arguments(new byte[]{0xf, 0x0, 0x0, 0x0, (byte) 0xf0}, new byte[]{0xf, 0x0, 0x0, 0x0, (byte) 0xf0}),
            arguments(new byte[]{0x0, 0x0, 0x0}, new byte[]{}),
            arguments(new byte[]{0xf, 0x0, 0x0, 0x0, 0x0}, new byte[]{0xf})
        );
    }

    public static Stream<Arguments> leftPaddedBytes() {
        return Stream.of(
            arguments(0, new byte[]{}, new byte[]{}),
            arguments(0, new byte[]{0xf, 0xf}, new byte[]{}),
            arguments(1, new byte[]{0x10}, new byte[]{0x10}),
            arguments(4, new byte[]{}, new byte[]{0x0, 0x0, 0x0, 0x0}),
            arguments(4, new byte[]{0xa}, new byte[]{0x0, 0x0, 0x0, 0xa}),
            arguments(16, new byte[]{0xf, 0xf}, new byte[]{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf, 0xf}),
            arguments(8, new byte[]{0x1, 0x2, 0x3, 0x4, 0x5, 0x6, 0x7, 0x8, 0x9, 0xa, 0xb, 0xc, 0xd, 0xe, 0xf, 0x10}, new byte[]{0x9, 0xa, 0xb, 0xc, 0xd, 0xe, 0xf, 0x10}),
            arguments(1, new byte[]{0xf, 0xe, 0xd, 0xc, 0xb}, new byte[]{0xb})
        );
    }

    public static Stream<Arguments> rightPaddedBytes() {
        return Stream.of(
            arguments(0, new byte[]{}, new byte[]{}),
            arguments(0, new byte[]{0xf, 0xf}, new byte[]{}),
            arguments(1, new byte[]{0x10}, new byte[]{0x10}),
            arguments(4, new byte[]{}, new byte[]{0x0, 0x0, 0x0, 0x0}),
            arguments(4, new byte[]{0xa}, new byte[]{0xa, 0x0, 0x0, 0x0}),
            arguments(16, new byte[]{0xf, 0xf}, new byte[]{0xf, 0xf, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}),
            arguments(8, new byte[]{0x1, 0x2, 0x3, 0x4, 0x5, 0x6, 0x7, 0x8, 0x9, 0xa, 0xb, 0xc, 0xd, 0xe, 0xf, 0x10}, new byte[]{0x1, 0x2, 0x3, 0x4, 0x5, 0x6, 0x7, 0x8}),
            arguments(1, new byte[]{0xf, 0xe, 0xd, 0xc, 0xb}, new byte[]{0xf})
        );
    }

    public static Stream<Arguments> leftPaddedFFBytes() {
        return Stream.of(
            arguments(0, new byte[]{}, new byte[]{}),
            arguments(0, new byte[]{0xf, 0xf}, new byte[]{}),
            arguments(1, new byte[]{0x10}, new byte[]{0x10}),
            arguments(4, new byte[]{}, new byte[]{(byte) 0xff, (byte) 0xff, (byte) 0xff, (byte) 0xff}),
            arguments(4, new byte[]{0xa}, new byte[]{(byte) 0xff, (byte) 0xff, (byte) 0xff, (byte) 0xa}),
            arguments(16, new byte[]{0xf, 0xf}, new byte[]{(byte) 0xff, (byte) 0xff, (byte) 0xff, (byte) 0xff, (byte) 0xff, (byte) 0xff, (byte) 0xff, (byte) 0xff, (byte) 0xff, (byte) 0xff, (byte) 0xff, (byte) 0xff, (byte) 0xff, (byte) 0xff, 0xf, 0xf}),
            arguments(8, new byte[]{0x1, 0x2, 0x3, 0x4, 0x5, 0x6, 0x7, 0x8, 0x9, 0xa, 0xb, 0xc, 0xd, 0xe, 0xf, 0x10}, new byte[]{0x9, 0xa, 0xb, 0xc, 0xd, 0xe, 0xf, 0x10}),
            arguments(1, new byte[]{0xf, 0xe, 0xd, 0xc, 0xb}, new byte[]{0xb})
        );
    }

}
