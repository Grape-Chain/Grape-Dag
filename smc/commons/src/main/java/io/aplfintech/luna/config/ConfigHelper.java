package io.aplfintech.luna.config;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.module.SimpleModule;
import com.fasterxml.jackson.databind.ser.std.ToStringSerializer;
import lombok.AccessLevel;
import lombok.NoArgsConstructor;
import lombok.NonNull;
import lombok.SneakyThrows;

import java.io.InputStream;
import java.math.BigDecimal;
import java.math.BigInteger;
import java.text.DateFormat;
import java.text.SimpleDateFormat;
import java.util.Date;
import java.util.SimpleTimeZone;

import static com.fasterxml.jackson.databind.DeserializationFeature.FAIL_ON_UNKNOWN_PROPERTIES;
import static java.time.ZoneOffset.UTC;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@NoArgsConstructor(access = AccessLevel.PRIVATE)
public class ConfigHelper {
    public static final String TIMESTAMP_PATTERN = "yyyy-MM-dd'T'HH:mm:ss";
    public static final ObjectMapper MAPPER = new ObjectMapper();
    public static final DateFormat TIMESTAMP_FORMAT;

    static {
        MAPPER.configure(FAIL_ON_UNKNOWN_PROPERTIES, true);//Don't Ignore unknown properties
        SimpleModule module = new SimpleModule();
        module.addSerializer(BigInteger.class, new ToStringSerializer());
        module.addSerializer(BigDecimal.class, new ToStringSerializer());
        MAPPER.registerModule(module);
        TIMESTAMP_FORMAT = new SimpleDateFormat(TIMESTAMP_PATTERN);
        TIMESTAMP_FORMAT.setTimeZone(new SimpleTimeZone(UTC.getTotalSeconds(), UTC.getId()));
        MAPPER.setDateFormat(TIMESTAMP_FORMAT);
    }

    @SneakyThrows
    public static Date parseTimestamp(@NonNull String date) {
        return TIMESTAMP_FORMAT.parse(date);
    }

    @SneakyThrows
    public static String writeValueAsString(Object value) {
        return MAPPER.writeValueAsString(value);
    }

    @SneakyThrows
    public static <T> T readValue(String content, Class<T> valueType) {
        return MAPPER.readValue(content, valueType);
    }

    @SneakyThrows
    public static <T> T readValue(InputStream is, Class<T> valueType) {
        return MAPPER.readValue(is, valueType);
    }

}
