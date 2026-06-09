package io.aplfintech.grape.utils;

import com.google.common.base.Preconditions;
import lombok.AccessLevel;
import lombok.NoArgsConstructor;
import lombok.NonNull;

import java.math.BigInteger;
import java.util.Arrays;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@NoArgsConstructor(access = AccessLevel.PRIVATE)
public class Bytes {
    public static byte[] alloc(int size, int fill) {
        Preconditions.checkArgument(size > 0, "size must be greater than 0");
        var rc = new byte[size];
        Arrays.fill(rc, (byte) fill);
        return rc;
    }

    public static byte[] alloc(int size) {
        Preconditions.checkArgument(size > 0, "size must be greater than 0");
        return new byte[size];
    }

    public static long toLong(byte[] inBytes) {
        var bytes = leftPadBytes(inBytes, Long.BYTES);
        long result = 0;
        for (int i = 0; i < Long.BYTES; i++) {
            result <<= Byte.SIZE;
            result |= (bytes[i] & 0xFF);
        }
        return result;
    }

    public static byte[] toBytes(final long value) {
        int longSize = Long.BYTES;
        byte[] result = new byte[longSize];
        long l = value;
        for (int i = longSize - 1; i >= 0; i--) {
            result[i] = (byte) (l & 0xFF);
            l >>= longSize;
        }
        return result;
    }

    public static boolean isAllZero(byte @NonNull [] input) {
        for (byte byt : input) {
            if (byt != 0) {
                return false;
            }
        }
        return true;
    }

    /**
     * Applies 'not' (bitwise compliment) operation for each byte in the given array
     */
    public static byte[] not(byte[] input) {
        byte[] result = new byte[input.length];
        for (int i = 0; i < input.length; i++) {
            result[i] = (byte) ~input[i];
        }
        return result;
    }


    /**
     * Returns the copy of input bytes
     *
     * @param input given input to be copied
     * @return the copy of input bytes
     */
    public static byte[] copy(byte @NonNull [] input) {
        return Arrays.copyOf(input, input.length);
    }

    /**
     * Returns the slice of input bytes started from start inclusive to end exclusive
     *
     * @param input given input to be sliced
     * @param start start position of byte array
     * @param end   end position of byte array
     * @return the slice of code started from start inclusive to end exclusive
     */
    public static byte[] slice(byte @NonNull [] input, int start, int end) {
        if (start == end) {
            if (input.length > 0) {
                Preconditions.checkPositionIndex(start, input.length);
            }
            return new byte[0];
        }
        Preconditions.checkElementIndex(start, input.length);
        Preconditions.checkPositionIndex(end, input.length);
        int len = end - start;
        Preconditions.checkArgument(len > 0, "start index (%s) must be less than (%s)", start, end);
        byte[] dst = new byte[len];
        System.arraycopy(input, start, dst, 0, len);
        return dst;
    }

    /**
     * Returns the slice of input bytes started from start inclusive to the end of input
     *
     * @param input given input to be sliced
     * @param start start position of byte array
     * @return the slice of code started from start inclusive to the end of input
     */
    public static byte[] slice(byte @NonNull [] input, int start) {
        return slice(input, start, input.length);
    }

    /**
     * Returns the copy of input bytes right padded by zeros up to length len
     *
     * @param input input byte array
     * @param len   length to which be padded the input
     * @return copy of input bytes right padded by zeros up to length len
     */
    public static byte[] rightPadBytes(byte[] input, int len) {
        return Arrays.copyOf(input, len);
    }

    /**
     * Returns the copy of input bytes left padded by zeros up to length len
     *
     * @param input input byte array
     * @param len   length to which be padded the input
     * @return copy of input bytes left padded by zeros up to length len
     */
    public static byte[] leftPadBytes(byte[] input, int len) {
        return leftPadBytes(input, len, (byte) 0);
    }

    /**
     * Returns the copy of input bytes left padded by the given value up to length len
     *
     * @param input input byte array
     * @param len   length to which be padded the input
     * @param fill  value to fill padded bytes
     * @return copy of input bytes left padded by the given value up to length len
     */
    public static byte[] leftPadBytes(byte[] input, int len, byte fill) {
        byte[] result;
        if (len <= input.length) {
            result = new byte[len];
            System.arraycopy(input, input.length - len, result, 0, len);
        } else {
            result = alloc(len, fill);
            System.arraycopy(input, 0, result, len - input.length, input.length);
        }
        return result;
    }

    /**
     * Returns a sub-slice of input without leading zeroes
     *
     * @param input input byte array
     * @return a sub-slice of input without leading zeroes
     */
    public static byte[] trimLeftZeros(byte[] input) {
        int i = 0;
        for (byte byt : input) {
            if (byt != 0) {
                break;
            }
            i++;
        }
        if (i >= input.length) {
            return new byte[0];
        }
        return slice(input, i);
    }

    /**
     * Returns a sub-slice of input without trailing zeroes
     *
     * @param input input byte array
     * @return a sub-slice of input without trailing zeroes
     */
    public static byte[] trimRightZeros(byte @NonNull [] input) {
        int i = input.length;
        for (; i > 0; i--) {
            if (input[i - 1] != 0) {
                break;
            }
        }
        if (i == 0) {
            return new byte[0];
        }
        return slice(input, 0, i);
    }

    /**
     * Returns a slice from the data based on the start and size and pads
     * up to size with zero's.
     * It's safe to any 'out of bounds' exceptions
     *
     * @param data  input byte array
     * @param start start position
     * @param size  number of sliced bytes
     * @return a slice from the data based on the start and size and pads up to size with zero's
     */
    public static byte[] slicePadded(byte @NonNull [] data, int start, int size) {
        var length = data.length;
        if (start > length) {
            start = length;
        }
        var end = start + size;
        if (end > length) {
            end = length;
        }
        return rightPadBytes(slice(data, start, end), size);
    }

    /**
     * Returns the byte array length
     * it's safe to NPE if a byte array is null and returns 0
     *
     * @param data input byte array
     * @return the byte array length
     */
    public static int length(byte[] data) {
        if (data == null) {
            return 0;
        }
        return data.length;
    }

    /**
     * Returns TRUE if the input data is null or byte array length equals zero
     * it's safe to NPE if a byte array is null and returns 0
     *
     * @param data input byte array
     * @return TRUE if the input data is null or byte array length equals zero
     */
    public static boolean isEmpty(byte[] data) {
        return length(data) == 0;
    }

    /**
     * Returns byte array as result of concatenation of the given arrays
     *
     * @param data source arrays
     * @return byte array as result of concatenation of the given arrays
     */
    public static byte[] concat(byte[]... data) {
        int totalLength = 0;
        for (byte[] bytes : data) {
            totalLength += bytes.length;
        }
        var buffer = new byte[totalLength];
        int pos = 0;
        for (byte[] bytes : data) {
            System.arraycopy(bytes, 0, buffer, pos, bytes.length);
            pos += bytes.length;
        }
        return buffer;
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
}
