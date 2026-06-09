package io.aplfintech.grape.interpreter;

import io.aplfintech.grape.l1vm.code.EofCodeReader;
import io.aplfintech.grape.l1vm.code.InvalidEofFormatException;
import io.aplfintech.grape.utils.HexUtils;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.CsvSource;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.catchThrowableOfType;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
class EofCodeReaderTest {

    EofCodeReader reader;

    @CsvSource(value = {
        "EF,  Incomplete magic"
        , "EFFF0101000302000400600000AABBCCDD, Invalid magic"
    })
    @ParameterizedTest
    void isEof(String hexCode, String errorDescription) {
        //GIVEN
        byte[] code = HexUtils.fromHex(hexCode);
        //WHEN
        var rc = EofCodeReader.isEof(code);
        //THEN
        assertThat(rc)
            .as(errorDescription)
            .isFalse();
    }

    @CsvSource(value = {
        "EF00, Invalid version,No version"
        , "EF0001, Invalid version, No header"
        , "EF000001000302000400600000AABBCCDD, Invalid version, Invalid version"
        , "EF000201000302000400600000AABBCCDD, Invalid version, Invalid version"
        , "EF00FF01000302000400600000AABBCCDD, Invalid version, Invalid version"
        , "EF000100, No code section, No code section"
        , "EF000101, No section size or size incomplete, No code section siz"
        , "EF00010100, No section size or size incomplete, Code section size incomplete "
        , "EF0001010003, Terminator not found, No section terminator"
        , "EF0001010003600000, Unknown section id=0x60, No section terminator"
        , "EF000101000200, The entire container must be scanned, No code section contents"
        , "EF00010100020060, The entire container must be scanned, Code section contents incomplete"
        , "EF000101000300600000DEADBEEF, The entire container must be scanned, Trailing bytes after code section"
        , "EF000101000301000300600000600000, Multiple sections with the same id=0x01, Multiple code sections"
        , "EF000101000000, Empty section with id=0x01, Empty code section"
        , "EF000101000002000200AABB, Empty section with id=0x01, Empty code section (with non-empty data section)"
        , "EF000102000401000300AABBCCDD600000, Data section must precede code section, Data section preceding code section"
        , "EF000102000400AABBCCDD, Data section must precede code section, Data section without code section"
        , "EF000101000202, No section size or size incomplete, No data section size"
        , "EF00010100020200, No section size or size incomplete, Data section size incomplete"
        , "EF0001010003020004, Terminator not found, No section terminator"
        , "EF0001010003020004600000AABBCCDD, Unknown section id=0x60, No section terminator"
        , "EF000101000302000400600000, The entire container must be scanned, No data section contents"
        , "EF000101000302000400600000AABBCC, The entire container must be scanned, Data section contents incomplete"
        , "EF000101000302000400600000AABBCCDDEE, The entire container must be scanned, Trailing bytes after data section"
        , "EF000101000302000402000400600000AABBCCDDAABBCCDD, Multiple sections with the same id=0x02, Multiple data sections"
        , "EF000101000101000102000102000100FEFEAABB, Multiple sections with the same id=0x01, Multiple code and data sections"
        , "EF000101000302000000600000, Empty section with id=0x02, Empty data section"
        , "EF0001010002030004006000AABBCCDD, Unknown section id=0x03, Unknown section (id = 3)"
    })
    @ParameterizedTest
    void validate_wrong_EOF_format(String hexCode, String expected, String detailErrorDescription) {
        //GIVEN
        byte[] code = HexUtils.fromHex(hexCode);
        //WHEN
        var ife = catchThrowableOfType(() -> EofCodeReader.validateFormat(code), InvalidEofFormatException.class);
        //THEN
        assertThat(ife)
            .as("check for validation error: " + detailErrorDescription)
            .hasMessage(expected);
    }

}