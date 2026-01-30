package io.swcat.maven;

import org.apache.maven.repository.internal.MavenRepositorySystemUtils;
import org.apache.maven.settings.Profile;
import org.apache.maven.settings.Settings;
import org.apache.maven.settings.building.DefaultSettingsBuilderFactory;
import org.apache.maven.settings.building.DefaultSettingsBuildingRequest;
import org.apache.maven.settings.building.SettingsBuilder;
import org.apache.maven.settings.building.SettingsBuildingException;
import org.apache.maven.settings.building.SettingsBuildingRequest;
import org.eclipse.aether.DefaultRepositorySystemSession;
import org.eclipse.aether.RepositorySystem;
import org.eclipse.aether.RepositorySystemSession;
import org.eclipse.aether.artifact.Artifact;
import org.eclipse.aether.artifact.DefaultArtifact;
import org.eclipse.aether.connector.basic.BasicRepositoryConnectorFactory;
import org.eclipse.aether.impl.DefaultServiceLocator;
import org.eclipse.aether.repository.Authentication;
import org.eclipse.aether.repository.LocalRepository;
import org.eclipse.aether.repository.Proxy;
import org.eclipse.aether.repository.RemoteRepository;
import org.eclipse.aether.resolution.ArtifactRequest;
import org.eclipse.aether.resolution.ArtifactResolutionException;
import org.eclipse.aether.resolution.ArtifactResult;
import org.eclipse.aether.resolution.VersionRangeRequest;
import org.eclipse.aether.resolution.VersionRangeResolutionException;
import org.eclipse.aether.resolution.VersionRangeResult;
import org.eclipse.aether.spi.connector.RepositoryConnectorFactory;
import org.eclipse.aether.spi.connector.transport.TransporterFactory;
import org.eclipse.aether.transport.file.FileTransporterFactory;
import org.eclipse.aether.transport.http.HttpTransporterFactory;
import org.eclipse.aether.util.repository.AuthenticationBuilder;
import org.eclipse.aether.util.repository.DefaultAuthenticationSelector;
import org.eclipse.aether.util.repository.DefaultMirrorSelector;
import org.eclipse.aether.util.repository.DefaultProxySelector;
import org.eclipse.aether.version.Version;

import java.io.File;
import java.util.ArrayList;
import java.util.Collections;
import java.util.List;
import java.util.Map;

public class MavenResolver {

    private final RepositorySystem repositorySystem;
    private final RepositorySystemSession session;
    private final List<RemoteRepository> repositories;

    public MavenResolver() {
        this.repositorySystem = newRepositorySystem();
        Settings settings = loadSettings();
        this.session = newSession(repositorySystem, settings);
        this.repositories = new ArrayList<>();
        
        // Always add Central (Mirrors will override this if configured)
        this.repositories.add(new RemoteRepository.Builder("central", "default", "https://repo.maven.apache.org/maven2/").build());
        
        // Add repositories from active profiles
        Map<String, Profile> profiles = settings.getProfilesAsMap();
        for (String profileId : settings.getActiveProfiles()) {
            Profile profile = profiles.get(profileId);
            if (profile != null) {
                for (org.apache.maven.settings.Repository repo : profile.getRepositories()) {
                    RemoteRepository.Builder builder = new RemoteRepository.Builder(repo.getId(), "default", repo.getUrl());
                    // Aether handles policy (snapshots/releases) conversion if needed, but keeping it simple for now
                    this.repositories.add(builder.build());
                }
            }
        }
    }

    public File resolveLatest(String groupId, String artifactId, String classifier, String packaging, boolean includeSnapshots) throws VersionRangeResolutionException, ArtifactResolutionException {
        // Find latest version
        Artifact artifactConfig = new DefaultArtifact(groupId, artifactId, classifier, packaging, "[0,)");
        VersionRangeRequest rangeRequest = new VersionRangeRequest();
        rangeRequest.setArtifact(artifactConfig);
        rangeRequest.setRepositories(repositories);

        VersionRangeResult rangeResult = repositorySystem.resolveVersionRange(session, rangeRequest);
        
        Version latestVersion = null;
        List<Version> versions = rangeResult.getVersions();
        // Versions are sorted ascending
        for (Version v : versions) {
            boolean isSnapshot = v.toString().endsWith("SNAPSHOT");
            if (includeSnapshots || !isSnapshot) {
                latestVersion = v;
            }
        }

        if (latestVersion == null) {
            throw new RuntimeException("No suitable version found for " + groupId + ":" + artifactId + " (includeSnapshots=" + includeSnapshots + ")");
        }

        System.err.println("Resolved latest version: " + latestVersion);

        // Resolve artifact
        Artifact artifactToResolve = new DefaultArtifact(groupId, artifactId, classifier, packaging, latestVersion.toString());
        ArtifactRequest artifactRequest = new ArtifactRequest();
        artifactRequest.setArtifact(artifactToResolve);
        artifactRequest.setRepositories(repositories);

        ArtifactResult artifactResult = repositorySystem.resolveArtifact(session, artifactRequest);
        return artifactResult.getArtifact().getFile();
    }

