package io.aplfintech.grape.bcei;

import com.fasterxml.jackson.core.JsonProcessingException;
import io.aplfintech.grape.grap3.crypto.Hashers;
import io.aplfintech.grape.l1vm.Storage;
import io.aplfintech.grape.l1vm.TransientStorage;
import io.aplfintech.grape.l1vm.VmAddress;
import io.aplfintech.grape.model.Address;
import io.aplfintech.grape.model.Hash;
import io.aplfintech.grape.model.Key;
import io.aplfintech.grape.vm.Log;
import lombok.experimental.Delegate;
import lombok.extern.slf4j.Slf4j;

import java.util.ArrayList;
import java.util.HashSet;
import java.util.List;
import java.util.Set;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
public class VmStateManager implements VmStateAccess {
    @Delegate(excludes = {ExcludeDumpState.class})
    private final StateAccess stateAccess;
    private final Storage transientStorage;
    private long refundGas;
    private final List<Log> eventLog;
    private final Set<Address> warmedAddresses;
    private final Set<Key> warmedSlots;

    //TODO vvv  - verify if these fields are needed

    /**
     * Map of addresses to selfdestruct. Key is the unprefixed address.
     * Value is a boolean when marked for destruction and replaced with a Buffer containing the address where the remaining funds are sent.
     */
    //selfdestruct?: { [key: string]: boolean } | { [key: string]: Buffer }
    //TODO ^^^
    public VmStateManager(StateAccess stateAccess) {
        this.stateAccess = stateAccess;
        this.transientStorage = new TransientStorage();
        this.refundGas = 0;
        this.eventLog = new ArrayList<>();
        this.warmedAddresses = new HashSet<>();
        this.warmedSlots = new HashSet<>();
    }

    @Override
    public long getRefundGas() {
        return refundGas;
    }

    @Override
    public void addRefundGas(long gas) {
        refundGas += gas;
    }

    @Override
    public void subRefundGas(long gas) {
        if (gas > refundGas) {
            throw new IllegalStateException("Refund counter below zero, gas=" + gas + " refund=" + refundGas);
        }
        refundGas -= gas;
    }

    @Override
    public byte[] getTransientStorage(Address address, byte[] key) {
        return transientStorage.get(address, key);
    }

    @Override
    public void putTransientStorage(Address address, byte[] key, byte[] data) {
        transientStorage.put(address, key, data);
    }

    @Override
    public void addLog(Log event) {
        eventLog.add(event);
    }

    @Override
    public List<Log> getLog() {
        return eventLog;
    }

    @Override
    public void reset() {
        stateAccess.clearContractStorage(VmAddress.UNDEFINED_ADDRESS);
        transientStorage.clear();
        warmedAddresses.clear();
        warmedSlots.clear();
        eventLog.clear();
        refundGas = 0;
    }

    @Override
    public boolean isWarmedAddress(Address address) {
        return warmedAddresses.contains(address);
    }

    @Override
    public void addWarmedAddress(Address address) {
        warmedAddresses.add(address);
    }

    @Override
    public boolean isWarmedSlot(Address address, byte[] slot) {
        var hasher = Hashers.keccak256();
        hasher.update(address.bytes());
        var key = new Hash(hasher.digest(slot));
        return warmedSlots.contains(key);
    }

    @Override
    public void addWarmedSlot(Address address, byte[] slot) {
        var hasher = Hashers.keccak256();
        hasher.update(address.bytes());
        var key = new Hash(hasher.digest(slot));
        warmedSlots.add(key);
    }

    @Override
    public String dumpState() {
        try {
            var transientStorageJson = transientStorage.toJSON();
            return stateAccess.dumpState() +
                    "\n --- vm state ---\n" +
                    " transient storage = " + transientStorageJson + "\n" +
                    "        refund gas = " + refundGas + "\n";
        } catch (JsonProcessingException e) {
            log.error("State dump error", e);
            return "\n\n=== STATE MANAGER === :\nState dump error";
        }
    }

    private interface ExcludeDumpState {
        String dumpState();
    }
}
