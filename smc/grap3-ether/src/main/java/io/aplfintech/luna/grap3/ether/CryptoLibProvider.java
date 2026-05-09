package io.aplfintech.luna.grap3.ether;

import io.aplfintech.luna.grap3.crypto.Hashers;
import io.aplfintech.luna.grap3.crypto.wallet.Addresses;
import io.aplfintech.luna.grap3.ether.crypto.ECDSA;
import io.aplfintech.luna.crypto.CryptoLib;

import java.math.BigInteger;

/**
 * @author andrew.zinchenko@gmail.com
 */
public class CryptoLibProvider {
    private static final CryptoLib INSTANCE;

    static {
        INSTANCE = new CryptoLibImpl();
    }

    public static CryptoLib crypto() {
        return INSTANCE;

    }

    private static class CryptoLibImpl implements CryptoLib {

        @Override
        public boolean validateSignatureValues(byte v, BigInteger r, BigInteger s) {

            return new ECDSA().validateSignatureValues(v, r, s);
        }

        @Override
        public byte[] ecRecover(byte[] hash, byte[] signature) {
            return new ECDSA().recover(hash, signature);
        }

        @Override
        public byte[] keccak256(byte[] bytes) {
            return Hashers.keccak256().digest(bytes);
        }

        @Override
        public byte[] sha256sum(byte[] bytes) {
            return Hashers.sha256().digest(bytes);
        }

        @Override
        public byte[] ripemd160sum(byte[] bytes) {
            return Hashers.ripemd160().digest(bytes);
        }

        @Override
        public byte[] createAddress(byte[] address, long nonce) {
            return Addresses.createAddress(address, nonce);
        }

        @Override
        public byte[] createAddress2(byte[] address, byte[] salt, byte[] code) {
            return Addresses.createAddress2(address, salt, code);
        }

        @Override
        public byte[] recoverAddress(byte[] publicKey) {
            return Addresses.createAddress(publicKey);
        }
    }
}
