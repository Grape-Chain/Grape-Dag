package io.aplfintech.grape.vm.stack;

import io.aplfintech.grape.exception.StackOverflowException;
import io.aplfintech.grape.exception.StackUnderflowException;

import java.util.List;

/**
 * The general behavior of the Stack for VM
 *
 * @param <T> the type of stack item
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public interface Stack<T> {
    /**
     * Returns the number of elements in this stack
     *
     * @return the number of elements in this stack
     */
    int size();

    /**
     * Pushes an element onto the stack
     *
     * @param value the element to push
     * @throws StackOverflowException if stack.size() >= maxHeight
     */
    void push(T value) throws StackOverflowException;

    /**
     * Pops an element from the stack
     *
     * @return an element from the stack
     * @throws StackUnderflowException if stack is empty
     */
    T pop() throws StackUnderflowException;

    /**
     * Retrieves, but does not remove items from the stack. Top of stack is first item in the returned list
     *
     * @param num Number of elements to return
     * @throws StackUnderflowException if stack.size() < num
     */
    List<T> peek(int num) throws StackUnderflowException;

    /**
     * Swap top of stack with an element in the stack.
     *
     * @param position - Index of element from top of the stack (0-indexed)
     * @throws StackUnderflowException if stack.size() <= position
     */
    void swap(int position) throws StackUnderflowException;

    /**
     * Pushes a copy of an element in the stack.
     *
     * @param position - Index of element to be copied (1-indexed)
     * @throws StackUnderflowException if stack.size() <= position
     */
    void dup(int position) throws StackUnderflowException;
}
