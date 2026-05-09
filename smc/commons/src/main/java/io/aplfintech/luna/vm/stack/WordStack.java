package io.aplfintech.luna.vm.stack;

import io.aplfintech.luna.math.Word256;

/**
 * The general behavior of the VM Stack with 256-bits word as a stack item
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
public class WordStack extends CappedStack<Word256> {
    public WordStack(int maxHeight) {
        super(maxHeight);
    }

    @Override
    public String toString() {
        var buf = new StringBuilder();
        buf.append("Stack[").append(elements.size()).append(':').append(getMaxHeight()).append("]{");
        if (!elements.isEmpty()) {
            boolean addDelimiter = false;
            for (int idx = elements.size() - 1; idx >= 0; idx--) {
                if (addDelimiter) {
                    buf.append(',');
                }
                buf.append(elements.get(idx).hex());
                addDelimiter = true;
            }
        }
        buf.append('}');
        return buf.toString();
    }
}
