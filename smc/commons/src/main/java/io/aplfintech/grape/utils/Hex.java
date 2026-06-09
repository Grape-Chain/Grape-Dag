package io.aplfintech.grape.utils;

import io.aplfintech.grape.math.Math256;
import io.aplfintech.grape.model.Address;
import io.aplfintech.grape.utils.HexUtils;
import lombok.AccessLevel;
import lombok.NoArgsConstructor;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@NoArgsConstructor(access = AccessLevel.PRIVATE)
public class Hex {

    /**
     * Convert Address to prefixed hex string
     *
     * @param address the converted address
     * @return the given address as a prefixed hex string
     */
    public static String toHex(Address address) {
        if (address == null) {
            return "null";
        }
        return HexUtils.toHex(address.bytes(), true);
    }

    /**
     * Converts hex string without or with prefix '0x' to byte array left padded by zeros up to length 32
     */
    public static byte[] fromHexToWord(String hex) {
        return Math256.padToWord(HexUtils.parseHex(hex));
    }
    
}
