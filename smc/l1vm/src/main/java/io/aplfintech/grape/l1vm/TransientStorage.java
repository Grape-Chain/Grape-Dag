package io.aplfintech.grape.l1vm;

import lombok.experimental.Delegate;

/**
 * Transient storage is a simple in-memory storage without journaling
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
//TODO This store implementation used by instructions TLOAD (0xb3) and TSTORE (oxb4), the behavior is not tested yet
public class TransientStorage implements Storage {
    @Delegate
    private final Storage storage;//in-memory storage

    public TransientStorage() {
        this.storage = new SimpleStorage();
    }

}
