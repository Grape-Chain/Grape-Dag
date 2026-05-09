package io.aplfintech.luna.grap3.wallet;

import io.aplfintech.luna.grap3.crypto.wallet.Addresses;
import io.aplfintech.luna.utils.Bytes;
import io.aplfintech.luna.utils.HexUtils;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.CsvSource;
import org.junit.jupiter.params.provider.ValueSource;

import java.util.HexFormat;

import static org.assertj.core.api.Assertions.assertThat;

/**
 * @author andrew.zinchenko@gmail.com
 */
class AddressesTest {
    byte[] caller = HexUtils.fromHex("0102030405060708090a0b0c0d0e0f0000000020");
    byte[] contract = HexUtils.fromHex("608060405234801561001057600080fd5b50610150806100206000396000f3fe608060405234801561001057600080fd5b50600436106100365760003560e01c80632e64cec11461003b5780636057361d14610059575b600080fd5b610043610075565b60405161005091906100d9565b60405180910390f35b610073600480360381019061006e919061009d565b61007e565b005b60008054905090565b8060008190555050565b60008135905061009781610103565b92915050565b6000602082840312156100b3576100b26100fe565b5b60006100c184828501610088565b91505092915050565b6100d3816100f4565b82525050565b60006020820190506100ee60008301846100ca565b92915050565b6000819050919050565b600080fd5b61010c816100f4565b811461011757600080fd5b5056fea26469706673582212209a159a4f3847890f10bfb87871a61eba91c5dbf5ee3cf6398207e292eee22a1664736f6c63430008070033");

    @ValueSource(longs = {0, 1, 2, 3, 4, 5})
    @ParameterizedTest
    void create(long nonce) {
        //GIVEN
        //WHEN
        var rc = Addresses.createAddress(caller, nonce);
        //THEN
        assertThat(rc)
            .isNotEqualTo(caller)
            .hasSize(20);
    }

    @Test
    void create() {
        byte[] address = Addresses.createAddress(HexFormat.of().parseHex("75704b2f3334eea055803fb410995a87d610c5ee"), 0);

        assertThat(address)
            .isEqualTo(HexFormat.of().parseHex("2e57dd414fB6d16B69A8d1D6Cf844a676CD22051"));
    }

    @ValueSource(longs = {0, 1, 2, 3, 4, 5})
    @ParameterizedTest
    void createEq(long nonce) {
        //GIVEN
        //WHEN
        var rc1 = Addresses.createAddress(caller, nonce);
        var rc2 = Addresses.createAddress(caller, nonce);
        //THEN
        assertThat(rc1)
            .isEqualTo(rc2)
            .hasSize(20);
    }

    @ValueSource(longs = {0, 1, 2, 3, 4, 5})
    @ParameterizedTest
    void create2(long salt) {
        //GIVEN
        byte[] saltBytes = Bytes.toBytes(salt);
        //WHEN
        var rc = Addresses.createAddress2(caller, saltBytes, contract);
        //THEN
        assertThat(rc)
            .isNotEqualTo(caller)
            .hasSize(20);
    }

    @ValueSource(longs = {0, 1, 2, 3, 4, 5})
    @ParameterizedTest
    void create2Eq(long salt) {
        //GIVEN
        byte[] saltBytes = Bytes.toBytes(salt);
        //WHEN
        var rc1 = Addresses.createAddress2(caller, saltBytes, contract);
        var rc2 = Addresses.createAddress2(caller, saltBytes, contract);
        //THEN
        assertThat(rc1)
            .isEqualTo(rc2)
            .hasSize(20);
    }

    @CsvSource({
        "0x0000000000000000000000000000000000000000, 0x0000000000000000000000000000000000000000, 0x00, 0x4d1a2e2bb4f88f0250f26ffff098b0b30b26bf38",
        "0xdeadbeef00000000000000000000000000000000, 0x0000000000000000000000000000000000000000, 0x00, 0xB928f69Bb1D91Cd65274e3c79d8986362984fDA3",
        "0xdeadbeef00000000000000000000000000000000, 0xfeed000000000000000000000000000000000000, 0x00, 0xD04116cDd17beBE565EB2422F2497E06cC1C9833",
        "0x0000000000000000000000000000000000000000, 0x0000000000000000000000000000000000000000, 0xdeadbeef, 0x70f2b2914A2a4b783FaEFb75f459A580616Fcb5e",
        "0x00000000000000000000000000000000deadbeef, 0xcafebabe, 0xdeadbeef, 0x60f3f640a8508fC6a86d45DF051962668E1e8AC7",
        "0x00000000000000000000000000000000deadbeef, 0xcafebabe, 0xdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef, 0x1d8bfDC5D46DC4f61D6b6115972536eBE6A8854C",
        "0x0000000000000000000000000000000000000000, 0x0000000000000000000000000000000000000000, 0x, 0xE33C0C7F7df4809055C3ebA6c09CFe4BaF1BD9e0"
    })
    @ParameterizedTest
    void createAddress2(String originArg, String saltArg, String codeArg, String expectedArg) {
        //GIVEN
        byte[] origin = HexUtils.parseHex(originArg);
        byte[] salt = HexUtils.parseHex(saltArg);
        byte[] code = HexUtils.parseHex(codeArg);
        byte[] expected = HexUtils.parseHex(expectedArg);

        //WHEN
        var rc = Addresses.createAddress2(origin, salt, code);
        //THEN
        assertThat(rc)
            .isEqualTo(expected)
            .hasSize(20);
    }

    @CsvSource({
        "eeadfb7b03a08cf7c4e305c1351188ee70a49fe2fc0919775759706f15edd1304366f498065854dccfec9cf0ed41a3eec1c46a71579b10e5f3f0a1d574d35587, 0x6484f4f4c4ebaf71300ced4ce1b66f8b6437f42b",
    })
    @ParameterizedTest
    void createAddress(String pubKey, String expectedArg) {
        //GIVEN
        byte[] publicKey = HexUtils.parseHex(pubKey);
        byte[] expected = HexUtils.parseHex(expectedArg);

        //WHEN
        var rc = Addresses.createAddress(publicKey);
        //THEN
        assertThat(rc)
            .isEqualTo(expected)
            .hasSize(20);
    }

}