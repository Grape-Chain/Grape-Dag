package io.aplfintech.grape;

import io.aplfintech.grape.bcei.DEI;
import io.aplfintech.grape.config.CryptoConfig;
import io.aplfintech.grape.l1vm.Constants;
import io.aplfintech.grape.l1vm.VmAccount;
import io.aplfintech.grape.math.Math256;
import io.aplfintech.grape.model.Account;
import io.aplfintech.grape.model.Address;
import io.aplfintech.grape.vm.Log;
import lombok.AllArgsConstructor;
import lombok.extern.slf4j.Slf4j;

import java.math.BigInteger;
import java.util.Arrays;
import java.util.Optional;

@AllArgsConstructor
@Slf4j
public class StateServerDei implements DEI {
    private final StateClient stateClient;

    @Override
    public BigInteger chainId() {
        return stateClient.getChainId();
    }

    @Override
    public byte[] getBlockHash(BigInteger num) { // order from transactions
        return CryptoConfig.crypto().keccak256(Math256.padToWord(Math256.asUnsignedByteArray(num)));
    }

    // account store
    @Override
    public boolean isAccountExists(Address address) {
        return stateClient.getAccount(address.bytes()).isPresent();
    }

    @Override
    public void createAccount(Address address) {
        stateClient.createAccount(address.bytes());
    }

    @Override
    public boolean accountIsEmpty(Address address) {
        var accountOpt = stateClient.getAccount(address.bytes());
        if (accountOpt.isEmpty()) {
            return true;
        }
        var account = accountOpt.get();
        return account.balance().signum() == 0 && account.codeHash().length == 0 && account.nonce() == 0;
    }

    @Override
    public Account getAccount(Address address) {
        return stateClient.getAccount(address.bytes())
            .orElse(new VmAccount(address, 0, BigInteger.ZERO));
    }

    @Override
    public void putAccount(Address address, Account account) {

    }

    @Override
    public void deleteAccount(Address address) {

    }

    @Override
    public BigInteger getBalance(Address address) {
        var optAccount = stateClient.getAccount(address.bytes());
        if (optAccount.isEmpty()) {
            return BigInteger.ZERO;
        }
        return optAccount.get().balance();
    }

    @Override
    public void addBalance(Address address, BigInteger amount) {
        stateClient.addBalance(address.bytes(), amount);
    }

    @Override
    public void subBalance(Address address, BigInteger amount) {
        stateClient.subBalance(address.bytes(), amount);
    }

    @Override
    public long getNonce(Address address) {
        var accountOpt = stateClient.getAccount(address.bytes());
        if (accountOpt.isEmpty()) {
            return 0;
        }
        long nonce = accountOpt.get().nonce();
        log.info("Get nonce for account {}, nonce = {}", address.hexAddress(), nonce);
        return nonce;
    }

    @Override
    public void setNonce(Address address, long nonce) {
        var account = getAccount(address);
        log.info("Set nonce for account {}, nonce = {}, prevNonce = {}", address.hexAddress(), nonce, account.nonce());
        stateClient.setNonceForContract(address.bytes(), nonce);
    }

    @Override
    public void putContractCode(Address address, byte[] data) {
        stateClient.putContractCode(address.bytes(), data);
    }

    @Override
    public byte[] getContractCode(Address address) {
        return stateClient.getContractCode(address.bytes());
    }

    @Override
    public long getContractCodeSize(Address contractAddress) {
        return getContractCode(contractAddress).length;
    }

    @Override
    public byte[] getContractCodeHash(Address extContractAddress) {
        Optional<Account> contractAccountOpt = stateClient.getAccount(extContractAddress.bytes());
        if (contractAccountOpt.isEmpty() || contractAccountOpt.get().codeHash() == null || contractAccountOpt.get().codeHash().length == 0) {
            return Constants.KECCAK256_NULL;
        }
        return getAccount(extContractAddress).codeHash();
    }

    @Override
    public byte[] getContractStorage(Address address, byte[] key) {
        return stateClient.getContractStorage(address.bytes(), key);
    }

    @Override
    public byte[] getCommittedContractStorage(Address address, byte[] key) {
        return stateClient.getCommittedContractStorage(address.bytes(), key);
    }

    @Override
    public void putContractStorage(Address address, byte[] key, byte[] data) {
        stateClient.putContractStorage(address.bytes(), key, data);
    }

    @Override
    public void clearContractStorage(Address address) {

    }

    @Override
    public void checkpoint() {
        stateClient.checkpoint();
    }

    @Override
    public void commit() {
        stateClient.commit();
    }

    @Override
    public void revert() {
        stateClient.revert();
    }

    @Override
    public boolean hasSuicided(Address address) {
        //TODO not implemented yet
        return false;
    }

    @Override
    public void suicide(Address address) {
        //TODO not implemented yet
        deleteAccount(address);
    }

    @Override
    public String dumpState() {
        return "NotImplemented";
    }

    @Override
    public void saveLog(Log[] eventLogs) {
        log.info("Save Events: {}", Arrays.stream(eventLogs).toList());
        stateClient.saveLogs(eventLogs);
    }
}
