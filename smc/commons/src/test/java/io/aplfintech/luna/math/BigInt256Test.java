package io.aplfintech.luna.math;

import io.aplfintech.luna.utils.Bytes;
import io.aplfintech.luna.utils.HexUtils;
import lombok.extern.slf4j.Slf4j;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.CsvSource;

import static io.aplfintech.luna.utils.HexArgs.get32Bytes;
import static io.aplfintech.luna.utils.HexArgs.uintFromHexArg;
import static io.aplfintech.luna.utils.HexUtils.toBin;
import static io.aplfintech.luna.utils.HexUtils.toHex;
import static org.assertj.core.api.Assertions.assertThat;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
class BigInt256Test {

    @CsvSource(value = {
        "0, 0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, true"
        , "0xf, 0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff00ff, false"
        , "0x10, 0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff00ff, true"
        , "0xff, 0x8000000000000000000000000000000000000000000000000000000000000000, true"
    })
    @ParameterizedTest
    void isBitSet(String hexBit, String hexValue, boolean expected) {
        //GIVEN
        var bit = uintFromHexArg(hexBit).intValue();
        var v = uintFromHexArg(hexValue);
        //WHEN
        var rc = v.isBitSet(bit);
        //THEN
        assertThat(rc)
            .isEqualTo(expected);
    }

    @CsvSource(value = {
        "0,0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
        , "0xff00,0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff00ff"
        , "0x1234567800,0xffffffffffffffffffffffffffffffffffffffffffffffffffffffedcba987ff"
        , "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff00ff, 0xff00"
        , "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffedcba987ff, 0x1234567800"
    })
    @ParameterizedTest
    void op_not(String hex, String expectedHex) {
        //GIVEN
        var v = uintFromHexArg(hex);
        var expected = uintFromHexArg(expectedHex);
        //WHEN
        var rc = v.not();
        //THEN
        log.debug("v={}, not v={}", v.asWord().hex(), rc.asWord().hex());
        assertThat(rc)
            .isEqualTo(expected);
    }

    @CsvSource(value = {"0, 0, 0", "-1, 0, -1", "0, -1, -1", "-1, -1, -2", "4, -1, 3",
        "0x8000000000000000000000000000000000000000000000000000000000000000, 1, 0x8000000000000000000000000000000000000000000000000000000000000001"
    })
    @ParameterizedTest
    void add(String xArg, String yArg, String zArg) {
        //GIVEN
        var x = uintFromHexArg(xArg);
        var y = uintFromHexArg(yArg);
        var z = uintFromHexArg(zArg);
        //WHEN
        var rc = x.add(y);

        //THEN
        assertThat(rc)
            .isEqualTo(z);
    }

    @CsvSource(value = {"23, 1, 22", "2, 3, -1", "0, 23, -23", "0, -1, 1", "-1, 0, -1",
        "0x8000000000000000000000000000000000000000000000000000000000000001, 1, 0x8000000000000000000000000000000000000000000000000000000000000000",
        "0x8000000000000000000000000000000000000000000000000000000000000000, 1, 0x7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
        "0, 1, 0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
    })
    @ParameterizedTest
    void sub(String xArg, String yArg, String zArg) {
        //GIVEN
        var x = uintFromHexArg(xArg);
        var y = uintFromHexArg(yArg);
        var z = uintFromHexArg(zArg);
        //WHEN
        var rc = x.sub(y);

        //THEN
        assertThat(rc)
            .isEqualTo(z);
    }

    @CsvSource(value = {
        "-0x7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0x7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
        "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, -0x7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0",
        "0x8000000000000000000000000000000000000000000000000000000000000000, 0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0x8000000000000000000000000000000000000000000000000000000000000000",
        "0x8000000000000000000000000000000000000000000000000000000000000000, -1, 0x8000000000000000000000000000000000000000000000000000000000000000",
        "-0x8000000000000000000000000000000000000000000000000000000000000000, -1, 0x8000000000000000000000000000000000000000000000000000000000000000",
        "-0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, -1, 0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
        "-1, 1, -1",
        "-2, -4, 0",
        "4, -2, -2",
        "5, -4, -1",
        "-1, 25, 0",
        "-1, -1, 1",
        "-1, 1, -1",
        "0, 0, 0",
        "1, 0, 0",
        "-3, 0, 0",
        "-0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0, 0"
    })
    @ParameterizedTest
    void sdiv(String xArg, String yArg, String zArg) {
        //GIVEN
        var x = uintFromHexArg(xArg);
        var y = uintFromHexArg(yArg);
        var z = uintFromHexArg(zArg);
        //WHEN
        var rc = x.sdiv(y);

        //THEN
        assertThat(rc)
            .isEqualTo(z);
    }

