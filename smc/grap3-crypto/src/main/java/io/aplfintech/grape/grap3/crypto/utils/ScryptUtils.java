package io.aplfintech.grape.grap3.crypto.utils;

import io.aplfintech.grape.grap3.crypto.Crypto;
import io.aplfintech.grape.grap3.crypto.CryptoLibException;
import io.aplfintech.grape.grap3.crypto.Hashers;
import io.aplfintech.grape.grap3.crypto.spec.AESCipherAlg;
import io.aplfintech.grape.grap3.crypto.spec.CipherParams;
import io.aplfintech.grape.grap3.crypto.spec.CryptoSpec;
import io.aplfintech.grape.grap3.crypto.spec.Kdf;
import io.aplfintech.grape.grap3.crypto.spec.KdfParams;
import lombok.AccessLevel;
import lombok.NoArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.bouncycastle.crypto.generators.SCrypt;

import javax.crypto.Cipher;
import javax.crypto.spec.GCMParameterSpec;
import javax.crypto.spec.SecretKeySpec;
import java.security.GeneralSecurityException;
import java.security.NoSuchAlgorithmException;
import java.security.SecureRandom;
import java.util.Arrays;

/**
 * @author andrew.zinchenko@gmail.com
 */
@Slf4j
@NoArgsConstructor(access = AccessLevel.PRIVATE)
public class ScryptUtils {
    private static final SecureRandom RND;
    public static final int KEY_LEN = 256;
    public static final int KEY_LEN_BYTE = KEY_LEN / 8;
    private static final int T_LEN = 128;

    static {
        var p = Crypto.getProvider();
        if (p == null) {
            throw new IllegalStateException("Undefined crypto provider");
        }
        try {
            RND = SecureRandom.getInstanceStrong();
            RND.nextBoolean();
        } catch (NoSuchAlgorithmException e) {
            var msg = "No algorithm available: DRBG";
            log.error(msg);
            throw new CryptoLibException(msg, e);
        }
    }

    public static CryptoSpec encryptData(byte[] data, byte[] password) {
        var salt = new byte[KEY_LEN_BYTE];
        RND.nextBytes(salt);
        var iv = new byte[20];
        RND.nextBytes(iv);
        return encryptData(data, password, salt, iv);
    }

    public static CryptoSpec encryptData(byte[] data, byte[] password, byte[] salt, byte[] iv) {
        try {
            var kdfParams = new KdfParams(salt);
            var encryptionKey = generateSecretKey(password, salt);
            var keySpec = new SecretKeySpec(encryptionKey, AESCipherAlg.NAME);//AES
            var gcmParam = new GCMParameterSpec(T_LEN, iv);
            var cipher = Cipher.getInstance(AESCipherAlg.AES_256_GCM.algorithm, Crypto.getProvider());//AES/GCM/NoPadding
            cipher.init(Cipher.ENCRYPT_MODE, keySpec, gcmParam);
            var ciphertext = cipher.doFinal(data);
            var mac = mac(ciphertext, encryptionKey);
            return new CryptoSpec(ciphertext, new CipherParams(iv), AESCipherAlg.AES_256_GCM, Kdf.SCRYPT, kdfParams, mac);
        } catch (GeneralSecurityException e) {
            throw new CryptoLibException("Can't encrypt data, cause " + e.getMessage(), e);
        }
    }

    public static byte[] decryptData(CryptoSpec cryptoSpec, byte[] password) {
        try {
            var encryptionKey = generateSecretKey(password, cryptoSpec.getKdfParams());
            var alg = cryptoSpec.getCipher().algorithm.split("/")[0];//AES
            var keySpec = new SecretKeySpec(encryptionKey, alg);
            //verify mac
            var mac = mac(cryptoSpec.getCipherText(), encryptionKey);
            if (!Arrays.equals(mac, cryptoSpec.getMac())) {
                throw new CryptoLibException("Incorrect password");
            }
            var gcmParam = new GCMParameterSpec(T_LEN, cryptoSpec.getCipherParams().getIv());
            var cipher = Cipher.getInstance(cryptoSpec.getCipher().algorithm, Crypto.getProvider());//AES/GCM/NoPadding
            cipher.init(Cipher.DECRYPT_MODE, keySpec, gcmParam);
            return cipher.doFinal(cryptoSpec.getCipherText());
        } catch (GeneralSecurityException e) {
            throw new CryptoLibException("Can't decrypt data, cause " + e.getMessage(), e);
        }
    }

    public static byte[] mac(byte[] cipherText, byte[] key) {
        var hasher = Hashers.sha256();
        hasher.update(cipherText);
        return hasher.digest(key);
    }

    public static byte[] generateSecretKey(byte[] password) {
        var salt = new byte[32];
        RND.nextBytes(salt);
        var kdfParams = new KdfParams(salt);
        return generateSecretKey(password, kdfParams);
    }

    public static byte[] generateSecretKey(byte[] password, byte[] salt) {
        var kdfParams = new KdfParams(salt);
        return generateSecretKey(password, kdfParams);
    }

    public static byte[] generateSecretKey(byte[] password, KdfParams kdfParams) {
        return SCrypt.generate(password, kdfParams.getSalt(), kdfParams.getN(), kdfParams.getR(), kdfParams.getP(), kdfParams.getDklen());
    }
}
