package io.aplfintech.grape.l1vm;

import io.aplfintech.grape.bcei.VmStateAccess;
import io.aplfintech.grape.config.ExecutionConfig;
import io.aplfintech.grape.config.GasPrice;
import io.aplfintech.grape.config.VmConfig;
import io.aplfintech.grape.crypto.CryptoLib;
import io.aplfintech.grape.env.Context;
import io.aplfintech.grape.exception.InterpreterExecutionException;
import io.aplfintech.grape.exception.VmException;
import io.aplfintech.grape.interpreter.Interpreter;
import io.aplfintech.grape.interpreter.InterpreterResult;
import io.aplfintech.grape.l1vm.code.Codes;
import io.aplfintech.grape.l1vm.contract.CodeAndHashImpl;
import io.aplfintech.grape.l1vm.contract.GasPool;
import io.aplfintech.grape.l1vm.contract.VmContract;
import io.aplfintech.grape.l1vm.interpreter.BaseInterpreter;
import io.aplfintech.grape.l1vm.precompiled.PrecompiledContracts;
import io.aplfintech.grape.model.Address;
import io.aplfintech.grape.model.Hash;
import io.aplfintech.grape.utils.Exceptions;
import io.aplfintech.grape.vm.ExecutionStatus;
import io.aplfintech.grape.vm.Log;
import io.aplfintech.grape.vm.PrecompiledFn;
import io.aplfintech.grape.vm.Vm;
import io.aplfintech.grape.vm.VmStatus;
import io.aplfintech.grape.vm.contract.Code;
import io.aplfintech.grape.vm.contract.CodeAndHash;
import io.aplfintech.grape.vm.contract.ContractRef;
import io.aplfintech.grape.vm.contract.ContractResult;
import io.aplfintech.grape.vm.contract.GasValve;
import io.aplfintech.grape.vm.opcode.FnExecResult;
import io.aplfintech.grape.vm.opcode.FnResult;
import io.aplfintech.grape.utils.Bytes;
import io.aplfintech.grape.utils.HexUtils;
import lombok.Getter;
import lombok.NonNull;
import lombok.extern.slf4j.Slf4j;

import java.math.BigInteger;
import java.util.Arrays;

