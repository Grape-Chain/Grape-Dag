package io.aplfintech.luna.bcei;

import com.fasterxml.jackson.core.JsonProcessingException;
import io.aplfintech.luna.config.CryptoConfig;
import io.aplfintech.luna.l1vm.SimpleStorage;
import io.aplfintech.luna.l1vm.Storage;
import io.aplfintech.luna.l1vm.VmAccount;
import io.aplfintech.luna.l1vm.VmAddress;
import io.aplfintech.luna.l1vm.VmStorage;
import io.aplfintech.luna.math.Math256;
import io.aplfintech.luna.model.Account;
import io.aplfintech.luna.model.Address;
import io.aplfintech.luna.model.Hash;
import io.aplfintech.luna.model.Key;
import io.aplfintech.luna.vm.Log;
import io.aplfintech.luna.utils.HexUtils;
import io.aplfintech.luna.utils.JsonUtils;
import lombok.NonNull;
import lombok.extern.slf4j.Slf4j;

import java.math.BigInteger;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.HashMap;
import java.util.HashSet;
import java.util.List;
import java.util.Map;
import java.util.Set;

import static io.aplfintech.luna.l1vm.Constants.KECCAK256_NULL;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
public class InMemoryStateAccess implements StateAccess {
    private final BigInteger chainId;

    protected final Storage localStorage;
    private final Map<Key, Account> accounts;
    private final Set<Address> suicidedAddresses;
    private final Map<Key, byte[]> contracts;

    private final List<Log> eventsLog;

    public InMemoryStateAccess(BigInteger chainId, Account... accounts) {
        this.chainId = chainId;
        this.localStorage = new VmStorage(new SimpleStorage());
        this.suicidedAddresses = new HashSet<>();
        this.contracts = new HashMap<>();
        this.eventsLog = new ArrayList<>();
        this.accounts = new HashMap<>();
        for (var acc : accounts) {
            putAccount(acc.address(), acc);
        }
    }

    @Override
    public BigInteger chainId() {
        return chainId;
    }

    @Override
    public boolean isAccountExists(Address address) {
        return accounts.containsKey(Hash.from(address));
    }

    @Override
    public void createAccount(Address address) {
        VmAccount newAccount = createEmptyVmAccount(address);
        accounts.put(Hash.from(address), newAccount);
        log.debug("Created account={}", newAccount);
    }

    public static VmAccount createEmptyVmAccount(Address address) {
        return new VmAccount(address, 0, BigInteger.ZERO);
    }

    @Override
    public Account getAccount(Address address) {
        if (!isAccountExists(address)) {
            createAccount(address);
        }
        var account = accounts.get(Hash.from(address));
        log.debug("Requested address={} found account={}", address.hexAddress(), account);
        return account;
    }

    @Override
    public void putAccount(Address address, Account account) {
        accounts.put(Hash.from(address), account);
        log.debug("Account added to state, account={}", account);
    }

    @Override
    public boolean accountIsEmpty(Address address) {
        var account = accounts.get(Hash.from(address));
        return account.nonce() == 0
            && account.balance().signum() == 0
            && Arrays.equals(KECCAK256_NULL, account.codeHash())
            && Arrays.equals(KECCAK256_NULL, account.storageRoot());
    }

    @Override
    public void deleteAccount(Address address) {
        accounts.remove(Hash.from(address));
    }

    @Override
    public BigInteger getBalance(Address address) {
        return getAccount(address).balance();
    }

    @Override
    public void addBalance(Address address, BigInteger amount) {
        log.debug("Add balance: address={} amount={}", address.hexAddress(), amount);
        getAccount(address).addBalance(amount);
    }

    @Override
    public void subBalance(Address address, BigInteger amount) {
        log.debug("Sub balance: address={} amount={}", address.hexAddress(), amount);
        getAccount(address).subBalance(amount);
    }

    @Override
    public long getNonce(Address address) {
        return getAccount(address).nonce();
    }

    @Override
    public void setNonce(Address address, long nonce) {
        log.debug("Set new nonce={} for address={}, prev nonce={}", nonce, address.hexAddress(), getAccount(address).nonce());
        getAccount(address).setNonce(nonce);
    }

