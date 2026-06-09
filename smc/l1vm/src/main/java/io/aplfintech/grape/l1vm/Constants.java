package io.aplfintech.grape.l1vm;

import io.aplfintech.grape.model.Hash;
import io.aplfintech.grape.utils.HexUtils;
import lombok.AccessLevel;
import lombok.NoArgsConstructor;

/**
 * VM Constants
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@NoArgsConstructor(access = AccessLevel.PRIVATE)
public class Constants {
    /**
     * Keccak-256 hash of null
     */
    public static final String KECCAK256_NULL_S = "c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470";

    /**
     * Keccak-256 hash of null
     */
    public static final byte[] KECCAK256_NULL = HexUtils.fromHex(KECCAK256_NULL_S);

    public static final Hash KECCAK256_NULL_HASH = new Hash(KECCAK256_NULL);


}
