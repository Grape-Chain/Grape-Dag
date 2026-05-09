package io.aplfintech.luna.utils;

import com.google.common.base.Strings;
import lombok.AccessLevel;
import lombok.NoArgsConstructor;
import lombok.NonNull;
import lombok.SneakyThrows;
import lombok.extern.slf4j.Slf4j;
import org.apache.commons.io.HexDump;
import org.apache.commons.io.output.NullWriter;
import org.apache.commons.io.output.WriterOutputStream;

import java.io.FileOutputStream;
import java.io.IOException;
import java.io.OutputStream;
import java.io.PrintWriter;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Paths;
import java.text.SimpleDateFormat;
import java.util.Date;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
@NoArgsConstructor(access = AccessLevel.PRIVATE)
public class TracerUtils {
    public static void hexDump(PrintWriter writer, byte[] data) throws IOException {
        writer.flush();
        OutputStream os = new WriterOutputStream(writer, StandardCharsets.UTF_8);
        HexDump.dump(data, 0, os, 0);
        writer.flush();
    }

    public static PrintWriter nullWriter() {
        return new PrintWriter(new NullWriter());
    }

    public static PrintWriter stdOutWriter() {
        return new PrintWriter(System.out, true);
    }

    public static PrintWriter openWriterToAppend(@NonNull String fileName) {
        var header = "\n+++++++\n" +
                "+++ Contract execution at " + new SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ss.SSSZ").format(new Date()) + "\n" +
                "+++++++\n";
        return openWriterToAppend(fileName, header);
    }

    public static PrintWriter newWriter(@NonNull String fileName) {
        return openWriter(fileName, false, null);
    }

    public static PrintWriter openWriterToAppend(@NonNull String fileName, String header) {
        return openWriter(fileName, true, header);
    }

    @SneakyThrows
    public static PrintWriter openWriter(@NonNull String fileName, boolean append, String header) {
        Files.createDirectories(Paths.get(fileName).getParent());
        var printer = new PrintWriter(new FileOutputStream(fileName, append), true);
        if (!Strings.isNullOrEmpty(header)) {
            printer.println(header);
        }
        return printer;
    }

    public static String addTimestampPrefix(@NonNull String name) {
        return new SimpleDateFormat("yyyyMMdd'T'HHmmss_SSS").format(new Date()) + name;
    }

    public static String addDateTimePrefix(@NonNull String name) {
        return new SimpleDateFormat("yyyyMMdd'T'HHmmss").format(new Date()) + name;
    }

    public static String addDatePrefix(@NonNull String name) {
        return new SimpleDateFormat("yyyyMMdd").format(new Date()) + name;
    }
}
