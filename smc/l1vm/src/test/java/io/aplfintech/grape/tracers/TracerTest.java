package io.aplfintech.grape.tracers;

import io.aplfintech.grape.utils.TracerUtils;
import io.aplfintech.grape.utils.HexUtils;
import lombok.SneakyThrows;
import org.junit.jupiter.api.Test;

import java.io.PrintWriter;
import java.io.StringWriter;

import static org.assertj.core.api.AssertionsForClassTypes.assertThat;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
class TracerTest {
    static final String STR_DATA = "608060405234801561001057600080fd5b50610150806100206000396000f3fe608060405234801561001057600080fd5b50600436106100365760003560e01c80632e64cec11461003b5780636057361d14610059575b600080fd5b610043610075565b60405161005091906100d9565b60405180910390f35b610073600480360381019061006e919061009d565b61007e565b005b60008054905090565b8060008190555050565b60008135905061009781610103565b92915050565b6000602082840312156100b3576100b26100fe565b5b60006100c184828501610088565b91505092915050565b6100d3816100f4565b82525050565b60006020820190506100ee60008301846100ca565b92915050565b6000819050919050565b600080fd5b61010c816100f4565b811461011757600080fd5b5056fea26469706673582212209a159a4f3847890f10bfb87871a61eba91c5dbf5ee3cf6398207e292eee22a1664736f6c63430008070033";

    static final byte[] data = HexUtils.fromHex(STR_DATA);

    @SneakyThrows
    @Test
    void hexDump() {
        //print dump to the console, don't remove
        //TracerUtils.hexDump(TracerUtils.stdOutWriter(), data);
        //GIVEN
        var strWriter = new StringWriter();
        var writer = new PrintWriter(strWriter);

        //WHEN
        TracerUtils.hexDump(writer, data);
        var rc = strWriter.getBuffer().toString();
        //THEN
        assertThat(rc)
            .startsWith("00000000 60 80 60 40 52 34 80 15 61 00 10 57 60 00 80 FD `.`@R4..a..W`...")
            .endsWith("00000160 92 EE E2 2A 16 64 73 6F 6C 63 43 00 08 07 00 33 ...*.dsolcC....3\n");
    }
}