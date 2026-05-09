package io.aplfintech.luna.l1vm.code;

import io.aplfintech.luna.vm.contract.Code;
import io.aplfintech.luna.utils.HexUtils;
import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;
import static org.junit.jupiter.api.Assertions.assertArrayEquals;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
class CodesTest {
    byte[] data = HexUtils.fromHex("0102030405060708090a0b0c0d0e0f");

    @Test
    void testByteCode() {
        //GIVEN
        var code = Codes.from(data);
        testBytes(code);
        testGet(code, false);
        testSlice(code);
    }

    @Test
    void testCodeReader() {
        //TODO tested only legacy format
        //TODO OEF V2 not tested
        //GIVEN
        var code = Codes.from2(data);
        testBytes(code);
        testGet(code, true);
        testSlice(code);
    }

    void testBytes(Code code) {
        //THEN
        assertEquals(15, code.size());
        assertArrayEquals(data, code.bytes());
    }

    void testGet(Code code, boolean isV2) {
        //WHEN THEN
        assertEquals((byte) 1, code.getOpCode(0));
        assertEquals((byte) 5, code.getOpCode(4));
        var ex1 = assertThrows(IndexOutOfBoundsException.class, () -> code.get(16));
        var ex2 = assertThrows(IndexOutOfBoundsException.class, () -> code.get(-1));
        if (isV2) {
            assertThat(ex1)
                .hasMessage("End of Buffer reached, buffer.length=15, buffer.pos=0, requested=16 bytes");
            assertThat(ex2)
                .hasMessage("Index -1 out of bounds for length 15");
        } else {
            assertThat(ex1)
                .hasMessage("index (16) must be less than size (15)");
            assertThat(ex2)
                .hasMessage("index (-1) must not be negative");
        }

    }

    void testSlice(Code code) {
        //GIVEN
        byte[] slice1 = HexUtils.fromHex("0102030405");
        byte[] slice2 = HexUtils.fromHex("0405060708090a0b0c");
        //WHEN //THEN
        assertArrayEquals(slice1, code.slice(0, 5));
        assertArrayEquals(slice2, code.slice(3, 12));
        var ex1 = assertThrows(IndexOutOfBoundsException.class, () -> code.slice(1, 16));
        assertThat(ex1)
            .hasMessage("index (16) must not be greater than size (15)");
        var ex2 = assertThrows(IndexOutOfBoundsException.class, () -> code.slice(-1, 2));
        assertThat(ex2)
            .hasMessage("index (-1) must not be negative");
    }


}