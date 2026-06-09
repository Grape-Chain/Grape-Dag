package io.aplfintech.grape.vm;

import io.aplfintech.grape.interpreter.RunContext;
import io.aplfintech.grape.vm.opcode.OpTable;

import java.util.function.Function;

/**
 * The dynamic gas handler.
 * This handler takes a runState and returns the calculated dynamic part of gas value for the opcode.
 * Notice, the base fee of the opcode is not included in calculation and should be separately charged.
 *
 * @author andrew.zinchenko@gmail.com
 * @see RunContext
 * @see OpTable
 * @since 0.1
 */
@FunctionalInterface
public interface DynamicGasHandler extends Function<RunContext, Long> {

}
