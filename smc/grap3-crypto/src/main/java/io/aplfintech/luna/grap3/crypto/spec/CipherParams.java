package io.aplfintech.luna.grap3.crypto.spec;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonProperty;
import lombok.Data;

/**
 * @author andrew.zinchenko@gmail.com
 */
@Data
public class CipherParams {
    private byte[] iv;

    @JsonCreator
    public CipherParams(@JsonProperty("iv") byte[] iv) {
        this.iv = iv;
    }
}
