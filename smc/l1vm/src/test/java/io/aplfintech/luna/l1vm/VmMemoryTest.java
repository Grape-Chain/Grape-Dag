package io.aplfintech.luna.l1vm;

import io.aplfintech.luna.exception.VmException;
import io.aplfintech.luna.vm.Memory;
import io.aplfintech.luna.vm.VmStatus;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import static io.aplfintech.luna.math.Math256.WORD_SIZE;
import static org.assertj.core.api.Assertions.assertThat;
import static org.junit.jupiter.api.Assertions.assertArrayEquals;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
class VmMemoryTest {
    Memory mem;

    @BeforeEach
    void setUp() {
        mem = new VmMemory();
    }

    @Test
    void checkInitialCapacity() {
        //GIVEN
        //WHEN
        var rc = mem.size();
        //THEN
        assertEquals(0, mem.size(), "must have 0 capacity initially");
    }

    @Test
    void checkRead() {
        //GIVEN
        //WHEN
        var rez = mem.read(0, 3);
        //THEN
        assertArrayEquals(new byte[]{0, 0, 0}, rez);
        assertEquals(WORD_SIZE, mem.size(), "must extend capacity to word boundary");

        //WHEN
        rez = mem.read(10, 3);
        //THEN
        assertArrayEquals(new byte[]{0, 0, 0}, rez, "must return zeros before writing");
    }

    @Test
    void checkExpand() {
        //GIVEN
        //WHEN
        mem.expand(0, 3);
        //THEN
        assertEquals(WORD_SIZE, mem.size(), "must extend capacity to word boundary");
    }

    @Test
    void checkWrite() {
        //GIVEN
        //WHEN
        mem.write(29, 3, new byte[]{1, 2, 3});
        var rez = mem.read(29, 9);
        //THEN
        assertArrayEquals(new byte[]{1, 2, 3, 0, 0, 0, 0, 0, 0}, rez);
        assertEquals(WORD_SIZE * 2, mem.size(), "must extend capacity");

        //WHEN
        //should fail when value len and size are inconsistent
        var ex = assertThrows(VmException.class, () -> mem.write(0, 5, new byte[]{1, 2, 3}));
        assertThat(ex)
            .hasMessage(VmStatus.VM_ARGUMENT_OUT_OF_RANGE.fullName());

    }

    @Test
    void writeInUntouchedMem() {
        //GIVEN
        //WHEN
        mem.write(Integer.MAX_VALUE, 0, new byte[]{1});
        //THEN
        assertEquals(0, mem.size(), "doesn't extend capacity, because size==0");
        //WHEN
        mem.write(0, 1, new byte[]{1});
        //THEN
        assertEquals(WORD_SIZE, mem.size(), "must extend capacity to word boundary");
        //WHEN
        mem.write(1, 3, new byte[]{2, 2, 2});
        //THEN
        assertEquals(WORD_SIZE, mem.size(), "must extend capacity to word boundary");
        //WHEN
        mem.write(4, 32, new byte[32]);
        //THEN
        assertEquals(2 * WORD_SIZE, mem.size(), "must extend capacity to word boundary");
        //WHEN
        mem.write(36, 32, new byte[32]);
        //THEN
        assertEquals(3 * WORD_SIZE, mem.size(), "must extend capacity to word boundary");
    }

    @Test
    void readFromUntouchedMem() {
        //GIVEN
        //WHEN
        var rc = mem.read(Integer.MAX_VALUE, 0);
        //THEN
        assertArrayEquals(new byte[0], rc, "doesn't extend capacity, because size==0");
        //WHEN
        mem.read(0, 8);
        //THEN
        assertEquals(WORD_SIZE, mem.size(), "must extend capacity to word boundary");
        //WHEN
        mem.read(1, 16);
        //THEN
        assertEquals(WORD_SIZE, mem.size(), "must extend capacity to word boundary");
        //WHEN
        mem.read(1, 32);
        //THEN
        assertEquals(2 * WORD_SIZE, mem.size(), "must extend capacity to word boundary");
        //WHEN
        mem.read(32, 32);
        //THEN
        assertEquals(2 * WORD_SIZE, mem.size(), "must extend capacity to word boundary");
        //WHEN
        mem.read(33, 32);
        //THEN
        assertEquals(3 * WORD_SIZE, mem.size(), "must extend capacity to word boundary");
    }

}