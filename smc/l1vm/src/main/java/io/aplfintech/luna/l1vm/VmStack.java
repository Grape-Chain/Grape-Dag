package io.aplfintech.luna.l1vm;

import com.google.common.base.Preconditions;
import io.aplfintech.luna.exception.StackOverflowException;
import io.aplfintech.luna.exception.StackUnderflowException;
import io.aplfintech.luna.exception.VmException;
import io.aplfintech.luna.math.Math256;
import io.aplfintech.luna.math.Word256;
import io.aplfintech.luna.utils.Exceptions;
import io.aplfintech.luna.vm.VmStatus;
import io.aplfintech.luna.vm.stack.WordStack;

import java.util.List;
import java.util.function.Consumer;
import java.util.function.Supplier;

/**
 * The implementation of the Stack for VM
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public class VmStack extends WordStack {

    public VmStack() {
        this(1024);
    }

    public VmStack(int maxHeight) {
        super(Preconditions.checkElementIndex(maxHeight, 1025, "Initial stack size"));
    }

    /**
     * Pushes an element onto the stack
     *
     * @param value the element to push
     * @throws VmException with message {@link VmStatus#VM_ARGUMENT_OUT_OF_RANGE} if element value  exceed the MAX_INTEGER_BIGINT
     */
    @Override
    public void push(Word256 value) {
        if (value.bytes().length > Math256.WORD_SIZE) {
            throw Exceptions.from(VmStatus.VM_ARGUMENT_OUT_OF_RANGE);
        }
        trapVoid(unused -> super.push(value));
    }

    @Override
    public Word256 pop() {
        return trap(super::pop);
    }

    @Override
    public List<Word256> peek(int num) {
        return trap(() -> super.peek(num));
    }

    @Override
    public void swap(int position) {
        trapVoid(unused -> super.swap(position));
    }

    @Override
    public void dup(int position) {
        trapVoid(unused -> super.dup(position));
    }

    private <T> T trap(Supplier<T> supplier) {
        try {
            return supplier.get();
        } catch (StackUnderflowException e) {
            throw Exceptions.from(VmStatus.VM_STACK_UNDERFLOW);
        }
    }

    private void trapVoid(Consumer<Void> consumer) {
        try {
            consumer.accept(null);
        } catch (StackUnderflowException e) {
            throw Exceptions.from(VmStatus.VM_STACK_UNDERFLOW);
        } catch (StackOverflowException e) {
            throw Exceptions.from(VmStatus.VM_STACK_OVERFLOW);
        }
    }

}
