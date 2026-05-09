package io.aplfintech.luna.vm.impl;

import io.aplfintech.luna.bcei.DEI;
import io.aplfintech.luna.config.ChainConfig;
import io.aplfintech.luna.crypto.CryptoLib;
import io.aplfintech.luna.vm.opcode.OpTable;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.mockito.Mockito.mock;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
class ExecutorFactoryTest {
    ChainConfig chainConfig;
    CryptoLib cryptoLib;
    DEI dei;
    OpTable opTable;

    @BeforeEach
    void setUp() {
        chainConfig = mock(ChainConfig.class);
        dei = mock(DEI.class);
        opTable = mock(OpTable.class);
        cryptoLib = mock(CryptoLib.class);

    }

    @Test
    void applier() {
        //WHEN
        var rc = ExecutorFactory.newFactory(chainConfig)
            .configuration().opTable(opTable).cryptoLib(cryptoLib).debugEnabled(true).noBaseFee(true)
            .applier(dei);
        //THEN
        assertNotNull(rc, "Applier must be NOT null");
        //WHEN
        var cfg = ((AbstractMessageExecutor) rc).getExecutionConfig();
        //THEN
        assertThat(cfg)
            .hasNoNullFieldsOrProperties();
    }

    @Test
    void executor() {
        //WHEN
        var rc = ExecutorFactory.newFactory(chainConfig)
            .configuration().opTable(opTable).cryptoLib(cryptoLib).debugEnabled(true).noBaseFee(true)
            .executor(dei);
        //THEN
        assertNotNull(rc, "Applier must be NOT null");
        //WHEN
        var cfg = ((AbstractMessageExecutor) rc).getExecutionConfig();
        //THEN
        assertThat(cfg)
            .hasNoNullFieldsOrProperties();
    }

}