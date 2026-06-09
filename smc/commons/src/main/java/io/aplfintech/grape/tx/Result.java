package io.aplfintech.grape.tx;

/**
 * The serialization result
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface Result {

    byte[] array();

    /**
     * Returns the real size of the serialized transaction
     *
     * @return size
     */
    default int size() {
        return array().length;
    }

}
