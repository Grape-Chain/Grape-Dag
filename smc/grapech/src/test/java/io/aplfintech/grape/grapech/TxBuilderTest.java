package io.aplfintech.grape.grapech;

import io.aplfintech.grape.grap3.crypto.KeyPair;
import io.aplfintech.grape.grap3.crypto.utils.KeyUtils;
import io.aplfintech.grape.grap3.crypto.wallet.Addresses;
import io.aplfintech.grape.utils.HexUtils;
import lombok.SneakyThrows;
import lombok.extern.slf4j.Slf4j;
import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.api.Test;
import pb.TxvX;

import static org.assertj.core.api.Assertions.assertThat;
import static org.junit.jupiter.api.Assertions.assertArrayEquals;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
class TxBuilderTest {

    //the genesis account of current testnet, chainId=2
    static String genesisAddrHex = "0xd09ec4a81cde61b57de012d3fe80beae3f28fb68";
    static String genesisPubKeyHex = "940d33cca77608545439c121b679c23b6915f7990ff279cb902c2ccaff2057e3";
    static String genesisPrivKeyHex = "6f1c1e3f54a6699be61d927f804a191b90912820d89d8d5a8b143e1990fcc0af";

    static KeyPair keyPair;

    @BeforeAll
    static void beforeAll() {
        keyPair = KeyUtils.getEd25519Generator().generateRandom();
    }

    @SneakyThrows
    @Test
    void signTransferTx() {
        var privKey = keyPair.privateKey();
        var pubKey = keyPair.publicKey();
        var recipient = Addresses.createAddress(pubKey);

        int amount = 10_000_000;
        int fuelLimit = 21_000;
        int fuelPrice = 1;
        TxBuilder builder = Grapech.newTxBuilder()
                .chainId(2)
                .type(TxBuilder.Type.PAYMENT)
                .senderPublicKey(pubKey)
                .recipient(recipient)
                .amount(amount)
                .fuelLimit(fuelLimit)
                .fuelPrice(fuelPrice);

        var rawTx = builder.build();
        log.info("   rawTx=\"{}\"", HexUtils.toHex(rawTx, true));
        var signature = builder.sign(privKey);
        var signedTx = builder.build(privKey);
        log.info("signedTx=\"{}\"", HexUtils.toHex(signedTx, true));
        var tx = TxvX.Txv1.parseFrom(signedTx);
        assertArrayEquals(signature, tx.getSignature().toByteArray());
    }

    @SneakyThrows
    @Test
    void genesisAddressTest() {
        //GIVEN
        var privKey = HexUtils.parseHex(genesisPrivKeyHex);
        var pubKey = HexUtils.parseHex(genesisPubKeyHex);
        var genesis = HexUtils.parseHex(genesisAddrHex);
        //WHEN
        byte[] address = Addresses.createAddress(pubKey);
        //THEN
        assertThat(address)
                .isEqualTo(genesis);
    }

}