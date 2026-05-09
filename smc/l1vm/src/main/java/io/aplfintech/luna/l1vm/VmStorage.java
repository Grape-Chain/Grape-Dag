package io.aplfintech.luna.l1vm;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.google.common.base.Preconditions;
import io.aplfintech.luna.model.Address;
import io.aplfintech.luna.model.Key;
import lombok.NonNull;

import java.util.ArrayDeque;
import java.util.Deque;
import java.util.Map;

/**
 * Abstract storage with journal of modifications for VM
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public class VmStorage implements Storage {
    private final Storage storage;
    private final Deque<VmStorage.Modification> changeJournal;
    private final Deque<Integer> indices;

    public VmStorage(@NonNull Storage storage) {
        this.storage = storage;
        this.changeJournal = new ArrayDeque<>();
        this.indices = new ArrayDeque<>();
        indices.push(0);
    }

    @Override
    public boolean containsMapping(Address address) {
        Preconditions.checkNotNull(address, "address");
        return storage.containsMapping(address);
    }

    @Override
    public Map<Key, byte[]> getMapping(Address address) {
        Preconditions.checkNotNull(address, "address");
        return storage.getMapping(address);
    }

    /**
     * {@inheritDoc}
     */
    public byte[] get(Address address, byte[] key) {
        Preconditions.checkNotNull(address, "address");
        Preconditions.checkNotNull(key, "key");
        return storage.get(address, key);
    }

    /**
     * Put the given value for the address and key
     *
     * @param address the address of the contract for which the key is being set
     * @param key     the slot to set for the address
     * @param value   the new value of the transient storage slot to set
     */
    public byte[] put(Address address, byte[] key, byte[] value) {
        Preconditions.checkNotNull(address, "address");
        Preconditions.checkNotNull(key, "key");
        Preconditions.checkNotNull(value, "value");
        Preconditions.checkArgument(key.length == 32, "Storage key must be 32 bytes long");
        Preconditions.checkArgument(value.length <= 32, "Storage value cannot be longer than 32 bytes");

        var prevValue = storage.put(address, key, value);
        changeJournal.push(new Modification(address, key, prevValue));
        return prevValue;
    }

    /**
     * Commit all the changes since the last checkpoint
     */
    public void commit() {
        Preconditions.checkState(!indices.isEmpty(), "Nothing to commit");
        indices.pop();
    }

    /**
     * To be called whenever entering a new context. If revert is called after checkpoint,
     * all changes after the latest checkpoint are reverted.
     */
    public void checkpoint() {
        indices.push(changeJournal.size());
    }

    /**
     * Reverts the transient storage to the last checkpoint
     */
    public void revert() {
        Preconditions.checkState(!indices.isEmpty(), "Nothing to revert");
        var lastCheckpoint = indices.pop();
        for (int i = changeJournal.size(); i > lastCheckpoint; i--) {
            var m = changeJournal.pop();
            if (storage.containsMapping(m.address)) {
                storage.put(m.address, m.key, m.value);
            }
        }
    }

    /**
     * Clears the entire storage and it's state
     */
    public void clear() {
        storage.clear();
        changeJournal.clear();
        indices.clear();
    }

    /**
     * Returns the content of storage as JSON string
     * Json format:
     * { [address: string]: { [key: string]: string } }
     *
     * @return content of the storage as a JSON string
     */
    public String toJSON() throws JsonProcessingException {
        return storage.toJSON();
    }

    protected record Modification(Address address, byte[] key, byte[] value) {
    }

}
