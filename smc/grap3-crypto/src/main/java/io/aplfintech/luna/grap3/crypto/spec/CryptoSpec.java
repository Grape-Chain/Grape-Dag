package io.aplfintech.luna.grap3.crypto.spec;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonProperty;
import lombok.Data;

/**
 * @author andrew.zinchenko@gmail.com
 */
@Data
public class CryptoSpec {
    @JsonProperty("ciphertext")
    private byte[] cipherText;
    @JsonProperty("cipherparams")
    private CipherParams cipherParams;
    private AESCipherAlg cipher;
    private Kdf kdf;
    @JsonProperty("kdfparams")
    private KdfParams kdfParams;
    private byte[] mac;

    @JsonCreator
    public CryptoSpec(@JsonProperty("ciphertext") byte[] cipherText,
                      @JsonProperty("cipherparams") CipherParams cipherParams,
                      @JsonProperty("cipher") AESCipherAlg cipher,
                      @JsonProperty("kdf") Kdf kdf,
                      @JsonProperty("kdfparams") KdfParams kdfParams,
                      @JsonProperty("mac") byte[] mac) {
        this.cipherText = cipherText;
        this.cipherParams = cipherParams;
        this.cipher = cipher;
        this.kdf = kdf;
        this.kdfParams = kdfParams;
        this.mac = mac;
    }
}
