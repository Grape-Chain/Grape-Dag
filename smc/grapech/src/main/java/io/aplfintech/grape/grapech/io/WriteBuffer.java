package io.aplfintech.grape.grapech.io;

import java.math.BigInteger;
import java.nio.ByteBuffer;
import java.nio.ByteOrder;

/**
 * @author andrew.zinchenko@gmail.com
 */
public interface WriteBuffer extends ReadBuffer {
    ByteOrder order();

    ByteOrder setOrder(ByteOrder order);

    default boolean isBigEndian() {
        return ByteOrder.BIG_ENDIAN.equals(order());
    }

    default int size() {
        return toByteArray().length;
    }

    byte[] toByteArray();

    @Override
    default ByteBuffer buffer() {
        return ByteBuffer.wrap(toByteArray());
    }

    WriteBuffer write(byte value);

    WriteBuffer write(byte[] value);

    WriteBuffer write(boolean value);

    WriteBuffer write(short value);

    WriteBuffer write(int value);

    WriteBuffer write(long value);

    WriteBuffer write(String hex);

    WriteBuffer write(BigInteger value);

    /**
     * Copies bytes to the output buffer without any encoding
     *
     * @param bytes the byte array
     * @return the output buffer
     */
    default WriteBuffer concat(byte[] bytes) {
        return write(bytes);
    }
}