    @CsvSource(value = {
        "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 2, -1",
        "0x8000000000000000000000000000000000000000000000000000000000000000, 0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0",
        "0x8000000000000000000000000000000000000000000000000000000000000000, -0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0",
        "-0x8000000000000000000000000000000000000000000000000000000000000000, -1, 0",
        "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0x8000000000000000000000000000000000000000000000000000000000000000, 0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
        "-0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, -0x8000000000000000000000000000000000000000000000000000000000000000, -0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
        "-2, 3, -2",
        "2, 3, 2",
        "0, -1, 0",
        "0, 0, 0",
        "1, 0, 0",
        "3, 0, 0",
        "-3, 0, 0"
    })
    @ParameterizedTest
    void smod(String xArg, String yArg, String zArg) {
        //GIVEN
        var x = uintFromHexArg(xArg);
        var y = uintFromHexArg(yArg);
        var z = uintFromHexArg(zArg);
        //WHEN
        var rc = x.smod(y);

        //THEN
        assertThat(rc)
            .isEqualTo(z);
    }

    @CsvSource(value = {"23, -23", "1, -1", "0, 0", "-2, 2", "-1, 1", "-255, 255"})
    @ParameterizedTest
    void neg_int(long xArg, long zArg) {
        //GIVEN
        var x = Math256.int256(xArg);
        byte[] z32 = get32Bytes(zArg);
        //WHEN
        var rc = x.neg().asWord().bytes32();
        //THEN
        assertThat(rc)
            .isEqualTo(z32);
    }

    @CsvSource(value = {"0x23, 0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffdd",
        "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffdd, 0x23",
        "1, 0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
        "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0x01",
        "0, 0",
        "ff, 0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff01",
        "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff01, 0x00000000000000000000000000000000000000000000000000000000000000ff",
        "0x12345, 0xfffffffffffffffffffffffffffffffffffffffffffffffffffffffffffedcbb",
        "0x8000000000000000000000000000000000000000000000000000000000000000,0x8000000000000000000000000000000000000000000000000000000000000000"//2^255 == -(2^255)
    })
    @ParameterizedTest
    void neg_uint(String xArg, String zArg) {
        //GIVEN
        var x = uintFromHexArg(xArg);
        var expected = uintFromHexArg(zArg);
        //WHEN
        var rc = x.neg();
        //THEN
        assertThat(rc)
            .isEqualTo(expected);
    }

    @CsvSource(value = {
        "0x8000000000000000000000000000000000000000000000000000000000000000, 0x4000000000000000000000000000000000000000000000000000000000000000, 0xc000000000000000000000000000000000000000000000000000000000000000",
        "0xf000000000000000000000000000000000000000000000000000000000000000, 0x4000000000000000000000000000000000000000000000000000000000000444, 0xf000000000000000000000000000000000000000000000000000000000000444",
        "0x0, 0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
        "0x100000, 0x200000, 0x300000",
        "0x0, 0x01, 0x01",
        "0x0, 0x0, 0x0"
    })
    @ParameterizedTest
    void or(String xArg, String yArg, String zArg) {
        //GIVEN
        var x = uintFromHexArg(xArg);
        var y = uintFromHexArg(yArg);
        var expected = uintFromHexArg(zArg);
        //WHEN
        var rc = x.or(y);
        //THEN
        assertThat(rc)
            .isEqualTo(expected);
    }

    @CsvSource(value = {
        "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
        "0x8000000000000000000000000000000000000000000000000000000000000000, 0x4000000000000000000000000000000000000000000000000000000000000000, 0x",
        "0xf000000000000000000000000000000000000000000000000000000000000606, 0xc000000000000000000000000000000000000000000000000000000000000444, 0xc000000000000000000000000000000000000000000000000000000000000404",
        "0x0, 0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0x0",
        "0x010305070, 0x010101010, 0x010101010",
        "0x0, 0x01, 0x00",
        "0x0, 0x0, 0x0"
    })
    @ParameterizedTest
    void and(String xArg, String yArg, String zArg) {
        //GIVEN
        var x = uintFromHexArg(xArg);
        var y = uintFromHexArg(yArg);
        var expected = uintFromHexArg(zArg);
        //WHEN
        var rc = x.and(y);
        //THEN
        assertThat(rc)
            .isEqualTo(expected);
    }

