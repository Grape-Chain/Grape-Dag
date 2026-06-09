package io.aplfintech.grape.l1vm;

import io.aplfintech.grape.exception.VmException;
import io.aplfintech.grape.math.Math256;
import io.aplfintech.grape.vm.VmStatus;
import io.aplfintech.grape.vm.stack.WordStack;
import org.junit.jupiter.api.Test;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
class VmStackTest {

    @Test
    void checkNewStack() {
        //WHEN
        var ex = assertThrows(IllegalArgumentException.class, () -> new WordStack(0));
        //THEN
        assertThat(ex)
            .hasMessage("Incorrect stack size");

        //WHEN
        var ex2 = assertThrows(IndexOutOfBoundsException.class, () -> new VmStack(2048));
        //THEN
        assertThat(ex2)
            .hasMessage("Initial stack size (2048) must be less than size (1025)");

        //WHEN
        ex2 = assertThrows(IndexOutOfBoundsException.class, () -> new VmStack(-1));
        //THEN
        assertThat(ex2)
            .hasMessage("Initial stack size (-1) must not be negative");
    }

    @Test
    void checkPreconditions() {
        //GIVEN
        var stack = new VmStack();
        //WHEN
        var ex = assertThrows(IllegalArgumentException.class, () -> stack.peek(0));
        //THEN
        assertThat(ex)
            .hasMessage("num must be greater then 0");

        //WHEN
        ex = assertThrows(IllegalArgumentException.class, () -> stack.swap(-1));
        //THEN
        assertThat(ex)
            .hasMessage("num must be positive");

        //WHEN
        ex = assertThrows(IllegalArgumentException.class, () -> stack.dup(0));
        //THEN
        assertThat(ex)
            .hasMessage("num must be greater then 0");

        //WHEN
        var value = Math256.uint256(Math256.TWO_POW256.toByteArray());
        var ex2 = assertThrows(VmException.class, () -> stack.push(value));
        //THEN
        assertThat(ex2)
            .hasMessage(VmStatus.VM_ARGUMENT_OUT_OF_RANGE.fullName());

    }

    @Test
    void checkPushPop() {
        //GIVEN
        var stack = new VmStack();
        //WHEN
        stack.push(Math256.UINT_256_TEN);
        //THEN
        var rc = stack.pop();
        assertEquals(Math256.UINT_256_TEN, rc);
    }

    @Test
    void checkUnderflow() {
        //GIVEN
        var stack = new VmStack();
        //WHEN
        var ex = assertThrows(VmException.class, stack::pop);
        //THEN
        assertThat(ex)
            .hasMessage(VmStatus.VM_STACK_UNDERFLOW.fullName());

        //GIVEN
        stack.push(Math256.UINT_256_TEN);

        //WHEN
        ex = assertThrows(VmException.class, () -> stack.peek(2));
        //THEN
        assertThat(ex)
            .hasMessage(VmStatus.VM_STACK_UNDERFLOW.fullName());
        assertEquals(1, stack.size(), "Stack size must be 1");

        //WHEN
        ex = assertThrows(VmException.class, () -> stack.swap(1));
        //THEN
        assertThat(ex)
            .hasMessage(VmStatus.VM_STACK_UNDERFLOW.fullName());
        assertEquals(1, stack.size(), "Stack size must be 1");

        //WHEN
        ex = assertThrows(VmException.class, () -> stack.dup((2)));//dup uses the 1-indexed position
        //THEN
        assertThat(ex)
            .hasMessage(VmStatus.VM_STACK_UNDERFLOW.fullName());
        assertEquals(1, stack.size(), "Stack size must be 1");

    }

    @Test
    void checkOverflow() {
        //GIVEN
        var stack = new VmStack();
        for (int i = 0; i < 1024; i++) {
            stack.push(Math256.UINT_256_TEN);
        }
        //WHEN
        var ex = assertThrows(VmException.class, () -> stack.push(Math256.UINT_256_ONE));
        //THEN
        assertThat(ex)
            .hasMessage(VmStatus.VM_STACK_OVERFLOW.fullName());
    }

    @Test
    void checkPeekN() {
        //GIVEN
        var expected = List.of(Math256.UINT_256_ZERO, Math256.UINT_256_ONE, Math256.UINT_256_TWO, Math256.UINT_256_TEN);
        var stack = new VmStack();
        stack.push(Math256.UINT_256_TWO);
        for (int i = expected.size() - 1; i >= 0; i--) {//reverse ordering
            stack.push(expected.get(i));
        }
        //WHEN
        var rez = stack.peek(4);
        //THEN
        assertThat(rez).isEqualTo(expected);
        assertEquals(5, stack.size(), "Stack must be unchanged");
    }

    @Test
    void checkSwap() {
        //GIVEN
        var initial = List.of(Math256.UINT_256_ZERO, Math256.UINT_256_ONE, Math256.UINT_256_TWO, Math256.UINT_256_TEN);
        var expected = List.of(Math256.UINT_256_TWO, Math256.UINT_256_ONE, Math256.UINT_256_ZERO, Math256.UINT_256_TEN);
        var stack = new VmStack();
        stack.push(Math256.UINT_256_TWO);
        for (int i = initial.size() - 1; i >= 0; i--) {//reverse ordering
            stack.push(initial.get(i));
        }
        //WHEN
        stack.swap(2);
        var rez = stack.peek(4);
        //THEN
        assertThat(rez).isEqualTo(expected);
    }

    @Test
    void checkDup() {
        //GIVEN
        var initial = List.of(Math256.UINT_256_ZERO, Math256.UINT_256_ONE, Math256.UINT_256_TWO, Math256.UINT_256_TEN);
        var expected = List.of(Math256.UINT_256_TWO, Math256.UINT_256_ZERO, Math256.UINT_256_ONE, Math256.UINT_256_TWO, Math256.UINT_256_TEN);
        var stack = new VmStack();
        stack.push(Math256.UINT_256_TWO);
        for (int i = initial.size() - 1; i >= 0; i--) {//reverse ordering
            stack.push(initial.get(i));
        }
        //WHEN
        stack.dup(3);
        var rez = stack.peek(5);
        //THEN
        assertThat(rez).isEqualTo(expected);
        assertEquals(6, stack.size(), "dup must increase the stack size");
    }
}