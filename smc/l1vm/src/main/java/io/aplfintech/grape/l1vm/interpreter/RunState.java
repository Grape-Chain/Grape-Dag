package io.aplfintech.grape.l1vm.interpreter;

import io.aplfintech.grape.interpreter.Interpreter;
import io.aplfintech.grape.interpreter.RunContext;
import io.aplfintech.grape.l1vm.VmMemory;
import io.aplfintech.grape.l1vm.VmStack;
import io.aplfintech.grape.vm.contract.Contract;
import io.aplfintech.grape.utils.HexUtils;
import lombok.Builder;
import lombok.Getter;
import lombok.Setter;

/**
 * The context (state) of the code execution
 * i.e. the state of the interpreter execution step
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Getter
@Builder
public class RunState implements RunContext {
    //program counter
    @Setter
    @Builder.Default
    private long pc = 0;
    //operation code
    private int opCode;
    private VmMemory memory;
    @Setter
    @Builder.Default
    private long memoryWordCount = 0;
    @Setter
    @Builder.Default
    private long highestMemCost = 0;
    private VmStack stack;
    private Contract contract;
    private Interpreter interpreter;

    @Override
    public void setOpCode(byte opCode) {
        this.opCode = opCode & 0xff;
    }

    @Override
    public long addPc(long num) {
        pc += num;
        return pc;
    }

    @Override
    public String toString() {
        return "RunState{" +
            "pc=" + pc +
            ", opCode=" + HexUtils.toHex(new byte[]{(byte) opCode}, true) +
            ", memory=" + memory +
            ", memoryWordCount=" + memoryWordCount +
            ", highestMemCost=" + highestMemCost +
            ", stack=" + stack +
            ", contract=" + contract +
            ", interpreter=" + interpreter +
            '}';
    }
}
