package io.aplfintech.grape.grapech;

import lombok.NonNull;

import java.math.BigInteger;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface MessageBuilder {
    MessageBuilder from(@NonNull String senderHex);

    MessageBuilder from(byte @NonNull [] sender);

    MessageBuilder to(@NonNull String recipientHex);

    MessageBuilder to(byte @NonNull [] recipient);

    MessageBuilder amount(long amount);

    MessageBuilder amount(@NonNull BigInteger amount);

    MessageBuilder data(@NonNull String dataHex);

    MessageBuilder data(byte @NonNull [] data);

    String buildJson();

}
