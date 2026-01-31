package io.swcat.maven;

import org.apache.commons.cli.*;

import java.io.*;
import java.util.ArrayList;
import java.util.List;
import java.util.zip.ZipEntry;
import java.util.zip.ZipInputStream;

public class ArtifactExtractor {

    public static void main(String[] args) {
        Options options = new Options();
        options.addOption("g", "groupId", true, "Group ID");
        options.addOption("a", "artifactId", true, "Artifact ID");
        options.addOption("c", "classifier", true, "Classifier");
        options.addOption("p", "packaging", true, "Packaging");
        options.addOption("f", "file", true, "File to extract");
        options.addOption("o", "output", true, "Output file path (optional, defaults to stdout)");
        options.addOption("s", "snapshots", false, "Include SNAPSHOT versions");

        CommandLineParser parser = new DefaultParser();
        try {
            CommandLine cmd = parser.parse(options, args);

            if (!cmd.hasOption("g") || !cmd.hasOption("a") || !cmd.hasOption("f")) {
                HelpFormatter formatter = new HelpFormatter();
                formatter.printHelp("artifact-extractor", options);
                System.exit(1);
            }

            String groupId = cmd.getOptionValue("g");
            String artifactId = cmd.getOptionValue("a");
            String classifier = cmd.getOptionValue("c", ""); // Default to empty
            String packaging = cmd.getOptionValue("p", "jar"); // Default to jar
            String fileToExtract = cmd.getOptionValue("f");
            String outputFile = cmd.getOptionValue("o");
            boolean includeSnapshots = cmd.hasOption("s");

            MavenResolver resolver = new MavenResolver();
            File artifactFile = resolver.resolveLatest(groupId, artifactId, classifier, packaging, includeSnapshots);
            System.err.println("Resolved artifact to: " + artifactFile.getAbsolutePath());

            extractFile(artifactFile, fileToExtract, outputFile);

        } catch (ParseException e) {
            System.err.println("Error parsing arguments: " + e.getMessage());
            System.exit(1);
        } catch (Exception e) {
            System.err.println("Failed to resolve artifact: " + e.getMessage());
            System.exit(1);
        }
    }

    private static void extractFile(File artifactFile, String fileToExtract, String outputPath) throws IOException {
        List<String> entries = new ArrayList<>();
        try (ZipInputStream zis = new ZipInputStream(new FileInputStream(artifactFile))) {
            ZipEntry entry;
            while ((entry = zis.getNextEntry()) != null) {
                entries.add(entry.getName());
                if (entry.getName().equals(fileToExtract)) {
                    if (outputPath != null) {
                        try (FileOutputStream fos = new FileOutputStream(outputPath)) {
                            copyStream(zis, fos);
                        }
                    } else {
                        copyStream(zis, System.out);
                    }
                    return;
                }
            }
            System.err.println("File " + fileToExtract + " not found in artifact. Available files:");
            for (String name : entries) {
                System.err.println(" - " + name);
            }
            System.exit(1);
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
