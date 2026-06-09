package io.aplfintech.grape.l1vm.code;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface CodeReader {
    /**
     * Returns the byte array of the code
     *
     * @return the byte array of the code
     */
    byte[] bytes();

    /**
     * Returns the code length
     *
     * @return the code length
     */
    int codeSize();

    /**
     * Returns current state of reader
     *
     * @return current state
     */
    State state();

    /**
     * Reset program counter to start of code section
     */
    void reset();

    /**
     * Sets the program counter to the given position
     *
     * @param pc new position of the program counter
     */
    void jump(int pc);

    /**
     * Returns a byte pointed by the program counter,
     * program counter keeps not changed
     *
     * @return byte
     */
    byte get();

    /**
     * Returns a byte pointed by the program counter,
     * and increments the program counter
     *
     * @return byte
     */
    byte read();

    /**
     * Returns num bytes pointed by the program counter,
     * and increases the program counter by num value
     *
     * @param num the number bytes to read
     * @return byte
     */
    byte[] read(int num);

    /**
     * Returns true if program counter in range of code section
     *
     * @return true if program counter in range of code section
     */
    boolean hasBytes();

    /**
     * The reader state record
     */
    record State(long pc) {
    }

    default void checkBufferState(int offset, int num) {
        if (offset + num >= codeSize()) {
            String errorMessage = "End of Buffer reached, buffer.length=%s, buffer.pos=%s";
            if (num > 0) {
                errorMessage += ", requested=%s bytes";
            }
            var msg = String.format(errorMessage, codeSize(), offset, num);
            throw new IndexOutOfBoundsException(msg);
        }
    }
}
