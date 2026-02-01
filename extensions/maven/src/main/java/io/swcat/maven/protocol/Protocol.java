package io.swcat.maven.protocol;

import java.util.List;
import java.util.Map;

/**
 * Defines the JSON protocol for the ExternalPlugin interface.
 * <p>
 * This class models the data exchanged between the Go-based generic ExternalPlugin
 * and the external process (in this case, the Java Maven Extractor) via stdin/stdout.
 * </p>
 */

public class Protocol {

    /**
     * The input JSON structure sent to the external process's standard input.
     */
    public static class Input {
        /** The catalog entity being processed. */
        public Entity entity;
        
        /** Plugin-specific configuration from the 'spec.config' section of the plugin definition. */
        public Config config;
        
        /** A temporary directory created for this plugin execution. 
         * If a plugin writes output files, they should be placed in this directory. 
         */
        public String tempDir;
        
        /** Runtime arguments passed to the plugin invocation. (Not used by this plugin.)*/
        public Map<String, Object> args;
    }

    /**
     * Plugin-specific configuration for the Maven Artifact Extractor.
     */
    public static class Config {
        /** The default groupId to use if not specified in the entity annotations. */
        public String defaultGroupId;
        
        /** The default packaging (e.g., "jar", "war"). Defaults to "jar". */
        public String defaultPackaging = "jar";
        
        /** The default classifier (e.g., "sources", "javadoc"). Defaults to empty/none. */
        public String defaultClassifier;
        
        /** Whether to include SNAPSHOT versions in the resolution search. */
        public boolean includeSnapshots;
        
        /** The path of the file inside the artifact to extract. */
        public String file;

        /** Whether to replace placeholders in the extracted file with values from .properties files. */
        public boolean replaceProperties;
    }

    /**
     * Represents a Catalog Entity.
     * 
     * The spec part is missing, because it is not needed by this plugin.
     * Plugins should ideally not depend on the spec part, because it is subject to change.
     */
    public static class Entity {
        /** The kind of the entity, e.g. "Component". */
        public String kind;
        
        /** The API version of the entity, e.g. "swcat.io/v1". */
        public String apiVersion;

        /** The metadata of the entity. */
        public Metadata metadata;
    }

    /**
     * Standard metadata common to all Catalog Entities.
     * Matches the fields defined in internal/catalog/catalog.go.
     */
    public static class Metadata {
        /** The unique name of the entity. */
        public String name;
        
        /** The namespace the entity belongs to (optional). */
        public String namespace;
        
        /** A human-readable title. */
        public String title;
        
        /** A description of the entity. */
        public String description;
        
        /** Key-value pairs for categorization. */
        public Map<String, String> labels;
        
        /** Key-value pairs for non-identifying metadata. */
        public Map<String, String> annotations;
        
        /** List of tags. */
        public List<String> tags;
        
        /** External links. */
        public List<Link> links;
    }

    public static class Link {
        public String url;
        public String title;
        public String icon;
    }

    /**
     * The output JSON structure expected on standard output.
     */
    public static class Output {
        /** True if the operation succeeded, false otherwise. */
        public boolean success;
        
        /** Error message, populated only if success is false. */
        public String error;
        
        /** List of absolute paths to files generated or extracted by the plugin. */
        public List<String> generatedFiles;
        
        /** New annotations to be added to the entity. 
         * The values must be JSON-serializable.
         * Not used by this plugin.
        */
        public Map<String, Object> annotations;

        public static Output success(List<String> generatedFiles) {
            Output o = new Output();
            o.success = true;
            o.generatedFiles = generatedFiles;
            return o;
        }

        public static Output failure(String error) {
            Output o = new Output();
            o.success = false;
            o.error = error;
            return o;
        }
    }
}
