package io.aplfintech.grape.vm.env;

import io.aplfintech.grape.bcei.StateAccess;
import io.aplfintech.grape.env.BlockContext;
import io.aplfintech.grape.env.Context;
import io.aplfintech.grape.model.Address;
import lombok.NonNull;
import lombok.ToString;
import lombok.experimental.Delegate;
import lombok.extern.slf4j.Slf4j;

import java.math.BigInteger;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
@ToString
public class VmContext implements Context {
    private final Address origin;
    private final BigInteger gasPrice;
    @Delegate
    private final BlockContext blockContext;

    public VmContext(@NonNull Address origin, BigInteger gasPrice, @NonNull BlockContext blockContext) {
        this.origin = origin;
        this.gasPrice = gasPrice;
        this.blockContext = blockContext;
    }

    @Override
    public Address getOrigin() {
        return origin;
    }

    @Override
    public BigInteger gasPrice() {
        return gasPrice;
    }

    @Override
    public boolean canTransfer(StateAccess stateAccess, Address address, BigInteger value) {
        var balance = stateAccess.getBalance(address);
        boolean b = balance.compareTo(value) >= 0;
        log.trace("Transfer is permitted={}, have={} want={}", b, balance, value);
        return b;
    }

    @Override
    public void transfer(StateAccess stateAccess, Address from, Address to, BigInteger value) {
        stateAccess.subBalance(from, value);
        stateAccess.addBalance(to, value);
        log.trace("Transferred from={} to={} value={}", from.hexAddress(), to.hexAddress(), value);
    }
}
