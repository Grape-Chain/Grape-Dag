package io.aplfintech.luna.vm.tx;

import io.aplfintech.luna.env.BlockContext;
import io.aplfintech.luna.l1vm.VmAddress;
import io.aplfintech.luna.model.Address;
import io.aplfintech.luna.utils.HexUtils;

import java.math.BigInteger;
import java.util.Random;

/**
 * The fake block for local testing
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public class MockBlock implements BlockContext {
    private static final Address COIN_BASE = VmAddress.from(HexUtils.parseHex("0x8626f6940E2eb28930eFb4CeF49B2d1F2C9C1199"));
    private static final long TIME_STAMP = 1666073130L;//block timestamp in seconds, UTC 10/18/2022 @ 6:05am;
    private static final byte[] RANDAO;

    static {
        RANDAO = new byte[32];
        var rnd = new Random(TIME_STAMP);
        rnd.nextBytes(RANDAO);
    }

    private final long timestamp;

    public MockBlock() {
        this(TIME_STAMP);
    }

    public MockBlock(long timestamp) {
        this.timestamp = timestamp;
    }

    @Override
    public BigInteger blockNumber() {
        return BigInteger.ONE;
    }

    @Override
    public Address coinbase() {
        return COIN_BASE;
    }

    @Override
    public long timestamp() {
        return timestamp;
    }

    @Override
    public byte[] prevRandao() {
        return RANDAO;
    }

    @Override
    public BigInteger gasLimit() {
        return BigInteger.valueOf(10_000_000L);
    }

    @Override
    public BigInteger baseFeePerGas() {
        return BigInteger.ONE;
    }

    @Override
    public String toString() {
        return "Block{" +
            "number=" + blockNumber() +
            ", coinBase=" + coinbase().hexAddress() +
            ", timestamp=" + timestamp() +
            ", gasLimit=" + gasLimit() +
            ", baseFee=" + baseFeePerGas() +
            '}';
    }
}
