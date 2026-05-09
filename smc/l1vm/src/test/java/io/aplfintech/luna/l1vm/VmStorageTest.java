package io.aplfintech.luna.l1vm;

import io.aplfintech.luna.model.Address;
import io.aplfintech.luna.utils.Bytes;
import lombok.SneakyThrows;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;
import static org.junit.jupiter.api.Assertions.assertThrows;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
class VmStorageTest {
    Storage storage;
    Address address;

    @BeforeEach
    void setUp() {
        storage = new VmStorage(new SimpleStorage());
        address = VmAddress.from("0xff00000000000000000000000000000000000002");
    }

    @Test
    void checkPreconditions() {
        //GIVEN
        var key = Bytes.alloc(10);
        var data = Bytes.alloc(1);
        var key2 = Bytes.alloc(32);
        var data2 = Bytes.alloc(33);
        //check NPE
        //WHEN
        var ex0 = assertThrows(NullPointerException.class, () -> storage.get(null, key));
        //THEN
        assertThat(ex0)
            .hasMessage("address");

        //WHEN
        ex0 = assertThrows(NullPointerException.class, () -> storage.get(address, null));
        //THEN
        assertThat(ex0)
            .hasMessage("key");

        //WHEN
        ex0 = assertThrows(NullPointerException.class, () -> storage.containsMapping(null));
        //THEN
        assertThat(ex0)
            .hasMessage("address");

        //WHEN
        ex0 = assertThrows(NullPointerException.class, () -> storage.put(null, key, data));
        //THEN
        assertThat(ex0)
            .hasMessage("address");

        //WHEN
        ex0 = assertThrows(NullPointerException.class, () -> storage.put(address, null, data));
        //THEN
        assertThat(ex0)
            .hasMessage("key");

        //check key and data size
        //WHEN
        var ex = assertThrows(IllegalArgumentException.class, () -> storage.put(address, key, data));
        //THEN
        assertThat(ex)
            .hasMessage("Storage key must be 32 bytes long");

        //WHEN
        ex = assertThrows(IllegalArgumentException.class, () -> storage.put(address, key2, data2));
        //THEN
        assertThat(ex)
            .hasMessage("Storage value cannot be longer than 32 bytes");
    }

    @Test
    void checkContainsMapping() {
        //GIVEN
        //WHEN
        var rc = storage.containsMapping(address);
        //THEN
        assertThat(rc)
            .as("The mapping for the given address must be absent")
            .isFalse();

        //GIVEN
        var key = Bytes.alloc(32, 0xff);
        var value = Bytes.alloc(32, 0x99);
        var got = storage.put(address, key, value);
        //WHEN
        rc = storage.containsMapping(address);
        //THEN
        assertThat(rc)
            .as("The mapping for the given address must be existent")
            .isTrue();
    }

    @Test
    void checkSetAndGet() {
        //GIVEN
        var key = Bytes.alloc(32, 0xff);
        var value = Bytes.alloc(32, 0x99);

        //WHEN
        storage.put(address, key, value);
        var got = storage.get(address, key);
        //THEN
        assertThat(got)
            .isEqualTo(value);
    }

    @Test
    void checkZeroFilling() {
        //GIVEN
        var key = Bytes.alloc(32, 0xff);
        var value = Bytes.alloc(32, 0x11);

        //WHEN
        // No address set
        var rc = storage.get(address, key);
        //THEN
        assertThat(rc)
            .isEqualTo(Bytes.alloc(32, 0x00));

        //WHEN
        // Address set, no key set
        //check prev value
        rc = storage.put(address, key, value);
        //THEN
        assertThat(rc)
            .isEqualTo(Bytes.alloc(32, 0x00));
        //WHEN
        rc = storage.get(address, Bytes.alloc(32, 0x22));
        //THEN
        assertThat(rc)
            .isEqualTo(Bytes.alloc(32, 0x00));
    }

    @Test
    void commit() {
        //GIVEN
        var key = Bytes.alloc(32, 0xff);
        var value = Bytes.alloc(32, 0x99);

        storage.put(address, key, value);
        storage.checkpoint();
        //WHEN
        storage.commit();
        var got = storage.get(address, key);
        //THEN
        assertThat(got)
            .isEqualTo(value);
    }

    @Test
    void commitBatches() {
        //GIVEN
        var key = Bytes.alloc(32, 0xff);
        var value1 = Bytes.alloc(32, 0x01);
        var value2 = Bytes.alloc(32, 0x02);
        var value3 = Bytes.alloc(32, 0x03);

        storage.put(address, key, value1);
        storage.checkpoint();
        storage.put(address, key, value2);
        storage.put(address, key, value3);
        storage.checkpoint();
        storage.put(address, key, value2);
        storage.checkpoint();

        //WHEN
        // clears empty checkpoint
        storage.commit();
        // now revert should go all the way to 1
        storage.commit();
        //THEN
        var got = storage.get(address, key);
        assertThat(got)
            .isEqualTo(value2);

        //WHEN
        storage.revert();
        got = storage.get(address, key);
        //THEN
        assertThat(got)
            .isEqualTo(value1);
    }

