package io.aplfintech.luna.config;

import io.aplfintech.luna.crypto.CryptoLib;
import io.aplfintech.luna.tracers.Tracer;
import io.aplfintech.luna.vm.opcode.OpTable;

/**
 * General execution configuration used by the interpreter and VM
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface ExecutionConfig {
    /**
     * Returns true in Debug mode
     * <p/>In Debug mode All tracers are notified
     *
     * @return true in Debug mode
     */
    boolean isDebugEnabled();

    /**
     * Returns tracer that works in Debug mode
     *
     * @return tracer instance
     */
    Tracer tracer();

    OpTable opTable();

    /**
     * Returns the chain config for current execution
     */
    ChainConfig chainConfig();

    /**
     * Returns true for simulation calls (needed for 0 prices calls)
     */
    boolean isNoBaseFee();

    /**
     * Returns the instance of CryptoLib implementation
     */
    CryptoLib cryptoLib();
}
