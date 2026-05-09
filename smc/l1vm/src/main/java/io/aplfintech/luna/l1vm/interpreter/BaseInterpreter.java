package io.aplfintech.luna.l1vm.interpreter;

import io.aplfintech.luna.config.ExecutionConfig;
import io.aplfintech.luna.exception.InterpreterExecutionException;
import io.aplfintech.luna.exception.VmException;
import io.aplfintech.luna.interpreter.Interpreter;
import io.aplfintech.luna.interpreter.InterpreterResult;
import io.aplfintech.luna.interpreter.RunContext;
import io.aplfintech.luna.l1vm.VmMemory;
import io.aplfintech.luna.l1vm.VmStack;
import io.aplfintech.luna.l1vm.opcode.OpCodes;
import io.aplfintech.luna.vm.ExecutionStatus;
import io.aplfintech.luna.vm.Vm;
import io.aplfintech.luna.vm.VmStatus;
import io.aplfintech.luna.vm.contract.Contract;
import io.aplfintech.luna.vm.opcode.FnExecResult;
import io.aplfintech.luna.vm.opcode.FnResult;
import io.aplfintech.luna.vm.opcode.OpCode;
import io.aplfintech.luna.vm.opcode.OpTable;
import io.aplfintech.luna.utils.HexUtils;
import lombok.NonNull;
import lombok.experimental.Delegate;
import lombok.extern.slf4j.Slf4j;

import static io.aplfintech.luna.l1vm.opcode.OpCodes.isInvalidOpcode;

