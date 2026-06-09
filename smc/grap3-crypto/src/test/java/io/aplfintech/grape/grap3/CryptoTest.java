package io.aplfintech.grape.grap3;

import io.aplfintech.grape.grap3.crypto.Crypto;
import io.aplfintech.grape.grap3.crypto.DSA;
import io.aplfintech.grape.grap3.crypto.Hasher;
import io.aplfintech.grape.grap3.crypto.Hashers;
import io.aplfintech.grape.grap3.crypto.KeyPair;
import io.aplfintech.grape.grap3.crypto.utils.KeyUtils;
import io.aplfintech.grape.utils.HexUtils;
import lombok.extern.slf4j.Slf4j;
import org.junit.jupiter.api.Test;

import java.util.HexFormat;

import static org.assertj.core.api.Assertions.assertThat;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

@Slf4j
class CryptoTest {

    private static final String SERIALIZED_PRIV_KEY_SEED = "8e38269f2cc110b31572e2d9e74aa466c770e82eec34c2f0037e0531822e1e4b";
    private static final String SERIALIZED_PUBLIC_KEY = "2bd4e8de88c1578aeb38f8c04f7af4d66c99cc0fe100d06641b4dd4ee0ae3220";
    private static final String MESSAGE_TO_SIGN = "52fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c64981855ad8681d0d86d1e91e00167939cb6694d2c422acd208a0072939487f6999eb9d18a44784045d87f3c67cf22746e995af5a25367951baa2ff6cd471c483f15fb90badb37c5821b6d95526a41a9504680b4e7c8b763a1b";
    private static final String MESSAGE_TO_HASH = "3a48c09448e6d63e8af03838e0eccb5c9e506bddf6b1706b8cb0ba84fe3fa1d23a48c09448e6d63e8af03838e0eccb5c9e506bddf6b1706b8cb0ba84fe3fa1d2";
    private static final String HASH_RESULT = "51075006e31a5f33696394ab289af7010c76ee8700e5a74202e9870ee3c8bfa3";

    private static final String SIGNATURE = "b92c458fd8127ac4b9038b1b072da2fad301e1d6265fb9c4fa0231ca5e69cc13a81c647fc5d189d75bb57ef668eb08cd04bfcfc5722682acf62bf3769aa46c0b";

    @Test
    void keccak256OfEmptyArray() {
        //GIVEN
        byte[] data = new byte[0];
        byte[] expected = HexUtils.fromHex("c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470");
        //WHEN
        var hash = Hashers.keccak256().digest(data);
        log.info("hash={}", HexUtils.toHex(hash, true));
        //THEN
        assertThat(hash)
            .isEqualTo(expected);
    }

    @Test
    void keccak256() {
        byte[] bytesToHash = HexUtils.parseHex("536f6d6520737472696e6720746f2068617368");
        var output = Hashers.keccak256().digest(bytesToHash);
        var expected = HexUtils.fromHex("1472341e8646578e6a7a933a157795450e747dbab233b3fca0be0dd6b606802d");

        assertThat(output)
            .isEqualTo(expected);
    }

    @Test
    void testCreate() {
        KeyPair keys = KeyUtils.getEd25519Generator().generateRandom();

        byte[] privKey = keys.privateKey();
        byte[] pubKey = keys.publicKey();

        assertNotNull(privKey);
        assertNotNull(pubKey);

        System.out.println("Generated private key: " + HexFormat.of().formatHex(privKey));
        System.out.println("Generated public key: " + HexFormat.of().formatHex(pubKey));
    }

    @Test
    void testSignVerify() {
        DSA dsa = Crypto.currentDSA();

        byte[] privKey = KeyUtils.fromHex(SERIALIZED_PRIV_KEY_SEED);
        byte[] pubKey = KeyUtils.fromHex(SERIALIZED_PUBLIC_KEY);
        byte[] message = HexFormat.of().parseHex(MESSAGE_TO_SIGN);

        byte[] signature = dsa.sign(privKey, message);
        String serializedSignature = HexFormat.of().formatHex(signature);
        System.out.println("Signature: " + serializedSignature);
        assertEquals(SIGNATURE, serializedSignature, "Signature mismatch after signing the same message with the same keys");


        boolean verified = dsa.verify(pubKey, signature, message);

        assertTrue(verified, "Message must pass validation of the signature");
    }

    @Test
    void sha256() {
        Hasher hasher = Crypto.newHasher();
        byte[] bytes = HexFormat.of().parseHex(MESSAGE_TO_HASH);
        hasher.update(bytes);
        assertEquals(HASH_RESULT,
            HexFormat.of().formatHex(hasher.digest()));
    }
}
