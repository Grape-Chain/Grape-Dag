package io.aplfintech.luna.grap3.crypto.wallet;

import io.aplfintech.luna.grap3.crypto.spec.AESCipherAlg;
import io.aplfintech.luna.grap3.crypto.spec.Kdf;
import io.aplfintech.luna.utils.FileUtils;
import io.aplfintech.luna.utils.JsonUtils;
import lombok.SneakyThrows;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.ValueSource;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;

/**
 * @author andrew.zinchenko@gmail.com
 */
class WalletSpecTest {

    @SneakyThrows
    @ValueSource(strings = {"genesis.json", "wallet1.json", "wallet-0x7265f3dd-js.json", "wallet-0xfbd169d0-js-hex.json"})
    @ParameterizedTest
    void deserialize_serialize(String fileName) {
        //GIVEN
        var content = FileUtils.readResourceContent("wallet/" + fileName);
        //WHEN
        var walletSpec = JsonUtils.HEX_MAPPER.readValue(content, WalletSpec.class);
        var json = JsonUtils.HEX_MAPPER.writeValueAsString(walletSpec);
        //THEN
        assertNotNull(json);
    }

    @SneakyThrows
    @ValueSource(strings = {"genesis.json", "wallet1.json", "wallet-0x7265f3dd-js.json", "wallet-0xfbd169d0-js-hex.json"})
    @ParameterizedTest
    void deserialize(String fileName) {
        //GIVEN
        var content = FileUtils.readResourceContent("wallet/" + fileName);
        //WHEN
        var walletSpec = JsonUtils.HEX_MAPPER.readValue(content, WalletSpec.class);
        //THEN
        assertNotNull(walletSpec);
        assertEquals(AESCipherAlg.AES_256_GCM, walletSpec.getCrypto().getCipher());
        assertEquals(Kdf.SCRYPT, walletSpec.getCrypto().getKdf());
        assertEquals(1024, walletSpec.getCrypto().getKdfParams().getN());
    }

}