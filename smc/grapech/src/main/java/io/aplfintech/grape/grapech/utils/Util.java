package io.aplfintech.grape.grapech.utils;

import com.google.protobuf.ByteString;
import lombok.AccessLevel;
import lombok.NoArgsConstructor;
import lombok.extern.slf4j.Slf4j;

import java.math.BigInteger;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
@NoArgsConstructor(access = AccessLevel.PRIVATE)
public class Util {
    public static BigInteger unsignedBigIntFromBytes(ByteString byteString) {
        if (byteString.toByteArray().length == 0) {
            return BigInteger.ZERO;
        }
        return new BigInteger(1, byteString.toByteArray());
    }
}
