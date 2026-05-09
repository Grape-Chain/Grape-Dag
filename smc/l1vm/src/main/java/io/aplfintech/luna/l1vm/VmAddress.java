package io.aplfintech.luna.l1vm;

import com.fasterxml.jackson.annotation.JsonProperty;
import com.google.common.base.Preconditions;
import io.aplfintech.luna.model.Address;
import io.aplfintech.luna.utils.Bytes;
import io.aplfintech.luna.utils.HexUtils;
import lombok.NonNull;

import java.util.Arrays;

/**
 * Simple implementation of addressable instance to represent a contract address or account address
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public class VmAddress implements Address {

    public static final Address UNDEFINED_ADDRESS = new VmAddress(new byte[0]);
    public static final Address ZERO_ADDRESS = VmAddress.from(new byte[]{0});

    @JsonProperty("bytes")
    private final byte[] bytes;

    private VmAddress(byte[] bytes) {
        Preconditions.checkNotNull(bytes, "address is null");
        this.bytes = bytes;
    }

    public static VmAddress from(String hex) {
        Preconditions.checkNotNull(hex, "hex is null");
        return from(HexUtils.parseHex(hex));
    }

    /**
     * Creates address from the lower 20 bytes of the give array
     */
    public static VmAddress from(byte @NonNull [] bytes) {
        if (bytes.length > ADDRESS_LENGTH) {
            return new VmAddress(Bytes.slice(bytes, bytes.length - ADDRESS_LENGTH));
        }
        if (bytes.length < ADDRESS_LENGTH) {
            return new VmAddress(Bytes.leftPadBytes(bytes, ADDRESS_LENGTH));
        }
        return new VmAddress(bytes);
    }

    @Override
    public byte[] bytes() {
        return bytes;
    }

    @Override
    public String hexAddress() {
        return HexUtils.toHex(bytes, true);
    }

    @Override
    public String toString() {
        return hexAddress();
    }

    @Override
    public boolean equals(Object o) {
        if (this == o) return true;
        if (o == null || getClass() != o.getClass()) return false;

        VmAddress vmAddress = (VmAddress) o;

        return Arrays.equals(bytes, vmAddress.bytes);
    }

    @Override
    public int hashCode() {
        return Arrays.hashCode(bytes);
    }
}