    @CsvSource(value = {
        "0xf000000000000000000000000000000000000000000000000000000000000000, 0xc000000000000000000000000000000000000000000000000000000000000404, 0x3000000000000000000000000000000000000000000000000000000000000404",
        "0x8000000000000000000000000000000000000000000000000000000000000000, 0x4000000000000000000000000000000000000000000000000000000000000000, 0xc000000000000000000000000000000000000000000000000000000000000000",
        "0xf0000000, 0xc0000444, 0x30000444",
        "0x010305070, 0x010101010, 0x0204060",
        "0x0, 0x01, 0x01",
        "0x0, 0x0, 0x0",
        "0xf, 0xf, 0x0",
        "0xff, 0xff, 0x0",
        "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0x0, 0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
        "0x0, 0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
    })
    @ParameterizedTest
    void xor(String xArg, String yArg, String zArg) {
        //GIVEN
        var x = uintFromHexArg(xArg);
        var y = uintFromHexArg(yArg);
        var expected = uintFromHexArg(zArg);
        //WHEN
        var rc = x.xor(y);
        //THEN
        log.info("{} ^ {} = {}", x.asWord().hex(), y.asWord().hex(), rc.asWord().hex());
        assertThat(rc)
            .isEqualTo(expected);
    }

    @CsvSource(value = {
        "0xffffffffffffffff, 0x0, 0xffffffffffffffff",
        "0x0, 0xffffffffffffffff, 0xffffffffffffffff",
        "0x8000000000000000, 0x4000000000000000, 0xc000000000000000",
        "0xf000000000000000, 0xc000000000000444, 0x3000000000000444",
        "0xf00000, 0xc00444, 0x300444",
        "0x010305070, 0x010101010, 0x0204060",
        "0x0, 0x01, 0x01",
        "0x0, 0x0, 0x0"
    })
    @ParameterizedTest
    void xorLong(String xArg, String yArg, String zArg) {
        //GIVEN
        var x = Bytes.toLong(HexUtils.parseHex(xArg));
        var y = Bytes.toLong(HexUtils.parseHex(yArg));
        var expected = Bytes.toLong(HexUtils.parseHex(zArg));
        //WHEN
        var rc = x ^ y;
        //THEN
        log.info("bin: {} ^ {} = {}", toBin(x), toBin(y), toBin(rc));
        log.info("hex: {} ^ {} = {}", toHex(x), toHex(y), toHex(rc));
        assertThat(rc)
            .isEqualTo(expected);
    }

    @CsvSource(value = {
        "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
        "0x8000000000000000000000000000000000000000000000000000000000000000, 0x01, 0xc000000000000000000000000000000000000000000000000000000000000000",
        "0x8000000000000000000000000000000000000000000000000000000000000000, 0x0100, 0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
        "0x8000000000000000000000000000000000000000000000000000000000000000, 0xff, 0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
        "0x0000000000000000000000000000000000000000000000000000000000000000, 0x01, 0x0000000000000000000000000000000000000000000000000000000000000000",
        "0x0000000000000000000000000000000000000000000000000000000000000001, 0x00, 0x0000000000000000000000000000000000000000000000000000000000000001",
        "0x0000000000000000000000000000000000000000000000000000000000000001, 0x01, 0x0000000000000000000000000000000000000000000000000000000000000000",
        "0x8000000000000000000000000000000000000000000000000000000000000000, 0x0101, 0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
        "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0x00, 0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
        "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0x01, 0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
        "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0xff, 0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
        "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0x100, 0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
        "0x4000000000000000000000000000000000000000000000000000000000000000, 0xfe, 0x0000000000000000000000000000000000000000000000000000000000000001",
        "0x7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0xf8, 0x000000000000000000000000000000000000000000000000000000000000007f",
        "0x7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0xfe, 0x0000000000000000000000000000000000000000000000000000000000000001",
        "0x7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0xff, 0x0000000000000000000000000000000000000000000000000000000000000000"
    })
    @ParameterizedTest
    void sar(String vArg, String sArg, String zArg) {
        //GIVEN
        var value = uintFromHexArg(vArg);
        var shift = uintFromHexArg(sArg);
        var expected = uintFromHexArg(zArg);
        //WHEN
        var rc = value.sar(shift);
        //THEN
        assertThat(rc)
            .isEqualTo(expected);
    }