    @Override
    public void putContractCode(Address address, byte[] data) {
        log.debug("putContractCode address={} code.length={}", address.hexAddress(), data.length);
        contracts.put(Hash.from(address), data);
    }

    @Override
    public byte[] getContractCode(Address address) {
        var key = Hash.from(address);
        byte[] rc;
        if (contracts.containsKey(key)) {
            rc = contracts.get(key);
        } else {
            log.debug("Contract code doesn't exist in the state, returns default empty array.");
            rc = new byte[0];
        }
        if (log.isTraceEnabled()) {
            log.trace("Contract code by address={} code=[{}]", key.hex(), HexUtils.toHex(rc, true));
        } else {
            log.debug("Contract code by address={} code.length=[{}]", key.hex(), rc.length);
        }
        return rc;
    }

    @Override
    public long getContractCodeSize(Address address) {
        return this.getContractCode(address).length;
    }

    @Override
    public byte[] getContractCodeHash(Address address) {
        var code = getContractCode(address);
        return code.length == 0 ? KECCAK256_NULL : CryptoConfig.crypto().keccak256(code);
    }

    @Override
    public byte[] getContractStorage(Address address, byte[] key) {
        var value = localStorage.get(address, key);
        log.debug("getContractStorage address={} key={} value={}", address.hexAddress(), HexUtils.toHex(key, true), HexUtils.toHex(value, true));
        return value;
    }

    @Override
    public byte[] getCommittedContractStorage(Address address, byte[] key) {
        return getContractStorage(address, key);
    }

    @Override
    public void putContractStorage(Address address, byte[] key, byte[] data) {
        log.debug("putContractStorage address={} key={} value={}", address.hexAddress(), HexUtils.toHex(key, true), HexUtils.toHex(data, true));
        localStorage.put(address, key, data);
    }

    @Override
    public void saveLog(@NonNull Log[] events) {
        if (events.length > 0) {
            StringBuilder eventsStr = new StringBuilder();
            int i = 0;
            for (var event : events) {
                eventsStr.append(String.format("%d event=%s;", ++i, event));
                eventsLog.add(event);
            }
            log.debug("Save the events LOG: {}", eventsStr);
        } else {
            log.debug("Events LOG is empty, nothing to save.");
        }
    }

    @Override
    public void clearContractStorage(Address address) {
        if (address.equals(VmAddress.UNDEFINED_ADDRESS)) {
            localStorage.clear();
        }
    }

    @Override
    public boolean hasSuicided(Address address) {
        return suicidedAddresses.contains(address);
    }

    @Override
    public void suicide(Address address) {
        suicidedAddresses.add(address);
        deleteAccount(address);
    }

    /**
     * Returns block hash by number
     * TODO: Use cache to increase performance
     *
     * @param num block number
     * @return the block hash
     */
    @Override
    public byte[] getBlockHash(BigInteger num) {
        //TODO it's a stub, not implemented yet
        return CryptoConfig.crypto().keccak256(Math256.padToWord(Math256.asUnsignedByteArray(num)));
    }

    //delegated from local storage
    @Override
    public void checkpoint() {
        localStorage.checkpoint();
    }

    @Override
    public void commit() {
        localStorage.commit();
    }

    @Override
    public void revert() {
        localStorage.revert();
    }

    @Override
    public String dumpState() {
        try {
            var accountsJson = JsonUtils.HEX_MAPPER.writeValueAsString(accounts);
            var localStorageJson = localStorage.toJSON();
            var contractsJson = JsonUtils.HEX_MAPPER.writeValueAsString(contracts);
            var eventsLogJson = JsonUtils.HEX_MAPPER.writeValueAsString(eventsLog);
            return "\n\n=== STATE MANAGER ===:\n" +
                "          accounts = " + accountsJson + "\n" +
                "persistent storage = " + localStorageJson + "\n" +
                "         contracts = " + contractsJson + "\n" +
                "        events LOG = " + eventsLogJson + "\n";
        } catch (JsonProcessingException e) {
            log.error("State dump error", e);
            return "\n\n=== STATE MANAGER === :\nState dump error";
        }
    }
}