    @Test
    void revert() {
        //GIVEN
        var key = Bytes.alloc(32, 0xff);
        var value = Bytes.alloc(32, 0x99);

        storage.put(address, key, value);

        storage.checkpoint();

        var value2 = Bytes.alloc(32, 0x22);
        storage.put(address, key, value2);
        var got = storage.get(address, key);
        assertThat(got)
            .isEqualTo(value2);
        //WHEN
        storage.revert();
        got = storage.get(address, key);
        //THEN
        assertThat(got)
            .isEqualTo(value);
    }

    @Test
    void checkRevertOrdering() {
        //GIVEN
        var key = Bytes.alloc(32, 0xff);
        var value1 = Bytes.alloc(32, 0x01);
        var value2 = Bytes.alloc(32, 0x02);
        var value3 = Bytes.alloc(32, 0x03);

        storage.put(address, key, value1);
        storage.checkpoint();
        storage.put(address, key, value2);
        storage.put(address, key, value3);
        //WHEN
        storage.revert();
        //THEN
        //revert applies changes in correct order
        var got = storage.get(address, key);
        //THEN
        assertThat(got)
            .isEqualTo(value1);
    }

    @Test
    void checkNestedRevert() {
        //GIVEN
        var key = Bytes.alloc(32, 0xff);
        var value0 = Bytes.alloc(32, 0x00);
        var value1 = Bytes.alloc(32, 0x01);
        var value2 = Bytes.alloc(32, 0x02);
        var value3 = Bytes.alloc(32, 0x03);

        storage.put(address, key, value1);
        storage.checkpoint();
        storage.put(address, key, value2);
        storage.put(address, key, value3);
        storage.checkpoint();
        storage.put(address, key, value2);
        storage.checkpoint();
        //WHEN
        var got = storage.get(address, key);
        //THEN
        assertThat(got)
            .isEqualTo(value2);

        //WHEN
        storage.revert();
        got = storage.get(address, key);
        //THEN
        assertThat(got)
            .isEqualTo(value2);
        // not changed since nothing happened after latest checkpoint
        //WHEN
        storage.revert();
        got = storage.get(address, key);
        //THEN
        assertThat(got)
            .isEqualTo(value3);

        //WHEN
        storage.revert();
        got = storage.get(address, key);
        //THEN
        assertThat(got)
            .isEqualTo(value1);

        //WHEN
        storage.revert();
        got = storage.get(address, key);
        //THEN
        assertThat(got)
            .isEqualTo(value0);
    }
    @Test
    void checkRevertCommitNestedTransaction() {
        //GIVEN
        var key = Bytes.alloc(32, 0xff);
        var key2 = Bytes.alloc(32, 0Xfa);
        var value0 = Bytes.alloc(32, 0x00);
        var value1 = Bytes.alloc(32, 0x01);
        var value2 = Bytes.alloc(32, 0x02);
        var value3 = Bytes.alloc(32, 0x03);
        var value4 = Bytes.alloc(32, 0x04);
        storage.put(address, key, value1);
        storage.checkpoint();
        storage.put(address, key, value2);
        storage.put(address, key, value3);
        storage.checkpoint();
        storage.put(address, key2, value0);
        storage.commit();
        storage.put(address, key2, value4);
        storage.checkpoint();
        //WHEN
        var got = storage.get(address, key);
        var got2 = storage.get(address, key2);
        //THEN
        assertThat(got)
            .isEqualTo(value3);
        assertThat(got2)
            .isEqualTo(value4);


        //WHEN
        storage.revert();
        got = storage.get(address, key);
        got2 = storage.get(address, key2);
        //THEN
        assertThat(got)
            .isEqualTo(value3);
        assertThat(got2)
            .isEqualTo(value4);

        //WHEN
        storage.revert();
        got = storage.get(address, key);
        got2 = storage.get(address, key2);
        //THEN
        assertThat(got)
            .isEqualTo(value1);
        assertThat(got2)
            .isEqualTo(value0);

        //WHEN
        storage.revert();
        got = storage.get(address, key);
        got2 = storage.get(address, key2);
        //THEN
        assertThat(got)
            .isEqualTo(value0);
        assertThat(got2)
            .isEqualTo(value0);

    }

    @Test
    void clear() {
        //GIVEN
        var key = Bytes.alloc(32, 0xff);
        var value = Bytes.alloc(32, 0x99);

        storage.put(address, key, value);
        var got = storage.get(address, key);
        assertThat(got)
            .isEqualTo(value);
        //WHEN
        storage.clear();
        got = storage.get(address, key);
        //THEN
        assertThat(got)
            .isEqualTo(Bytes.alloc(32, 0x00));
    }

    @SneakyThrows
    @Test
    void toJSON() {
        //WHEN
        var rc = storage.toJSON();
        //THEN
        assertThat(rc)
            .isEqualTo("{}");
    }
}