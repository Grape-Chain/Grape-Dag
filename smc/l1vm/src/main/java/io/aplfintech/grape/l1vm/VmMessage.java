package io.aplfintech.grape.l1vm;

import com.google.common.base.Preconditions;
import io.aplfintech.grape.model.Address;
import io.aplfintech.grape.utils.Hex;
import io.aplfintech.grape.vm.Message;
import io.aplfintech.grape.utils.HexUtils;
import lombok.NonNull;
import lombok.Setter;

import java.math.BigInteger;

/**
 * Simple implementation of the 'message' entity.
 * Message call is a model by the means of contract can call other contracts or send coins to non-contract accounts.
 * Message calls are similar to transactions, in that they have a source, a target, data payload, amount, gas and return data.
 * In fact, every transaction consists of a top-level message call which in turn can create further message calls.
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public class VmMessage implements Message {
    /**
     * Message sender nonce
     */
    @Setter
    long nonce;
    /**
     * Message sender address
     */
    @Setter
    Address from;
    /**
     * Message destination address.
     * This is the address of the account which storage/balance/nonce is going to be modified
     * by the message execution
     */
    @Setter
    Address to;
    /**
     * The amount of coins transferred with the message
     */
    @Setter
    BigInteger amount;
    /**
     * The gas limit for the call
     */
    @Setter
    BigInteger gasLimit;
    /**
     * The gas price for the call
     */
    @Setter
    BigInteger gasPrice;
    /**
     * Input data
     */
    @Setter
    byte[] data;
    boolean fake;

    public VmMessage(long nonce, Address from, Address to, BigInteger amount, BigInteger gasLimit, BigInteger gasPrice, byte[] data, boolean fake) {
        this.nonce = nonce;
        this.from = from;
        this.to = to;
        this.amount = amount;
        this.gasLimit = gasLimit;
        this.gasPrice = gasPrice;
        this.data = data;
        this.fake = fake;
    }

    @Override
    public long nonce() {
        return nonce;
    }

    @Override
    public Address from() {
        return from;
    }

    @Override
    public Address to() {
        return to;
    }

    @Override
    public BigInteger amount() {
        return amount;
    }

    @Override
    public BigInteger gasPrice() {
        return gasPrice;
    }

    @Override
    public BigInteger gasLimit() {
        return gasLimit;
    }

    @Override
    public byte[] data() {
        return data;
    }

    @Override
    public boolean isFake() {
        return fake;
    }

    @Override
    public String toString() {
        return "VmMessage{" +
                "from=" + Hex.toHex(from) +
                ", to=" + Hex.toHex(to) +
                ", amount=" + amount +
                ", nonce=" + nonce +
                ", gasLimit=" + gasLimit +
                ", gasPrice=" + gasPrice +
                ", data=" + HexUtils.toHex(data) +
                ", fake=" + fake +
                '}';
    }

    public static Builder builder() {
        return new Builder();
    }

    public static Builder toBuilder(Message message) {
        return new Builder(message.nonce(), message.from(), message.to(), message.amount(),
                message.gasLimit(), message.gasPrice(), message.data(), message.isFake());
    }

    public static final class Builder {
        private long nonce = 0;
        private Address from;
        private Address to;
        private BigInteger amount;
        private BigInteger gasLimit;
        private BigInteger gasPrice;
        private byte[] data;
        private boolean fake;

        private Builder() {
        }

        private Builder(long nonce, Address from, Address to, BigInteger amount, BigInteger gasLimit, BigInteger gasPrice, byte[] data, boolean fake) {
            this.nonce = nonce;
            this.from = from;
            this.to = to;
            this.amount = amount;
            this.gasLimit = gasLimit;
            this.gasPrice = gasPrice;
            this.data = data;
            this.fake = fake;
        }

        public Builder nonce(long nonce) {
            Preconditions.checkArgument(nonce >= 0, "Nonce must be positive");
            this.nonce = nonce;
            return this;
        }

        public Builder from(@NonNull Address from) {
            this.from = from;
            return this;
        }

        public Builder to(@NonNull Address to) {
            this.to = to;
            return this;
        }

        public Builder amount(@NonNull BigInteger amount) {
            Preconditions.checkArgument(amount.signum() >= 0, "Amount must be positive");
            this.amount = amount;
            return this;
        }

        public Builder gasLimit(@NonNull BigInteger gasLimit) {
            Preconditions.checkArgument(gasLimit.signum() >= 0, "Fuel limit must be positive");
            this.gasLimit = gasLimit;
            return this;
        }

        public Builder gasPrice(@NonNull BigInteger gasPrice) {
            Preconditions.checkArgument(gasPrice.signum() >= 0, "Gas price must be positive");
            this.gasPrice = gasPrice;
            return this;
        }

        public Builder data(byte[] data) {
            this.data = data;
            return this;
        }

        public Builder fake(boolean fake) {
            this.fake = fake;
            return this;
        }

        public VmMessage build() {
            if (amount == null) {
                amount = BigInteger.ZERO;
            }
            if (gasLimit == null) {
                gasLimit = BigInteger.ZERO;
            }
            if (gasPrice == null) {
                gasPrice = BigInteger.ZERO;
            }
            return new VmMessage(nonce, from, to, amount, gasLimit, gasPrice, data, fake);
        }
    }
}
