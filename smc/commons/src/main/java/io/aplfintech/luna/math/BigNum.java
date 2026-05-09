package io.aplfintech.luna.math;

import java.math.BigInteger;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface BigNum extends Comparable<BigNum> {
    /**
     * Converts the current number to the VM 256-bits word
     */
    Word256 asWord();

    BigInteger bigIntegerValue();

    /**
     * Converts this number to a long, checking for lost information.
     * if this BigInteger is too big to fit in a long, only the low-order 64 bits are returned
     */
    long longValue();

    /**
     * Converts this number to a long.
     * If safeOverflow flag is true, the checking for lost information is performed
     * If the value of this BigInt is out of the range of the long type then
     * If overflow is checking then returns the MAX_LONG = 0x7fffffffffffffff value
     * otherwise an ArithmeticException is thrown.
     */
    long longValue(boolean safeOverflow);

    /**
     * Converts this BigInt to an int, without checking for the lost information.
     * If this BigInteger is too big to fit in an integer, only the low-order 32 bits are returned
     */
    int intValue();

    /**
     * Converts this number to an integer.
     * If safeOverflow flag is true, the checking for lost information is performed
     * If the value of this BigInt is out of the range of the integer type then
     * If overflow is checking then returns the MAX_INTEGER = 0x7fffffff value
     * otherwise an ArithmeticException is thrown.
     */
    int intValue(boolean safeOverflow);

    int byteLength();

    /**
     * Returns -1, 0 or 1 as the value of this number
     * interpreted as a two's complement signed number
     * is negative, zero or positive.
     */
    int sign();

    default boolean isNegative() {
        return sign() == -1;
    }

    default BigNum abs() {
        return isNegative() ? this.neg() : this;
    }

    /**
     * Returns true if and only if the designated bit is set
     *
     * @param n checked bit
     * @return true if n'th bit is set
     */
    boolean isBitSet(int n);

    boolean isZero();

    boolean isNonZero();

    boolean eq(BigNum b);

    boolean gt(BigNum b);

    boolean gte(BigNum b);

    boolean sgt(BigNum b);

    boolean lt(BigNum b);

    boolean lte(BigNum b);

    boolean slt(BigNum b);

    BigNum and(BigNum b);

    BigNum or(BigNum b);

    BigNum xor(BigNum b);

    BigNum not();

    BigNum neg();

    BigNum add(BigNum term);

    BigNum sub(BigNum term);

    BigNum mul(BigNum factor);

    BigNum modmul(BigNum mulValue, BigNum modValue);

    BigNum div(BigNum factor);

    BigNum sdiv(BigNum factor);

    BigNum mod(BigNum factor);

    BigNum smod(BigNum factor);

    BigNum modadd(BigNum addValue, BigNum modValue);

    BigNum pow(BigNum exponent);

    BigNum shl(int bits);

    BigNum shr(int bits);

    BigNum sar(BigNum shift);
}
