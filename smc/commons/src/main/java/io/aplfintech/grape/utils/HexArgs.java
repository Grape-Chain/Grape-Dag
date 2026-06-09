package io.aplfintech.grape.utils;

import io.aplfintech.grape.math.BigNum;
import io.aplfintech.grape.math.Math256;
import io.aplfintech.grape.utils.Bytes;
import io.aplfintech.grape.utils.HexUtils;
import lombok.NoArgsConstructor;
import lombok.extern.slf4j.Slf4j;

/**
 * Helper for tests
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
@NoArgsConstructor
public class HexArgs {
    /**
     * Parses the given hex value and converts to the BigNum value.
     * The signed negative value interpreted as result of the expression: sub(0, value)
     *
     * @param signedHex source value
     * @return the converted BigNum value
     */
    public static BigNum uintFromHexArg(String signedHex) {
        if (signedHex.startsWith("-")) {
            var arg = Math256.uint256(HexUtils.parseHex(signedHex.replace("-", "")));
            return Math256.UINT_256_ZERO.sub(arg);
        }
        return Math256.uint256(HexUtils.parseHex(signedHex));
    }

    public static byte[] get32Bytes(long value) {
        byte[] bytes = Bytes.trimLeftZeros(Bytes.toBytes(value));
        return value < 0 ? Math256.padToWordFF(bytes) : Math256.padToWord(bytes);
    }
}
