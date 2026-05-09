package io.aplfintech.luna.interpreter;


import io.aplfintech.luna.vm.opcode.FnExecResult;

/**
 * The interpreter result
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface InterpreterResult extends FnExecResult {
    /**
     * Returns the interpreter execution state
     */
    RunContext runContext();

}
