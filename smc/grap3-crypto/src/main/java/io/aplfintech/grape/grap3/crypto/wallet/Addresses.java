package io.aplfintech.grape.grap3.crypto.wallet;

import io.aplfintech.grape.grap3.crypto.Hashers;
import io.aplfintech.grape.utils.Bytes;
import lombok.AccessLevel;
import lombok.NoArgsConstructor;
import org.web3j.crypto.ContractUtils;

import java.math.BigInteger;
import java.util.Arrays;

/**
 * Utility class to manipulate with wallet addresses
 *
 * @author andrew.zinchenko@gmail.com
 */
@NoArgsConstructor(access = AccessLevel.PRIVATE)
public class Addresses {

    /**
     * Recovers the address by the sender public key
     *
     * @param publicKey the sender public key
     * @return created address
     */
    public static byte[] createAddress(byte[] publicKey) {
        var pkHash = Hashers.keccak256().digest(publicKey);
        return sliceAddress(pkHash);
    }

    /**
     * Creates the new contract address by thw given sender address and sender nonce
     * Used inside VM to create new contract address
     *
     * @param address sender address
     * @param nonce   sender nonce
     * @return the new created address
     */
    public static byte[] createAddress(byte[] address, long nonce) {
        return ContractUtils.generateContractAddress(address, BigInteger.valueOf(nonce));
    }

    /**
     * Creates the new contract address by thw given sender address, salt and contract code
     * Used inside VM to create new contract address
     *
     * @param address sender address
     * @param salt    sender nonce
     * @param code    contract code
     * @return the new created address
     */
    public static byte[] createAddress2(byte[] address, byte[] salt, byte[] code) {
        var md = Hashers.keccak256();
        md.update(new byte[]{(byte) 0xff});
        md.update(address);
        md.update(Bytes.leftPadBytes(salt, 32));
        md.update(Hashers.keccak256().digest(code));
        var hash = md.digest();
        return sliceAddress(hash);
    }

    /**
     * Returns latest 20 bytes from the byte array
     *
     * @param hash byte array
     */
    private static byte[] sliceAddress(byte[] hash) {
        return Arrays.copyOfRange(hash, 12, 32);
    }
}
