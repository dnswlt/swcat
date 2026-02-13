package io.swcat.grpc.plugins.maven;

import io.swcat.grpc.Plugin;
import swcat.plugin.v1.Plugin.ExecuteRequest;
import swcat.plugin.v1.Plugin.ExecuteResponse;
import swcat.plugin.v1.Plugin.GeneratedFile;
import com.google.protobuf.ByteString;
import com.google.protobuf.Value;

import java.io.*;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.zip.ZipEntry;
import java.util.zip.ZipInputStream;

/**
 * A gRPC plugin that resolves a Maven artifact and extracts a specific file from it.
 * <p>
 * This is a port of the CLI-based ArtifactExtractor.
 */
public class MavenArtifactExtractorPlugin implements Plugin {

    public static final String COORD_ANNOTATION = "maven.apache.org/coords";
    private final MavenResolver resolver = new MavenResolver();

    @Override
    public ExecuteResponse execute(ExecuteRequest request) {
        try {
            if (!request.hasConfig()) {
                return failure("Missing configuration");
            }

            Map<String, Value> config = request.getConfig().getFieldsMap();
            String defaultGroupId = getString(config, "defaultGroupId", "");
            String defaultPackaging = getString(config, "defaultPackaging", "jar");
            String defaultClassifier = getString(config, "defaultClassifier", "");
            boolean includeSnapshots = getBool(config, "includeSnapshots", false);
            String fileToExtract = getString(config, "file", null);
            boolean replaceProperties = getBool(config, "replaceProperties", false);

            if (fileToExtract == null || fileToExtract.isEmpty()) {
                return failure("Missing 'file' in configuration");
            }

            // Extract coordinates from entity
            String artifactId = request.getEntity().getMetadata().getName();
            Map<String, String> annotations = request.getEntity().getMetadata().getAnnotationsMap();

            String groupId = defaultGroupId;
            String packaging = defaultPackaging;
            String classifier = defaultClassifier;
            String version = null;

            if (annotations.containsKey(COORD_ANNOTATION)) {
                String val = annotations.get(COORD_ANNOTATION);
                String[] parts = val.split(":");
                if (parts.length >= 1 && !parts[0].isEmpty()) groupId = parts[0];
                if (parts.length >= 2 && !parts[1].isEmpty()) artifactId = parts[1];
                if (parts.length >= 3 && !parts[2].isEmpty()) version = parts[2];
                if (parts.length >= 4 && !parts[3].isEmpty()) packaging = parts[3];
                if (parts.length >= 5 && !parts[4].isEmpty()) classifier = parts[4];
            }

            if (groupId == null || groupId.isEmpty()) {
                return failure("groupId is required but not set (check defaultGroupId or annotation " + COORD_ANNOTATION + ")");
            }

            // Resolve
            File artifactFile = resolver.resolve(groupId, artifactId, classifier, packaging, version, includeSnapshots);

            // Extract
            byte[] content = extractFile(artifactFile, fileToExtract, replaceProperties);

            // Success Response
            return ExecuteResponse.newBuilder()
                    .setSuccess(true)
                    .addFiles(GeneratedFile.newBuilder()
                            .setPath(new File(fileToExtract).getName())
                            .setContent(ByteString.copyFrom(content))
                            .build())
                    .build();

        } catch (Exception e) {
            return failure("MavenArtifactExtractorPlugin failed: " + e.getMessage());
        }
    }

    private byte[] extractFile(File artifactFile, String fileToExtract, boolean replaceProperties) throws IOException {
        Map<String, String> properties = new HashMap<>();
        byte[] extractedContent = null;
        List<String> entries = new ArrayList<>();

        try (ZipInputStream zis = new ZipInputStream(new FileInputStream(artifactFile))) {
            ZipEntry entry;
            while ((entry = zis.getNextEntry()) != null) {
                String name = entry.getName();
                entries.add(name);

                if (name.equals(fileToExtract)) {
                    ByteArrayOutputStream baos = new ByteArrayOutputStream();
                    copyStream(zis, baos);
                    extractedContent = baos.toByteArray();
                } else if (replaceProperties && name.endsWith(".properties")) {
                    java.util.Properties props = new java.util.Properties();
                    props.load(zis);
                    for (String key : props.stringPropertyNames()) {
                        properties.put(key, props.getProperty(key));
                    }
                }
            }
        }

        if (extractedContent == null) {
            StringBuilder sb = new StringBuilder();
            sb.append("File ").append(fileToExtract).append(" not found in artifact. Available files:\n");
            for (String name : entries) sb.append(" - ").append(name).append("\n");
            throw new FileNotFoundException(sb.toString());
        }

        if (replaceProperties && !properties.isEmpty()) {
            extractedContent = performReplacement(extractedContent, properties);
        }

        return extractedContent;
    }

    private byte[] performReplacement(byte[] content, Map<String, String> properties) throws IOException {
        String text = new String(content, "UTF-8");
        for (Map.Entry<String, String> entry : properties.entrySet()) {
            String placeholder = "@@" + entry.getKey() + "@@";
            text = text.replace(placeholder, entry.getValue());
        }
        text = text.replaceAll("@@[^@]+@@", "undefined");
        return text.getBytes("UTF-8");
    }

    private static void copyStream(InputStream in, OutputStream out) throws IOException {
        byte[] buffer = new byte[1024];
        int len;
        while ((len = in.read(buffer)) > 0) out.write(buffer, 0, len);
    }

    private String getString(Map<String, Value> config, String key, String defaultValue) {
        Value v = config.get(key);
        return (v != null && v.hasStringValue()) ? v.getStringValue() : defaultValue;
    }

    private boolean getBool(Map<String, Value> config, String key, boolean defaultValue) {
        Value v = config.get(key);
        return (v != null && v.hasBoolValue()) ? v.getBoolValue() : defaultValue;
    }

    private ExecuteResponse failure(String error) {
        return ExecuteResponse.newBuilder().setSuccess(false).setError(error).build();
    }
}
