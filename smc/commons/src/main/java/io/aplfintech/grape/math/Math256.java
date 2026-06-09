package io.aplfintech.grape.math;

import io.aplfintech.grape.utils.Bytes;
import lombok.AccessLevel;
import lombok.NoArgsConstructor;

import java.math.BigInteger;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@NoArgsConstructor(access = AccessLevel.PRIVATE)
public class Math256 {
    public static final int WORD_SIZE = 32;//256-bits word
    public static final BigInteger TWO_POW256 = new BigInteger("10000000000000000000000000000000000000000000000000000000000000000", 16);
    public static final BigInteger MAX_UNSIGNED_BIGINTEGER = TWO_POW256.subtract(BigInteger.ONE);
    public static final UInt256 UINT_256_MAX_BIGINT;
    public static final UInt256 UINT_256_ZERO;
    public static final UInt256 UINT_256_ONE;
    public static final UInt256 UINT_256_TWO;
    public static final UInt256 UINT_256_TEN;
    public static final UInt256 UINT_256_256;

    static {
        //The max integer that the VM can handle (2^256-1)
        UINT_256_MAX_BIGINT = uint256(MAX_UNSIGNED_BIGINTEGER);
        UINT_256_ZERO = uint256(0);
        UINT_256_ONE = uint256(1);
        UINT_256_TWO = uint256(2);
        UINT_256_TEN = uint256(10);
        UINT_256_256 = uint256(256);
    }

    /**
     * Translates the BigInteger value to Unsigned BigInt
     *
     * @param value the BigInteger value to be translated to Unsigned BigInt
     * @return the Unsigned BigInt value translated from the BigInteger value
     */
    public static UInt256 uint256(BigInteger value) {
        var bytes = asUnsignedByteArray(value);
        return new BigInt256(bytes);
    }

    /**
     * Translates the long value to BigInt
     *
     * @param value the long value to be translated to BigInt
     * @return the Unsigned BigInt value translated from the long value
     */
    public static UInt256 uint256(long value) {
        return new BigInt256(padToWord(Bytes.toBytes(value)));
    }

    /**
     * Translates the magnitude representation of binary to positive BigInt
     *
     * @param bytes binary representation of the magnitude of the number
     * @return the Unsigned BigInt value translated from the magnitude representation of binary
     */
    public static UInt256 uint256(byte[] bytes) {
        return new BigInt256(bytes);
    }

    /**
     * Translates a byte array containing the two's-complement binary representation of the number
     *
     * @param bytes big-endian two's-complement binary representation of the number
     * @return the BigInt value translated from the two's-complement binary
     */
    public static UInt256 int256(byte[] bytes) {
        var value = new BigInteger(bytes);
        return BigInt256.valueOf(value);
    }

    /**
     * Translates a signed long value to BigInt
     *
     * @param longValue a signed longValue
     * @return the BigInt value translated from the signed long value
     */
    public static UInt256 int256(long longValue) {
        var value = BigInteger.valueOf(longValue);
        return BigInt256.valueOf(value);
    }

    /**
     * Returns number of words what would fit to provided number of bytes,
     * i.e. it rounds up the number bytes to number of words.
     *
     * @param sizeInBytes given number of bytes
     * @return returns the ceiled word size required for memory expansion
     */
    public static int toWordSize(int sizeInBytes) {
        return divCeil(sizeInBytes, WORD_SIZE);
    }

    public static int toWordSize(long sizeInBytes) {
        return divCeil(Math.toIntExact(sizeInBytes), WORD_SIZE);
    }

    /* Some operations */

    /**
     * Multiplies two long values and returns the result.
     * Each long value converts to big integer before multiplication.
     *
     * @param x first argument
     * @param y second argument
     * @return the result of multiplication of two long values
     */
    public static BigInteger mul(long x, long y) {
        return BigInteger.valueOf(x).multiply(BigInteger.valueOf(y));
    }

