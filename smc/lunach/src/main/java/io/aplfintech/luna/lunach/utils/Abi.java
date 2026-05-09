package io.aplfintech.luna.lunach.utils;

import io.aplfintech.luna.grap3.crypto.Hashers;
import io.aplfintech.luna.utils.Bytes;
import lombok.AccessLevel;
import lombok.NoArgsConstructor;
import lombok.extern.slf4j.Slf4j;

import java.math.BigInteger;
import java.nio.charset.StandardCharsets;
import java.util.Arrays;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
@NoArgsConstructor(access = AccessLevel.PRIVATE)
public class Abi {
    public static final byte[] REVERT_SELECTOR = Bytes.slice(Hashers.keccak256().digest("Error(string)".getBytes(StandardCharsets.UTF_8)), 0, 4);

    public static boolean isRevertedOutput(byte[] output) {
        return output != null && output.length > 4 && Arrays.equals(REVERT_SELECTOR, Bytes.slice(output, 0, 4));
    }

    public static String unpackRevert(byte[] data) {
        if (!isRevertedOutput(data) || data.length < 100) {
            throw new IllegalArgumentException("invalid data for unpacking");
        }
        var offset = new BigInteger(1, Bytes.slice(data, 4, 36)).intValueExact();
        var length = new BigInteger(1, Bytes.slice(data, 36, 36 + offset)).intValueExact();
        var msg = new String(Bytes.slice(data, 36 + offset, 36 + offset + length));
        return msg;
    }
}
