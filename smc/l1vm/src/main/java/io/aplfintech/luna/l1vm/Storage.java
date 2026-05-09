package io.aplfintech.luna.l1vm;

import com.fasterxml.jackson.core.JsonProcessingException;
import io.aplfintech.luna.model.Address;
import io.aplfintech.luna.model.Key;

import java.util.Map;

/**
 * Simple storage model for VM
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface Storage {
    /**
     * Returns true if the storage contains mapping for the given address
     *
     * @param address the address
     * @return true if the storage contains mapping for the given address
     */
    boolean containsMapping(Address address);

    /**
     * Returns the mapping for the given address
     *
     * @param address the address
     * @return the mapping for the given address
     */
    Map<Key, byte[]> getMapping(Address address);

    /**
     * Get the value for the given address and key or zero-filled 32-byte array
     * if storage doesn't contain a mapping from an address and a key to a value
     *
     * @param address the address for which transient storage is accessed
     * @param key     the key of the address to get
     * @return the value for the given address and key or zero-filled 32-byte array
     * if storage doesn't contain a mapping from an address and a key to a value
     */
    byte[] get(Address address, byte[] key);

    /**
     * Put the given value for the address and key and returns the previous value or zero-filled 32-byte array
     * if storage doesn't contain a mapping from an address and a key to a value
     *
     * @param address the address of the contract for which the key is being set
     * @param key     the slot to set for the address
     * @param value   the new value of the transient storage slot to set
     * @return the previous value or zero-filled 32-bytes array
     * if storage doesn't contain a mapping from an address and a key to a value
     */
    byte[] put(Address address, byte[] key, byte[] value);

    /**
     * Commit all the changes since the last checkpoint
     */
    void commit();

    /**
     * To be called whenever entering a new context. If revert is called after checkpoint,
     * all changes after the latest checkpoint are reverted.
     */
    void checkpoint();

    /**
     * Reverts the transient storage to the last checkpoint
     */
    void revert();

    /**
     * Clears the entire storage and it's state
     */
    void clear();

    /**
     * Returns the content of storage as JSON string
     * Json format:
     * { [address: string]: { [key: string]: string } }
     *
     * @return content of the storage as a JSON string
     */
    String toJSON() throws JsonProcessingException;
}
