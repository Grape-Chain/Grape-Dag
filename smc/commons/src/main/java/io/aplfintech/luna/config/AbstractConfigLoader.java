package io.aplfintech.luna.config;

import com.google.common.base.Preconditions;
import com.google.common.base.Strings;
import lombok.extern.slf4j.Slf4j;

import java.io.File;
import java.io.FileInputStream;
import java.io.FileNotFoundException;
import java.io.IOException;
import java.io.InputStream;
import java.nio.file.Paths;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
public abstract class AbstractConfigLoader<T> implements ConfigLoader<T> {
    protected static final String DEFAULT_CONF_DIR = "config";
    private final boolean ignoreResources;
    private final boolean ignoreUserConfig;
    private String configDir;
    private T config;
    private final String resourceName;

    protected AbstractConfigLoader(String resourceName, boolean ignoreResources) {
        this(DEFAULT_CONF_DIR, resourceName, ignoreResources);
    }

    protected AbstractConfigLoader(String configDir, String resourceName, boolean ignoreResources) {
        Preconditions.checkArgument(!Strings.isNullOrEmpty(resourceName), "Resource name is blank or empty");
        this.ignoreUserConfig = Strings.isNullOrEmpty(configDir);
        if (ignoreUserConfig && ignoreResources) {
            throw new IllegalArgumentException("No locations for config loading provided. Resources and user defined configs ignored");
        }
        this.ignoreResources = ignoreResources;
        if (!Strings.isNullOrEmpty(configDir)) {
            this.configDir = configDir;
        }
        this.resourceName = resourceName;
    }

    public String getResourceName() {
        return configDir + File.separator + resourceName;
    }

    @Override
    public T load() {
        if (!ignoreResources) {
            loadFromResources();
        } else {
            log.warn("Will ignore resources!");
        }
        if (!ignoreUserConfig) {
            loadFromDirectory();
        } else {
            log.warn("Will ignore user defined config!");
        }
        if (config == null) {
            throw new ConfigurationException("Can't load config, path=" + getResourceName());
        }
        return config;
    }

    protected abstract T read(InputStream is) throws IOException;

    private void loadFromResources() {
        ClassLoader classloader = Thread.currentThread().getContextClassLoader();
        try (InputStream is = classloader.getResourceAsStream(getResourceName())) {
            if (is == null) {
                log.info("Resource not found, resource={}", getResourceName());
            } else {
                config = read(is);
            }
        } catch (IOException | IllegalArgumentException e) {
            log.error("Config IO error, resource={}", getResourceName());
            throw new ConfigurationException(e);
        }
    }

    private void loadFromDirectory() {
        var path = getCurrentDirConfigLocation() + File.separator + getResourceName();
        try (FileInputStream is = new FileInputStream(path)) {
            config = read(is);
        } catch (FileNotFoundException ignored) {
            log.info("File not found, path={}", path);
        } catch (IOException | IllegalArgumentException e) {
            log.error("Config IO error, path=" + path, e);
            throw new ConfigurationException(e);
        }
    }

    private String getCurrentDirConfigLocation() {
        return Paths.get("").toAbsolutePath().toString();
    }
}
