package io.aplfintech.luna.lunach;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonProperty;
import io.aplfintech.luna.grap3.crypto.Crypto;
import io.aplfintech.luna.grap3.crypto.KeyPair;
import io.aplfintech.luna.grap3.crypto.utils.KeyUtils;
import io.aplfintech.luna.grap3.crypto.wallet.Addresses;
import io.aplfintech.luna.utils.HexUtils;
import lombok.*;
import lombok.extern.slf4j.Slf4j;

import javax.annotation.Nullable;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
@NoArgsConstructor(access = AccessLevel.PRIVATE)
public class Lunach {

    /**
     * Returns the new instance of the transaction builder to build the transaction bytes
     * <p>
     * Example:
     * <code>
     *
     * </code>
     */
    public static TxBuilder newTxBuilder() {
        return new TxBuilderImpl();
    }

    /**
     * Returns the new instance of the message builder to build the message bytes
     */
    public static MessageBuilder newMsgBuilder() {
        return new MessageBuilderImpl();
    }

    /**
     * Creates the new contract address by thw given sender address and sender nonce
     */
    public static byte[] createAddress(byte @NonNull [] address, long nonce) {
        return Addresses.createAddress(address, nonce);
    }

    /**
     * Restore the address from the given public key
     */
    public static byte[] restoreAddress(byte @NonNull [] publicKey) {
        return Addresses.createAddress(publicKey);
    }

    /**
     * Restore the address from the given public key
     */
    public static String restoreAddress(@NonNull String publicKeyHex) {
        return HexUtils.toHex(restoreAddress(HexUtils.parseHex(publicKeyHex)), true);
    }

    /**
     * Returns the signature bytes for the input message using the EdDSA12259 algorithm
     * The input message should be a SHA-256 hash of the real message
     *
     * @param messageHash input message hash
     * @param privateKey  private key for signing
     * @return the signature
     */
    public static byte[] sign(byte[] messageHash, byte[] privateKey) {
        return Crypto.currentDSA().sign(messageHash, privateKey);
    }


    /**
     * Verifies the passed-in signature.
     * Returns true if the signature was verified, false if not.
     * The input message should be a SHA-256 hash of the real message
     *
     * @param publicKey the public key
     * @param signature the signature bytes
     * @param message   the signed message
     * @return true if the signature was verified, false if not.
     */
    public static boolean verify(byte[] publicKey, byte[] signature, byte[] message) {
        return Crypto.currentDSA().verify(publicKey, signature, message);
    }

    /**
     * Generates random key pair and returns the wallet instance
     */
    public static Wallet createRandomWallet() {
        var keys = KeyUtils.getEd25519Generator().generateRandom();
        return Wallet.from(keys);
    }

    /**
     * Returns the wallet for the given public key
     */
    public static Wallet createWallet(@NonNull String publicKeyHex) {
        return Wallet.from(publicKeyHex);
    }

    /**
     * Returns the wallet for the given keys
     */
    public static Wallet createWallet(@NonNull String privateKeyHex, @NonNull String publicKeyHex) {
        return Wallet.from(privateKeyHex, publicKeyHex);
    }

    @ToString
    @EqualsAndHashCode
    @Getter
    public static class Wallet {
        @JsonProperty("address")
        String address;
        @JsonProperty("privateKey")
        String privateKey;
        @JsonProperty("publicKey")
        String publicKey;

        @JsonCreator
        Wallet(@NonNull @JsonProperty("address") String address,
               @Nullable @JsonProperty("privateKey") String privateKey,
               @NonNull @JsonProperty("publicKey") String publicKey) {
            this.address = address;
            this.privateKey = privateKey;
            this.publicKey = publicKey;
        }

        public static Wallet from(@NonNull String privateKeyHex, @NonNull String publicKeyHex) {
            var publicKey = HexUtils.parseHex(publicKeyHex);
            var address = HexUtils.toHex(restoreAddress(publicKey), true);
            return new Wallet(address, privateKeyHex, publicKeyHex);
        }

        public static Wallet from(KeyPair keys) {
            var privateKey = HexUtils.toHex(keys.privateKey(), true);
            var publicKey = HexUtils.toHex(keys.publicKey(), true);
            var address = HexUtils.toHex(restoreAddress(keys.publicKey()), true);
            return new Wallet(address, privateKey, publicKey);
        }

        public static Wallet from(String publicKeyHex) {
            var publicKey = HexUtils.parseHex(publicKeyHex);
            var address = HexUtils.toHex(restoreAddress(publicKey), true);
            return new Wallet(address, null, publicKeyHex);
        }

        public String hex() {
            return address;
        }
    }
}
