package io.aplfintech.grape.model;

import java.util.Arrays;

/**
 * General interface for all addressable entities, such as {@link Account}
 *
 * @author andrew.zinchenko@gmail.com
 * @see Account
 * @since 0.1
 */
public interface Address extends Addressable, Key {
    int ADDRESS_LENGTH = 20;

    /**
     * Returns the byte array of address value
     *
     * @return the byte array of address value
     */
    @Override
    byte[] bytes();

    /**
     * Returns the hex string of address value
     *
     * @return the hex string of address value
     */
    String hexAddress();

    @Override
    default String hex() {
        return hexAddress();
    }

    /**
     * Returns true if current address value is undefined
     * i.e. the length of byte array equals 0
     *
     * @return true iif current address is undefined
     */
    default boolean isUndefined() {
        for (byte b : bytes()) { // zero length arrays and arrays having all zeros are considered undefined for compatibility reasons
            if (b != 0) {
                return false;
            }
        }
        return true;
    }

    default Address address() {
        return this;
    }
}
