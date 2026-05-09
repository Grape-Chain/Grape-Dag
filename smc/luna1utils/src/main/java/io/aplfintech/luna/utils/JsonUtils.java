package io.aplfintech.luna.utils;

import com.fasterxml.jackson.core.JsonGenerator;
import com.fasterxml.jackson.core.JsonParser;
import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.core.Version;
import com.fasterxml.jackson.databind.DeserializationContext;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.SerializerProvider;
import com.fasterxml.jackson.databind.deser.std.StdDeserializer;
import com.fasterxml.jackson.databind.module.SimpleModule;
import com.fasterxml.jackson.databind.ser.std.StdSerializer;
import lombok.AccessLevel;
import lombok.NoArgsConstructor;

import java.io.IOException;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@NoArgsConstructor(access = AccessLevel.PRIVATE)
public class JsonUtils {

    public static ObjectMapper HEX_MAPPER = getJsonMapper();

    public static class ByteArrayHexSerializer extends StdSerializer<byte[]> {
        public ByteArrayHexSerializer() {
            super(byte[].class);
        }

        @Override
        public void serialize(byte[] value, JsonGenerator gen, SerializerProvider provider) throws IOException {
            gen.writeString(HexUtils.toHex(value, true));
        }

    }

    public static class ByteArrayHexDeserializer extends StdDeserializer<byte[]> {
        public ByteArrayHexDeserializer() {
            super(byte[].class);
        }

        @Override
        public byte[] deserialize(JsonParser p, DeserializationContext ctxt) throws IOException, JsonProcessingException {
            JsonNode node = p.getCodec().readTree(p);
            return HexUtils.parseHex(node.asText());
        }
    }

    private static ObjectMapper getJsonMapper() {
        ObjectMapper jsonMapper = new ObjectMapper();
        SimpleModule module = new SimpleModule("ByteArraySerializer", new Version(0, 1, 0, "", "io.aplfintech.luna", "smc-module"));
        module.addSerializer(byte[].class, new ByteArrayHexSerializer());
        module.addDeserializer(byte[].class, new ByteArrayHexDeserializer());
        jsonMapper.registerModule(module);
        return jsonMapper;
    }
}
