package io.aplfintech.luna.l1vm;

import com.fasterxml.jackson.annotation.JsonProperty;
import io.aplfintech.luna.model.Account;
import io.aplfintech.luna.model.Address;
import io.aplfintech.luna.model.Hash;
import lombok.EqualsAndHashCode;
import lombok.NonNull;

import java.math.BigInteger;

import static io.aplfintech.luna.l1vm.Constants.KECCAK256_NULL_HASH;

/**
 * The Luna1 representation of accounts
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@EqualsAndHashCode
public class VmAccount implements Account {
    private final Address address;
    private long nonce;
    @JsonProperty
    private BigInteger balance;
    @JsonProperty
    private final Hash root; // merkle root of the storage trie
    @JsonProperty
    private final Hash codeHash;

    public VmAccount(@NonNull Address address, long nonce, @NonNull BigInteger balance, @NonNull Hash root, @NonNull Hash codeHash) {
        this.address = address;
        this.nonce = nonce;
        this.balance = balance;
        this.root = root;
        this.codeHash = codeHash;
    }

    public VmAccount(@NonNull Address address, long nonce, @NonNull BigInteger balance) {
        this(address, nonce, balance, KECCAK256_NULL_HASH, KECCAK256_NULL_HASH);
    }

    public VmAccount(@NonNull Address address, long nonce, long balance) {
        this(address, nonce, BigInteger.valueOf(balance));
    }

    /**
     * Returns the clone of given account
     *
     * @param account source account
     * @return the clone of given account
     */
    public static Account from(Account account) {
        return new VmAccount(account.address(), account.nonce(), account.balance(),
            new Hash(account.storageRoot()), new Hash(account.codeHash()));
    }

    @Override
    public Address address() {
        return address;
    }

    @Override
    public BigInteger balance() {
        return balance;
    }

    @Override
    public void addBalance(BigInteger value) {
        this.balance = balance.add(value);
    }

    @Override
    public void subBalance(BigInteger amount) {
        var newBalance = balance.subtract(amount);
        if (newBalance.signum() < 0) {
            throw new IllegalArgumentException("The subtracted amount greater than current balance; address="
                + address.hex() + " balance=" + balance + " amount=" + amount);
        }
        this.balance = newBalance;
    }

    @Override
    public long nonce() {
        return nonce;
    }

    @Override
    public void setNonce(long nonce) {
        this.nonce = nonce;
    }

    @Override
    public byte[] storageRoot() {
        return root.bytes();
    }

    @Override
    public byte[] codeHash() {
        return codeHash.bytes();
    }

    @Override
    public String toString() {
        return "VmAccount{" +
            "address=" + address.hex() +
            ", nonce=" + nonce +
            ", balance=" + balance +
            ", root=" + root.hex() +
            ", codeHash=" + codeHash.hex() +
            '}';
    }
}
