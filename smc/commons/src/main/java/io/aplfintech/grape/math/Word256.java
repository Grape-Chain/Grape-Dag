package io.aplfintech.grape.math;

import io.aplfintech.grape.utils.HexUtils;

import java.util.Arrays;

/**
 * 32-byte word
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public abstract class Word256 {

    public abstract byte[] bytes();

    /**
     * Returns byte array left-padded to 32 bytes
     */
    public abstract byte[] bytes32();

    public abstract BigNum asBigNum();

    public String hex() {
        return HexUtils.toHex(bytes(), true);
    }

    @Override
    public String toString() {
        return "[" + hex() + "]";
    }

    @Override
    public boolean equals(Object o) {
        if (this == o) return true;
        if (o == null || getClass() != o.getClass()) return false;

        Word256 word256 = (Word256) o;

        return Arrays.equals(bytes(), word256.bytes());
    }

    @Override
    public int hashCode() {
        return Arrays.hashCode(bytes());
    }
}
