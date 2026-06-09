package io.aplfintech.grape.l1vm.code;

import io.aplfintech.grape.utils.HexUtils;
import lombok.extern.slf4j.Slf4j;

import java.util.HashMap;
import java.util.Map;
import java.util.Objects;
import java.util.Optional;

/**
 * The EOF (Ethereum Object Format) contract reader
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
public class EofCodeReader implements CodeReader {
    public static final byte MAGIC = (byte) 0xef;
    public static final byte VERSION = 0x01;
    public static final byte S_TERMINATOR = 0x00;
    public static final byte S_CODE = 0x01;
    public static final byte S_DATA = 0x02;

    private final byte[] data;
    private final SectionSize sectionSize;
    private final int codePos;
    private int pos;

    EofCodeReader(byte[] data, SectionSize sectionSize) {
        this.data = data;
        if (sectionSize == null) {
            //legacy format V1
            this.sectionSize = new SectionSize(data.length, 0);
            codePos = 0;
        } else {
            this.sectionSize = sectionSize;
            //Set code to EOF container code section which starts at byte position 7 if data section is absent
            // and 10 if data section is present
            codePos = sectionSize.dataSize > 0 ? 10 : 7;
        }
        reset();
    }

    public static EofCodeReader from(byte[] code) {
        var sizes = validateFormat(code);
        return sizes.map(size -> new EofCodeReader(code, size))
            .orElseGet(() -> new EofCodeReader(code, null));
    }

    @Override
    public byte[] bytes() {
        return data;
    }

    @Override
    public int codeSize() {
        return data.length;
    }

    @Override
    public State state() {
        return new State(pos);
    }

    @Override
    public void reset() {
        //set code position counter to first byte in code section
        pos = codePos;
    }

    @Override
    public void jump(int pc) {
        checkBufferState(0, codePos + pc);
        pos = codePos + pc;
    }

    @Override
    public byte get() {
        checkBufferState(0);
        return data[pos];
    }

    @Override
    public byte read() {
        checkBufferState(1);
        return data[pos++];
    }

    @Override
    public byte[] read(int num) {
        checkBufferState(num);
        var rez = new byte[num];
        System.arraycopy(data, pos, rez, 0, num);
        pos += num;
        return rez;
    }

    @Override
    public boolean hasBytes() {
        return pos < data.length;
    }

    public static Optional<SectionSize> validateFormat(byte[] code) {
        Objects.requireNonNull(code);
        //Check Magic
        if (isEof(code)) {
            return Optional.of(validateEof(code));
        }
        return Optional.empty();
    }

    /**
     * Determines if code is in EOF format of any version
     *
     * @return true iif code is in EOF format of any version
     */
    public static boolean isEof(byte[] code) {
        return code.length >= 2 && code[0] == MAGIC && code[1] == S_TERMINATOR;
    }

    /**
     * Validate EOF code
     */
    private static SectionSize validateEof(byte[] code) {
        //Check version
        assertThat(code.length > 3 && code[2] == VERSION, "Invalid version");
        //Process section headers
        var sectionSizes = new HashMap<>(Map.of(S_CODE, 0, S_DATA, 0));
        int position = 3;
        while (true) {
            assertThat(position < code.length, "Terminator not found");
            var sectionId = code[position];
            position++;
            if (sectionId == S_TERMINATOR) {
                break;
            }
            //Disallow unknown sections
            assertThat(sectionSizes.containsKey(sectionId), "Unknown section id=" + HexUtils.toHex(sectionId, true));
            assertThat(sectionId != S_DATA || sectionSizes.get(S_CODE) != 0, "Data section must precede code section");
            assertThat(sectionSizes.get(sectionId) == 0, "Multiple sections with the same id=" + HexUtils.toHex(sectionId, true));
            assertThat(position + 1 < code.length, "No section size or size incomplete");
            sectionSizes.put(sectionId, (code[position] << 8) | (code[position + 1] & 0xff));
            position += 2;
            assertThat(sectionSizes.get(sectionId) != 0, "Empty section with id=" + HexUtils.toHex(sectionId, true));
        }
        //Code section cannot be absent
        assertThat(sectionSizes.get(S_CODE) != 0, "No code section");
        assertThat(code.length == (position + sectionSizes.get(S_CODE) + sectionSizes.get(S_DATA)),
            "The entire container must be scanned");
        return new SectionSize(sectionSizes.get(S_CODE), sectionSizes.get(S_DATA));
    }

    private void checkBufferState(int num) {
        checkBufferState(pos, num);
    }


    private static void assertThat(boolean predicate, String message) {
        if (!predicate) {
            log.error(message);
            throw new InvalidEofFormatException(message);
        }
    }

    private record SectionSize(int codeSize, int dataSize) {
    }
}