/**
 * The implementation of L1Vm
 * It's not thread safe. Each instance should be only used once.
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
public class VmImpl implements Vm {
    @Getter
    private final Context context;
    private final VmConfig vmConfig;

    private final ExecutionConfig cfg;
    private final VmStateAccess stateAccess;
    private final GasPrice price;

    // callGas holds the gas available for the current call.
    // This is needed because the available gas is dynamically calculated and
    // applied in function returned by makeXXXCallHandler.
    private long callGas;
    // Depth is the current call stack
    private int depth;

    @Getter
    private final PrecompiledFn[] precompiled;
    //@Delegate
    //private final OpTable opTable;

    private final Interpreter interpreter;

    private final CryptoLib crypto;

    public VmImpl(@NonNull Context context, @NonNull ExecutionConfig cfg, @NonNull VmConfig vmConfig, @NonNull VmStateAccess stateAccess) {
        this.context = context;
        this.vmConfig = vmConfig;
        this.stateAccess = stateAccess;
        this.cfg = cfg;
        this.depth = 0;
        this.callGas = 0;
        //Initialize the operations functions and dynamic handlers
        //this.opTable = cfg.getOpTable();
        this.interpreter = new BaseInterpreter(this, cfg);
        this.price = cfg.chainConfig().gasPriceMap();
        this.crypto = cfg.cryptoLib();
        //Initialize precompiled contracts
        this.precompiled = PrecompiledContracts.createPrecompiledContracts(cfg.chainConfig(), crypto);
    }

    public ExecutionConfig executionConfig() {
        return cfg;
    }

    @Override
    public ContractResult runCall(ContractRef caller, Address addr, byte[] input, long gasLimit, BigInteger value) {
        return executeCall(CallKind.CALL, caller, addr, input, gasLimit, value);
    }

    @Override
    public ContractResult runCallCode(ContractRef caller, Address addr, byte[] input, long gasLimit, BigInteger value) {
        return executeCall(CallKind.CALL_CODE, caller, addr, input, gasLimit, value);
    }

    @Override
    public ContractResult runDelegateCall(ContractRef caller, Address addr, byte[] input, long gasLimit) {
        return executeCall(CallKind.DELEGATE_CALL, caller, addr, input, gasLimit, BigInteger.ZERO);
    }

    @Override
    public ContractResult runStaticCall(ContractRef caller, Address addr, byte[] input, long gasLimit) {
        return executeCall(CallKind.STATIC_CALL, caller, addr, input, gasLimit, BigInteger.ZERO);
    }

    private ContractResult executeCall(Vm.CallKind callKind, ContractRef caller, Address contractAddress, byte[] input, long gas, BigInteger amount) {
        var callerAddress = caller.address();
        log.debug("Execute call: {} from={} to={} gas={} amount={} input={}",
            callKind.name(), callerAddress.hexAddress(), contractAddress.hexAddress(), gas, amount, HexUtils.toHex(input, true));
        if (amount == null) {
            throw new IllegalStateException("Value can't be null.");
        }
        if (depth > vmConfig.getCallCreateDepth()) {
            return debugVmResult(VmResult.error(VmStatus.VM_CALL_DEPTH_EXCEEDED, gas, contractAddress));
        }
        // Fail if we're trying to transfer more than the available balance
        if (amount.signum() != 0 && !context.canTransfer(stateAccess, callerAddress, amount)) {
            return debugVmResult(VmResult.error(VmStatus.VM_INSUFFICIENT_BALANCE, gas, contractAddress));
        }

        var marker = callKind.name();
        beginStateTransaction(marker);
        long startTime = System.nanoTime();
        if (CallKind.CALL == callKind) {
            if (!stateAccess.isAccountExists(contractAddress)) {
                if (!isPrecompiled(contractAddress) && amount.signum() == 0) {
                    log.debug("+++ {}: Returns immediately, because account not exist and amount == 0, addr={} ", marker, contractAddress.hexAddress());
                    captureStart(callerAddress, contractAddress, false, input, gas, amount);
                    var result = VmResult.success(gas, contractAddress);
                    captureEnd(gas, startTime, result);

                    return result;
                }
                log.debug("+++ {}: Create new account, address={}", marker, contractAddress);
                stateAccess.createAccount(contractAddress);
            }
            if (amount.signum() != 0) {
                log.debug("+++ {}: Transfer from={} to={} amount={}", marker, callerAddress.hexAddress(), contractAddress.hexAddress(), amount);
                context.transfer(stateAccess, callerAddress, contractAddress, amount);
            }
        }

        captureStart(callerAddress, contractAddress, false, input, gas, amount);

        ContractResult result;
        if (isPrecompiled(contractAddress)) {
            log.debug("+++ {}: Run precompiled addr={}", marker, contractAddress.hexAddress());
            var valve = new GasPool(gas);
            var rc = runPrecompiledContract(contractAddress, asPrecompiled(contractAddress), input, valve, marker);
            result = new VmResult(rc.executionStatus(), valve, new Log[0], rc.output(), contractAddress);
        } else {
            //Load contract code
            log.debug("+++ {}: Load contract code, addr={}", marker, contractAddress.hexAddress());
            var data = stateAccess.getContractCode(contractAddress);
            // Initialise a new contract and set the code that is to be used by the VM.
            // The contract is a scoped environment for this execution context only.
            if (data.length == 0) {
                log.warn("Contract code length iz ZERO, address={}", contractAddress.hexAddress());
                //gas is unchanged
                result = VmResult.success(gas, contractAddress);
            } else {
                //create the new contract instance
                VmContract contract;
                switch (callKind) {
                    case CALL, STATIC_CALL -> contract = new VmContract(caller, contractAddress, amount, gas);
                    case CALL_CODE -> contract = new VmContract(caller, caller.address(), amount, gas);
                    case DELEGATE_CALL -> contract = new VmContract(caller, caller.address(), amount, gas).asDelegate();
                    default -> throw Exceptions.internal("Unsupported call kind=" + callKind);
                }
                Code code = Codes.from(data);
                Hash codeHash = new Hash(crypto.keccak256(code.bytes()));
                var codeAndHash = new CodeAndHashImpl(code, codeHash);
                contract.setCallCode(contractAddress, codeAndHash);

                log.debug("+++ {}: Run contract addr={}, input={}, gas={}, readOnly=true", marker,
                    contractAddress.hexAddress(), HexUtils.toHex(input), gas);
                //========= run the contract ===========
                InterpreterResult runResult;
                try {

                    runResult = interpreter.run(contract, input, CallKind.STATIC_CALL == callKind);

                } catch (InterpreterExecutionException e) {
                    log.error("+++ {}: error, status={} cause {}", marker, e.getStatus(), e.getMessage());
                    throw new VmException(e.getStatus(), e.getMessage());
                }
                //======================================
                Log[] eventLog;
                if (runResult.hasError()) {
                    //reset event log if any error is occurred
                    log.debug("{}: reset all emitted logs", runResult.executionStatus());
                    eventLog = new Log[0];
                } else {
                    eventLog = stateAccess.getLog().toArray(new Log[0]);
                }
                result = new VmResult(runResult.executionStatus(), contract, eventLog, runResult.output(), contractAddress);
            }
        }
        endStateTransaction(contractAddress, marker, result);

        captureEnd(gas, startTime, result);

        return result;
    }

    @Override
    public ContractResult create(ContractRef caller, Code code, long gas, BigInteger value) {
        var contractAddress = VmAddress.from(crypto.createAddress(caller.address().bytes(), stateAccess.getNonce(caller.address())));
        Hash codeHash = new Hash(crypto.keccak256(code.bytes()));
        return create(caller, new CodeAndHashImpl(code, codeHash), gas, value, contractAddress, Vm.CreateKind.CREATE);
    }

    @Override
    public ContractResult create2(ContractRef caller, Code code, long gas, BigInteger value, byte[] saltBytes) {
        var contractAddress = VmAddress.from(crypto.createAddress2(caller.address().bytes(), saltBytes, code.bytes()));
        Hash codeHash = new Hash(crypto.keccak256(code.bytes()));
        return create(caller, new CodeAndHashImpl(code, codeHash), gas, value, contractAddress, Vm.CreateKind.CREATE2);
    }

    //creates a new contract using code as deployment code.
    private ContractResult create(ContractRef caller, CodeAndHash codeAndHash, final long gas, BigInteger amount, Address contractAddress,
                                  Vm.CreateKind createKind) {
        if (depth > vmConfig.getCallCreateDepth()) {
            log.trace("Current frame depth={}, permitted depth={}", depth, vmConfig.getCallCreateDepth());
            return debugVmResult(VmResult.error(VmStatus.VM_CALL_DEPTH_EXCEEDED, gas, contractAddress));
        }
        var callerAddr = caller.address();
        // Fail if we're trying to transfer more than the available balance
        if (amount.signum() != 0 && !context.canTransfer(stateAccess, callerAddr, amount)) {
            return debugVmResult(VmResult.error(VmStatus.VM_INSUFFICIENT_BALANCE, gas, contractAddress));
        }
        //Increment caller nonce
        var nonce = stateAccess.getNonce(callerAddr);
        if (nonce + 1 < nonce) {
            return debugVmResult(VmResult.error(VmStatus.VM_NONCE_UNIT_OVERFLOW, gas, contractAddress));
        }
        stateAccess.setNonce(callerAddr, nonce + 1);

        // Ensure there's no existing contract already at the designated address
        var codeHash = stateAccess.getContractCodeHash(contractAddress);
        long contractNonce = stateAccess.getNonce(contractAddress);
        if (contractNonce != 0 || (codeHash != null && !Arrays.equals(codeHash, Constants.KECCAK256_NULL))) {
            log.debug("contract address={} nonce={} codeHash=[{}] codeHash.length={}",
                contractAddress.hexAddress(), contractNonce, HexUtils.toHex(codeHash, true),
                codeHash != null ? codeHash.length : "null");
            return debugVmResult(VmResult.error(VmStatus.VM_CONTRACT_ADDRESS_COLLISION, gas, contractAddress));
        }
        var marker = createKind.name();

        beginStateTransaction(marker);

        long startTime = System.nanoTime();
        // Create a new account on the state
        log.debug("+++ {}: Create account, address={}", marker, contractAddress);
        stateAccess.createAccount(contractAddress);
        if (amount.signum() != 0) {
            log.debug("+++ {}: Transfer from={} to={} amount={}", marker, callerAddr.hexAddress(), contractAddress.hexAddress(), amount);
            context.transfer(stateAccess, callerAddr, contractAddress, amount);
        }
        // Initialise a new contract and set the code that is to be used by the VM.
        // The contract is a scoped environment for this execution context only.
        var contract = new VmContract(caller, contractAddress, amount, gas);
        contract.setCallCode(contractAddress, codeAndHash);
        log.debug("+++ {}: Created contract caller={} codeAddr={} gas={} amount={}", marker, callerAddr.hexAddress(), contractAddress.hexAddress(), gas, amount);
        captureStart(callerAddr, contractAddress, true, codeAndHash.bytes(), gas, amount);

        //========= run the contract ===========
        InterpreterResult runResult;
        try {
            runResult = interpreter.run(contract,
                null/*the constructor arguments already attached to the end of contract code*/,
                false);
        } catch (InterpreterExecutionException e) {
            log.error("+++ {}: error, status={} cause {}", marker, e.getStatus(), e.getMessage());
            throw new VmException(e.getStatus(), e.getMessage());
        }
        //======================================

        ExecutionStatus status = runResult.executionStatus();
        byte[] resultOutput = runResult.output();
        if (status.isSuccess() && runResult.output().length > vmConfig.getMaxCodeSize()) {
            log.warn("+++ {}: max code size has been exceeded, maxCodeSize={}, codeSize={}, caller={}, contract={}",
                marker, vmConfig.getMaxCodeSize(), runResult.output().length, callerAddr.hexAddress(), contractAddress.hexAddress());
            status = VmStatus.VM_CODESIZE_EXCEEDS_MAXIMUM;
            resultOutput = new byte[0];
        }
        // Reject code starting with 0xEF
        if (status.isSuccess() && resultOutput.length >= 1 && resultOutput[0] == (byte) 0xEF) {
            log.debug("+++ {}: Rejected code starting with 0xEF caller={}, contract={}", marker,
                callerAddr.hexAddress(), contractAddress.hexAddress());
            status = VmStatus.VM_INVALID_CONTRACT_CODE;
            resultOutput = new byte[0];
        }
        if (status.isSuccess()) {
            var createDtaPrice = price.lookForGasPrice("createData");//gas required to store contract code
            if (contract.useGas(createDtaPrice)) {
                log.trace("+++ {}: GAS: cost={} to store contract code", marker, createDtaPrice);
                stateAccess.putContractCode(contractAddress, runResult.output());
                log.debug("+++ {}: Put contract code to state, address={}, codeSize={}",
                    marker, contractAddress.hexAddress(), resultOutput.length);
                if (log.isTraceEnabled()) {
                    log.trace("+++ {}: Put contract code to state, address={}, codeSize={}, code={}",
                        marker, contractAddress.hexAddress(), resultOutput.length, HexUtils.toHex(resultOutput));
                }
            } else {
                status = VmStatus.VM_CODESTORE_OUT_OF_GAS;
                resultOutput = new byte[0];
            }
        }
        Log[] eventLog;
        if (status.isFailure()) {
            //reset event log if any error is occurred
            log.debug("{}: reset all emitted logs", status);
            eventLog = new Log[0];
        } else {
            eventLog = stateAccess.getLog().toArray(new Log[0]);
        }
        VmResult result = new VmResult(status, contract, eventLog, resultOutput, contractAddress);

        endStateTransaction(contractAddress, marker, result);

        captureEnd(gas, startTime, result);

        return debugVmResult(result);
    }

    private void beginStateTransaction(String marker) {
        log.debug("+++ {}:Save checkpoint for a further revert", marker);
        stateAccess.checkpoint();
    }

    private void endStateTransaction(Address addr, String marker, ContractResult result) {
        if (result.hasError()) {
            log.debug("+++ {}: Revert state.", marker);
            stateAccess.revert();
            if (VmStatus.VM_REVERT != result.executionStatus()) {
                log.debug("+++ {}: Reset all unused gas, addr={}, gas={}", marker, addr.hexAddress(), result.gas());
                result.resetGas();
            }
        } else {
            // save events in the state
            log.debug("+++ {}: Save events log into state, {} items.", marker, result.getEventLog().length);
            if (result.getEventLog().length > 0) {
                stateAccess.saveLog(result.getEventLog());
            }

            log.debug("+++ {}: Commit state.", marker);
            stateAccess.commit();
        }
    }

    //increase call depth
    @Override
    public void enter() {
        depth++;
        log.trace("VM enter, new frame depth={}", depth);
    }

    //decrease call depth
    @Override
    public void leave() {
        log.trace("VM leave frame={}, current frame depth={}", depth, depth - 1);
        if (depth == 0) {
            throw new IllegalStateException("Can't leave the root call stack.");
        }
        depth--;
    }

    @Override
    public int getDepth() {
        return depth;
    }

    @Override
    public Context context() {
        return context;
    }

    @Override
    public VmStateAccess stateAccess() {
        return stateAccess;
    }

    @Override
    public Interpreter interpreter() {
        return interpreter;
    }

    @Override
    public long getCallGas() {
        return callGas;
    }

    @Override
    public void setCallGas(long value) {
        callGas = value < 0 ? 0 : value;
    }

    @Override
    public void addLog(@NonNull Log event) {
        event.setBlockNumber(context.blockNumber());
        stateAccess.addLog(event);
    }

    /**
     * Returns true if current address associates with a precompiled contract
     *
     * @return true if current address associates with a precompiled contract
     */
    private boolean isPrecompiled(Address address) {
        var idx = toOneByteAddress(address);
        return idx != null && idx < precompiled.length;
    }

    /**
     * Returns the precompiled function
     * use it instead of the code for all precompiled contracts
     *
     * @return the precompiled function instance
     */
    private PrecompiledFn asPrecompiled(Address address) {
        var idx = toOneByteAddress(address);
        if (idx == null) {
            throw new IllegalStateException("Wrong precompiled contract address, address=" + address.hexAddress());
        }
        if (idx >= precompiled.length) {
            throw new IllegalStateException("Unregistered precompiled contract, address=" + address.hexAddress());
        }
        if (precompiled[idx] == null) {
            throw new IllegalStateException("Can't find precompiled contract, address=" + address.hexAddress());
        }
        return precompiled[idx];
    }

    private static Byte toOneByteAddress(Address address) {
        var bytes = Bytes.trimLeftZeros(address.bytes());
        if (bytes.length != 1) {
            return null;
        }
        return bytes[0];
    }

    private static VmResult debugVmResult(VmResult result) {
        log.debug("VmResult >>> {} contract={} gas={}", result.executionStatus().fullName(), result.contract().hexAddress(), result.gas());
        return result;
    }

    @Override
    public String toFullString() {
        return "Vm{" +
            "context=" + context +
            ", stateAccess=" + stateAccess +
            ", price=" + price +
            ", callGasLimit=" + callGas +
            ", depth=" + depth +
            ", cfg=" + vmConfig +
            '}';
    }

    @Override
    public String toString() {
        return "Vm{" +
            "context=" + context +
            ", callGasLimit=" + callGas +
            ", depth=" + depth +
            '}';
    }

    public static FnExecResult runPrecompiledContract(Address addr, PrecompiledFn p, byte[] input, GasValve gasValve, String marker) {
        var gasCost = p.requiredGas(input);
        if (!gasValve.useGas(gasCost)) {
            log.debug("+++ {}: Out of gas to execute the precompiled contract, addr={}", marker, addr.hexAddress());
            return new FnResult(VmStatus.VM_OUT_OF_GAS, null);
        }
        var execResult = p.run(input);
        log.debug("+++ {}: Precompiled contract executed, addr={}, result={}", marker, addr.hexAddress(), execResult);
        return execResult;
    }

    private void captureStart(Address caller, Address addr, boolean isCreated, byte[] input, long gas, BigInteger value) {
        if (cfg.isDebugEnabled()) {
            if (depth == 0) {
                cfg.tracer().notifyExecutionStart(stateAccess, caller, addr, isCreated, input, gas, value);
            } else {
                cfg.tracer().notifyExecutionEnter(stateAccess, caller, addr, input, gas, value);
            }
        } else {
            log.trace("Capture Start, cfg.debug=false");
        }
    }

    private void captureEnd(long startGas, long startTime, ContractResult result) {
        if (cfg.isDebugEnabled()) {
            if (depth == 0) {
                var time = System.nanoTime();
                cfg.tracer().notifyExecutionEnd(result.output(), startGas - result.gas(), time - startTime, result.executionStatus());
            } else {
                cfg.tracer().notifyExecutionLeave(result.output(), startGas - result.gas(), result.executionStatus());
            }
        } else {
            log.trace("Capture End, cfg.debug=false");
        }

    }

}
