package io.aplfintech.grape.grap3.crypto.spec;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonProperty;
import lombok.Data;
import lombok.NonNull;

/**
 * @author andrew.zinchenko@gmail.com
 */
@Data
public class KdfParams {
    int dklen = 32;
    byte[] salt;
    int n = 1024;
    int r = 8;
    int p = 1;

    public KdfParams(byte @NonNull [] salt) {
        this.salt = salt;
    }

    @JsonCreator
    public KdfParams(@JsonProperty(value = "dklen") Integer dklen,
                     @JsonProperty("salt") byte @NonNull [] salt,
                     @JsonProperty(value = "n") Integer n,
                     @JsonProperty(value = "r") Integer r,
                     @JsonProperty(value = "p") Integer p) {
        this.salt = salt;

        if (dklen != null) {
            this.dklen = dklen;
        }
        if (n != null) {
            this.n = n;
        }
        if (r != null) {
            this.r = r;
        }
        if (p != null) {
            this.p = p;
        }
    }
}
