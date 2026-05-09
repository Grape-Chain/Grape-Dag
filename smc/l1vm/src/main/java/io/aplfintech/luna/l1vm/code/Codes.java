package io.aplfintech.luna.l1vm.code;

import com.google.common.base.Preconditions;
import io.aplfintech.luna.vm.contract.Code;
import io.aplfintech.luna.utils.Bytes;
import io.aplfintech.luna.utils.HexUtils;
import lombok.AccessLevel;
import lombok.NoArgsConstructor;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@NoArgsConstructor(access = AccessLevel.PRIVATE)
public class Codes {

    /**
     * Returns the code instance using reader of the legacy code format
     *
     * @param code input byte code
     * @return the code instance using reader of the legacy code format
     */
    public static Code from(byte[] code) {
        return new ByteCode(code);
    }

    /**
     * Returns the code instance using reader of the new EOF code format
     *
     * @param code input byte code
     * @return the code instance using reader of the new EOF code format
     */
    public static Code from2(byte[] code) {
        CodeReader reader = EofCodeReader.from(code);
        return new ByteCodeReader(reader);
    }

    private static final class ByteCode implements Code {
        private final byte[] code;

        private ByteCode(byte[] code) {
            this.code = code;
        }

        @Override
        public byte[] bytes() {
            return code;
        }

        @Override
        public long size() {
            return code.length;
        }

        @Override
        public byte getOpCode(long n) {
            return get(n);
        }

        @Override
        public byte get(long n) {
            Preconditions.checkElementIndex((int) n, code.length);
            return code[(int) n];
        }

        @Override
        public byte[] slice(long start, long end) {
            return Bytes.slice(code, Math.toIntExact(start), Math.toIntExact(end));
        }

        @Override
        public String toString() {
            return "ByteCode[" +
                "code=" + HexUtils.toHex(code) + ']';
        }

    }

    private static class ByteCodeReader implements Code {
        private final CodeReader reader;

        public ByteCodeReader(CodeReader reader) {
            this.reader = reader;
        }

        @Override
        public byte[] bytes() {
            return reader.bytes();
        }

        @Override
        public long size() {
            return reader.codeSize();
        }

        @Override
        public byte getOpCode(long n) {
            reader.jump((int) n);
            return reader.read();
        }

        @Override
        public byte get(long n) {
            return getOpCode(n);
        }

        @Override
        public byte[] slice(long start, long end) {
            return Bytes.slice(reader.bytes(), Math.toIntExact(start), Math.toIntExact(end));
        }
    }
}
