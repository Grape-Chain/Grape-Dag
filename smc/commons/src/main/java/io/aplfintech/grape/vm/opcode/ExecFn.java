package io.aplfintech.grape.vm.opcode;

import io.aplfintech.grape.interpreter.RunContext;

import java.util.function.Function;

/**
 * Operation code interface
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@FunctionalInterface
public interface ExecFn extends Function<RunContext, FnExecResult> {
}