    @CsvSource(value = {
        "0x0000000000000000000000000000000000000000000000000000000000000001, 0x00, 0x0000000000000000000000000000000000000000000000000000000000000001",
        "0x0000000000000000000000000000000000000000000000000000000000000001, 0x01, 0x0000000000000000000000000000000000000000000000000000000000000000",
        "0x8000000000000000000000000000000000000000000000000000000000000000, 0x01, 0x4000000000000000000000000000000000000000000000000000000000000000",
        "0x8000000000000000000000000000000000000000000000000000000000000000, 0xff, 0x0000000000000000000000000000000000000000000000000000000000000001",
        "0x8000000000000000000000000000000000000000000000000000000000000000, 0x0100, 0x0000000000000000000000000000000000000000000000000000000000000000",
        "0x8000000000000000000000000000000000000000000000000000000000000000, 0x0101, 0x0000000000000000000000000000000000000000000000000000000000000000",
        "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0x00, 0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
        "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0x01, 0x7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
        "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0xff, 0x0000000000000000000000000000000000000000000000000000000000000001",
        "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0x0100, 0x0000000000000000000000000000000000000000000000000000000000000000",
        "0x0000000000000000000000000000000000000000000000000000000000000000, 0x01, 0x0000000000000000000000000000000000000000000000000000000000000000"
    })
    @ParameterizedTest
    void shr(String vArg, String sArg, String zArg) {
        //GIVEN
        var value = uintFromHexArg(vArg);
        var shift = Bytes.toLong(HexUtils.parseHex(sArg));
        var expected = uintFromHexArg(zArg);
        //WHEN
        var rc = value.shr((int) shift);
        //THEN
        assertThat(rc)
            .isEqualTo(expected);
    }

    @CsvSource(value = {
        "0x0000000000000000000000000000000000000000000000000000000000000001, 0x00, 0x0000000000000000000000000000000000000000000000000000000000000001",
        "0x0000000000000000000000000000000000000000000000000000000000000001, 0x01, 0x0000000000000000000000000000000000000000000000000000000000000002",
        "0x0000000000000000000000000000000000000000000000000000000000000001, 0xff, 0x8000000000000000000000000000000000000000000000000000000000000000",
        "0x0000000000000000000000000000000000000000000000000000000000000001, 0x0100, 0x0000000000000000000000000000000000000000000000000000000000000000",
        "0x0000000000000000000000000000000000000000000000000000000000000001, 0x0101, 0x0000000000000000000000000000000000000000000000000000000000000000",
        "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0x00, 0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
        "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0x01, 0xfffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe",
        "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0xff, 0x8000000000000000000000000000000000000000000000000000000000000000",
        "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0x0100, 0x0000000000000000000000000000000000000000000000000000000000000000",
        "0x0000000000000000000000000000000000000000000000000000000000000000, 0x01, 0x0000000000000000000000000000000000000000000000000000000000000000"
    })
    @ParameterizedTest
    void shl(String vArg, String sArg, String zArg) {
        //GIVEN
        var value = uintFromHexArg(vArg);
        var shift = Bytes.toLong(HexUtils.parseHex(sArg));
        var expected = uintFromHexArg(zArg);
        //WHEN
        var rc = value.shl((int) shift);
        //THEN
        assertThat(rc)
            .isEqualTo(expected);
    }

    @CsvSource(value = {
        "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 2, 1",
        "0x8000000000000000000000000000000000000000000000000000000000000000, 0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 1",
        "0x8000000000000000000000000000000000000000000000000000000000000000, -1, 1",
        "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0x8000000000000000000000000000000000000000000000000000000000000000, 0",
        "-0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, -0x8000000000000000000000000000000000000000000000000000000000000000, 0",
        "-2, 3, 1",
        "2, 3, 1",
        "0, -1, 0",
        "0, 0, 0",
        "1, 0, 0",
        "3, 0, 0",
        "-3, 0, 1"
    })
    @ParameterizedTest
    void slt(String xArg, String yArg, int zArg) {
        //GIVEN
        var x = uintFromHexArg(xArg);
        var y = uintFromHexArg(yArg);
        var z = zArg == 1;
        //WHEN
        var rc = x.slt(y);

        //THEN
        assertThat(rc)
            .isEqualTo(z);
    }

    @CsvSource(value = {
        "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 2, 0",
        "0x8000000000000000000000000000000000000000000000000000000000000000, 0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0",
        "0x8000000000000000000000000000000000000000000000000000000000000000, -1, 0",
        "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, 0x8000000000000000000000000000000000000000000000000000000000000000, 1",
        "-0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff, -0x8000000000000000000000000000000000000000000000000000000000000000, 1",
        "-2, 3, 0",
        "2, 3, 0",
        "0, -1, 1",
        "0, 0, 0",
        "1, 0, 1",
        "3, 0, 1",
        "-3, 0, 0"
    })
    @ParameterizedTest
    void sgt(String xArg, String yArg, int zArg) {
        //GIVEN
        var x = uintFromHexArg(xArg);
        var y = uintFromHexArg(yArg);
        var z = zArg == 1;
        //WHEN
        var rc = x.sgt(y);

        //THEN
        assertThat(rc)
            .isEqualTo(z);
    }

}