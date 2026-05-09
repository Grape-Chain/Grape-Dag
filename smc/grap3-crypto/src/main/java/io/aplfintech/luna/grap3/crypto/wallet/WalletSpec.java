package io.aplfintech.luna.grap3.crypto.wallet;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonProperty;
import io.aplfintech.luna.grap3.crypto.spec.CryptoSpec;
import lombok.Data;

/**
 * @author andrew.zinchenko@gmail.com
 */
@Data
public class WalletSpec {
    private String version;
    private String address;
    @JsonProperty("Crypto")
    private CryptoSpec crypto;

    @JsonCreator
    public WalletSpec(@JsonProperty("version") String version,
                      @JsonProperty("address") String address,
                      @JsonProperty("Crypto") CryptoSpec crypto) {
        this.version = version;
        this.address = address;
        this.crypto = crypto;
    }

}

