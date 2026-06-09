package io.aplfintech.grape.interpreter;

import io.aplfintech.grape.vm.Memory;
import io.aplfintech.grape.vm.contract.Contract;
import io.aplfintech.grape.vm.stack.WordStack;


/**
 * The interpreter execution state
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface RunContext {

    Contract getContract();

    long getPc();

    void setPc(long pc);

    int getOpCode();

    void setOpCode(byte value);

    long getMemoryWordCount();

    void setMemoryWordCount(long value);

    long getHighestMemCost();

    void setHighestMemCost(long value);

    Interpreter getInterpreter();

    WordStack getStack();

    Memory getMemory();

    /**
     * Increases pc on given value
     *
     * @param num value by which the counter will be increased
     * @return the increased pc value
     */
    long addPc(long num);
}
