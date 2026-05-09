package io.aplfintech.luna.utils;

import io.aplfintech.luna.math.Math256;
import io.aplfintech.luna.utils.HexUtils;
import lombok.extern.slf4j.Slf4j;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.CsvSource;

import java.math.BigInteger;

import static org.assertj.core.api.Assertions.assertThat;
import static org.junit.jupiter.api.Assertions.assertEquals;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
class Math256Test {

    @CsvSource(value = {
        "1,2,1",
        "5,4,2",
        "9,1,9",
        "9,2,5",
        "9,3,3",
        "0,1,0",
        "1, 32, 1",
        "16, 32, 1",
        "33, 32, 2"
    })
    @ParameterizedTest
    void divCeil(long a, int b, long r) {
        //WHEN
        var rc = Math256.divCeil(a, b);
        //THEN
        assertEquals(r, rc);
    }

    @CsvSource(value = {
        "0, 0",
        "1, 1",
        "8, 1",
        "16, 1",
        "32, 1",
        "33, 2",
        "64, 2",
        "65, 3",
        "96, 3",
        "97, 4",
        "128, 4",
        "129, 5",
        "255, 8",
        "256, 8",
        "257, 9"
    })
    @ParameterizedTest
    void numWords(long a, long r) {
        //WHEN
        var rc = Math256.toWordSize(a);
        //THEN
        assertEquals(r, rc);
    }

    @CsvSource(value = {
        "0, 1, 0",
        "1, 1, 1",
        "9, 1, 9",
        "1, 2, 2",
        "9, 2, 10",
        "9, 3, 9",
        "5, 4, 8",
        "1, 32, 32",
        "16, 32 ,32",
        "32, 32 ,32",
        "33, 32, 64"
    })
    @ParameterizedTest
    void ceil(int a, int b, int r) {
        //WHEN
        var rc = Math256.ceil(a, b);
        //THEN
        assertEquals(r, rc);
    }

    @CsvSource(value = {
        "0, 0"
        , "01, 1"
        , "0f, 1"
        , "010, 1"
        , "0aa, 1"
        , "0aa1, 2"
        , "12345, 3"
        , "123456, 3"
        , "aabbccd, 4"

    })
    @ParameterizedTest
    void byteLength(String value, int expected) {
        //GIVEN
        var bigint = new BigInteger(value, 16);
        //WHEN
        var byteCount = Math256.byteLength(bigint);
        //THEN
        assertEquals(expected, byteCount);
    }

    @CsvSource(value = {
        "0, 0, 0",
        "0, 0x133f6a, 0x6a",//extend sign of positive 6a
        "1, 0x136af3, 0x6af3",//extend sign of positive 6af4
        "1, 0x13faf3, 0xfffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffaf3",//extend faf3 sign
        "0, 0x133ff3, 0xfffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff3",//extend f3 sign
        "31, 1, 0x01",//doesn't do anything, because value is positive
        "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0x0, 0x0",//byte num overflow (-1)
        "0xf0000000000001, 0xffff, 0xffff",//byte num overflow
        "0xf00000000000000001, 0xff, 0xff",//byte num overflow
        "0x010000000000000001, 0x8000, 0x8000",//byte num overflow
        "60, 0x135af3, 0x135af3",////byte num overflow
        "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",//byte num overflow
        "31, 0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",// signExt(31, sub(0,1))
        "0, 0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"// signExt(0, sub(0,1))
    })
    @ParameterizedTest
    void signExt(String sArg, String vArg, String zArg) {
        //GIVEN
        var s = Math256.uint256(HexUtils.parseHex(sArg));
        var value = Math256.uint256(HexUtils.parseHex(vArg));
        var expected = Math256.uint256(HexUtils.parseHex(zArg));
        //WHEN
        var rc = Math256.signExt(value, s);
        //THEN
        assertThat(rc)
            .isEqualTo(expected);
    }

    @Test
    void getByte() {
        //GIVEN
        var v = Math256.uint256(5);
        var s = 31;
        var expected = Math256.uint256(5);
        //WHEN
        var rc = Math256.getByte(s, v);
        //THEN
        assertThat(rc)
            .isEqualTo(expected);
    }

    @CsvSource(value = {"0, 0", "123, 123", "-12345, -12345"})
    @ParameterizedTest
    void int256(long value, long expect) {
        //GIVEN
        var bytes = BigInteger.valueOf(value).toByteArray();
        var expected = BigInteger.valueOf(expect);

        //WHEN
        var rc = Math256.int256(bytes);
        //THEN
        assertThat(rc.bigIntegerValue())
            .isEqualTo(expected);
    }

    @CsvSource(value = {"0, 0", "123, 123", "-1, 255", "-12345, 53191"})
    @ParameterizedTest
    void uint256(long value, long expect) {
        //GIVEN
        var v = BigInteger.valueOf(value);
        var expected = BigInteger.valueOf(expect);
        var bytes = v.toByteArray();
        //WHEN
        var rc = Math256.uint256(bytes);
        //THEN
        assertThat(rc.bigIntegerValue())
            .isEqualTo(expected);
    }
}