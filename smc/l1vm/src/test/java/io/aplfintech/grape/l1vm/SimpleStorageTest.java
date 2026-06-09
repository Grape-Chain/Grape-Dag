package io.aplfintech.grape.l1vm;

import io.aplfintech.grape.model.Address;
import io.aplfintech.grape.model.Hash;
import io.aplfintech.grape.model.Key;
import io.aplfintech.grape.utils.HexUtils;
import lombok.SneakyThrows;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

import static org.assertj.core.api.Assertions.assertThat;
import static org.junit.jupiter.api.Assertions.assertEquals;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
class SimpleStorageTest {
    Storage storage;

    @BeforeEach
    void setUp() {
        storage = new SimpleStorage();
    }

    @Test
    void putAndGet() {
        //WHEN
        storage.put(addresses.get(0), prKeys.get(0), prKeys.get(0));
        var rc = storage.get(addresses.get(0), prKeys.get(0));
        //THEN
        assertThat(rc)
            .isEqualTo(prKeys.get(0));
    }

    @Test
    void getMapping() {
        //GIVEN
        var mappings = fillStorage();
        //WHEN
        var rc = storage.getMapping(addresses.get(0));
        //THEN
        assertEquals(10, rc.size());
        assertThat(rc)
            .containsAllEntriesOf(mappings.get(0));
    }

    @SneakyThrows
    @Test
    void toJson() {
        //GIVEN
        storage.put(addresses.get(0), prKeys.get(0), prKeys.get(0));
        //WHEN
        var rc = storage.toJSON();
        //THEN
        assertThat(rc)
            .isEqualTo(storageJson);

    }

    private List<Map<Key, byte[]>> fillStorage() {
        List<Map<Key, byte[]>> maps = new ArrayList<>();
        for (int j = 0; j < 2; j++) {
            var addr = addresses.get(j);
            var map = new HashMap<Key, byte[]>();
            for (int i = j * 10; i < 10 + j * 10; i++) {
                storage.put(addr, prKeys.get(i), prKeys.get(i));
                map.put(new Hash(prKeys.get(i)), prKeys.get(i));
            }
            maps.add(map);
        }
        return maps;
    }

    static String storageJson = "{\"0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266\":{\"0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80\":\"0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80\"}}";
    static List<Address> addresses = List.of(
        VmAddress.from("0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266")
        , VmAddress.from("0x70997970C51812dc3A010C7d01b50e0d17dc79C8")
        , VmAddress.from("0x3C44CdDdB6a900fa2b585dd299e03d12FA4293BC")
        , VmAddress.from("0x90F79bf6EB2c4f870365E785982E1f101E93b906")
        , VmAddress.from("0x15d34AAf54267DB7D7c367839AAf71A00a2C6A65")
        , VmAddress.from("0x9965507D1a55bcC2695C58ba16FB37d819B0A4dc")
        , VmAddress.from("0x976EA74026E726554dB657fA54763abd0C3a0aa9")
        , VmAddress.from("0x14dC79964da2C08b23698B3D3cc7Ca32193d9955")
        , VmAddress.from("0x23618e81E3f5cdF7f54C3d65f7FBc0aBf5B21E8f")
        , VmAddress.from("0xa0Ee7A142d267C1f36714E4a8F75612F20a79720")
        , VmAddress.from("0xBcd4042DE499D14e55001CcbB24a551F3b954096")
        , VmAddress.from("0x71bE63f3384f5fb98995898A86B02Fb2426c5788")
        , VmAddress.from("0xFABB0ac9d68B0B445fB7357272Ff202C5651694a")
        , VmAddress.from("0x1CBd3b2770909D4e10f157cABC84C7264073C9Ec")
        , VmAddress.from("0xdF3e18d64BC6A983f673Ab319CCaE4f1a57C7097")
        , VmAddress.from("0xcd3B766CCDd6AE721141F452C550Ca635964ce71")
        , VmAddress.from("0x2546BcD3c84621e976D8185a91A922aE77ECEc30")
        , VmAddress.from("0xbDA5747bFD65F08deb54cb465eB87D40e51B197E")
        , VmAddress.from("0xdD2FD4581271e230360230F9337D5c0430Bf44C0")
        , VmAddress.from("0x8626f6940E2eb28930eFb4CeF49B2d1F2C9C1199")
    );
    static List<byte[]> prKeys = List.of(
        HexUtils.parseHex("0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80")
        , HexUtils.parseHex("0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d")
        , HexUtils.parseHex("0x5de4111afa1a4b94908f83103eb1f1706367c2e68ca870fc3fb9a804cdab365a")
        , HexUtils.parseHex("0x7c852118294e51e653712a81e05800f419141751be58f605c371e15141b007a6")
        , HexUtils.parseHex("0x47e179ec197488593b187f80a00eb0da91f1b9d0b13f8733639f19c30a34926a")
        , HexUtils.parseHex("0x8b3a350cf5c34c9194ca85829a2df0ec3153be0318b5e2d3348e872092edffba")
        , HexUtils.parseHex("0x92db14e403b83dfe3df233f83dfa3a0d7096f21ca9b0d6d6b8d88b2b4ec1564e")
        , HexUtils.parseHex("0x4bbbf85ce3377467afe5d46f804f221813b2bb87f24d81f60f1fcdbf7cbf4356")
        , HexUtils.parseHex("0xdbda1821b80551c9d65939329250298aa3472ba22feea921c0cf5d620ea67b97")
        , HexUtils.parseHex("0x2a871d0798f97d79848a013d4936a73bf4cc922c825d33c1cf7073dff6d409c6")
        , HexUtils.parseHex("0xf214f2b2cd398c806f84e317254e0f0b801d0643303237d97a22a48e01628897")
        , HexUtils.parseHex("0x701b615bbdfb9de65240bc28bd21bbc0d996645a3dd57e7b12bc2bdf6f192c82")
        , HexUtils.parseHex("0xa267530f49f8280200edf313ee7af6b827f2a8bce2897751d06a843f644967b1")
        , HexUtils.parseHex("0x47c99abed3324a2707c28affff1267e45918ec8c3f20b8aa892e8b065d2942dd")
        , HexUtils.parseHex("0xc526ee95bf44d8fc405a158bb884d9d1238d99f0612e9f33d006bb0789009aaa")
        , HexUtils.parseHex("0x8166f546bab6da521a8369cab06c5d2b9e46670292d85c875ee9ec20e84ffb61")
        , HexUtils.parseHex("0xea6c44ac03bff858b476bba40716402b03e41b8e97e276d1baec7c37d42484a0")
        , HexUtils.parseHex("0x689af8efa8c651a91ad287602527f3af2fe9f6501a7ac4b061667b5a93e037fd")
        , HexUtils.parseHex("0xde9be858da4a475276426320d5e9262ecfc3ba460bfac56360bfa6c4c28b4ee0")
        , HexUtils.parseHex("0xdf57089febbacf7ba0bc227dafbffa9fc08a93fdc68e1e42411a14efcf23656e")
    );

}