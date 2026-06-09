package io.aplfintech.grape.bcei;

import io.aplfintech.grape.model.Address;
import io.aplfintech.grape.vm.Log;

import java.util.List;

/**
 * API for Virtual Machine Sate access
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface VmStateAccess extends StateAccess {

    byte[] getTransientStorage(Address address, byte[] key);

    void putTransientStorage(Address address, byte[] key, byte[] data);

    /**
     * Returns the value of the refunded gas
     */
    long getRefundGas();

    /**
     * Adds the positive amount to the gas counter
     *
     * @param gas amount of gas refunded
     */
    void addRefundGas(long gas);

    /**
     * Reduces amount of gas to be refunded by a positive value
     *
     * @param gas amount to subtract from gas refunds
     */
    void subRefundGas(long gas);


    /**
     * Add the event in the VM state
     */
    void addLog(Log event);

    /**
     * Returns the list of event log from the VM state
     */
    List<Log> getLog();

    /**
     * Resets VM state to the initial (empty) state
     */
    void reset();

    /**
     * Returns true if the given address not cold
     */
    boolean isWarmedAddress(Address address);

    /**
     * Adds address to already accessed addresses set if not already included
     */
    void addWarmedAddress(Address address);

    /**
     * Returns true if the given slot not cold
     */
    boolean isWarmedSlot(Address address, byte[] slot);

    /**
     * Adds slot to already accessed slots set if not already included
     */
    void addWarmedSlot(Address address, byte[] slot);
}
