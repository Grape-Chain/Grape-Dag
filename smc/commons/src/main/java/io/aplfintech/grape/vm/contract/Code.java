package io.aplfintech.grape.vm.contract;

/**
 * General model for representation a contract code and precompiled function
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface Code {

    /**
     * Returns the byte array of the contract byte-code
     *
     * @return the byte array of the contract byte-code
     */
    byte[] bytes();

    /**
     * Returns the byte code size
     *
     * @return the byte code size
     */
    long size();

    /**
     * Returns the n'th element in the contract's code section
     *
     * @param n position in the contract's code section
     * @return the n'th element in the contract's code section
     */
    byte getOpCode(long n);

    /**
     * Returns the n'th element in the contract's byte array
     *
     * @param n position in the contract's byte array
     * @return the n'th element in the contract's byte array
     */
    byte get(long n);

    /**
     * Returns the slice of code started from start inclusive to end exclusive
     *
     * @param start start position of byte array
     * @param end   end position of byte array
     * @return the slice of code started from start inclusive to end exclusive
     */
    byte[] slice(long start, long end);
}