/**
 * Basic implementation of the interpreter
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
public class BaseInterpreter implements Interpreter {

    protected final Vm vm;
    protected final ExecutionConfig cfg;
    protected final OpTable opTable;
    // Whether to throw on stateful modifications
    protected boolean readOnly;
    protected byte[] returnData; // Last CALL's return data for subsequent reuse

    public BaseInterpreter(@NonNull Vm vm, @NonNull ExecutionConfig config) {
        this.vm = vm;
        this.cfg = config;
        this.opTable = config.opTable();
        this.readOnly = false;
    }

    public InterpreterResult run(Contract contract, byte[] input, boolean readOnly) throws InterpreterExecutionException {
        // Increment the call depth which is restricted to 1024
        vm.enter();
        // Make sure the readOnly is only set if Interpreter isn't in readOnly yet.
        // This also makes sure that the readOnly flag isn't removed for child calls.
        boolean resetReadonly = false;
        if (readOnly && !this.readOnly) {
            this.readOnly = true;
            resetReadonly = true;
        }
        // Reset the previous call's return data
        clearReturnData();
        RunContext runState = null;
        try {
            // Return if there's no code.
            if (contract.code().size() == 0) {
                return new InterpreterResultImpl(VmStatus.VM_SUCCESS);
            }
            var memory = new VmMemory();
            var stack = new VmStack();
            runState = RunState.builder()
                .pc(0)
                //state fields
                .opCode(OpCodes.INVALID.getCode())
                .memory(memory)
                .stack(stack)
                .interpreter(this)
                .contract(contract)
                .build();
            //Set contract input
            contract.setInput(input);
            log.debug("Interpreter Main loop: start,  contract={}", contract);
            FnExecResult rc;
            // Iterate through the given code (main run loop)
            while (true) {
                var opCode = contract.getOPCode(runState.getPc());

                rc = runStep(opCode, runState);
                if (rc.hasError() || VmStatus.VM_STOP_TOKEN == rc.executionStatus()) {
                    break;
                }
                //increment program counter
                runState.addPc(1);
            }
            if (VmStatus.VM_STOP_TOKEN == rc.executionStatus()) {
                //clear stop token
                rc = new FnResult(VmStatus.VM_SUCCESS, rc.output());
            }
            var result = new InterpreterResultImpl(rc, runState);
            log.debug("Main loop: end, result={}, contract={}", result, contract);
            return result;
        } catch (Exception e) {
            log.error("Interpreter Main loop: Caught exception: " + e.getMessage() + ", runContext=" + String.valueOf(runState), e);
            throw new InterpreterExecutionException(VmStatus.VM_INTERNAL_ERROR, e.getMessage());
        } finally {
            vm.leave();
            if (resetReadonly) {
                this.readOnly = false;
            }
        }
    }

    protected FnExecResult runStep(byte vmCode, RunContext runState) throws InterpreterExecutionException {
        var opCode = opTable.locateOpCode(vmCode);
        final long pc = runState.getPc();
        long cost = 0;//the cost of current opcode execution
        final long contractGas = runState.getContract().gas();
        if (isInvalidOpcode(opCode.getCode())) {
            captureFault(vmCode, runState, cost, contractGas, VmStatus.VM_INVALID_INSTRUCTION);
            return status(VmStatus.VM_INVALID_INSTRUCTION);
        }
        runState.setOpCode(vmCode);
        long gasFee = opCode.getFee();
        cost += gasFee;
        if (!runState.getContract().useGas(gasFee)) {
            log.error("Out of gas. static fee={}, contract remaining gas={}, used gas={}", gasFee, runState.getContract().gas(), runState.getContract().gasUsed());
            captureFault(vmCode, runState, cost, contractGas, VmStatus.VM_OUT_OF_GAS);
            return status(VmStatus.VM_OUT_OF_GAS);
        }

        if (opCode.isDynamicGas()) {
            var handler = opTable.locateDynamicGasHandler(opCode.getCode());
            long dynamicFee;
            try {
                dynamicFee = handler.apply(runState);
                cost = Math.addExact(cost, dynamicFee);
            } catch (ArithmeticException e) {
                log.error("Gas unit overflow.", e);
                captureFault(vmCode, runState, cost, contractGas, VmStatus.VM_OUT_OF_GAS);
                return status(VmStatus.VM_OUT_OF_GAS);
            } catch (VmException e) {
                return status(e.getStatus());
            }

            if (!runState.getContract().useGas(dynamicFee)) {
                log.error("Out of gas. dynamic fee={}, contract: remaining gas={}, used gas={}", dynamicFee, runState.getContract().gas(), runState.getContract().gasUsed());
                captureFault(vmCode, runState, cost, contractGas, VmStatus.VM_OUT_OF_GAS);
                return status(VmStatus.VM_OUT_OF_GAS);
            }
        }
        captureState(pc, opCode, runState, cost, contractGas);
        var opFn = opTable.locateFn(opCode.getCode());

        // === Execute opcode handler ===
        FnExecResult rc = opFn.apply(runState);
        // ==============================

        if (rc.executionStatus() == null) {
            var err = String.format("Unexpected behavior of instruction opCode=%s, pc=%d, ExecutionStatus is null", opCode.fullName(), pc);
            log.error(err);
            throw new InterpreterExecutionException(VmStatus.VM_UNEXPECTED_BEHAVIOR, err);
        }
        if (rc.hasError()) {
            //in case of error trace the state once more
            captureState(pc, opCode, runState, cost, runState.getContract().gas(), rc);
        }

        return rc;
    }

    @Override
    public Vm getVm() {
        return vm;
    }

    @Override
    public byte[] getReturnData() {
        return returnData == null ? new byte[0] : returnData;
    }

    @Override
    public boolean isReadonly() {
        return readOnly;
    }

    @Override
    public void setReturnData(byte[] data) {
        this.returnData = data;
    }

    @Override
    public void clearReturnData() {
        this.returnData = new byte[0];
    }

    @Override
    public String toFullString() {
        return "BaseInterpreter{" +
            "vm=" + vm +
            ", opTable=" + opTable +
            ", readOnly=" + readOnly +
            ", returnData=[" + HexUtils.toHex(returnData) + "]" +
            '}';
    }

    @Override
    public String toString() {
        return "Interpreter{" +
            "vm=" + vm +
            ", readOnly=" + readOnly +
            ", returnData=[" + HexUtils.toHex(returnData) + "]" +
            '}';
    }

    private void captureState(long pc, OpCode opCode, RunContext runState, long cost, long contractGas) {
        if (cfg.isDebugEnabled()) {
            //capture the current state
            cfg.tracer()
                .notifyInstructionStart(pc, opCode, contractGas, cost, runState, null, vm.getDepth(), VmStatus.VM_SUCCESS);
        } else {
            log.trace("Capture State, cfg.debug=false");
        }
    }

    private void captureState(long pc, OpCode opCode, RunContext runState, long cost, long contractGas, FnExecResult result) {
        if (cfg.isDebugEnabled()) {
            //capture the current state
            cfg.tracer()
                .notifyInstructionStart(pc, opCode, contractGas, cost, runState, result.output(), vm.getDepth(), result.executionStatus());
        } else {
            log.trace("Capture State, cfg.debug=false");
        }
    }

    private void captureFault(byte opCode, RunContext runState, long cost, long contractGas, ExecutionStatus status) {
        if (cfg.isDebugEnabled()) {
            //capture fault
            cfg.tracer()
                .notifyFault(runState.getPc(), opCode, contractGas, cost, runState, vm.getDepth(), status);
        } else {
            log.trace("Capture Fault, cfg.debug=false");
        }
    }

    private static FnResult status(ExecutionStatus err) {
        return new FnResult(err, null);
    }

    /**
     * The interpreter result
     * If runContext in result is null that points that contract has not been performed
     */
    private static final class InterpreterResultImpl implements InterpreterResult {
        private final RunContext runContext;
        @Delegate
        private final FnExecResult execResult;

        /**
         * @param runContext The interpreter execution state
         * @param execResult Result of the opCode execution
         */
        public InterpreterResultImpl(@NonNull FnExecResult execResult, @NonNull RunContext runContext) {
            this.execResult = execResult;
            this.runContext = runContext;
        }

        public InterpreterResultImpl(@NonNull ExecutionStatus executionStatus) {
            this.execResult = new FnResult(executionStatus, new byte[0]);
            this.runContext = null;
        }

        @Override
        public RunContext runContext() {
            return runContext;
        }

        @Override
        public String toString() {
            return execResult.toString();
        }
    }
}
