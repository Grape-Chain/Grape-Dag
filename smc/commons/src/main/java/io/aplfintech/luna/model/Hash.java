package io.aplfintech.luna.model;

import com.fasterxml.jackson.annotation.JsonProperty;
import io.aplfintech.luna.utils.Bytes;
import io.aplfintech.luna.utils.HexUtils;
import lombok.EqualsAndHashCode;
import lombok.NonNull;

/**
 * Represents the 32 byte array of arbitrary data
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@EqualsAndHashCode
public class Hash implements Key {
    @JsonProperty
    private final byte[] bytes;

    public Hash(byte @NonNull [] bytes) {
        this.bytes = Bytes.leftPadBytes(bytes, 32);
    }

    public static Hash from(@NonNull Key key) {
        return new Hash(key.bytes());
    }

    @Override
    public byte[] bytes() {
        return bytes;
    }

    @Override
    public String hex() {
        return HexUtils.toHex(bytes, true);
    }

    @Override
    public String toString() {
        return hex();
    }
}
