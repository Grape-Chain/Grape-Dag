package io.aplfintech.grape.vm.utils;

import com.fasterxml.jackson.databind.json.JsonMapper;
import io.aplfintech.grape.vm.contract.CompiledContract;
import io.aplfintech.grape.vm.contract.CompiledContractInfo;
import io.aplfintech.grape.vm.tx.TxData;
import io.aplfintech.grape.utils.FileUtils;
import lombok.AccessLevel;
import lombok.NoArgsConstructor;
import lombok.SneakyThrows;

/**
 * Compiled contracts helper
 *
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@NoArgsConstructor(access = AccessLevel.PRIVATE)
public class ContractHelper {

    private static final JsonMapper MAPPER = new JsonMapper();
    public static final TxData txData = new TxData();

    @SneakyThrows
    public static CompiledContractInfo readContractInfo(String jsonFileName) {
        var json = FileUtils.readResourceContent(jsonFileName);
        assert json != null;
        var cc = MAPPER.readValue(json, CompiledContractInfo.class);
        assert cc != null;
        cc.setName(jsonFileName);
        return cc;
    }

    public static CompiledContract createCompiledContract(String jsonFileName) {
        var contractInfo = readContractInfo(jsonFileName);
        return new CompiledContract(contractInfo);
    }

}
