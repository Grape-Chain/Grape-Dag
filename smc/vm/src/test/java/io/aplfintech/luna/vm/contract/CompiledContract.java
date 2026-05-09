package io.aplfintech.luna.vm.contract;

import io.aplfintech.luna.utils.HexUtils;
import lombok.extern.slf4j.Slf4j;

import java.util.HashMap;
import java.util.Map;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
public class CompiledContract {

    private final String fileName;
    private final ByteCode byteCode;
    private final Map<String, String> functions;

    public CompiledContract(CompiledContractInfo contractInfo) {
        this.fileName = contractInfo.getName();
        this.byteCode = contractInfo.getByteCode();
        this.functions = new HashMap<>();
        contractInfo.getFunctionHashes().forEach((k, v) -> functions.put(v.split("\\(")[0], k));
    }

    public byte[] code() {
        return HexUtils.parseHex(byteCode.getObject());
    }

    public byte[] runtimeCode() {
        return HexUtils.parseHex(byteCode.getRuntimeCode());
    }

    public Map<String, String> functions() {
        return functions;
    }

    public String fileName() {
        return fileName;
    }

}
