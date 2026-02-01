package io.swcat.maven;

import io.swcat.maven.protocol.Protocol;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.DeserializationFeature;

import java.io.*;
import java.util.ArrayList;
import java.util.Collections;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.zip.ZipEntry;
import java.util.zip.ZipInputStream;

public class ArtifactExtractor {

    /**
     * Annotation key for Maven coordinates.
     * 
     * Format: groupId:artifactId[:version[:packaging[:classifier]]]
     * 
     * version can be empty or "LATEST" to resolve the latest version.
     * 
     * empty groupId, packaging and classifier are resolved using the default values
     * from the config.
     */
    public static final String COORD_ANNOTATION = "maven.apache.org/coords";

    public static void main(String[] args) {
        ObjectMapper mapper = new ObjectMapper();
        mapper.configure(DeserializationFeature.FAIL_ON_UNKNOWN_PROPERTIES, false);

        try {
            // Read Input JSON from stdin
            Protocol.Input input = mapper.readValue(System.in, Protocol.Input.class);

            if (input == null || input.config == null) {
                printOutput(mapper, Protocol.Output.failure("Invalid input: config is missing"));
                return;
            }

            Protocol.Config config = input.config;

            // Extract Configuration
            String defaultGroupId = config.defaultGroupId;
            String defaultPackaging = config.defaultPackaging != null ? config.defaultPackaging : "jar";
            String defaultClassifier = config.defaultClassifier != null ? config.defaultClassifier : "";
            boolean includeSnapshots = config.includeSnapshots;
            String fileToExtract = config.file;

            if (fileToExtract == null) {
                printOutput(mapper, Protocol.Output.failure("Missing 'file' in configuration"));
                return;
            }

            // Parse Entity to get Name and Annotations
            if (input.entity == null || input.entity.metadata == null) {
                printOutput(mapper, Protocol.Output.failure("Invalid input: entity metadata is missing"));
                return;
            }

            Protocol.Metadata metadata = input.entity.metadata;
            String artifactId = metadata.name; // Default artifactId is entity name
            Map<String, String> annotations = metadata.annotations != null ? metadata.annotations : new HashMap<>();

            String groupId = defaultGroupId;
            String packaging = defaultPackaging;
            String classifier = defaultClassifier;
            String version = null;

            // Allow overriding coordinates via annotation
            // We use "maven.apache.org/coords" as the standard key.
            // Format: groupId:artifactId[:version[:packaging[:classifier]]]
            if (annotations.containsKey(COORD_ANNOTATION)) {
                String val = annotations.get(COORD_ANNOTATION);
                String[] parts = val.split(":");
                if (parts.length >= 1 && !parts[0].isEmpty()) {
                    groupId = parts[0];
                }
                if (parts.length >= 2 && !parts[1].isEmpty()) {
                    artifactId = parts[1];
                }
                if (parts.length >= 3 && !parts[2].isEmpty()) {
                    version = parts[2];
                }
                if (parts.length >= 4 && !parts[3].isEmpty()) {
                    packaging = parts[3];
                }
                if (parts.length >= 5 && !parts[4].isEmpty()) {
                    classifier = parts[4];
                }
            }

            if (groupId == null || groupId.isEmpty()) {
                printOutput(mapper,
                        Protocol.Output.failure("groupId is required but not set (check defaultGroupId or annotation "
                                + COORD_ANNOTATION + ")"));
                return;
            }

            // Resolve
            MavenResolver resolver = new MavenResolver();
            File artifactFile = resolver.resolve(groupId, artifactId, classifier, packaging, version, includeSnapshots);

            // Extract
            String outputDir = input.tempDir;
            if (outputDir == null) {
                outputDir = System.getProperty("java.io.tmpdir");
            }

            String outputFileName = new File(fileToExtract).getName();
            File outputFile = new File(outputDir, outputFileName);

            extractFile(artifactFile, fileToExtract, outputFile.getAbsolutePath(), config.replaceProperties);

            // Success Response
            printOutput(mapper, Protocol.Output.success(Collections.singletonList(outputFile.getAbsolutePath())));

        } catch (Exception e) {
            e.printStackTrace(System.err); // Log trace to stderr
            try {
                printOutput(mapper, Protocol.Output.failure(e.getMessage()));
            } catch (IOException ioException) {
                ioException.printStackTrace(System.err);
            }
        }
    }

    private static void printOutput(ObjectMapper mapper, Protocol.Output output) throws IOException {
        mapper.writeValue(System.out, output);
    }

    private static void extractFile(File artifactFile, String fileToExtract, String outputPath,
            boolean replaceProperties) throws IOException {
        Map<String, String> properties = new HashMap<>();
        boolean fileFound = false;
        List<String> entries = new ArrayList<>();

        try (ZipInputStream zis = new ZipInputStream(new FileInputStream(artifactFile))) {
            ZipEntry entry;
            while ((entry = zis.getNextEntry()) != null) {
                String name = entry.getName();
                entries.add(name);

                if (name.equals(fileToExtract)) {
                    try (FileOutputStream fos = new FileOutputStream(outputPath)) {
                        copyStream(zis, fos);
                    }
                    fileFound = true;
                } else if (replaceProperties && name.endsWith(".properties")) {
                    // Load properties
                    java.util.Properties props = new java.util.Properties();
                    props.load(zis);
                    for (String key : props.stringPropertyNames()) {
                        properties.put(key, props.getProperty(key));
                    }
                }
            }
        }

        if (!fileFound) {
            StringBuilder sb = new StringBuilder();
            sb.append("File ").append(fileToExtract).append(" not found in artifact. Available files:\n");
            for (String name : entries) {
                sb.append(" - ").append(name).append("\n");
            }
            throw new FileNotFoundException(sb.toString());
        }

        if (replaceProperties && !properties.isEmpty()) {
            performReplacement(outputPath, properties);
        }
    }

    private static void performReplacement(String filePath, Map<String, String> properties) throws IOException {
        File file = new File(filePath);
        ByteArrayOutputStream baos = new ByteArrayOutputStream();
        try (FileInputStream fis = new FileInputStream(file)) {
            copyStream(fis, baos);
        }

        String text = new String(baos.toByteArray(), "UTF-8");
        for (Map.Entry<String, String> entry : properties.entrySet()) {
            String placeholder = "@@" + entry.getKey() + "@@";
            text = text.replace(placeholder, entry.getValue());
        }
        
        // Replace any remaining placeholders with "undefined"
        text = text.replaceAll("@@[^@]+@@", "undefined");

        try (FileOutputStream fos = new FileOutputStream(file)) {
            fos.write(text.getBytes("UTF-8"));
        }
    }

    private static void copyStream(InputStream in, OutputStream out) throws IOException {
        byte[] buffer = new byte[1024];
        int len;
        while ((len = in.read(buffer)) > 0) {
            out.write(buffer, 0, len);
        }
    }
}
