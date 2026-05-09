package io.aplfintech.luna.math;

/**
 * The implementation of UInt256 based on java.math.BigInteger
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
/*

class BigInt256BigInteger extends UInt256 {

    private final BigInteger value;

    BigInt256BigInteger(BigInteger value) {
        super(value.toByteArray());
        this.value = value;
    }

    */
/**
     * Translates the magnitude representation of binary to positive BigInt
     *
     * @param bytes binary representation of the magnitude of the number
     * @return the Unsigned BigInt value translated from the magnitude representation of binary
     *//*

    static BigInt256BigInteger from(byte[] bytes) {
        var value = new BigInteger(1, bytes);
        return new BigInt256BigInteger(value);
    }

    @Override
    public byte[] bytes() {
        return asUnsignedByteArray(value);
    }

    @Override
    public byte[] bytes32() {
        if (value.signum() < 0) {
            return Bytes.toWordFF(asUnsignedByteArray(value));
        }
        return Bytes.toWord(asUnsignedByteArray(value));
    }

    @Override
    public BigNum asInt() {
        return this;
    }

    @Override
    public BigInteger bigIntegerValue() {
        return value;
    }

    @Override
    public long longValue() {
        return value.longValue();
    }

    @Override
    public long longValue(boolean checkOverflow) {
        if (checkOverflow) {
            try {
                return value.longValueExact();
            } catch (ArithmeticException e) {
                return 0xffffffffffffffffL;
            }
        } else {
            return value.longValueExact();
        }
    }

    @Override
    public int intValue() {
        return value.intValue();
    }

    @Override
    public int intValue(boolean checkOverflow) {
        if (checkOverflow) {
            try {
                return value.intValueExact();
            } catch (ArithmeticException e) {
                return 0xffffffff;
            }
        } else {
            return value.intValue();
        }
    }

    */
/**
     * Returns number of bytes what would fit to current BigInt value
     *
     * @return number of bytes what would fit to current BigInt value
     *//*

    @Override
    public int byteLength() {
        var abs = value.abs();
        var byteCount = abs.bitLength() / 8;
        if (abs.bitLength() % 8 > 0) {
            byteCount++;
        }
        return byteCount;
    }

    @Override
    public boolean isNegative() {
        return value.signum() < 0;
    }

    @Override
    public boolean isZero() {
        return value.signum() == 0;
    }

    @Override
    public boolean isNonZero() {
        return value.signum() != 0;
    }

    @Override
    public boolean eq(BigNum b) {
        return value.equals(b.bigIntegerValue());
    }

    @Override
    public boolean gt(BigNum b) {
        return this.value.compareTo(b.bigIntegerValue()) > 0;
    }

    @Override
    public boolean gte(BigNum b) {
        return this.value.compareTo(b.bigIntegerValue()) >= 0;
    }

    @Override
    public boolean lt(BigNum b) {
        return this.value.compareTo(b.bigIntegerValue()) < 0;
    }

    @Override
    public boolean lte(BigNum b) {
        return this.value.compareTo(b.bigIntegerValue()) <= 0;
    }

    @Override
    public BigNum and(BigNum b) {
        return new BigInt256(this.value.and(b.bigIntegerValue()).remainder(TWO_POW256));
    }

    @Override
    public BigNum or(BigNum b) {
        return new BigInt256(this.value.or(b.bigIntegerValue()).remainder(TWO_POW256));
    }

    @Override
    public BigNum xor(BigNum b) {
        return new BigInt256(this.value.xor(b.bigIntegerValue()).remainder(TWO_POW256));
    }

    @Override
    public BigNum not() {
        var bytes = Bytes.toWordFF(this.value.not().toByteArray());
        return BigInt256.from(bytes);
    }

    */
/**
     * Returns BigInt value whose value is (-this)
     *//*

    @Override
    public BigNum neg() {
        return new BigInt256(value.negate().remainder(TWO_POW256));
    }

    @Override
    public BigNum add(BigNum term) {
        return new BigInt256(this.value.add(term.bigIntegerValue()).remainder(TWO_POW256));
    }

    @Override
    public BigNum sub(BigNum term) {
        BigInteger rez = this.value.subtract(term.bigIntegerValue()).remainder(TWO_POW256);

        return new BigInt256(rez);
    }

    @Override
    public BigNum mul(BigNum factor) {
        return new BigInt256(this.value.multiply(factor.bigIntegerValue()).remainder(TWO_POW256));
    }

    */
