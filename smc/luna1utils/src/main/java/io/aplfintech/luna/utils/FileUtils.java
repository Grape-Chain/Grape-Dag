package io.aplfintech.luna.utils;

import lombok.AccessLevel;
import lombok.NoArgsConstructor;
import lombok.SneakyThrows;

import java.io.InputStream;
import java.nio.charset.StandardCharsets;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@NoArgsConstructor(access = AccessLevel.PRIVATE)
public class FileUtils {
    /**
     * Load file from classpath, resources folder
     *
     * @param fileName the file name
     * @return the file content
     */
    @SneakyThrows
    public static String readResourceContent(String fileName) {
        try (InputStream resourceAsStream = FileUtils.class.getClassLoader().getResourceAsStream(fileName)) {
            if (resourceAsStream == null) {
                return null;
            } else {
                return new String(
                    resourceAsStream.readAllBytes(),
                    StandardCharsets.UTF_8
                );
            }
        }
    }
}
