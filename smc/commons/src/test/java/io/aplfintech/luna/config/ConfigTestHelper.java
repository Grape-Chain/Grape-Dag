package io.aplfintech.luna.config;

import io.aplfintech.luna.utils.FileUtils;
import lombok.AccessLevel;
import lombok.NoArgsConstructor;
import lombok.NonNull;
import lombok.SneakyThrows;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@NoArgsConstructor(access = AccessLevel.PRIVATE)
public class ConfigTestHelper {
    static BaseChainConfig createBaseConfig(@NonNull String resource) {
        return createObject(resource, BaseChainConfig.class);
    }

    static FeatureConfig createFeatureConfig(@NonNull String resource) {
        return createObject(resource, FeatureConfig.class);
    }

    static HardForkConfig createHardForkConfig(@NonNull String resource) {
        return createObject(resource, HardForkConfig.class);
    }

    @SneakyThrows
    static <T> T createObject(@NonNull String resource, @NonNull Class<T> clazz) {
        var json = FileUtils.readResourceContent("config/" + resource);
        return ConfigHelper.readValue(json, clazz);
    }

}
