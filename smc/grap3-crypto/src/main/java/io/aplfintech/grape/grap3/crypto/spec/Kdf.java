package io.aplfintech.grape.grap3.crypto.spec;

import com.fasterxml.jackson.annotation.JsonProperty;

/**
 * @author andrew.zinchenko@gmail.com
 */
public enum Kdf {
    @JsonProperty("scrypt")
    SCRYPT
}
