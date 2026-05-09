package io.aplfintech.luna.vm.opcode;

import io.aplfintech.luna.vm.DynamicGasHandler;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface OpTable {
    ExecFn locateFn(byte opCode);

    DynamicGasHandler locateDynamicGasHandler(byte opCode);

    OpCode locateOpCode(byte opCode);

    OpCode[] opCodes();

}
