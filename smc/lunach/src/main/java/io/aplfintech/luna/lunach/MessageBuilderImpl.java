package io.aplfintech.luna.lunach;

import com.fasterxml.jackson.annotation.JsonProperty;
import com.google.common.base.Preconditions;
import com.google.protobuf.ByteString;
import io.aplfintech.luna.utils.Bytes;
import io.aplfintech.luna.utils.HexUtils;
import io.aplfintech.luna.utils.JsonUtils;
import lombok.Builder;
import lombok.Getter;
import lombok.NonNull;
import lombok.SneakyThrows;
import lombok.extern.slf4j.Slf4j;
import pb.Vm;

import java.math.BigInteger;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
public class MessageBuilderImpl implements MessageBuilder {
    private byte[] senderAddress;

    /**
     * Recipient address
     */
    private byte[] recipient;

    /**
     * Amount of coin units (cents) being transferred,
     * must be >=0
     */
    private BigInteger amount;

    /**
     * user-defined information of the transaction
     */
    private byte[] data;

    MessageBuilderImpl() {
        //set default values
        this.recipient = null;
        this.amount = BigInteger.ZERO;
        this.data = null;
    }

    @Override
    public MessageBuilderImpl from(@NonNull String senderHex) {
        return from(HexUtils.parseHex(senderHex));
    }

    @Override
    public MessageBuilderImpl from(byte @NonNull [] sender) {
        this.senderAddress = sender;
        return this;
    }

    @Override
    public MessageBuilderImpl to(@NonNull String recipientHex) {
        return to(HexUtils.parseHex(recipientHex));
    }

    @Override
    public MessageBuilderImpl to(byte @NonNull [] recipient) {
        this.recipient = recipient;
        return this;
    }

    @Override
    public MessageBuilderImpl amount(long amount) {
        return amount(BigInteger.valueOf(amount));
    }

    @Override
    public MessageBuilderImpl amount(@NonNull BigInteger amount) {
        Preconditions.checkArgument(amount.signum() >= 0, "Amount=%s, is not positive value", amount.toString());
        this.amount = amount;
        return this;
    }

    @Override
    public MessageBuilderImpl data(@NonNull String dataHex) {
        return data(HexUtils.parseHex(dataHex));
    }

    @Override
    public MessageBuilderImpl data(byte @NonNull [] data) {
        this.data = data;
        return this;
    }

    /**
     * Returns the Message object for estimating the contract calling
     *
     * @return the Message object for estimating the contract calling
     */
    @SneakyThrows
    @Override
    public String buildJson() {
        Preconditions.checkNotNull(this.data, "contract code is NULL.");
        var message = CallMessage.builder()
            .sender(senderAddress == null ? "0x" : HexUtils.toHex(this.senderAddress, true))
            .recipient(recipient == null ? "0x" : HexUtils.toHex(this.recipient, true))
            .amount(this.amount.toString())
            .data(HexUtils.toHex(this.data, true))
            .build();
        return JsonUtils.HEX_MAPPER.writeValueAsString(message);
    }

    private Vm.CallMessage getPbMessageBytes() {
        return Vm.CallMessage.newBuilder()
            .setFrom(Vm.Address.newBuilder().setAddBytes(ByteString.copyFrom(this.senderAddress)))
            .setTo(Vm.Address.newBuilder().setAddBytes(ByteString.copyFrom(this.recipient)))
            .setAmount(ByteString.copyFrom(Bytes.asUnsignedByteArray(this.amount)))
            .setData(ByteString.copyFrom(this.data))
            .build();
    }

    @Builder
    @Getter
    private static class CallMessage {
        @JsonProperty("sender")
        private String sender;

        /**
         * Recipient address
         */
        @JsonProperty("recipient")
        private String recipient;

        /**
         * Amount of coin units (cents) being transferred,
         * must be >=0
         */
        @JsonProperty("amount")
        private String amount;

        /**
         * user-defined information of the transaction
         */
        @JsonProperty("data")
        private String data;

    }

}
