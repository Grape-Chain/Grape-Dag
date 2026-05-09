package io.aplfintech.luna.vm.impl;

import io.aplfintech.luna.vm.ExecutionStatus;
import io.aplfintech.luna.vm.MessageResult;
import io.aplfintech.luna.vm.contract.ContractResult;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
record VmMessageResult(ContractResult contractResult, long initGas, long refundGas, long usedGas,
                       long gas) implements MessageResult {

    @Override
    public ExecutionStatus executionStatus() {
        return contractResult.executionStatus();
    }

    @Override
    public byte[] output() {
        return contractResult.output();
    }

    @Override
    public String toString() {
        return "VmMessageResult{" +
                "contractResult=" + contractResult +
                ", initGas=" + initGas +
                ", refundGas=" + refundGas +
                ", usedGas=" + usedGas +
                ", gas=" + gas +
                '}';
    }
}
