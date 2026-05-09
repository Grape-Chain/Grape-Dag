package io.aplfintech.luna.math;

import io.aplfintech.luna.utils.Bytes;

import java.math.BigInteger;

import static io.aplfintech.luna.math.Math256.UINT_256_ONE;
import static io.aplfintech.luna.math.Math256.asUnsignedByteArray;

/**
 * The implementation of UInt256 based on java.math.BigInteger
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
class BigInt256 extends UInt256 {
    private static final BigInteger MAX_INT32 = new BigInteger("7fffffff", 16);

    /**
     * Translates the magnitude representation of binary to positive BigInt
     *
     * @param bytes binary representation of the magnitude of the number
     */
    BigInt256(byte[] bytes) {
        super(bytes);
    }

    static BigInt256 valueOf(BigInteger value) {
        return new BigInt256(asSignedByteArray(value));
    }

    @Override
    public BigInteger bigIntegerValue() {
        return getSignedBigInteger();
    }

    @Override
    public long longValue() {
        return getUnsignedBigInteger().longValue();
    }

    @Override
    public long longValue(boolean safeOverflow) {
        var value = getUnsignedBigInteger();
        if (safeOverflow) {
            try {
                return value.longValueExact();
            } catch (ArithmeticException e) {
                return Long.MAX_VALUE;
            }
        } else {
            return value.longValueExact();
        }
    }

    @Override
    public int intValue() {
        return getUnsignedBigInteger().intValue();
    }

    @Override
    public int intValue(boolean safeOverflow) {
        var value = getUnsignedBigInteger();
        if (safeOverflow) {
            try {
                return value.intValueExact();
            } catch (ArithmeticException e) {
                return Integer.MAX_VALUE;
            }
        } else {
            return value.intValueExact();
        }
    }

    /**
     * Returns number of bytes what would fit to current BigInt value
     *
     * @return number of bytes what would fit to current BigInt value
     */
    @Override
    public int byteLength() {
        var value = getUnsignedBigInteger();
        return (value.bitLength() + 7) / 8;
    }

    @Override
    public boolean isBitSet(int n) {
        var byt = n / 8;
        if (byt > 31) {
            throw new IllegalArgumentException("Queried bit " + n + " is Out of range of 32 byte size word");
        }
        var value = bytes[31 - byt] & (1 << (n % 8));
        return value != 0;
    }

    @Override
    public int sign() {
        if (isZero()) {
            return 0;
        }
        if (isBitSet(255)) {
            return -1;
        }
        return 1;
    }

    @Override
    public boolean isZero() {
        return Bytes.isAllZero(bytes);
    }

    @Override
    public boolean isNonZero() {
        return !isZero();
    }

    @Override
    public boolean eq(BigNum b) {
        return this.compareTo(b) == 0;
    }

    @Override
    public boolean gt(BigNum b) {
        return this.compareTo(b) > 0;
    }

    @Override
    public boolean gte(BigNum b) {
        return this.compareTo(b) >= 0;
    }

    @Override
    public boolean sgt(BigNum b) {
        var xs = this.sign();
        var ys = b.sign();
        if (xs >= 0 && ys < 0) {
            return true;
        }
        if (xs < 0 && ys >= 0) {
            return false;
        }
        return gt(b);
    }

    @Override
    public boolean lt(BigNum b) {
        return this.compareTo(b) < 0;
    }

    @Override
    public boolean lte(BigNum b) {
        return this.compareTo(b) <= 0;
    }

    @Override
    public boolean slt(BigNum b) {
        var xs = this.sign();
        var ys = b.sign();
        if (xs >= 0 && ys < 0) {
            return false;
        }
        if (xs < 0 && ys >= 0) {
            return true;
        }
        return lt(b);
    }


    @Override
    public BigNum and(BigNum b) {
        var x = getUnsignedBigInteger();
        var y = ((BigInt256) b).getUnsignedBigInteger();
        return BigInt256.valueOf(x.and(y));
    }

    @Override
    public BigNum or(BigNum b) {
        var x = getUnsignedBigInteger();
        var y = ((BigInt256) b).getUnsignedBigInteger();
        return BigInt256.valueOf(x.or(y));
    }

    @Override
    public BigNum xor(BigNum b) {
        var x = getUnsignedBigInteger();
        var y = ((BigInt256) b).getUnsignedBigInteger();
        return BigInt256.valueOf(x.xor(y));
    }

    @Override
    public BigNum not() {
        return new BigInt256(Bytes.not(bytes));
    }

    @Override
    public BigNum neg() {
        return Math256.UINT_256_ZERO.sub(this);
    }

    @Override
    public BigNum add(BigNum term) {
        var x = getSignedBigInteger();
        var y = term.bigIntegerValue();//signed value
        return BigInt256.valueOf(x.add(y).mod(Math256.TWO_POW256));
    }

    @Override
    public BigNum sub(BigNum term) {
        var x = getSignedBigInteger();
        var y = term.bigIntegerValue();//signed value
        return BigInt256.valueOf(x.subtract(y).mod(Math256.TWO_POW256));
    }

    @Override
    public BigNum mul(BigNum factor) {
        var x = getUnsignedBigInteger();
        var y = ((BigInt256) factor).getUnsignedBigInteger();
        return BigInt256.valueOf(x.multiply(y).mod(Math256.TWO_POW256));
    }

    /**
     * Returns the modulo-m multiplication of value and mulValue
     * If modValue == 0, returns 0, (doesn't throw ArithmeticException exception)
     *
     * @param mulValue value to multiply
     * @param modValue mod value
     */
    @Override
    public BigNum modmul(BigNum mulValue, BigNum modValue) {
        if (modValue.isZero() || mulValue.isZero() || this.isZero()) {
            return Math256.UINT_256_ZERO;
        }
        var x = getUnsignedBigInteger();
        var mul = ((BigInt256) mulValue).getUnsignedBigInteger();
        var mod = ((BigInt256) modValue).getUnsignedBigInteger();
        return BigInt256.valueOf(x.multiply(mul).mod(mod));
    }

    /**
     * Div operation
     * If factor == 0, returns 0, (doesn't throw ArithmeticException exception)
     */
    @Override
    public BigNum div(BigNum factor) {
        if (factor.isZero()) {
            return Math256.UINT_256_ZERO;
        }
        var x = getUnsignedBigInteger();
        var y = ((BigInt256) factor).getUnsignedBigInteger();
        return BigInt256.valueOf(x.divide(y).mod(Math256.TWO_POW256));
    }

    @Override
    public BigNum sdiv(BigNum factor) {
        if (factor.isZero()) {
            return Math256.UINT_256_ZERO;
        }
        var xs = this.sign();
        var ys = factor.sign();
        var x = xs < 0 ? this.neg() : this;
        var y = ys < 0 ? factor.neg() : factor;
        var z = x.div(y);
        return xs != ys ? z.neg() : z;
    }

    /**
     * Returns the modulus value % factor for factor != 0
     * If factor == 0, returns 0, (doesn't throw ArithmeticException exception)
     */
    @Override
    public BigNum mod(BigNum factor) {
        if (factor.isZero()) {
            return Math256.UINT_256_ZERO;
        }
        var x = getUnsignedBigInteger();
        var y = ((BigInt256) factor).getUnsignedBigInteger();
        return BigInt256.valueOf(x.mod(y));
    }

    /**
     * Mod operation between signed arguments.
     * SMod interprets value and factor as two's complement signed integers.
     * If factor == 0, returns 0, (doesn't throw ArithmeticException exception)
     * Result of operation keeps sign of value (this) argument.
     * i.e. result = (sign value) * { abs(value) modulus abs(factor) }
     */
    @Override
    public BigNum smod(BigNum factor) {
        if (factor.isZero()) {
            return Math256.UINT_256_ZERO;
        }
        var xs = this.sign();
        var x = xs < 0 ? this.neg() : this;
        var y = factor.abs();
        var z = x.mod(y);
        return xs < 0 ? z.neg() : z;
    }

    /**
     * Returns the result of sum ( value + addValue ) mod modValue
     * If modValue == 0, returns 0 (doesn't throw ArithmeticException exception)
     *
     * @param addValue value to add
     * @param modValue mod value
     */
    @Override
    public BigNum modadd(BigNum addValue, BigNum modValue) {
        if (modValue.isZero()) {
            return Math256.UINT_256_ZERO;
        }
        var x = getUnsignedBigInteger();
        var add = ((BigInt256) addValue).getUnsignedBigInteger();
        var mod = ((BigInt256) modValue).getUnsignedBigInteger();
        return BigInt256.valueOf(x.add(add).mod(mod));
    }

    /**
     * Returns base**exponent
     */
    @Override
    public BigNum pow(BigNum exponent) {
        if (this.isZero()) {
            return (exponent.isZero() ? UINT_256_ONE : this);
        }
        if (this.eq(UINT_256_ONE)) {
            return this;
        }
        var base = getUnsignedBigInteger();
        var exp = ((UInt256) exponent).getUnsignedBigInteger();
        BigInteger pow;
        if (exp.compareTo(MAX_INT32) <= 0) {
            int intExponent = exponent.intValue();
            pow = base.pow(intExponent).mod(Math256.TWO_POW256);
        } else {
            pow = exponentiation(base, exp);
        }
        return BigInt256.valueOf(pow);
    }

    private static BigInteger exponentiation(BigInteger base, BigInteger exponent) {
        BigInteger result = BigInteger.ONE;
        while (exponent.signum() > 0) {
            if (exponent.testBit(0))
                result = result.multiply(base).mod(Math256.TWO_POW256);
            base = base.multiply(base).mod(Math256.TWO_POW256);
            exponent = exponent.shiftRight(1);
        }
        return result;
    }

    @Override
    public BigNum shl(int bits) {
        return BigInt256.valueOf(getUnsignedBigInteger().shiftLeft(bits));
    }

    @Override
    public BigNum shr(int bits) {
        return BigInt256.valueOf(getUnsignedBigInteger().shiftRight(bits));
    }

    /**
     * (Signed/Arithmetic right shift)
     * considers value to be a signed integer, during right-shift
     * and returns  <code>value >> sift</code>
     *
     * @param shift number bits to shift
     * @return result of signed right shift
     */
    @Override
    public BigNum sar(BigNum shift) {
        BigNum word = this;
        if (shift.gt(Math256.uint256(256))) {
            return word.isBitSet(255) ? Math256.UINT_256_MAX_BIGINT : Math256.UINT_256_ZERO;
        }
        int bit = shift.intValue();
        if (bit == 0) {
            return word;
        }
        var shifted = word.shr(bit);
        if (word.isBitSet(255)) {
            if (bit == 256) {
                return Math256.UINT_256_MAX_BIGINT;
            }
            var outBits = 255 - bit;
            var mask = Math256.UINT_256_MAX_BIGINT.shr(outBits).shl(outBits);
            return shifted.or(mask);
        } else {
            return shifted;
        }
    }

    @Override
    public int compareTo(BigNum o) {
        var x = getUnsignedBigInteger();
        var y = ((BigInt256) o).getUnsignedBigInteger();
        return x.compareTo(y);
    }

    /**
     * Return the passed in value as a signed 32-byte size array.
     *
     * @param value the value to be converted.
     * @return a byte array left padded by the sign byte up to 32 byte size
     */
    public static byte[] asSignedByteArray(BigInteger value) {
        if (value.signum() < 0) {
            return Math256.padToWordFF(asUnsignedByteArray(value));
        }
        return Math256.padToWord(asUnsignedByteArray(value));
    }

}
