package io.aplfintech.grape.l1vm.opcode;

import io.aplfintech.grape.vm.opcode.ExecFn;
import io.aplfintech.grape.vm.opcode.OpCode;
import lombok.EqualsAndHashCode;
import lombok.Getter;
import lombok.NonNull;
import lombok.Setter;

import java.util.HexFormat;

/**
 * Operation code structure
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Getter
@EqualsAndHashCode
public class OpCodeImpl implements OpCode {
    private final byte code;
    private final String name;
    private final boolean dynamicGas;
    private final ExecFn fn;
    @Setter
    /* Base fee */
    private int fee;

    public OpCodeImpl(int code, @NonNull String name, boolean dynamicGas, @NonNull ExecFn fn) {
        this((byte) (code & 0xff), name, dynamicGas, fn);
    }

    public OpCodeImpl(byte code, @NonNull String name, boolean dynamicGas, @NonNull ExecFn fn) {
        this.code = code;
        this.name = name;
        this.dynamicGas = dynamicGas;
        this.fn = fn;
        //set fee as Undefined,
        //the base fee must be set later as gas price of the chain config
        this.fee = -1;
    }

    OpCodeImpl(OpCode opCode, @NonNull ExecFn fn) {
        this.code = opCode.getCode();
        this.name = opCode.getName();
        this.dynamicGas = opCode.isDynamicGas();
        this.fee = opCode.getFee();
        this.fn = fn;
    }

    @Override
    public String fullName() {
        return "0x" + HexFormat.of().toHexDigits(code) + ':' + name;
    }

    @Override
    public boolean validate() {
        return (getFee() >= 0 && getFn() != null);
    }

    @Override
    public String toString() {
        return "OpCode{" +
            "code=0x" + HexFormat.of().toHexDigits((byte) (code & 0xff)) + " (" + code + ')' +
            ", name='" + name + '\'' +
            ", dynamicGas=" + dynamicGas +
            ", fee=" + fee +
            '}';
    }
}
