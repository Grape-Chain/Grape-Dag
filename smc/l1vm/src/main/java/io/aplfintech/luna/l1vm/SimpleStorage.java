package io.aplfintech.luna.l1vm;

import com.fasterxml.jackson.core.JsonProcessingException;
import io.aplfintech.luna.model.Address;
import io.aplfintech.luna.model.Hash;
import io.aplfintech.luna.model.Key;
import io.aplfintech.luna.utils.JsonUtils;

import java.util.HashMap;
import java.util.Map;

/**
 * Simple in-memory storage without journaling
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public class SimpleStorage implements Storage {
    private final Map<Address, Map<Key, byte[]>> storage;

    public SimpleStorage() {
        this.storage = new HashMap<>();
    }

    @Override
    public boolean containsMapping(Address address) {
        return storage.containsKey(address);
    }

    @Override
    public Map<Key, byte[]> getMapping(Address address) {
        return storage.get(address);
    }

    @Override
    public byte[] get(Address address, byte[] key) {
        var map = storage.get(address);
        if (map == null) {
            return new byte[32];
        }
        return map.getOrDefault(new Hash(key), new byte[32]);
    }

    @Override
    public byte[] put(Address address, byte[] key, byte[] value) {
        storage.putIfAbsent(address, new HashMap<>());
        var map = storage.get(address);
        var prevValue = map.getOrDefault(new Hash(key), new byte[32]);
        map.put(new Hash(key), value);
        return prevValue;
    }

    @Override
    public void commit() {
        //do nothing, because journaling is not supported
    }

    @Override
    public void checkpoint() {
        //do nothing, because journaling is not supported
    }

    @Override
    public void revert() {
        //do nothing, because journaling is not supported
    }

    @Override
    public void clear() {
        storage.clear();
    }

    @Override
    public String toJSON() throws JsonProcessingException {
        return JsonUtils.HEX_MAPPER.writeValueAsString(storage);
    }
}
