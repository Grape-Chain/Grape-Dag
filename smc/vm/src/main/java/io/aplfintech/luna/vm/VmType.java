package io.aplfintech.luna.vm;

import lombok.Getter;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public enum VmType {
    EVM("Ethereum Virtual Machine"),
    WASM("WASM Virtual Machine; Not implemented yet");

    @Getter
    private final String fullName;

    VmType(String fullName) {
        this.fullName = fullName;
    }
}
