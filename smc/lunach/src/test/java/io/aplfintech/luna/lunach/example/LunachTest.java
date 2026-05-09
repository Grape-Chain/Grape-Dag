package io.aplfintech.luna.lunach.example;

import io.aplfintech.luna.grap3.crypto.KeyPair;
import io.aplfintech.luna.grap3.crypto.utils.KeyUtils;
import io.aplfintech.luna.grap3.crypto.wallet.Addresses;
import io.aplfintech.luna.lunach.Lunach;
import io.aplfintech.luna.lunach.TxBuilder;
import io.aplfintech.luna.utils.HexUtils;
import io.aplfintech.luna.utils.JsonUtils;
import lombok.SneakyThrows;
import lombok.extern.slf4j.Slf4j;
import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
class LunachTest {

    //the genesis account of current testnet, chainId=2
    static String addressHex = "0xd09ec4a81cde61b57de012d3fe80beae3f28fb68";
    static String publicKeyHex = "940d33cca77608545439c121b679c23b6915f7990ff279cb902c2ccaff2057e3";
    static String privateKeyHex = "6f1c1e3f54a6699be61d927f804a191b90912820d89d8d5a8b143e1990fcc0af";
    static String walletJson = "{" +
        "\"address\":\"" + addressHex + "\"," +
        "\"privateKey\":\"" + privateKeyHex + "\"," +
        "\"publicKey\":\"" + publicKeyHex + "\"" +
        "}";
    static KeyPair keyPair;

    @BeforeAll
    static void beforeAll() {
        keyPair = KeyUtils.getEd25519Generator().generateRandom();
    }

    @SneakyThrows
    @Test
    void createTransferTx() {
        //GIVEN
        var recipient = Addresses.createAddress(keyPair.publicKey());
        int amount = 10_000_000;
        //WHEN
        var signedTx = Lunach.newTxBuilder()
            .chainId(2)
            .type(TxBuilder.Type.PAYMENT)
            .senderPublicKey(publicKeyHex)
            .recipient(recipient)
            .amount(amount)
            .build(privateKeyHex);
        //THEN
        assertNotNull(signedTx);
        log.info("signedTx=\"{}\"", signedTx);
    }

    @Test
    void createPublishContractTx() {
        //GIVEN
        //code of the storage.sol contract
        var contractCode = "0x608060405234801561001057600080fd5b50610150806100206000396000f3fe608060405234801561001057600080fd5b50600436106100365760003560e01c80632e64cec11461003b5780636057361d14610059575b600080fd5b610043610075565b60405161005091906100d9565b60405180910390f35b610073600480360381019061006e919061009d565b61007e565b005b60008054905090565b8060008190555050565b60008135905061009781610103565b92915050565b6000602082840312156100b3576100b26100fe565b5b60006100c184828501610088565b91505092915050565b6100d3816100f4565b82525050565b60006020820190506100ee60008301846100ca565b92915050565b6000819050919050565b600080fd5b61010c816100f4565b811461011757600080fd5b5056fea26469706673582212209a159a4f3847890f10bfb87871a61eba91c5dbf5ee3cf6398207e292eee22a1664736f6c63430008070033";
        //WHEN
        var signedTx = Lunach.newTxBuilder()
            .chainId(2)
            .type(TxBuilder.Type.PUBLISH_CONTRACT)
            .senderPublicKey(publicKeyHex)
            .nonce(0)//!!!! Set the real nonce that should be requested from the account
            .amount(0)
            .fuelLimit(70_000)
            .fuelPrice(5)
            .data(contractCode)
            .build(privateKeyHex);
        //THEN
        assertNotNull(signedTx);
        log.info("signedTx=\"{}\"", signedTx);
    }

    @Test
    void createCallContractTx() {
        //GIVEN
        //storage.sol
        var contractCode = HexUtils.parseHex("0x6057361d0000000000000000000000000000000000000000000123456789aabbccddeeff");
        var privKey = HexUtils.parseHex(privateKeyHex);
        var pubKey = HexUtils.parseHex(publicKeyHex);
        var contractAddress = HexUtils.parseHex("0xa00f004a23729c98b032aa199e886c6af96a715d3caca0f6d833865d3e97dedb");
        //WHEN
        var signedTx = Lunach.newTxBuilder()
            .chainId(2)
            .type(TxBuilder.Type.PUBLISH_CONTRACT)
            .senderPublicKey(pubKey)
            .recipient(contractAddress)
            .nonce(1)//!!!! Set the real nonce that should be requested from the account
            .amount(0)
            .fuelLimit(70_000)
            .fuelPrice(5)
            .data(contractCode)
            .build(privKey);
        //THEN
        assertNotNull(signedTx);
        log.info("signedTx=\"{}\"", HexUtils.toHex(signedTx, true));
    }

