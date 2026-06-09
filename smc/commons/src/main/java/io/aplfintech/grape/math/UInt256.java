package io.aplfintech.grape.math;

import lombok.NonNull;

import java.math.BigInteger;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public abstract class UInt256 extends Word256 implements BigNum {

    protected final byte[] bytes;

    protected UInt256(byte @NonNull [] bytes) {
        this.bytes = bytes.length < Math256.WORD_SIZE ? Math256.padToWord(bytes) : bytes;
    }

    @Override
    public byte[] bytes() {
        return bytes;
    }

    @Override
    public byte[] bytes32() {
        return Math256.padToWord(bytes());
    }

    protected BigInteger getUnsignedBigInteger() {
        BigInteger value = new BigInteger(1, bytes);
        return value;
    }

    protected BigInteger getSignedBigInteger() {
        return new BigInteger(bytes);
    }

    @Override
    public Word256 asWord() {
        return this;
    }

    @Override
    public BigNum asBigNum() {
        return this;
    }

}