/**
     * Returns the modulo-m multiplication of value and mulValue
     * If modValue == 0, returns 0, (doesn't throw ArithmeticException exception)
     *
     * @param mulValue value to multiply
     * @param modValue mod value
     *//*

    @Override
    public BigNum modmul(BigNum mulValue, BigNum modValue) {
        if (modValue.isZero() || mulValue.isZero() || this.isZero()) {
            return ZERO;
        }
        return new BigInt256(this.value.multiply(mulValue.bigIntegerValue()).mod(modValue.bigIntegerValue()).remainder(TWO_POW256));
    }

    */
/**
     * Div operation
     * If factor == 0, returns 0, (doesn't throw ArithmeticException exception)
     *//*

    @Override
    public BigNum div(BigNum factor) {
        if (factor.isZero()) {
            return ZERO;
        }
        return new BigInt256(this.value.divide(factor.bigIntegerValue()).remainder(TWO_POW256));
    }

    */
/**
     * Returns the modulus value % factor for factor != 0
     * If factor == 0, returns 0, (doesn't throw ArithmeticException exception)
     *//*

    @Override
    public BigNum mod(BigNum factor) {
        if (factor.isZero()) {
            return ZERO;
        }
        return new BigInt256(this.value.mod(factor.bigIntegerValue()).remainder(TWO_POW256));
    }

    */
/**
     * Mod operation between signed arguments.
     * SMod interprets value and factor as two's complement signed integers.
     * If factor == 0, returns 0, (doesn't throw ArithmeticException exception)
     * Result of operation keeps sign of value argument.
     * i.e. result = (sign value) * { abs(value) modulus abs(factor) }
     *//*

*/
/*
  @Override
    public BigNum smod(BigNum factor) {
        if (factor.isZero()) {
            return ZERO;
        }
        var x = this.isNegative() ? this.neg() : this;
        var y = factor.isNegative() ? factor.neg() : factor;
        var z = x.mod(y);
        return this.isNegative() ? z.neg() : z;
    }
*//*


    */
/**
     * Returns the result of sum ( value + addValue ) mod modValue
     * If modValue == 0, returns 0 (doesn't throw ArithmeticException exception)
     *
     * @param addValue value to add
     * @param modValue mod value
     *//*

    @Override
    public BigNum modadd(BigNum addValue, BigNum modValue) {
        if (modValue.isZero()) {
            return ZERO;
        }
        return new BigInt256(value.add(addValue.bigIntegerValue()).mod(modValue.bigIntegerValue()).remainder(TWO_POW256));
    }

    */
/**
     * Returns base**exponent
     *//*

    @Override
    public BigNum pow(BigNum exponent) {
        return new BigInt256(this.value.pow(exponent.intValue()).remainder(TWO_POW256));
    }

    @Override
    public BigNum shl(int bits) {
        return new BigInt256(value.shiftLeft(bits));
    }

    @Override
    public BigNum shr(int bits) {
        return new BigInt256(value.shiftRight(bits));
    }

    */
/**
     * Return the passed in value as an unsigned byte array.
     *
     * @param value the value to be converted.
     * @return a byte array without a leading zero byte if present in the signed encoding.
     *//*

    public static byte[] asUnsignedByteArray(BigInteger value) {
        byte[] bytes = value.toByteArray();

        if (bytes[0] == 0 && bytes.length != 1) {
            byte[] tmp = new byte[bytes.length - 1];

            System.arraycopy(bytes, 1, tmp, 0, tmp.length);

            return tmp;
        }

        return bytes;
    }

    public static byte[] toBytes(final BigInteger value) {
        if (value.signum() == 0) {
            return new byte[]{0};
        }
        byte[] bytes = value.toByteArray();

        int i = 0;
        for (byte byt : bytes) {
            if (byt != 0) {
                break;
            }
            i++;
        }
        if (i == 0) {
            return bytes;
        }
        int len = bytes.length - i;
        byte[] dst = new byte[len];
        System.arraycopy(bytes, i, dst, 0, len);
        return dst;
    }

}
*/