    @Test
    void createPublishContractMessage() {
        //GIVEN
        //storage.sol
        var contractCode = "0x608060405234801561001057600080fd5b50610150806100206000396000f3fe608060405234801561001057600080fd5b50600436106100365760003560e01c80632e64cec11461003b5780636057361d14610059575b600080fd5b610043610075565b60405161005091906100d9565b60405180910390f35b610073600480360381019061006e919061009d565b61007e565b005b60008054905090565b8060008190555050565b60008135905061009781610103565b92915050565b6000602082840312156100b3576100b26100fe565b5b60006100c184828501610088565b91505092915050565b6100d3816100f4565b82525050565b60006020820190506100ee60008301846100ca565b92915050565b6000819050919050565b600080fd5b61010c816100f4565b811461011757600080fd5b5056fea26469706673582212209a159a4f3847890f10bfb87871a61eba91c5dbf5ee3cf6398207e292eee22a1664736f6c63430008070033";
        var sender = Addresses.createAddress(HexUtils.parseHex(publicKeyHex));
        //WHEN
        var message = Lunach.newMsgBuilder()
            .from(sender)
            .to(new byte[0])//!!!! IT'S CRUCIAL
            .amount(0)
            .data(contractCode)
            .buildJson();
        //THEN
        assertNotNull(message);
        log.info("message={}", message);
    }

    @Test
    void createCallContractMessage() {
        //GIVEN
        //storage.sol
        var contractCode = HexUtils.parseHex("0x6057361d0000000000000000000000000000000000000000000123456789aabbccddeeff");
        var sender = HexUtils.parseHex(addressHex);
        var contractAddress = HexUtils.parseHex("0xa00f004a23729c98b032aa199e886c6af96a715d3caca0f6d833865d3e97dedb");
        //WHEN
        var message = Lunach.newMsgBuilder()
            .from(sender)
            .to(contractAddress)
            .amount(0)
            .data(contractCode)
            .buildJson();
        //THEN
        assertNotNull(message);
        log.info("message={}", message);
    }

    @Test
    void restoreAddress() {
        //GIVEN
        //WHEN
        var addr1 = Lunach.restoreAddress(publicKeyHex);
        //THEN
        assertThat(addr1)
            .isEqualTo(addressHex);
        //WHEN
        var addr2 = Lunach.restoreAddress(HexUtils.parseHex(publicKeyHex));
        //THEN
        assertThat(addr2)
            .isEqualTo(HexUtils.parseHex(addressHex));
    }

    @SneakyThrows
    @Test
    void testWalletJson() {
        //GIVEN

        //WHEN
        var genesisWallet = JsonUtils.HEX_MAPPER.readValue(walletJson, Lunach.Wallet.class);
        //THEN
        assertEquals(addressHex, genesisWallet.hex());
        assertEquals(addressHex, genesisWallet.getAddress());
        assertEquals(privateKeyHex, genesisWallet.getPrivateKey());
        assertEquals(publicKeyHex, genesisWallet.getPublicKey());

        //WHEN
        var json = JsonUtils.HEX_MAPPER.writeValueAsString(genesisWallet);
        //THEN
        assertThat(json)
            //.isEqualTo(genesisWalletJson)
            .contains(addressHex, publicKeyHex, privateKeyHex);
    }

    @Test
    void createRandomWallet() {
        //GIVEN
        //WHEN
        var wallet1 = Lunach.createRandomWallet();
        var wallet2 = Lunach.createRandomWallet();
        //THEN
        assertThat(wallet1)
            .hasNoNullFieldsOrProperties();
        assertThat(wallet2)
            .hasNoNullFieldsOrProperties();
        assertThat(wallet1)
            .isNotEqualTo(wallet2);
    }

    @Test
    void createWallet() {
        //GIVEN
        //WHEN
        var wallet = Lunach.createWallet(publicKeyHex);
        //THEN
        assertThat(wallet)
            .hasNoNullFieldsOrPropertiesExcept("privateKey")
            .hasFieldOrPropertyWithValue("privateKey", null)
            .hasFieldOrPropertyWithValue("address", addressHex)
            .hasFieldOrPropertyWithValue("publicKey", publicKeyHex);
    }

    @SneakyThrows
    @Test
    void createWallet2() {
        //GIVEN
        //WHEN
        var wallet = Lunach.createWallet(privateKeyHex, publicKeyHex);
        //THEN
        assertThat(wallet)
            .hasNoNullFieldsOrProperties()
            .hasFieldOrPropertyWithValue("privateKey", privateKeyHex)
            .hasFieldOrPropertyWithValue("address", addressHex)
            .hasFieldOrPropertyWithValue("publicKey", publicKeyHex);
        //WHEN
        var json = JsonUtils.HEX_MAPPER.writeValueAsString(wallet);
        //THEN
        assertThat(json)
            .isEqualTo(walletJson);

    }

}