package io.aplfintech.luna.lunach.io;

import com.google.common.base.Preconditions;
import io.aplfintech.luna.tx.Result;
import lombok.Getter;
import lombok.extern.slf4j.Slf4j;

import java.nio.ByteBuffer;
import java.util.Objects;

/**
 * The serialization result as a {@link WriteBuffer}
 *
 * @author andrew.zinchenko@gmail.com
 * @see WriteBuffer
 * @since 0.1
 */
@Slf4j
public class PayloadResult implements Result {
    @Getter
    private final WriteBuffer buffer;

    public PayloadResult(WriteBuffer buffer) {
        Objects.requireNonNull(buffer);
        this.buffer = buffer;
    }

    public static PayloadResult createByteArrayResult() {
        return new PayloadResult(new ByteArrayStream());
    }

    public static PayloadResult createByteBufferResult(int capacity) {
        Preconditions.checkElementIndex(capacity, Integer.MAX_VALUE, "Wrong buffer's capacity");
        return new PayloadResult(new WriteByteBuffer(ByteBuffer.allocate(capacity)));
    }

    public static PayloadResult createByteBufferResult(byte[] buffer) {
        Preconditions.checkNotNull(buffer, "source buffer is null");
        return new PayloadResult(new WriteByteBuffer(ByteBuffer.wrap(buffer)));
    }

    @Override
    public byte[] array() {
        return buffer.toByteArray();
    }

}
