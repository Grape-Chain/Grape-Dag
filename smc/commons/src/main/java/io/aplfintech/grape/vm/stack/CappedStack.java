package io.aplfintech.grape.vm.stack;

import com.google.common.base.Preconditions;
import io.aplfintech.grape.exception.StackOverflowException;
import io.aplfintech.grape.exception.StackUnderflowException;
import lombok.Getter;

import java.util.ArrayList;
import java.util.List;

/**
 * The capped Stack for VM
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public class CappedStack<T> implements Stack<T> {
    public static final String NUM_MUST_BE_GREATER_THEN_0 = "num must be greater then 0";
    final ArrayList<T> elements;
    @Getter
    private final int maxHeight;

    public CappedStack(int maxHeight) {
        Preconditions.checkArgument(maxHeight > 0, "Incorrect stack size");
        this.maxHeight = maxHeight;
        this.elements = new ArrayList<>(maxHeight);
    }

    @Override
    public int size() {
        return this.elements.size();
    }

    @Override
    public void push(T value) {
        if (elements.size() >= this.maxHeight) {
            throw new StackOverflowException();
        }
        elements.add(value);
    }

    @Override
    public T pop() {
        if (elements.isEmpty()) {
            throw new StackUnderflowException();
        }
        return elements.remove(elements.size() - 1);
    }

    @Override
    public List<T> peek(int num) {
        Preconditions.checkArgument(num > 0, NUM_MUST_BE_GREATER_THEN_0);
        checkUnderflow(elements.size() < num);
        var result = new ArrayList<T>();
        var start = elements.size() - 1;
        int limit = start - num;
        for (int i = start; i > limit; i--) {
            result.add(elements.get(i));
        }
        return result;
    }

    @Override
    public void swap(int position) {
        Preconditions.checkArgument(position >= 0, "num must be positive");
        checkUnderflow(elements.size() <= position);
        var head = elements.size() - 1;
        var i = elements.size() - position - 1;

        var tmp = elements.get(head);
        elements.set(head, elements.get(i));
        elements.set(i, tmp);
    }

    @Override
    public void dup(int position) {
        Preconditions.checkArgument(position > 0, NUM_MUST_BE_GREATER_THEN_0);
        checkUnderflow(elements.size() < position);
        var i = this.elements.size() - position;
        this.push(this.elements.get(i));
    }

    protected void checkUnderflow(boolean predicate) {
        if (predicate) {
            throw new StackUnderflowException();
        }
    }

}