    private static RepositorySystem newRepositorySystem() {
        DefaultServiceLocator locator = MavenRepositorySystemUtils.newServiceLocator();
        locator.addService(RepositoryConnectorFactory.class, BasicRepositoryConnectorFactory.class);
        locator.addService(TransporterFactory.class, FileTransporterFactory.class);
        locator.addService(TransporterFactory.class, HttpTransporterFactory.class);

        locator.setErrorHandler(new DefaultServiceLocator.ErrorHandler() {
            @Override
            public void serviceCreationFailed(Class<?> type, Class<?> impl, Throwable exception) {
                exception.printStackTrace();
            }
        });

        return locator.getService(RepositorySystem.class);
    }

    private static RepositorySystemSession newSession(RepositorySystem system, Settings settings) {
        DefaultRepositorySystemSession session = MavenRepositorySystemUtils.newSession();

        // Define the local repository location. 
        // We use the system temp directory to ensure we can write to it regardless of where the JAR is executed.
        // We add a subfolder "swcat-maven-repo" to keep it isolated.
        String tempDir = System.getProperty("java.io.tmpdir");
        File repoDir = new File(tempDir, "swcat-maven-repo");
        LocalRepository localRepo = new LocalRepository(repoDir.getAbsolutePath());
        session.setLocalRepositoryManager(system.newLocalRepositoryManager(session, localRepo));

        // Apply Mirrors
        DefaultMirrorSelector mirrorSelector = new DefaultMirrorSelector();
        if (settings.getMirrors() != null) {
            for (org.apache.maven.settings.Mirror mirror : settings.getMirrors()) {
                mirrorSelector.add(mirror.getId(), mirror.getUrl(), mirror.getLayout(), false, mirror.getMirrorOf(), mirror.getMirrorOfLayouts());
            }
        }
        session.setMirrorSelector(mirrorSelector);

        // Apply Proxies
        DefaultProxySelector proxySelector = new DefaultProxySelector();
        if (settings.getProxies() != null) {
            for (org.apache.maven.settings.Proxy proxy : settings.getProxies()) {
                if (proxy.isActive()) {
                    Authentication auth = (proxy.getUsername() != null) ? new AuthenticationBuilder()
                            .addUsername(proxy.getUsername())
                            .addPassword(proxy.getPassword())
                            .build() : null;
                    proxySelector.add(new Proxy(proxy.getProtocol(), proxy.getHost(), proxy.getPort(), auth), proxy.getNonProxyHosts());
                }
            }
        }
        session.setProxySelector(proxySelector);

        // Apply Servers (Authentication)
        DefaultAuthenticationSelector authSelector = new DefaultAuthenticationSelector();
        if (settings.getServers() != null) {
            for (org.apache.maven.settings.Server server : settings.getServers()) {
                AuthenticationBuilder authBuilder = new AuthenticationBuilder();
                authBuilder.addUsername(server.getUsername()).addPassword(server.getPassword());
                authBuilder.addPrivateKey(server.getPrivateKey(), server.getPassphrase());
                authSelector.add(server.getId(), authBuilder.build());
            }
        }
        session.setAuthenticationSelector(authSelector);

        return session;
    }

    private static Settings loadSettings() {
        try {
            SettingsBuilder settingsBuilder = new DefaultSettingsBuilderFactory().newInstance();
            SettingsBuildingRequest request = new DefaultSettingsBuildingRequest();
            
            File userSettingsFile = new File(System.getProperty("user.home"), ".m2/settings.xml");
            if (userSettingsFile.exists()) {
                request.setUserSettingsFile(userSettingsFile);
            }
            
            return settingsBuilder.build(request).getEffectiveSettings();
        } catch (SettingsBuildingException e) {
            System.err.println("Could not load settings.xml, proceeding with defaults: " + e.getMessage());
            return new Settings();
        }
    }
}
