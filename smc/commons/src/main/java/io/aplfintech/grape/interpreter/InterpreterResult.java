package io.aplfintech.grape.interpreter;


import io.aplfintech.grape.vm.opcode.FnExecResult;

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
