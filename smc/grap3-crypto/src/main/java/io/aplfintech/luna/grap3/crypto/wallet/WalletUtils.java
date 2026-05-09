package io.aplfintech.luna.grap3.crypto.wallet;

import com.fasterxml.jackson.core.JsonProcessingException;
import io.aplfintech.luna.grap3.crypto.Crypto;
import io.aplfintech.luna.grap3.crypto.utils.ScryptUtils;
import io.aplfintech.luna.utils.HexUtils;
import io.aplfintech.luna.utils.JsonUtils;
import lombok.AccessLevel;
import lombok.NoArgsConstructor;
import lombok.NonNull;
import lombok.extern.slf4j.Slf4j;

import java.io.File;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;

/**
 * @author andrew.zinchenko@gmail.com
 */
@Slf4j
@NoArgsConstructor(access = AccessLevel.PRIVATE)
public class WalletUtils {
    public static final String WALLET_SPEC_VERSION = "1";

    public static Wallet importWallet(@NonNull File file, @NonNull String password) {
        try {
            var content = Files.readString(file.toPath());
            return importWallet(content, password);
        } catch (IOException e) {
            throw new ImportWalletException(e.getMessage(), e);
        }
    }

    public static Wallet importWallet(@NonNull String content, @NonNull String password) {
        WalletSpec walletSpec;
        try {
            walletSpec = JsonUtils.HEX_MAPPER.readValue(content, WalletSpec.class);
        } catch (JsonProcessingException e) {
            throw new ImportWalletException("Wrong content, can't parse json", e);
        }
        var pwd = password.getBytes(StandardCharsets.UTF_8);
        byte[] privateKey = ScryptUtils.decryptData(walletSpec.getCrypto(), pwd);
        if (privateKey.length == ScryptUtils.KEY_LEN_BYTE * 2) {
            privateKey = HexUtils.parseHex(new String(privateKey));
        }
        if (privateKey.length != ScryptUtils.KEY_LEN_BYTE) {
            throw new ImportWalletException("Suspicious Cipher text"+new String(walletSpec.getCrypto().getCipherText()));
        }
        //generate public key from private key
        var publicKey = Crypto.currentDSA().generatePublicKey(privateKey);
        //generate address by public key
        var address = Addresses.createAddress(publicKey);
        var addressHex = HexUtils.toHex(address, true);
        if (!walletSpec.getAddress().equals(addressHex)) {
            throw new ImportWalletException("Addresses are not equal, expected=" + walletSpec.getAddress() + " got=" + addressHex);
        }
        //create wallet
        return new Wallet(walletSpec.getAddress(), HexUtils.toHex(privateKey), HexUtils.toHex(publicKey));
    }

    public static File exportEncryptedWallet(@NonNull Wallet wallet, @NonNull String password, @NonNull Path dir) {
        String fileName = wallet.getAddress() + ".json";
        var file = dir.resolve(fileName).toFile();
        if (exportEncryptedWallet(wallet, password, file)) {
            return file;
        }
        throw new ExportWalletException("Can't write wallet to file:" + file.getAbsolutePath());
    }

    public static boolean exportEncryptedWallet(@NonNull Wallet wallet, @NonNull String password, @NonNull File out) {
        try {
            String encrypted = exportEncryptedWallet(wallet, password);
            Files.writeString(out.toPath(), encrypted);
            return true;
        } catch (IOException e) {
            log.error("Can't write wallet to file", e);
            return false;
        }
    }

    public static String exportEncryptedWallet(@NonNull Wallet wallet, @NonNull String password) {
        var walletSpec = encryptWallet(wallet, password.getBytes(StandardCharsets.UTF_8));
        try {
            return JsonUtils.HEX_MAPPER.writeValueAsString(walletSpec);
        } catch (JsonProcessingException e) {
            log.error("Can't serialize wallet spec: " + wallet, e);
            throw new ExportWalletException("Can't serialize wallet spec: " + wallet);
        }
    }

    public static WalletSpec encryptWallet(@NonNull Wallet wallet, byte @NonNull [] password) {
        var pk = HexUtils.parseHex(wallet.getPrivateKey());
        var cryptoSpec = ScryptUtils.encryptData(pk, password);
        return new WalletSpec(WALLET_SPEC_VERSION, wallet.getAddress(), cryptoSpec);
    }
}
