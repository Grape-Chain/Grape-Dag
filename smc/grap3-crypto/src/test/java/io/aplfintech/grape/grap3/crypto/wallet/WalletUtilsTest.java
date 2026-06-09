package io.aplfintech.grape.grap3.crypto.wallet;

import lombok.SneakyThrows;
import lombok.extern.slf4j.Slf4j;
import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.ValueSource;

import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;

import static org.junit.jupiter.api.Assertions.assertNotNull;

/**
 * @author andrew.zinchenko@gmail.com
 */
@Slf4j
class WalletUtilsTest {
    private static Path tmpDir;

    @SneakyThrows
    @BeforeAll
    static void beforeAll() {
        tmpDir = Files.createTempDirectory("exported-wallets-");
        tmpDir.toFile().deleteOnExit();
    }

    @ValueSource(strings = {"genesis.json", "wallet1.json", "wallet-0x7265f3dd-js.json", "wallet-0xfbd169d0-js-hex.json"})
    @ParameterizedTest
    void importWalletFile(String fileName) {
        var password = "qweasdzxc";
        var file = Paths.get("src", "test", "resources", "wallet").resolve(fileName).toFile();
        //WHEN
        var wallet = WalletUtils.importWallet(file, password);
        //THEN
        assertNotNull(wallet);
    }


    @ValueSource(strings = {"genesis.json", "wallet1.json", "wallet-0x7265f3dd-js.json", "wallet-0xfbd169d0-js-hex.json"})
    @ParameterizedTest
    void importExportWalletFile(String fileName) {
        var password = "qweasdzxc";
        var jsonWalletFile = Paths.get("src", "test", "resources", "wallet").resolve(fileName).toFile();
        //WHEN
        var wallet = WalletUtils.importWallet(jsonWalletFile, password);
        //THEN
        assertNotNull(wallet);
        //WHEN
        var file = WalletUtils.exportEncryptedWallet(wallet, password, tmpDir);
        //THEN
        assertNotNull(file);
        file.deleteOnExit();
    }

}