    /**
     * Returns number of ceil what would fit to provided value,
     * i.e. it rounds up the provide number to number of ceil.
     * <p>Note: it only works for positive values.
     *
     * @param value given value
     * @param ceil  given ceil
     * @return number of ceil what would fit to provided value
     */
    public static int divCeil(int value, int ceil) {
        return ((value + (ceil - 1)) / ceil);
    }

    public static int divCeil(long value, int ceil) {
        return divCeil(Math.toIntExact(value), ceil);
    }

    /**
     * Rounds up the number of bytes to number of ceiling
     * and returns the rounded value
     *
     * @param numBytes number of bytes
     * @param ceiling  number of ceiling (word size)
     * @return the rounded up the number of bytes
     */
    public static int ceil(int numBytes, int ceiling) {
        var r = numBytes % ceiling;
        if (r == 0) {
            return numBytes;
        } else {
            return numBytes + ceiling - r;
        }
    }

    /**
     * Returns number of bytes what would fit to provided BigInteger value
     *
     * @param value given value
     * @return number of bytes what would fit to provided BigInteger value
     */
    public static int byteLength(BigInteger value) {
        var abs = value.abs();
        var byteCount = abs.bitLength() / 8;
        if (abs.bitLength() % 8 > 0) {
            byteCount++;
        }
        return byteCount;
    }

    /**
     * Extends length of two’s complement signed integer and returns.
     * if byteNum > 31 returns value
     * If byteNum <= 31 then value interpreted as a signed number with sign-bit at (byteNum*8+7),
     * extended to the full 256 bits and returns
     *
     * @param byteNum byte number that is a sign-bit supplier
     * @return value extended to the full 256 bits
     */
    public static BigNum signExt(BigNum value, BigNum byteNum) {
        if (byteNum.gt(Math256.uint256(31))) {
            return value;
        }
        int bit = byteNum.intValue(true) * 8 + 7;
        var mask = UINT_256_ONE.shl(bit).sub(UINT_256_ONE);
        var s = value.shr(bit).and(UINT_256_ONE);
        return s.isZero() ? value.and(mask) : value.or(mask.not());
    }

    /**
     * Returns byte at position pos from the word,
     * word considered as a big-endian 32-byte integer
     * if pos > 32, returns 0
     * Example: getByte(31, 5) returns 5
     *
     * @param pos  position of the returned byte
     * @param word word from which byte is returned
     * @return byte at position pos from the word
     */
    public static BigNum getByte(int pos, BigNum word) {
        if (pos > 31) {
            return UINT_256_ZERO;
        }
        int bit = (31 - pos) * 8;
        return word.shr(bit).and(Math256.uint256(0xff));
    }

    /**
     * Return the passed in value as an unsigned byte array.
     *
     * @param value the value to be converted.
     * @return a byte array without a leading zero byte if present in the signed encoding.
     */
    public static byte[] asUnsignedByteArray(BigInteger value) {
        byte[] bytes = value.toByteArray();

        if (bytes[0] == 0 && bytes.length != 1) {
            byte[] buf = new byte[bytes.length - 1];
            System.arraycopy(bytes, 1, buf, 0, buf.length);
            return buf;
        }
        return bytes;
    }

    /**
     * Returns the copy of input bytes left padded by zeros up to 32 bytes
     *
     * @param input input byte array
     * @return copy of input bytes left padded by zeros up to length len
     */
    public static byte[] padToWord(byte[] input) {
        return Bytes.leftPadBytes(input, WORD_SIZE);
    }

    /**
     * Returns the copy of input bytes left padded by 0xff up to 32 bytes
     *
     * @param input input byte array
     * @return copy of input bytes left padded by 0xff up to length len
     */
    public static byte[] padToWordFF(byte[] input) {
        return Bytes.leftPadBytes(input, WORD_SIZE, (byte) 0xff);
    }
}
