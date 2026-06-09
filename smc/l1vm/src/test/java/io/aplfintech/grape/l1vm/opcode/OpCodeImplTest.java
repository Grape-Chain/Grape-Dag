package io.aplfintech.grape.l1vm.opcode;

import org.junit.jupiter.api.Test;

import static io.aplfintech.grape.l1vm.opcode.OpCodes.INVALID;
import static io.aplfintech.grape.l1vm.opcode.OpCodes.STOP;
import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.catchNullPointerException;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
class OpCodeImplTest {

    @Test
    void checkNullName() {
        var npe = catchNullPointerException(() -> new OpCodeImpl(0x00, null, false, INVALID.getFn()));
        assertThat(npe)
            .hasMessage("name is marked non-null but is null");
    }

    @Test
    void checkNullFn() {
        var npe = catchNullPointerException(() -> new OpCodeImpl(0x00, "CODE", false, null));
        assertThat(npe)
            .hasMessage("fn is marked non-null but is null");
    }

    @Test
    void fullName() {
        //GIVEN
        var expected = "0x00:STOP";
        var op = new OpCodeImpl(0x00, "STOP", false, STOP.getFn());
        //WHEN
        var rc = op.fullName();
        //THEN
        assertThat(rc)
            .isEqualTo(expected);
    }
}