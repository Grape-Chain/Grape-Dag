package io.aplfintech.luna.lunach;

import com.google.common.base.Preconditions;
import lombok.NonNull;

import java.math.BigInteger;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface TxBuilder {
    enum Type {
        PAYMENT,//0
        PUBLISH_CONTRACT,//1
        CALL_CONTRACT;//2

        public static Type from(int ordinal) {
            Preconditions.checkElementIndex(ordinal, 3);
            return values()[ordinal];
        }
    }

    TxBuilderImpl type(@NonNull Type type);

    TxBuilderImpl chainId(int chainId);

    TxBuilderImpl senderPublicKey(@NonNull String publicKeyHex);

    TxBuilderImpl senderPublicKey(byte @NonNull [] pk);

    TxBuilderImpl recipient(@NonNull String recipientHex);

    TxBuilderImpl recipient(byte @NonNull [] recipient);

    TxBuilderImpl nonce(long nonce);

    TxBuilderImpl amount(long amount);

    TxBuilderImpl amount(@NonNull BigInteger amount);

    TxBuilderImpl fuelLimit(long fuelLimit);

    TxBuilderImpl fuelLimit(@NonNull BigInteger fuelLimit);

    TxBuilderImpl fuelPrice(long fuelPrice);

    TxBuilderImpl fuelPrice(@NonNull BigInteger fuelPrice);

    TxBuilderImpl data(@NonNull String dataHex);

    TxBuilderImpl data(byte @NonNull [] data);

    String sign(@NonNull String privateKeyHex);

    byte[] sign(byte @NonNull [] privateKey);

    String build(@NonNull String privateKeyHex);

    byte[] build(byte @NonNull [] privateKey);

    byte[] build();

    String buildJson(@NonNull String privateKeyHex);

    String buildJson(byte @NonNull [] privateKey);

    String buildJson();
}
