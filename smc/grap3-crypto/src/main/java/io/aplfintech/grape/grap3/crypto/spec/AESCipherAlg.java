package io.aplfintech.grape.grap3.crypto.spec;

import com.fasterxml.jackson.annotation.JsonProperty;

/**
 * @author andrew.zinchenko@gmail.com
 */
public enum AESCipherAlg {
    @JsonProperty("aes-256-gcm")
    AES_256_GCM("AES/GCM/NoPadding");

    public final String algorithm;

    AESCipherAlg(String alg) {
        this.algorithm = alg;
    }

    public static final String NAME = "AES";
}
