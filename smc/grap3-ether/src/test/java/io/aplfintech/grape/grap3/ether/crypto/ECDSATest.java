package io.aplfintech.grape.grap3.ether.crypto;

import io.aplfintech.grape.grap3.crypto.Crypto;
import io.aplfintech.grape.grap3.crypto.DSARecoverable;
import io.aplfintech.grape.grap3.crypto.KeyPair;
import io.aplfintech.grape.grap3.crypto.utils.KeyUtils;
import lombok.extern.slf4j.Slf4j;
import org.bouncycastle.util.encoders.Hex;
import org.junit.jupiter.api.RepeatedTest;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.CsvSource;

import java.util.HexFormat;

import static org.assertj.core.api.AssertionsForClassTypes.assertThat;

/**
 * @author andrew.zinchenko@gmail.com
 */
@Slf4j
class ECDSATest {

    static byte[] pubKey = Hex.decode("04eeadfb7b03a08cf7c4e305c1351188ee70a49fe2fc0919775759706f15edd1304366f498065854dccfec9cf0ed41a3eec1c46a71579b10e5f3f0a1d574d35587");
    static byte[] privKey = Hex.decode("6b6be102606ea5d489814cefc366d95655c6406514f2599aecce151d30555ab5");
    static KeyPair keyPair = new KeyPair(privKey, pubKey);
    static byte[] msgHash = Hex.decode("b3eab5ad582b213ef429beee9f7b65682b093f7cdca4ace6d060cbb9077b1514");
    static byte[] signature = Hex.decode("a433f685bc69bda39e89c320b5795b96c789315635b0e0f1a5fee408a001ad72558ab8be54668bcee89442f469a87b430dfc0b69c78800f985a36716cd13d11f00");


    static String MESSAGE = "52fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64981855ad8681d0d86d1e91e00167939cb6694d2c422acd208a0072939487f6999eb9d18a44784045d87f3c67cf22746e995af5a25367951baa2ff6cd471c483f15fb90badb37c5821b6d95526a41a9504680b4e7c8b763a1b";
    static byte[] MESSAGE_TO_SIGN = HexFormat.of().parseHex(MESSAGE);
    static byte[] MESSAGE_HASH = Crypto.newHasher().digest(MESSAGE_TO_SIGN);

    @RepeatedTest(5)
    void signVerify() {
        KeyPair randomKeyPair;
        randomKeyPair = Secp256.generateRandom();
        DSARecoverable dsa = new ECDSA();
        //WHEN
        var signature = dsa.sign(randomKeyPair.privateKey(), MESSAGE_HASH);
        var rc = dsa.verify(randomKeyPair.publicKey(), signature, MESSAGE_HASH);
        //THEN
        assertThat(rc)
            .isTrue();
    }

    @CsvSource({
        "b3eab5ad582b213ef429beee9f7b65682b093f7cdca4ace6d060cbb9077b1514, 6b6be102606ea5d489814cefc366d95655c6406514f2599aecce151d30555ab5, a433f685bc69bda39e89c320b5795b96c789315635b0e0f1a5fee408a001ad72558ab8be54668bcee89442f469a87b430dfc0b69c78800f985a36716cd13d11f"
    })
    @ParameterizedTest
    void testEthereumSign(String msg, String privKey, String expSign) {
        var hash = Hex.decode(msg);
        var privateKey = Hex.decode(privKey);
        var expectedSign = Hex.decode(expSign);
        DSARecoverable dsa = new ECDSA();
        //WHEN
        var signature = dsa.sign(privateKey, hash);
        log.info("signature={}", KeyUtils.toHex(signature));
        log.info(" expected={}", expSign);
        //THEN
        assertThat(signature)
            .isEqualTo(expectedSign);
    }

    @CsvSource({
        "b3eab5ad582b213ef429beee9f7b65682b093f7cdca4ace6d060cbb9077b1514, a433f685bc69bda39e89c320b5795b96c789315635b0e0f1a5fee408a001ad72558ab8be54668bcee89442f469a87b430dfc0b69c78800f985a36716cd13d11f00, 04eeadfb7b03a08cf7c4e305c1351188ee70a49fe2fc0919775759706f15edd1304366f498065854dccfec9cf0ed41a3eec1c46a71579b10e5f3f0a1d574d35587"
    })
    @ParameterizedTest
    void testRecover(String in, String sign, String expPubKey) {
        var hash = Hex.decode(in);
        var signature = Hex.decode(sign);
        var expectedPubKey = Hex.decode(expPubKey);
        DSARecoverable dsa = new ECDSA();
        //WHEN
        var pubKey = dsa.recover(hash, signature);
        log.info("public key={}", KeyUtils.toHex(pubKey));
        log.info("  expected={}", expPubKey);
        //THEN
        assertThat(pubKey)
            .isEqualTo(expectedPubKey);
    }
}