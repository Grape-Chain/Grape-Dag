package io.aplfintech.luna.l1vm.contract;

import io.aplfintech.luna.vm.contract.GasValve;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public class GasPool implements GasValve {
    private final long gasLimit;
    private long gas;

    public GasPool(long gasLimit) {
        this.gasLimit = gasLimit;
        this.gas = gasLimit;
    }

    @Override
    public long gas() {
        return gas;
    }

    @Override
    public long gasUsed() {
        return gasLimit - gas;
    }

    @Override
    public void addGas(long value) {
        gas += value;
    }

    @Override
    public boolean useGas(long requestedGas) {
        if (gas < requestedGas) {
            return false;
        }
        gas -= requestedGas;
        return true;
    }

    @Override
    public void resetGas() {
        gas = 0;
    }

    @Override
    public String toString() {
        return String.valueOf(gas);
    }
}
