package io.aplfintech.grape.l1vm;

import com.fasterxml.jackson.annotation.JsonIgnore;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.google.common.base.Preconditions;
import io.aplfintech.grape.l1vm.contract.GasPool;
import io.aplfintech.grape.model.Address;
import io.aplfintech.grape.vm.ExecutionStatus;
import io.aplfintech.grape.vm.Log;
import io.aplfintech.grape.vm.VmStatus;
import io.aplfintech.grape.vm.contract.ContractResult;
import io.aplfintech.grape.vm.contract.GasValve;
import io.aplfintech.grape.utils.HexUtils;

/**
 * Result of executing a message via the VM
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */

public class VmResult implements ContractResult {
    private final Address contractAddress;
    private final ExecutionStatus status;
    private final GasValve gasValve;
    private final byte[] output;

    private final String errorDescription;

    /**
     * Array of logs that the contract emitted
     */
    @JsonIgnore
    private final Log[] eventLog;

    VmResult(ExecutionStatus executionStatus, GasValve gas, Log[] eventLog, byte[] output, Address contractAddress) {
        this(executionStatus, gas, eventLog, output, contractAddress, null);
    }

    VmResult(ExecutionStatus executionStatus, GasValve gas, Log[] eventLog, byte[] output, Address contractAddress, String errorDescription) {
        this.contractAddress = contractAddress;
        this.status = executionStatus;
        this.gasValve = gas;
        this.output = output;
        this.eventLog = eventLog;
        this.errorDescription = errorDescription;
    }

    public static VmResult error(ExecutionStatus executionStatus, long gas, Address contractAddress) {
        return error(executionStatus, gas, contractAddress, null);
    }

    public static VmResult error(ExecutionStatus executionStatus, long gas, Address contractAddress, String errorDescription) {
        Preconditions.checkArgument(executionStatus.isFailure(), "The execution status doesn't have an error.");
        return new VmResult(executionStatus, new GasPool(gas), new Log[0], null, contractAddress, errorDescription);
    }

    public static VmResult success(long gas, Address contractAddress) {
        return new VmResult(VmStatus.VM_SUCCESS, new GasPool(gas), new Log[0], null, contractAddress, null);
    }

    @Override
    @JsonProperty(value = "contract")
    public Address contract() {
        return contractAddress;
    }

    @JsonProperty("gas")
    @Override
    public long gas() {
        return gasValve.gas();
    }

    @JsonProperty("gasUsed")
    @Override
    public long gasUsed() {
        return gasValve.gasUsed();
    }

    @Override
    public void resetGas() {
        gasValve.resetGas();
    }

    @JsonProperty("status")
    @Override
    public ExecutionStatus executionStatus() {
        return status;
    }

    @JsonProperty("output")
    @Override
    public byte[] output() {
        return output;
    }

    @JsonIgnore
    @Override
    public Log[] getEventLog() {
        return eventLog;
    }

    @Override
    public String errorDescription() {
        return errorDescription;
    }

    @Override
    public String toString() {
        return "VmResult{" +
            "status=" + status +
            ", remaining gas=" + gasValve.gas() +
            ", used gas=" + gasValve.gasUsed() +
            ", output=" + HexUtils.toHex(output, true) +
            ", errorDescription=" + errorDescription +
            '}';
    }
}
