package io.aplfintech.luna.bcei;

import io.aplfintech.luna.config.CryptoConfig;
import io.aplfintech.luna.l1vm.VmAccount;
import io.aplfintech.luna.model.Account;
import io.aplfintech.luna.model.Address;
import io.aplfintech.luna.model.Hash;
import io.aplfintech.luna.vm.Log;

import java.math.BigInteger;

import static io.aplfintech.luna.l1vm.Constants.KECCAK256_NULL;

/**
 * This state access implementation allows any read operations in the state and keeps all changes in the local state.
 * The global state keeps unchanged
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public class CachedReadStateAccess implements StateAccess {

    private final StateAccess serverState;
    private final InMemoryStateAccess localState;

    public CachedReadStateAccess(StateAccess stateAccess, Account... accounts) {
        this.serverState = stateAccess;
        this.localState = new InMemoryStateAccess(serverState.chainId(), accounts);
    }

    @Override
    public BigInteger chainId() {
        return serverState.chainId();
    }

    @Override
    public boolean isAccountExists(Address address) {
        if (localState.isAccountExists(address)) {
            return true;
        }
        return serverState.isAccountExists(address);
    }

    @Override
    public void createAccount(Address address) {
        localState.createAccount(address);
    }

    @Override
    public boolean accountIsEmpty(Address address) {
        if (localState.isAccountExists(address)) {
            return localState.accountIsEmpty(address);
        }
        return serverState.accountIsEmpty(address);
    }

    @Override
    public Account getAccount(Address address) {
        if (localState.isAccountExists(address)) {
            return localState.getAccount(address);
        }
        var account = VmAccount.from(serverState.getAccount(address));
        localState.putAccount(address, account);
        return account;
    }

    @Override
    public void putAccount(Address address, Account account) {
        localState.putAccount(address, account);
    }

    @Override
    public void deleteAccount(Address address) {
        localState.deleteAccount(address);
    }

    @Override
    public BigInteger getBalance(Address address) {
        return getAccount(address).balance();
    }

    @Override
    public void addBalance(Address address, BigInteger amount) {
        getAccount(address).addBalance(amount);
    }

    @Override
    public void subBalance(Address address, BigInteger amount) {
        getAccount(address).subBalance(amount);
    }

    @Override
    public long getNonce(Address address) {
        return getAccount(address).nonce();
    }

    @Override
    public void setNonce(Address address, long nonce) {
        var account = getAccount(address);
        account.setNonce(nonce);
        putAccount(address, account);
    }

    @Override
    public void putContractCode(Address address, byte[] data) {
        localState.putContractCode(address, data);
    }

    @Override
    public byte[] getContractCode(Address address) {
        var code = localState.getContractCode(address);
        if (code.length == 0) {
            code = serverState.getContractCode(address);
            if (code.length > 0) {
                localState.putContractCode(address, code);
            }
        }
        return code;
    }

    @Override
    public long getContractCodeSize(Address contractAddress) {
        return getContractCode(contractAddress).length;
    }

    @Override
    public byte[] getContractCodeHash(Address extContractAddress) {
        var code = getContractCode(extContractAddress);
        return code.length == 0 ? KECCAK256_NULL : CryptoConfig.crypto().keccak256(code);
    }

    @Override
    public byte[] getContractStorage(Address address, byte[] key) {
        if (localState.localStorage.containsMapping(address)
            && localState.localStorage.getMapping(address).containsKey(new Hash(key))) {
            return localState.getContractStorage(address, key);
        }
        var data = serverState.getContractStorage(address, key);
        localState.putContractStorage(address, key, data);
        return data;
    }

    @Override
    public byte[] getCommittedContractStorage(Address address, byte[] key) {
        return serverState.getContractStorage(address, key);
    }

    @Override
    public void putContractStorage(Address address, byte[] key, byte[] data) {
        localState.putContractStorage(address, key, data);
    }

    @Override
    public byte[] getBlockHash(BigInteger num) {
        return serverState.getBlockHash(num);
    }

    @Override
    public void clearContractStorage(Address address) {
        localState.clearContractStorage(address);
    }

    @Override
    public void checkpoint() {
        //do nothing
    }

    @Override
    public void commit() {
        //do nothing
    }

    @Override
    public void revert() {
        //do nothing
    }

    @Override
    public void saveLog(Log[] eventLogs) {
        localState.saveLog(eventLogs);
    }

    @Override
    public boolean hasSuicided(Address address) {
        return localState.hasSuicided(address);
    }

    @Override
    public void suicide(Address address) {
        localState.suicide(address);
    }

    @Override
    public String dumpState() {
        return localState.dumpState();
    }
}
