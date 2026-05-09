package io.aplfintech.luna.l1vm;

import io.aplfintech.luna.utils.HexUtils;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.CsvSource;

import static org.assertj.core.api.Assertions.assertThat;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
class VmAddressTest {

    @CsvSource(value = {
        "0x010203040506070809000a0b0c0d0e0f, 0x00000000010203040506070809000a0b0c0d0e0f"
        , "0x11223344010203040506070809000a0b0c0d0e0f, 0x11223344010203040506070809000a0b0c0d0e0f"
        , "0x112233445566778899010203040506070809000a0b0c0d0e0f, 0x66778899010203040506070809000a0b0c0d0e0f"
        , "0x0, 0x0000000000000000000000000000000000000000"
    })
    @ParameterizedTest
    void from(String hex, String expectArg) {
        //GIVEN
        var expected = HexUtils.parseHex(expectArg);
        //WHEN
        var address = VmAddress.from(hex);
        //THEN
        assertThat(address.bytes())
            .isEqualTo(expected);
    }

    @Test
    void hexAddress() {
        //GIVEN
        var bytes = new byte[]{1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 0xa, 0xb, 0xc, 0xd, 0xe, 0xf};
        var address = VmAddress.from(bytes);
        var expected = "0x00000000010203040506070809000a0b0c0d0e0f";
        //WHEN
        var hex = address.hexAddress();
        //THEN
        assertThat(hex)
            .isEqualTo(expected);
    }
}