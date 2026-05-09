package io.aplfintech.luna.l1vm.opcode;

import lombok.NonNull;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public enum Feature {
    WARMED_ACCESS("Warmed access"),
    INIT_CODE_WORD_COST("Init code word cost"),
    SELF_DESTRUCT("Self destruct");

    private final String key;

    Feature(@NonNull String key) {
        this.key = key;
    }

    /**
     * Returns key for searching the feature in the chain config
     */
    public String key() {
        return key;
    }
}
