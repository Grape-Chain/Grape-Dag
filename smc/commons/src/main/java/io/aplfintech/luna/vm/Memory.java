package io.aplfintech.luna.vm;

/**
 * Simple memory model for the VM
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface Memory {

    /**
     * Expand the memory given an offset and size.
     *
     * @param offset starting position
     * @param size   size of word in bytes
     */
    void expand(int offset, int size);

    /**
     * Writes bytes to memory.
     * Writes a byte array with length size to memory, starting from offset.
     *
     * @param offset - starting position
     * @param size   - how many bytes to write
     * @param data   - byte array to write
     */
    void write(int offset, int size, byte[] data);

    /**
     * Reads bytes from memory.
     * Reads a slice of memory from <code>offset</code> till <code>offset+size</code> as a byte array.
     * It fills up the difference between memory's length and <code>offset+size</code> with zeros.
     *
     * @param offset - starting position
     * @param size   - how many bytes to read
     * @return byte array red from memory
     */
    byte[] read(int offset, int size);

    /**
     * Returns the size of memory
     *
     * @return the size of memory
     */
    int size();

}
