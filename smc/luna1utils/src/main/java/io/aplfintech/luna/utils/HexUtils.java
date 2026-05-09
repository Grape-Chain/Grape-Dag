package io.aplfintech.luna.utils;

import lombok.AccessLevel;
import lombok.NoArgsConstructor;

import java.math.BigInteger;
import java.util.HexFormat;
import java.util.Locale;
import java.util.Objects;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@NoArgsConstructor(access = AccessLevel.PRIVATE)
public class HexUtils {
    public static final String HEX_PREFIX = "0x";

    /**
     * Returns true if hex string starts with prefix '0x'
     */
    public static boolean isPrefixedHex(String hex) {
        String hexStr = Objects.requireNonNull(hex).toLowerCase(Locale.ROOT);
        return hexStr.startsWith(HEX_PREFIX) /*&& hexStr.length() % 2 == 0*/;
    }

    /**
     * Converts hex string without or with prefix '0x' to byte array
     */
    public static byte[] parseHex(String hexArg) {
        String hexStr = Objects.requireNonNull(hexArg).toLowerCase(Locale.ROOT);
        if (isPrefixedHex(hexStr)) {
            return fromHex(hexStr.substring(2));
        } else {
            return fromHex(hexStr);
        }
    }

    /**
     * Converts hex string without prefix to byte array
     */
    public static byte[] fromHex(String hexArg) {
        String hex = Objects.requireNonNull(hexArg).toLowerCase(Locale.ROOT);
        byte[] bytes;
        if (hex.equals("0") || hex.equals("00")) {
            return new byte[]{0};
        }
        if (hex.length() % 2 != 0) {
            hex = "0" + hex;
        }
        try {
            bytes = HexFormat.of().parseHex(hex);
        } catch (IllegalArgumentException e) {
            throw new NumberFormatException("The input is not a valid encoded string: " + hexArg + " cause:" + e.getMessage());
        }
        return bytes;
    }

    /**
     * Covert input long value to a hex string
     *
     * @param value     the input long value
     * @param addPrefix add 0x prefix iff true
     * @return the input byte array as a hex string
     */
    public static String toHex(long value, boolean addPrefix) {
        var hex = HexFormat.of().toHexDigits(value);
        return addPrefix(addPrefix, hex);
    }

    public static String toHex(long value) {
        return toHex(value, true);
    }

    /**
     * Covert input long value to a hex string
     *
     * @param value     the input int value
     * @param addPrefix add 0x prefix iff true
     * @return the input byte array as a hex string
     */
    public static String toHex(int value, boolean addPrefix) {
        var hex = HexFormat.of().toHexDigits(value);
        return addPrefix(addPrefix, hex);
    }

    public static String toHex(int value) {
        return toHex(value, false);
    }

    /**
     * Covert input byte value to a hex string
     *
     * @param value the input byte value
     * @return the input byte array as a hex string
     */
    public static String toHex(byte value) {
        return toHex(value, false);
    }

    /**
     * Covert input long value to a hex string
     *
     * @param value     the byte input
     * @param addPrefix add 0x prefix iff true
     * @return the input byte array as a hex string
     */
    public static String toHex(byte value, boolean addPrefix) {
        return toHex(new byte[]{value}, addPrefix);
    }

    /**
     * Covert input byte array as a hex string with 0x0 prefix.
     *
     * @param value the input array
     * @return the input byte array as a hex string
     */
    public static String toHex(byte[] value) {
        return toHex(value, false);
    }

    /**
     * Convert input byte array to hex string
     *
     * @param value     the input array
     * @param addPrefix add 0x prefix iff true
     * @return the input byte array as a hex string
     */
    public static String toHex(byte[] value, boolean addPrefix) {
        //Objects.requireNonNull(value);
        if (value == null) {
            return "null";
        }
        if (value.length == 0) {
            return "";
        }
        String hex = HexFormat.of().formatHex(value);
        return addPrefix(addPrefix, hex);
    }

    private static String addPrefix(boolean addPrefix, String hex) {
        return (addPrefix ? HEX_PREFIX : "") + hex;
    }

    /**
     * Converts long value to binary string
     */
    public static String toBin(long x) {
        return BigInteger.valueOf(x).toString(2);
    }
}
