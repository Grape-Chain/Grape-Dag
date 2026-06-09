package io.aplfintech.grape.l1vm;

import io.aplfintech.grape.math.Math256;
import io.aplfintech.grape.utils.Exceptions;
import io.aplfintech.grape.vm.Memory;
import io.aplfintech.grape.vm.VmStatus;

import java.util.Arrays;

/**
 * Simple memory model implementation for the VM
 * The word size is 256bit
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public class VmMemory implements Memory {
    private static final int MAX_ARRAY_SIZE = Integer.MAX_VALUE - 8;

    private byte[] store;

    public VmMemory() {
        this.store = new byte[]{};
    }

    /**
     * Expands the memory given an offset and size.
     * Rounds extended memory to word-size.
     *
     * @param offset starting position
     * @param size   size of word in bytes
     */
    @Override
    public void expand(int offset, int size) {
        if (size == 0) {
            return;
        }
        int newSize = Math256.ceil(offset + size, Math256.WORD_SIZE);
        var sizeDiff = newSize - store.length;
        if (sizeDiff > 0) {
            if (newSize - MAX_ARRAY_SIZE > 0) {
                throw Exceptions.from(VmStatus.VM_OUT_OF_MEMORY);
            }
            store = Arrays.copyOf(store, newSize);
        }
    }

    /**
     * {@inheritDoc}
     */
    @Override
    public void write(int offset, int size, byte[] data) {
        if (size == 0) {
            return;
        }
        expand(offset, size);
        if (data != null) {
            if (data.length < size) throw Exceptions.from(VmStatus.VM_ARGUMENT_OUT_OF_RANGE);
            if (offset + size > store.length) throw Exceptions.from(VmStatus.VM_MEMORY_CAPACITY_EXCEEDS);

            System.arraycopy(data, 0, store, offset, size);
        }
    }

    /**
     * {@inheritDoc}
     */
    @Override
    public byte[] read(int offset, int size) {
        if (size == 0) {
            return new byte[0];
        }
        expand(offset, size);
        var loaded = new byte[size];
        System.arraycopy(store, offset, loaded, 0, size);
        return loaded;
    }

    @Override
    public int size() {
        return store.length;
    }

    @Override
    public String toString() {
        return "VmMemory{" +
            "store.length=" + store.length +
            ", word.count=" + store.length / 32 +
            '}';
    }
}
