package io.aplfintech.grape.l1vm.contract;

import io.aplfintech.grape.l1vm.opcode.OpCodes;
import io.aplfintech.grape.model.Address;
import io.aplfintech.grape.model.Addressable;
import io.aplfintech.grape.vm.contract.Code;
import io.aplfintech.grape.vm.contract.CodeAndHash;
import io.aplfintech.grape.vm.contract.Contract;
import io.aplfintech.grape.vm.contract.ContractRef;
import io.aplfintech.grape.vm.contract.GasValve;
import io.aplfintech.grape.utils.HexUtils;
import lombok.Getter;
import lombok.NonNull;
import lombok.Setter;
import lombok.experimental.Delegate;
import lombok.extern.slf4j.Slf4j;

import java.math.BigInteger;
import java.util.Arrays;

/**
 * The contract representation in the state
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
public class VmContract implements Contract {
    // The address of the caller which initialised this contract
    private Address callerAddr;
    private final ContractRef caller;
    private final Addressable self;
    private BigInteger value;
    @Delegate
    private final GasValve gas;

    // contract code address
    private Address codeAddr;
    private byte[] codeHash;
    //contract code
    private Code code;
    /**
     * array of values where validJumps[index] has value 0 (default), 1 (jumpdest), 2 (beginsub)
     */
    private byte[] validJumps;

    @Getter
    @Setter
    private byte[] input;

    //Used by the VM to create new contract (transaction execution)
    public VmContract(@NonNull ContractRef caller, @NonNull Addressable contract, BigInteger value, long gas) {
        this.caller = caller;
        this.callerAddr = caller.address();
        this.self = contract;
        this.value = value;
        this.gas = new GasPool(gas);
    }

    /**
     * Sets the contract to be a delegate call and returns the current
     * contract (for chaining calls)
     *
     * @return the current contract for chaining call
     */
    public VmContract asDelegate() {
        if (caller instanceof VmContract contract) {
            var parent = contract.caller;
            this.callerAddr = parent.address();
            this.value = parent.value();
            return this;
        }
        throw new IllegalStateException("DELEGATE CALL is not permitted in this context because parent caller is NULL");
    }

    /**
     * Sets the code of the contract and address of the backing data object
     */
    public void setCallCode(Address address, CodeAndHash codeAndHash) {
        this.code = codeAndHash;
        this.codeHash = codeAndHash.codeHash();
        this.codeAddr = address;
    }

    @Override
    public Address caller() {
        return callerAddr;
    }

    @Override
    public Address address() {
        return self.address();
    }

    @Override
    public BigInteger value() {
        return value;
    }

    @Override
    public Code code() {
        return code;
    }

    @Override
    public byte getOPCode(long pc) {
        if (pc < code.size()) {
            return code.getOpCode(pc);
        }
        return OpCodes.STOP.getCode();
    }

    @Override
    public boolean isValidJumpDest(long dest, int jumpKind) {
        if (dest >= code.size()) {
            return false;
        }
        if (validJumps == null) {
            //TODO to increase the performance use memoization for validJumps,
            //Only use cache for regular contract (runtime code) not for init code (published code)
            //for example, it's helpfully for contracts such as Multicall
            validJumps = OpCodes.getValidJumps(code.bytes());
        }
        return validJumps[(int) dest] == jumpKind;
    }

    @Override
    public String toFullString() {
        return toStringInt(true);
    }

    @Override
    public String toString() {
        return toStringInt(false);
    }

    public String toStringInt(boolean isFull) {
        var out = new StringBuilder("Contract{");
        out.append("self=").append(self.address().hexAddress());
        out.append(", callerAddr=").append(callerAddr.hexAddress());
        if (isFull) {
            out.append(", caller=").append(caller);
        }
        out.append(", value=").append(value);
        out.append(", gas=").append(gas);
        out.append(", codeAddr=").append(codeAddr.hexAddress());
        if (isFull) {
            out.append(", code=").append(code);
        }
        out.append(", codeHash=").append(HexUtils.toHex(codeHash));
        out.append(", input=[").append(input == null ? "empty" : HexUtils.toHex(input)).append("]");
        if (isFull) {
            out.append(", validJumps=").append(Arrays.toString(validJumps));
        }
        out.append('}');

        return out.toString();
    }

}
