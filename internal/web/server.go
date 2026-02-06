package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/dnswlt/swcat"
	"github.com/dnswlt/swcat/internal/api"
	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/comments"
	"github.com/dnswlt/swcat/internal/config"
	"github.com/dnswlt/swcat/internal/dot"
	"github.com/dnswlt/swcat/internal/plugins"
	"github.com/dnswlt/swcat/internal/repo"
	"github.com/dnswlt/swcat/internal/store"
	"github.com/dnswlt/swcat/internal/svg"
	lru "github.com/hashicorp/golang-lru/v2"
	"gopkg.in/yaml.v3"
)

type ServerOptions struct {
	Addr            string        // E.g., "localhost:8080"
	BaseDir         string        // Directory from which resources (templates etc.) are read.
	DotPath         string        // E.g., "dot" (with dot on the PATH)
	DotTimeout      time.Duration // Time after which dot executions will be cancelled
	UseDotStreaming bool          // If true, keeps the dot process running and streams requests to it (Good for corporate Windows machines).
	ReadOnly        bool          // If true, no Edit/Clone/Delete operations will be supported.
	Version         string        // App version
	SVGCacheSize    int           // Size of the LRU cache for rendered SVGs
}

// storeData contains all data extracted from a given view of a Store at reference ref.
type storeData struct {
	ref      string
	repo     *repo.Repository
	config   *config.Bundle
	svgCache *lru.Cache[string, *svg.Result]
}

type Server struct {
	opts     ServerOptions
	template *template.Template
	// Mutex to synchronize access to the server's state across concurrent requests.
	mu           sync.RWMutex
	storeDataMap map[string]*storeData
	source       store.Source
	finder       *repo.Finder

	dotRunner dot.Runner

	// The optional plugins registry.
	// If set, plugins are available to update entity annotations.
	pluginRegistry *plugins.Registry

	commentsStore comments.Store

	// Server startup time. Used for cache busting JS/CSS resources.
	started time.Time
}

func NewServer(opts ServerOptions, source store.Source, pluginRegistry *plugins.Registry, commentsStore comments.Store) (*Server, error) {
	if opts.SVGCacheSize <= 0 {
		opts.SVGCacheSize = 128
	}
	var dotRunner dot.Runner
	if opts.UseDotStreaming {
		dotRunner = dot.NewStreamingRunner(opts.DotPath)
	} else {
		dotRunner = dot.NewRunner(opts.DotPath)
	}

	s := &Server{
		opts:           opts,
		storeDataMap:   make(map[string]*storeData),
		source:         source,
		dotRunner:      dotRunner,
		pluginRegistry: pluginRegistry,
		commentsStore:  commentsStore,
		finder:         repo.NewFinder(),
		started:        time.Now(),
	}

	if s.commentsStore != nil {
		s.finder.RegisterPropertyProvider(func(e catalog.Entity, prop string) ([]string, bool) {
			if prop != "comment" && prop != "comments" {
				return nil, false
			}
			comments, err := s.commentsStore.GetOpenComments(e.GetRef().String())
			if err != nil {
				// Warn but don't fail, treating as no comments found
				log.Printf("Failed to load comments for %s: %v", e.GetRef(), err)
				return nil, false
			}
			var values []string
			for _, c := range comments {
				values = append(values, c.Text, c.Author)
			}
			return values, true
		})
	}

	if err := s.reloadTemplates(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *storeData) lookupSVG(cacheKey string) (*svg.Result, bool) {
	return s.svgCache.Get(cacheKey)
}

func (s *storeData) storeSVG(cacheKey string, svg *svg.Result) {
	s.svgCache.Add(cacheKey, svg)
}

func (s *Server) commentsEnabled() bool {
	return s.commentsStore != nil
}

// loadStoreData retrieves the storeData for ref from the cache, if present,
// or else loads it from the associated store.
func (s *Server) loadStoreData(ref string) (*storeData, error) {
	// Fast path: already cached
	s.mu.RLock()
	cached, ok := s.storeDataMap[ref]
	s.mu.RUnlock()
	if ok {
		return cached, nil
	}

	// Try to load data from store and add to cache.
	s.mu.Lock()
	defer s.mu.Unlock()

	st, err := s.source.Store(ref)
	if err != nil {
		return nil, fmt.Errorf("failed to obtain store for ref %q: %w", ref, err)
	}

	cfg := &config.Bundle{} // Default (empty) config
	storeCfg, err := config.Load(st, store.ConfigFile)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if storeCfg != nil {
		cfg = storeCfg
	}

	repoInstance, err := repo.Load(st, cfg.Catalog)
	if err != nil {
		return nil, err
	}

	cache, err := lru.New[string, *svg.Result](s.opts.SVGCacheSize)
	if err != nil {
		panic(fmt.Sprintf("failed to create SVG cache (size: %d): %v", s.opts.SVGCacheSize, err))
	}

	data := &storeData{
		ref:      ref,
		config:   cfg,
		repo:     repoInstance,
		svgCache: cache,
	}
	s.storeDataMap[ref] = data
	return data, nil
}

// updateStoreData updates the data stored for data.ref with a new storeData that holds the given repo.
// The SVG cache of the new storeData is empty. Other values are copied from the given data.
//
// Callers of this method MUST ensure that s.mu is already held.
func (s *Server) updateStoreData(data *storeData, repository *repo.Repository) {
	// Create new empty SVG cache
	cache, err := lru.New[string, *svg.Result](s.opts.SVGCacheSize)
	if err != nil {
		panic(fmt.Sprintf("failed to create SVG cache (size: %d): %v", s.opts.SVGCacheSize, err))
	}

	s.storeDataMap[data.ref] = &storeData{
		ref:      data.ref,
		repo:     repository,
		config:   data.config,
		svgCache: cache,
	}
}

// withRequestLogging wraps a handler and logs each request.
// Logs include method, path, remote address, and duration.
func (s *Server) withRequestLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Capture requestURI in case it gets updated by middleware handlers
		urlPath := r.RequestURI
		// Wrap ResponseWriter to capture status code
		lrw := &loggingResponseWriter{ResponseWriter: w}

		next.ServeHTTP(lrw, r)

		duration := time.Since(start)
		log.Printf("%s %s %d %dms (remote=%s)",
			r.Method,
			urlPath,
			lrw.statusCode,
			duration.Milliseconds(),
			r.RemoteAddr,
		)
	})
}

func (s *Server) reloadTemplates() error {
	tmpl := template.New("root")
	tmpl = tmpl.Funcs(map[string]any{
		// These functions get replaced during request processing.
		"toURL":       undefinedTemplateFunction,
		"toEntityURL": undefinedTemplateFunction,
		// "Static" functions
		"markdown":     markdown,
		"formatTags":   formatTags,
		"formatLabels": formatLabels,
		"isCloneable":  isCloneable,
		"hasPlugins": func(e catalog.Entity) bool {
			return s.pluginRegistry != nil && s.pluginRegistry.Matches(e)
		},
		"entitySummary": entitySummary,
		"parentSystem":  parentSystem,
	})
	var err error
	if s.opts.BaseDir == "" {
		s.template, err = tmpl.ParseFS(swcat.Files, "templates/*.html")
	} else {
		s.template, err = tmpl.ParseGlob(path.Join(s.opts.BaseDir, "templates/*.html"))
	}
	return err
}

// svgRenderer returns a new Renderer that uses this server's SVG config
// and dotRunner. The repository has to be passed in; this is typically
// going to be the s.State() obtained at the beginning of request processing.
func (s *Server) svgRenderer(data *storeData) *svg.Renderer {
	layouter := svg.NewStandardLayouter(data.config.SVG)
	return svg.NewRenderer(data.repo, s.dotRunner, layouter)
}

// getStoreData retrieves the storeData from the request's context.
// It panics if no value is found under the expected context key.
func (s *Server) getStoreData(r *http.Request) *storeData {
	sd := r.Context().Value(ctxRefData)
	if sd == nil {
		panic(fmt.Sprintf("storeData called on request without %q context value (URL: %s)", ctxRefData, r.RequestURI))
	}
	return sd.(*storeData)
}

func (s *Server) serveComponents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := q.Get("q")
	data := s.getStoreData(r)
	components := s.finder.FindComponents(data.repo, query)
	params := map[string]any{
		"Components":    components,
		"SearchPath":    toListURLWithContext(r.Context(), catalog.KindComponent),
		"EntitiesLabel": "components",
		"Query":         query,
	}

	if r.Header.Get("HX-Request") == "true" {
		// htmx request: only render rows
		s.serveHTMLPage(w, r, "components_rows.html", params)
		return
	}
	// full page
	s.serveHTMLPage(w, r, "components.html", params)
}

func (s *Server) serveSystems(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := q.Get("q")
	data := s.getStoreData(r)

	systems := s.finder.FindSystems(data.repo, query)
	params := map[string]any{
		"Systems":       systems,
		"SearchPath":    toListURLWithContext(r.Context(), catalog.KindSystem),
		"EntitiesLabel": "systems",
		"Query":         query,
	}

	if r.Header.Get("HX-Request") == "true" {
		// htmx request: only render rows
		s.serveHTMLPage(w, r, "systems_rows.html", params)
		return
	}
	// full page
	s.serveHTMLPage(w, r, "systems.html", params)
}

type svgRoutes struct {
	// Maps each fully qualified entity reference to its /ui URL.
	Entities map[string]string `json:"entities"`
}

// svgMetadata is the struct that is rendered as JSON for SVG metadata
// in HTML responses (embedded in a <script> tag).
type svgMetadata struct {
	*dot.SVGGraphMetadata
	Routes svgRoutes `json:"routes"`
}

// Builds the JSON SVG metadata object. This includes both the given SVGGraphMetadata
// as well as "routes", i.e. the /ui URLs for all entities contained in the graph.
func (s *Server) svgMetadataJSON(r *http.Request, svgMeta *dot.SVGGraphMetadata) (template.JS, error) {
	ctx := r.Context()
	entities := make(map[string]string)
	for ref := range svgMeta.Nodes {
		u, err := toURLWithContext(ctx, ref)
		if err != nil {
			return "", fmt.Errorf("could not create URL for %s: %v", ref, err)
		}
		entities[ref] = u
	}
	meta := svgMetadata{
		SVGGraphMetadata: svgMeta,
		Routes: svgRoutes{
			Entities: entities,
		},
	}
	json, err := json.Marshal(meta)
	if err != nil {
		return "", fmt.Errorf("cannot marshal svgMetadata: %v", err)
	}
	return template.JS(json), nil
}

// graphURLFromMetadata builds a graph builder URL from SVG metadata. It filters out
// "Group" entities, as they tend to clutter the graph.
func (s *Server) graphURLFromMetadata(ctx context.Context, meta *dot.SVGGraphMetadata) (string, error) {
	var entities []string
	for refStr := range meta.Nodes {
		ref, err := catalog.ParseRef(refStr)
		if err != nil {
			return "", fmt.Errorf("failed to parse ref %q from SVG metadata: %w", refStr, err)
		}
		if ref.Kind != catalog.KindGroup {
			entities = append(entities, refStr)
		}
	}
	return toGraphURLWithContext(ctx, entities), nil
}

func (s *Server) withDotTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if s.opts.DotTimeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, s.opts.DotTimeout)
}

func (s *Server) serveSystem(w http.ResponseWriter, r *http.Request, systemID string) {
	systemRef, err := catalog.ParseRefAs(catalog.KindSystem, systemID)
	if err != nil {
		http.Error(w, "Invalid systemID", http.StatusBadRequest)
		return
	}
	params := map[string]any{}
	data := s.getStoreData(r)

	system := data.repo.System(systemRef)
	if system == nil {
		http.Error(w, "Invalid system", http.StatusNotFound)
		return
	}
	params["System"] = system

	// Extract neighbor systems from context parameter c=.
	var contextSystems []*catalog.System
	q := r.URL.Query()
	for _, v := range q["c"] {
		ref, err := catalog.ParseRefAs(catalog.KindSystem, v)
		if err != nil {
			continue // Ignore invalid refs
		}
		if c := data.repo.System(ref); c != nil {
			contextSystems = append(contextSystems, c)
		}
	}
	cacheKeyIDs := make([]string, 0, len(contextSystems))
	for _, s := range contextSystems {
		cacheKeyIDs = append(cacheKeyIDs, s.GetRef().String())
	}
	slices.Sort(cacheKeyIDs)
	internalView := r.URL.Query().Get("view") == "internal"
	cacheKey := fmt.Sprintf("%s?%s%t", system.GetRef(), strings.Join(cacheKeyIDs, ","), internalView)
	svgResult, ok := data.lookupSVG(cacheKey)
	if !ok {
		var err error
		ctx, cancel := s.withDotTimeout(r.Context())
		defer cancel()
		renderer := s.svgRenderer(data)
		if internalView {
			svgResult, err = renderer.SystemInternalGraph(ctx, system)
		} else {
			svgResult, err = renderer.SystemExternalGraph(ctx, system, contextSystems)
		}
		if err != nil {
			http.Error(w, "Failed to render SVG", http.StatusInternalServerError)
			log.Printf("Failed to render SVG: %v", err)
			return
		}
		data.storeSVG(cacheKey, svgResult)
	}
	params["SVGTabs"] = []struct {
		Active bool
		Name   string
		Href   string
	}{
		{Active: !internalView, Name: "External", Href: setQueryParam(r, "view", "external").RequestURI()},
		{Active: internalView, Name: "Internal", Href: setQueryParam(r, "view", "internal").RequestURI()},
	}
	params["SVG"] = template.HTML(svgResult.SVG)
	params["SVGMetadataJSON"], err = s.svgMetadataJSON(r, svgResult.Metadata)
	if err != nil {
		http.Error(w, "Failed to create metadata JSON", http.StatusInternalServerError)
		log.Printf("Failed to create metadata JSON: %v", err)
		return
	}

	activeComments, err := s.getActiveComments(system.GetRef())
	if err != nil {
		log.Printf("Failed to get comments for %s: %v", system.GetRef(), err)
	}
	params["Comments"] = activeComments
	params["Entity"] = system.GetRef()

	s.setCustomContent(system, &data.config.UI, params)

	s.serveHTMLPage(w, r, "system_detail.html", params)
}

func (s *Server) serveComponent(w http.ResponseWriter, r *http.Request, componentID string) {
	componentRef, err := catalog.ParseRefAs(catalog.KindComponent, componentID)
	if err != nil {
		http.Error(w, "Invalid componentID", http.StatusBadRequest)
		return
	}
	params := map[string]any{}
	data := s.getStoreData(r)

	component := data.repo.Component(componentRef)
	if component == nil {
		http.Error(w, "Invalid component", http.StatusNotFound)
		return
	}
	params["Component"] = component

	cacheKey := component.GetRef().String()
	svgResult, ok := data.lookupSVG(cacheKey)
	if !ok {
		var err error
		ctx, cancel := s.withDotTimeout(r.Context())
		defer cancel()
		svgResult, err = s.svgRenderer(data).ComponentGraph(ctx, component)
		if err != nil {
			http.Error(w, "Failed to render SVG", http.StatusInternalServerError)
			log.Printf("Failed to render SVG: %v", err)
			return
		}
		data.storeSVG(cacheKey, svgResult)
	}
	params["SVG"] = template.HTML(svgResult.SVG)
	params["SVGMetadataJSON"], err = s.svgMetadataJSON(r, svgResult.Metadata)
	if err != nil {
		http.Error(w, "Failed to create metadata JSON", http.StatusInternalServerError)
		log.Printf("Failed to create metadata JSON: %v", err)
		return
	}
	// Add link to graph explorer
	graphURL, err := s.graphURLFromMetadata(r.Context(), svgResult.Metadata)
	if err != nil {
		log.Printf("Failed to generate graph URL, skipping link: %v", err)
	} else {
		params["GraphURL"] = graphURL
		params["GraphIcon"] = template.HTML(svgIcons["Graph"])
	}

	activeComments, err := s.getActiveComments(component.GetRef())
	if err != nil {
		log.Printf("Failed to get comments for %s: %v", component.GetRef(), err)
	}
	params["Comments"] = activeComments
	params["Entity"] = component.GetRef()

	s.setCustomContent(component, &data.config.UI, params)

	s.serveHTMLPage(w, r, "component_detail.html", params)
}

func (s *Server) serveAPIs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := q.Get("q")
	data := s.getStoreData(r)

	apis := s.finder.FindAPIs(data.repo, query)
	params := map[string]any{
		"APIs":          apis,
		"SearchPath":    toListURLWithContext(r.Context(), catalog.KindAPI),
		"EntitiesLabel": "apis",
		"Query":         query,
	}

	if r.Header.Get("HX-Request") == "true" {
		// htmx request: only render rows
		s.serveHTMLPage(w, r, "apis_rows.html", params)
		return
	}
	// full page
	s.serveHTMLPage(w, r, "apis.html", params)
}

func (s *Server) setCustomContent(e catalog.Entity, cfg *config.UIConfig, params map[string]any) {
	customContent, err := customContentFromAnnotations(e.GetMetadata(), cfg.AnnotationBasedContent)
	if err != nil {
		log.Printf("Invalid custom content for %q: %v", e.GetQName(), err)
		return
	}
	params["CustomContent"] = customContent
}

func (s *Server) serveAPI(w http.ResponseWriter, r *http.Request, apiID string) {
	apiRef, err := catalog.ParseRefAs(catalog.KindAPI, apiID)
	if err != nil {
		http.Error(w, "Invalid apiID", http.StatusBadRequest)
		return
	}
	params := map[string]any{}
	data := s.getStoreData(r)

	ap := data.repo.API(apiRef)
	if ap == nil {
		http.Error(w, "Invalid API", http.StatusNotFound)
		return
	}
	params["API"] = ap

	cacheKey := ap.GetRef().String()
	svgResult, ok := data.lookupSVG(cacheKey)
	if !ok {
		var err error
		ctx, cancel := s.withDotTimeout(r.Context())
		defer cancel()
		svgResult, err = s.svgRenderer(data).APIGraph(ctx, ap)
		if err != nil {
			http.Error(w, "Failed to render SVG", http.StatusInternalServerError)
			log.Printf("Failed to render SVG: %v", err)
			return
		}
		data.storeSVG(cacheKey, svgResult)
	}
	params["SVG"] = template.HTML(svgResult.SVG)
	params["SVGMetadataJSON"], err = s.svgMetadataJSON(r, svgResult.Metadata)
	if err != nil {
		http.Error(w, "Failed to create metadata JSON", http.StatusInternalServerError)
		log.Printf("Failed to create metadata JSON: %v", err)
		return
	}

	// Add link to graph explorer
	graphURL, err := s.graphURLFromMetadata(r.Context(), svgResult.Metadata)
	if err != nil {
		log.Printf("Failed to generate graph URL, skipping link: %v", err)
	} else {
		params["GraphURL"] = graphURL
		params["GraphIcon"] = template.HTML(svgIcons["Graph"])
	}

	activeComments, err := s.getActiveComments(ap.GetRef())
	if err != nil {
		log.Printf("Failed to get comments for %s: %v", ap.GetRef(), err)
	}
	params["Comments"] = activeComments
	params["Entity"] = ap.GetRef()

	s.setCustomContent(ap, &data.config.UI, params)
	s.serveHTMLPage(w, r, "api_detail.html", params)
}

func (s *Server) serveResources(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := q.Get("q")
	data := s.getStoreData(r)

	resources := s.finder.FindResources(data.repo, query)
	params := map[string]any{
		"Resources":     resources,
		"SearchPath":    toListURLWithContext(r.Context(), catalog.KindResource),
		"EntitiesLabel": "resources",
		"Query":         query,
	}

	if r.Header.Get("HX-Request") == "true" {
		// htmx request: only render rows
		s.serveHTMLPage(w, r, "resources_rows.html", params)
		return
	}
	// full page
	s.serveHTMLPage(w, r, "resources.html", params)
}

func (s *Server) serveResource(w http.ResponseWriter, r *http.Request, resourceID string) {
	resourceRef, err := catalog.ParseRefAs(catalog.KindResource, resourceID)
	if err != nil {
		http.Error(w, "Invalid resourceID", http.StatusBadRequest)
		return
	}
	data := s.getStoreData(r)

	params := map[string]any{}
	resource := data.repo.Resource(resourceRef)
	if resource == nil {
		http.Error(w, "Invalid resource", http.StatusNotFound)
		return
	}
	params["Resource"] = resource

	cacheKey := resource.GetRef().String()
	svgResult, ok := data.lookupSVG(cacheKey)
	if !ok {
		var err error
		ctx, cancel := s.withDotTimeout(r.Context())
		defer cancel()
		svgResult, err = s.svgRenderer(data).ResourceGraph(ctx, resource)
		if err != nil {
			http.Error(w, "Failed to render SVG", http.StatusInternalServerError)
			log.Printf("Failed to render SVG: %v", err)
			return
		}
		data.storeSVG(cacheKey, svgResult)
	}
	params["SVG"] = template.HTML(svgResult.SVG)
	params["SVGMetadataJSON"], err = s.svgMetadataJSON(r, svgResult.Metadata)
	if err != nil {
		http.Error(w, "Failed to create metadata JSON", http.StatusInternalServerError)
		log.Printf("Failed to create metadata JSON: %v", err)
		return
	}
	// Add link to graph explorer
	graphURL, err := s.graphURLFromMetadata(r.Context(), svgResult.Metadata)
	if err != nil {
		log.Printf("Failed to generate graph URL, skipping link: %v", err)
	} else {
		params["GraphURL"] = graphURL
		params["GraphIcon"] = template.HTML(svgIcons["Graph"])
	}

	activeComments, err := s.getActiveComments(resource.GetRef())
	if err != nil {
		log.Printf("Failed to get comments for %s: %v", resource.GetRef(), err)
	}
	params["Comments"] = activeComments
	params["Entity"] = resource.GetRef()

	s.setCustomContent(resource, &data.config.UI, params)

	s.serveHTMLPage(w, r, "resource_detail.html", params)
}

func (s *Server) serveDomains(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := q.Get("q")
	data := s.getStoreData(r)

	domains := s.finder.FindDomains(data.repo, query)
	params := map[string]any{
		"Domains":       domains,
		"SearchPath":    toListURLWithContext(r.Context(), catalog.KindDomain),
		"EntitiesLabel": "domains",
		"Query":         query,
	}

	if r.Header.Get("HX-Request") == "true" {
		// htmx request: only render rows
		s.serveHTMLPage(w, r, "domains_rows.html", params)
		return
	}
	// full page
	s.serveHTMLPage(w, r, "domains.html", params)
}

func (s *Server) serveDomain(w http.ResponseWriter, r *http.Request, domainID string) {
	domainRef, err := catalog.ParseRefAs(catalog.KindDomain, domainID)
	if err != nil {
		http.Error(w, "Invalid domainID", http.StatusBadRequest)
		return
	}
	params := map[string]any{}
	data := s.getStoreData(r)

	domain := data.repo.Domain(domainRef)
	if domain == nil {
		http.Error(w, "Invalid domain", http.StatusNotFound)
		return
	}
	params["Domain"] = domain

	cacheKey := domain.GetRef().String()
	svgResult, ok := data.lookupSVG(cacheKey)
	if !ok {
		var err error
		ctx, cancel := s.withDotTimeout(r.Context())
		defer cancel()
		svgResult, err = s.svgRenderer(data).DomainGraph(ctx, domain)
		if err != nil {
			http.Error(w, "Failed to render SVG", http.StatusInternalServerError)
			log.Printf("Failed to render SVG: %v", err)
			return
		}
		data.storeSVG(cacheKey, svgResult)
	}
	params["SVG"] = template.HTML(svgResult.SVG)
	params["SVGMetadataJSON"], err = s.svgMetadataJSON(r, svgResult.Metadata)
	if err != nil {
		http.Error(w, "Failed to create metadata JSON", http.StatusInternalServerError)
		log.Printf("Failed to create metadata JSON: %v", err)
		return
	}

	activeComments, err := s.getActiveComments(domain.GetRef())
	if err != nil {
		log.Printf("Failed to get comments for %s: %v", domain.GetRef(), err)
	}
	params["Comments"] = activeComments
	params["Entity"] = domain.GetRef()

	s.setCustomContent(domain, &data.config.UI, params)

	s.serveHTMLPage(w, r, "domain_detail.html", params)
}

func (s *Server) serveGroups(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := q.Get("q")
	data := s.getStoreData(r)

	groups := s.finder.FindGroups(data.repo, query)
	params := map[string]any{
		"Groups":        groups,
		"SearchPath":    toListURLWithContext(r.Context(), catalog.KindGroup),
		"EntitiesLabel": "groups",
		"Query":         query,
	}

	if r.Header.Get("HX-Request") == "true" {
		// htmx request: only render rows
		s.serveHTMLPage(w, r, "groups_rows.html", params)
		return
	}
	// full page
	s.serveHTMLPage(w, r, "groups.html", params)
}

func (s *Server) serveGroup(w http.ResponseWriter, r *http.Request, groupID string) {
	groupRef, err := catalog.ParseRefAs(catalog.KindGroup, groupID)
	if err != nil {
		http.Error(w, "Invalid groupID", http.StatusBadRequest)
		return
	}
	params := map[string]any{}
	data := s.getStoreData(r)

	group := data.repo.Group(groupRef)
	if group == nil {
		http.Error(w, "Invalid group", http.StatusNotFound)
		return
	}
	params["Group"] = group

	activeComments, err := s.getActiveComments(group.GetRef())
	if err != nil {
		log.Printf("Failed to get comments for %s: %v", group.GetRef(), err)
	}
	params["Comments"] = activeComments
	params["Entity"] = group.GetRef()

	s.setCustomContent(group, &data.config.UI, params)

	s.serveHTMLPage(w, r, "group_detail.html", params)
}

func (s *Server) serveGraph(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	data := s.getStoreData(r)

	// Process e= query parameters to get selected entities.
	var hiddenParams []map[string]string
	var selectedEntities []catalog.Entity
	selectedIDs := make(map[string]bool)
	for _, e := range q["e"] {
		hiddenParams = append(hiddenParams, map[string]string{"Name": "e", "Value": e})
		if ref, err := catalog.ParseRef(e); err == nil {
			if entity := data.repo.Entity(ref); entity != nil {
				selectedEntities = append(selectedEntities, entity)
				selectedIDs[entity.GetRef().String()] = true
			}
		}
	}

	// Retrieve entities matching query q=, filtering out already selected ones.
	var entities []catalog.Entity
	query := q.Get("q")
	if query != "" {
		for _, e := range s.finder.FindEntities(data.repo, query) {
			if !selectedIDs[e.GetRef().String()] {
				entities = append(entities, e)
			}
		}
	}

	if r.Header.Get("HX-Request") == "true" {
		// htmx request: only render search result rows
		s.serveHTMLPage(w, r, "graph_rows.html", map[string]any{
			"Entities": entities,
		})
		return
	}

	params := map[string]any{
		"Entities":         entities,
		"SelectedEntities": selectedEntities,
		"SearchPath":       uiURLWithContext(r.Context(), "graph"),
		"EntitiesLabel":    "entities",
		"Query":            query,
		"HiddenParams":     hiddenParams,
	}

	if len(selectedEntities) > 0 {
		cacheKeyIDs := make([]string, 0, len(selectedEntities))
		for _, e := range selectedEntities {
			cacheKeyIDs = append(cacheKeyIDs, e.GetRef().String())
		}
		slices.Sort(cacheKeyIDs)
		cacheKey := fmt.Sprintf("graph?ids=%s", strings.Join(cacheKeyIDs, ","))
		svgResult, ok := data.lookupSVG(cacheKey)
		if !ok {
			var err error
			ctx, cancel := s.withDotTimeout(r.Context())
			defer cancel()
			svgResult, err = s.svgRenderer(data).Graph(ctx, selectedEntities)
			if err != nil {
				log.Printf("Failed to render SVG: %v", err)
				// Don't fail the whole page, just don't show SVG
			} else {
				data.storeSVG(cacheKey, svgResult)
			}
		}
		if svgResult != nil {
			params["SVG"] = template.HTML(svgResult.SVG)
			jsonMeta, err := s.svgMetadataJSON(r, svgResult.Metadata)
			if err != nil {
				log.Printf("Failed to create metadata JSON: %v", err)
			} else {
				params["SVGMetadataJSON"] = jsonMeta
			}
		}
	}

	// full page
	s.serveHTMLPage(w, r, "graph.html", params)
}

func (s *Server) serveEntities(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := q.Get("q")
	data := s.getStoreData(r)

	entities := s.finder.FindEntities(data.repo, query)
	params := map[string]any{
		"Entities":      entities,
		"SearchPath":    uiURLWithContext(r.Context(), "entities"),
		"EntitiesLabel": "entities",
		"Query":         query,
	}

	if r.Header.Get("HX-Request") == "true" {
		// htmx request: only render rows
		s.serveHTMLPage(w, r, "entities_rows.html", params)
		return
	}
	// full page
	s.serveHTMLPage(w, r, "entities.html", params)
}

func (s *Server) serveEntityYAML(w http.ResponseWriter, r *http.Request, entityRef string, templateFile string) {
	ref, err := catalog.ParseRef(entityRef)
	if err != nil {
		http.Error(w, "Invalid entity ref", http.StatusBadRequest)
		return
	}
	params := map[string]any{}
	data := s.getStoreData(r)

	entity := data.repo.Entity(ref)
	if entity == nil {
		http.Error(w, "Invalid entity", http.StatusNotFound)
		return
	}
	params["Entity"] = entity

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(store.YAMLIndent)
	if err := enc.Encode(entity.GetSourceInfo().Node); err != nil {
		http.Error(w, "Failed to get YAML", http.StatusInternalServerError)
		log.Printf("Failed to encode YAML for %q: %v", entityRef, err)
		return
	}
	params["YAML"] = buf.String()

	s.serveHTMLPage(w, r, templateFile, params)
}

func (s *Server) serveEntityClone(w http.ResponseWriter, r *http.Request, entityRef string) {
	s.serveEntityYAML(w, r, entityRef, "entity_clone.html")
}

func (s *Server) serveEntityEdit(w http.ResponseWriter, r *http.Request, entityRef string) {
	s.serveEntityYAML(w, r, entityRef, "entity_edit.html")
}

func (s *Server) serveEntityDelete(w http.ResponseWriter, r *http.Request, entityRef string) {
	s.serveEntityYAML(w, r, entityRef, "entity_delete.html")
}

func (s *Server) isHX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

func (s *Server) renderErrorSnippet(w http.ResponseWriter, errorMsg string) {
	w.Header().Set("Content-Type", "text/html; charset=UTF-8")
	// Clone the template before executing it here as well (even though we don't
	// add any request-scoped Funcs), b/c otherwise we cannot clone it anymore:
	// We'd get the error
	//
	// Failed to clone template: html/template: cannot Clone "root" after it has executed
	tmpl, err := s.template.Clone()
	if err != nil {
		log.Printf("Failed to clone template: %v", err)
		http.Error(w, "Template clone error", http.StatusInternalServerError)
		return
	}

	err = tmpl.ExecuteTemplate(w, "error_message.html", map[string]any{
		"Error": errorMsg,
	})
	if err != nil {
		log.Printf("Failed to render error message: %v", err)
	}
}

func (s *Server) createEntity(w http.ResponseWriter, r *http.Request) {
	if !s.isHX(r) {
		http.Error(w, "Entity updates must be done via HTMX", http.StatusBadRequest)
		return
	}
	if s.opts.ReadOnly {
		http.Error(w, "Cannot update entities in read-only mode", http.StatusPreconditionFailed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	clonedFrom := r.FormValue("cloned_from")
	clonedRef, err := catalog.ParseRef(clonedFrom)
	if err != nil {
		http.Error(w, "Invalid entity reference", http.StatusBadRequest)
		return
	}
	data := s.getStoreData(r)
	clonedEntity := data.repo.Entity(clonedRef)
	if clonedEntity == nil {
		http.Error(w, "Invalid entity", http.StatusNotFound)
		return
	}

	newYAML := r.FormValue("yaml")
	newAPIEntity, err := api.NewEntityFromString(newYAML)
	if err != nil {
		s.renderErrorSnippet(w, fmt.Sprintf("Failed to parse new YAML: %v", err))
		return
	}
	// Copy over path: the cloned entity will be stored in the same file.
	path := clonedEntity.GetSourceInfo().Path
	newAPIEntity.GetSourceInfo().Path = path

	newEntity, err := catalog.NewEntityFromAPI(newAPIEntity)
	if err != nil {
		s.renderErrorSnippet(w, fmt.Sprintf("Invalid entity: %v", err))
		return
	}

	if data.repo.Exists(newEntity) {
		s.renderErrorSnippet(w, fmt.Sprintf("Entity %s already exists", newEntity.GetRef()))
		return
	}

	newRepo, err := data.repo.InsertOrUpdateEntity(newEntity)
	if err != nil {
		s.renderErrorSnippet(w, fmt.Sprintf("Failed to insert entity into repo: %v", err))
		return
	}

	// Acquire write lock
	s.mu.Lock()
	defer s.mu.Unlock()

	// Update repository in server's cache
	s.updateStoreData(data, newRepo)

	// Update the YAML file.
	st, err := s.source.Store(data.ref)
	if err != nil {
		http.Error(w, "Failed to obtain store", http.StatusInternalServerError)
		log.Printf("Failed to obtain store for ref %q: %v", data.ref, err)
		return
	}
	err = store.InsertOrReplaceEntity(st, path, newAPIEntity)
	if err != nil {
		http.Error(w, "Failed to update store", http.StatusInternalServerError)
		log.Printf("Error updating store: %v", err)
		return
	}

	redirectURL, err := toURLWithContext(r.Context(), newEntity.GetRef())
	if err != nil {
		// This must not happen: we must always be able to get a URL for our own entities.
		panic(fmt.Sprintf("Failed to create entityURL for valid entity reference: %v", err))
	}

	w.Header().Set("HX-Redirect", redirectURL)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) deleteEntity(w http.ResponseWriter, r *http.Request, entityRef string) {
	if !s.isHX(r) {
		http.Error(w, "Entity updates must be done via HTMX", http.StatusBadRequest)
		return
	}
	if s.opts.ReadOnly {
		http.Error(w, "Cannot update entities in read-only mode", http.StatusPreconditionFailed)
		return
	}

	ref, err := catalog.ParseRef(entityRef)
	if err != nil {
		http.Error(w, "Invalid entity reference", http.StatusBadRequest)
		return
	}

	data := s.getStoreData(r)
	entity := data.repo.Entity(ref)
	if entity == nil {
		http.Error(w, "Invalid entity", http.StatusNotFound)
		return
	}

	// Update the repo
	newRepo, err := data.repo.DeleteEntity(entity.GetRef())
	if err != nil {
		s.renderErrorSnippet(w, fmt.Sprintf("Failed to delete entity from repo: %v", err))
		return
	}

	// Acquire write lock
	s.mu.Lock()
	defer s.mu.Unlock()

	// Update repository in server's cache
	s.updateStoreData(data, newRepo)

	// Update the YAML file.
	st, err := s.source.Store(data.ref)
	if err != nil {
		http.Error(w, "Failed to obtain store", http.StatusInternalServerError)
		log.Printf("Failed to obtain store for ref %q: %v", data.ref, err)
		return
	}
	err = store.DeleteEntity(st, entity.GetSourceInfo().Path, catalog.APIRef(ref))
	if err != nil {
		http.Error(w, "Failed to update store", http.StatusInternalServerError)
		log.Printf("Error updating store: %v", err)
		return
	}

	// Redirect to parent system, if it exists. Else redirect to list view.
	var redirectURL string
	if sp, ok := entity.(catalog.SystemPart); ok {
		redirectURL, err = toURLWithContext(r.Context(), sp.GetSystem())
	} else {
		redirectURL = toListURLWithContext(r.Context(), entity.GetKind())
	}
	if err != nil {
		// This must not happen: we must always be able to get a URL for our own entities.
		http.Error(w, "Failed to create entity URL", http.StatusInternalServerError)
		panic(fmt.Sprintf("Failed to create entity URL for valid entity reference: %v", err))
	}
	w.Header().Set("HX-Redirect", redirectURL)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) runPlugins(w http.ResponseWriter, r *http.Request, entityRef string) {
	if s.pluginRegistry == nil {
		http.Error(w, "No plugin registry available", http.StatusPreconditionFailed)
		return
	}
	if !s.isHX(r) {
		http.Error(w, "Plugin execution must be done via HTMX", http.StatusBadRequest)
		return
	}
	if s.opts.ReadOnly {
		http.Error(w, "Cannot run plugins in read-only mode", http.StatusPreconditionFailed)
		return
	}

	ref, err := catalog.ParseRef(entityRef)
	if err != nil {
		http.Error(w, "Invalid entity reference", http.StatusBadRequest)
		return
	}

	data := s.getStoreData(r)
	entity := data.repo.Entity(ref)
	if entity == nil {
		http.Error(w, "Invalid entity", http.StatusNotFound)
		return
	}

	exts, err := s.pluginRegistry.Run(r.Context(), entity)
	if err != nil {
		message := fmt.Sprintf("Failed to run plugins: %v", err)
		log.Println(message)
		s.renderErrorSnippet(w, message)
		return
	}

	// Acquire write lock
	s.mu.Lock()
	defer s.mu.Unlock()

	// Store new extensions on disk and clear storeDataMap, forcing a reload of the repository on the next request.

	// Read extensions file and merge new extensions.
	st, err := s.source.Store(data.ref)
	if err != nil {
		s.renderErrorSnippet(w, fmt.Sprintf("Failed to access store: %v", err))
		return
	}
	extPath := store.ExtensionFile(entity.GetSourceInfo().Path)
	existingExts, err := store.ReadExtensions(st, extPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		s.renderErrorSnippet(w, fmt.Sprintf("Failed to read extensions: %v", err))
		return
	}
	if existingExts == nil {
		existingExts = &api.CatalogExtensions{}
	}

	existingExts.Merge(exts)

	// Write extensions file to disk.
	if err := store.WriteExtensions(st, extPath, existingExts); err != nil {
		s.renderErrorSnippet(w, fmt.Sprintf("Failed to write extensions: %v", err))
		return
	}

	// Clear data, which forces a reload on the next request.
	err = s.source.Refresh()
	if err != nil {
		s.renderErrorSnippet(w, fmt.Sprintf("Failed to refresh: %v", err))
		return
	}
	s.storeDataMap = make(map[string]*storeData)

	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusOK)
}

func (s *Server) updateEntity(w http.ResponseWriter, r *http.Request, entityRef string) {
	if !s.isHX(r) {
		http.Error(w, "Entity updates must be done via HTMX", http.StatusBadRequest)
		return
	}
	if s.opts.ReadOnly {
		http.Error(w, "Cannot update entities in read-only mode", http.StatusPreconditionFailed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	ref, err := catalog.ParseRef(entityRef)
	if err != nil {
		http.Error(w, "Invalid entity reference", http.StatusBadRequest)
		return
	}

	data := s.getStoreData(r)
	originalEntity := data.repo.Entity(ref)
	if originalEntity == nil {
		http.Error(w, "Invalid entity", http.StatusNotFound)
		return
	}

	newYAML := r.FormValue("yaml")
	newAPIEntity, err := api.NewEntityFromString(newYAML)
	if err != nil {
		s.renderErrorSnippet(w, fmt.Sprintf("Failed to parse new YAML: %v", err))
		return
	}
	newEntity, err := catalog.NewEntityFromAPI(newAPIEntity)
	if err != nil {
		s.renderErrorSnippet(w, fmt.Sprintf("Invalid entity: %v", err))
		return
	}

	// Only update if the entity reference remains the same, i.e.:
	// - no changes of the kind, namespace, or name
	if !newEntity.GetRef().Equal(originalEntity.GetRef()) {
		errMsg := fmt.Sprintf("Updated entity ID does not match original (old: %q, new: %q)",
			newEntity.GetRef(), originalEntity.GetRef())
		s.renderErrorSnippet(w, errMsg)
		return
	}

	// Acquire write lock
	s.mu.Lock()
	defer s.mu.Unlock()

	// Update the repo
	newRepo, err := data.repo.InsertOrUpdateEntity(newEntity)
	if err != nil {
		s.renderErrorSnippet(w, fmt.Sprintf("Failed to update entity in repo: %v", err))
		return
	}
	// Copy over path information for re-editing later.
	path := originalEntity.GetSourceInfo().Path
	newEntity.GetSourceInfo().Path = path

	// Update repository in server's cache
	s.updateStoreData(data, newRepo)

	// Update the YAML file.
	st, err := s.source.Store(data.ref)
	if err != nil {
		http.Error(w, "Failed to obtain store", http.StatusInternalServerError)
		log.Printf("Failed to obtain store for ref %q: %v", data.ref, err)
		return
	}
	err = store.InsertOrReplaceEntity(st, path, newAPIEntity)
	if err != nil {
		http.Error(w, "Failed to update store", http.StatusInternalServerError)
		log.Printf("Error updating store: %v", err)
		return
	}

	redirectURL, err := toURLWithContext(r.Context(), ref)
	if err != nil {
		// This must not happen: we must always be able to get a URL for our own entities.
		panic(fmt.Sprintf("Failed to create entityURL for valid entity reference: %v", err))
	}

	w.Header().Set("HX-Redirect", redirectURL)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) updateAnnotationValue(w http.ResponseWriter, r *http.Request, entityRefStr string, annotationKey string) {
	if s.opts.ReadOnly {
		http.Error(w, "Cannot update entities in read-only mode", http.StatusPreconditionFailed)
		return
	}

	// Read the new value from the request body.
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusInternalServerError)
		return
	}
	newValue := string(body)

	// Only proceed if this is a valid annotation
	if !catalog.IsValidAnnotation(annotationKey, newValue) {
		http.Error(w, "Invalid annotation key/value", http.StatusBadRequest)
		return
	}

	// Parse the entity reference string
	ref, err := catalog.ParseRef(entityRefStr)
	if err != nil {
		http.Error(w, "Invalid entity reference", http.StatusBadRequest)
		return
	}

	// Look up the entity in the repository
	data := s.getStoreData(r)
	originalEntity := data.repo.Entity(ref)
	if originalEntity == nil {
		http.Error(w, "Entity not found", http.StatusNotFound)
		return
	}

	// Create new API entity with updated annotation
	originalNode := originalEntity.GetSourceInfo().Node
	newAPIEntity, err := api.NewEntityFromNodeWithAnnotation(originalNode, annotationKey, newValue)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create api.Entity from source node: %v", err), http.StatusInternalServerError)
		return
	}

	// Create a new catalog.Entity for repo update
	newEntity, err := catalog.NewEntityFromAPI(newAPIEntity)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid entity after modification: %v", err), http.StatusInternalServerError)
		return
	}
	// Copy over path information for re-editing later.
	path := originalEntity.GetSourceInfo().Path
	newEntity.GetSourceInfo().Path = path

	// Update the repo
	newRepo, err := data.repo.InsertOrUpdateEntity(newEntity)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to update entity in repo: %v", err), http.StatusInternalServerError)
		return
	}

	// Acquire write lock
	s.mu.Lock()
	defer s.mu.Unlock()

	// Update repository in server's cache
	s.updateStoreData(data, newRepo)

	// Update the YAML file.
	st, err := s.source.Store(data.ref)
	if err != nil {
		http.Error(w, "Failed to obtain store", http.StatusInternalServerError)
		log.Printf("Failed to obtain store for ref %q: %v", data.ref, err)
		return
	}
	err = store.InsertOrReplaceEntity(st, path, newAPIEntity)
	if err != nil {
		http.Error(w, "Failed to update store", http.StatusInternalServerError)
		log.Printf("Error updating store: %v", err)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "OK")
}

// Extracts the entity ref from a Referer header.
// Example: extracts "component:availability-aggregator" from
// http://localhost:9191/ui/entities/component:availability-aggregator/edit
func entityRefFromReferer(referer string) (*catalog.Ref, error) {
	// 1. Parse the URL to ensure we are safely handling schemes, hosts, and query params.
	u, err := url.Parse(referer)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %v", err)
	}

	pathSegments := strings.Split(u.EscapedPath(), "/")

	// 3. Iterate through segments to find the ref after "entities".
	var refStr string
	for i, seg := range pathSegments {
		if seg == "entities" {
			// Ensure there is actually a segment following "entities"
			if i+1 < len(pathSegments) {
				var err error
				refStr, err = url.PathUnescape(pathSegments[i+1])
				if err != nil {
					return nil, fmt.Errorf("failed to unescape entity ref from path: %v", err)
				}
			}
		}
	}
	if refStr == "" {
		return nil, fmt.Errorf("no entity ref found in path")
	}
	return catalog.ParseRef(refStr)
}

func (s *Server) serveAutocomplete(w http.ResponseWriter, r *http.Request) {
	field := r.URL.Query().Get("field")
	if strings.TrimSpace(field) == "" {
		http.Error(w, "Missing or empty field= query parameter", http.StatusBadRequest)
		return
	}
	// Determine entity ref from Referer header.
	referer := r.Header.Get("Referer")
	ref, err := entityRefFromReferer(referer)
	if err != nil {
		http.Error(w, fmt.Sprintf("Could not retrieve entity ref from Referer: %s: %v", referer, err), http.StatusBadRequest)
		return
	}
	data := s.getStoreData(r)
	// Get completions for requested field.
	fieldType := "plain"
	var completions []string
	switch field {
	case "metadata.annotations":
		fieldType = "key"
		completions = data.repo.AnnotationKeys(ref.Kind)
	case "metadata.labels":
		fieldType = "key"
		completions = data.repo.LabelKeys(ref.Kind)
	case "spec.consumesApis", "spec.providesApis":
		fieldType = "item"
		apis := s.finder.FindAPIs(data.repo, "")
		completions = make([]string, len(apis))
		for i, a := range apis {
			completions[i] = a.GetRef().QName()
		}
	case "spec.dependsOn":
		fieldType = "item"
		entities := s.finder.FindEntities(data.repo, "kind:component OR kind:resource")
		completions = make([]string, len(entities))
		for i, a := range entities {
			// Use fully qualified refs including the kind for dependsOn.
			completions[i] = a.GetRef().String()
		}
	case "spec.owner":
		fieldType = "value"
		groups := s.finder.FindGroups(data.repo, "")
		completions = make([]string, len(groups))
		for i, g := range groups {
			completions[i] = g.GetRef().QName()
		}
	case "spec.system":
		fieldType = "value"
		systems := s.finder.FindSystems(data.repo, "")
		completions = make([]string, len(systems))
		for i, s := range systems {
			completions[i] = s.GetRef().QName()
		}
	case "spec.lifecycle", "spec.type":
		fieldType = "value"
		_, fieldName, _ := strings.Cut(field, ".")
		var err error
		completions, err = data.repo.SpecFieldValues(ref.Kind, fieldName)
		if err != nil {
			http.Error(w,
				fmt.Sprintf("Cannot get values for kind %v and field %s: %v", ref.Kind, field, err),
				http.StatusBadRequest)
			return
		}
	}
	slices.Sort(completions)

	// Send completion list back in response.
	res, err := json.Marshal(map[string]any{
		"fieldType":   fieldType,
		"completions": completions,
	})
	if err != nil {
		log.Printf("Failed to encode completions as JSON: %v", err)
		http.Error(w, "JSON marshalling error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.Write(res)
}

func (s *Server) serveHTMLPage(w http.ResponseWriter, r *http.Request, templateFile string, params map[string]any) {
	var output bytes.Buffer

	nav := NewNavBar(
		NavItem(toListURLWithContext(r.Context(), catalog.KindDomain), "Domains"),
		NavItem(toListURLWithContext(r.Context(), catalog.KindSystem), "Systems"),
		NavItem(toListURLWithContext(r.Context(), catalog.KindComponent), "Components"),
		NavItem(toListURLWithContext(r.Context(), catalog.KindResource), "Resources"),
		NavItem(toListURLWithContext(r.Context(), catalog.KindAPI), "APIs"),
		NavItem(toListURLWithContext(r.Context(), catalog.KindGroup), "Groups"),
		NavIcon(uiURLWithContext(r.Context(), "graph"), "Graph"),
		NavIcon(uiURLWithContext(r.Context(), "entities"), "Search"),
	).SetActive(r.RequestURI)

	templateParams := map[string]any{
		"Now":             time.Now().Format("2006-01-02 15:04:05"),
		"NavBar":          nav,
		"ReadOnly":        s.opts.ReadOnly,
		"CommentsEnabled": s.commentsEnabled(),
		"CacheBustingKey": s.started.Format("20060102150405"),
		"Version":         s.opts.Version,
	}
	// Add Refs if we're running on a Git source.
	if g, ok := s.source.(*store.GitSource); ok {
		refs, err := g.ListReferences()
		if err != nil {
			log.Printf("Failed to list references: %v", err)
		} else {
			currentRef := g.DefaultRef()
			if v := r.Context().Value(ctxRef); v != nil {
				currentRef = v.(string)
			}
			templateParams["RefOptions"] = refOptions(refs, currentRef, r)
		}
	}
	// Copy over provided params
	for k, v := range params {
		templateParams[k] = v
	}

	// Add HelpLink from config if available
	if v := r.Context().Value(ctxRefData); v != nil {
		if sd, ok := v.(*storeData); ok && sd.config != nil {
			templateParams["HelpLink"] = sd.config.UI.HelpLink
		}
	}

	// Clone template so we can safely update Funcs on a per-request basis.
	tmpl, err := s.template.Clone()
	if err != nil {
		log.Printf("Failed to clone template: %v", err)
		http.Error(w, "Template clone error", http.StatusInternalServerError)
		return
	}
	// Overwrite URL-functions with context-aware analogs.
	tmpl = tmpl.Funcs(map[string]any{
		"toURL": func(s any) (string, error) {
			return toURLWithContext(r.Context(), s)
		},
		"toEntityURL": func(s any) (string, error) {
			return toEntityURLWithContext(r.Context(), s)
		},
	})

	err = tmpl.ExecuteTemplate(&output, templateFile, templateParams)
	if err != nil {
		log.Printf("Failed to render template %q: %v", templateFile, err)
		http.Error(w, "Template rendering error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=UTF-8")
	w.Write(output.Bytes())
}

func (s *Server) serveEntitiesJSON(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := q.Get("q")
	data := s.getStoreData(r)
	entities := s.finder.FindEntities(data.repo, query)

	result := make([]map[string]any, 0, len(entities))
	for _, e := range entities {
		if e.GetSourceInfo() == nil || e.GetSourceInfo().Node == nil {
			continue
		}
		var data map[string]any
		if err := e.GetSourceInfo().Node.Decode(&data); err != nil {
			log.Printf("Failed to decode yaml.Node to map: %v", err)
			http.Error(w, "JSON marshalling error", http.StatusInternalServerError)
			return
		}
		result = append(result, data)
	}

	output, err := json.Marshal(map[string]any{
		"entities": result,
	})
	if err != nil {
		log.Printf("Failed to encode map as JSON: %v", err)
		http.Error(w, "JSON marshalling error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.Write(output)
}

func (s *Server) reloadCatalog(w http.ResponseWriter, r *http.Request) {
	// Acquire write lock
	s.mu.Lock()
	defer s.mu.Unlock()

	started := time.Now()
	err := s.source.Refresh()
	if err != nil {
		log.Printf("Failed to refresh store: %v", err)
		s.renderErrorSnippet(w, fmt.Sprintf("Failed to refresh: %v", err))
		return
	}
	log.Printf("Store refreshed in %d ms.", time.Since(started).Milliseconds())

	// Clear repo cache
	s.storeDataMap = make(map[string]*storeData)

	// Force HTMX to refresh the page
	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusOK)
}

func (s *Server) ValidateCatalog(ref string) (size int, err error) {
	rd, err := s.loadStoreData(ref)
	if err != nil {
		return 0, fmt.Errorf("validation failed for %q: %v", ref, err)
	}
	return rd.repo.Size(), nil
}

// contextKey is the type used to store data in the request context.
type contextKey string

const (
	// ctxRef is the context key for the git reference (e.g., branch)
	// accessed by the current request.
	ctxRef contextKey = "ref"
	// ctxRefData is the context key for the reference data (repo, config, etc.)
	// accessed by the current request.
	ctxRefData contextKey = "refData"
)

func (s *Server) uiMux() *http.ServeMux {
	mux := http.NewServeMux()

	// Domains / Systems / Components / Resources / APIs pages
	mux.HandleFunc("GET /domains", func(w http.ResponseWriter, r *http.Request) {
		s.serveDomains(w, r)
	})
	mux.HandleFunc("GET /domains/{domainID...}", func(w http.ResponseWriter, r *http.Request) {
		domainID := r.PathValue("domainID")
		s.serveDomain(w, r, domainID)
	})
	mux.HandleFunc("GET /systems", func(w http.ResponseWriter, r *http.Request) {
		s.serveSystems(w, r)
	})
	mux.HandleFunc("GET /systems/{systemID...}", func(w http.ResponseWriter, r *http.Request) {
		systemID := r.PathValue("systemID")
		s.serveSystem(w, r, systemID)
	})
	mux.HandleFunc("GET /components", func(w http.ResponseWriter, r *http.Request) {
		s.serveComponents(w, r)
	})
	mux.HandleFunc("GET /components/{componentID...}", func(w http.ResponseWriter, r *http.Request) {
		componentID := r.PathValue("componentID")
		s.serveComponent(w, r, componentID)
	})
	mux.HandleFunc("GET /resources", func(w http.ResponseWriter, r *http.Request) {
		s.serveResources(w, r)
	})
	mux.HandleFunc("GET /resources/{resourceID...}", func(w http.ResponseWriter, r *http.Request) {
		resourceID := r.PathValue("resourceID")
		s.serveResource(w, r, resourceID)
	})
	mux.HandleFunc("GET /apis", func(w http.ResponseWriter, r *http.Request) {
		s.serveAPIs(w, r)
	})
	mux.HandleFunc("GET /apis/{apiID...}", func(w http.ResponseWriter, r *http.Request) {
		apiID := r.PathValue("apiID")
		s.serveAPI(w, r, apiID)
	})
	mux.HandleFunc("GET /groups", func(w http.ResponseWriter, r *http.Request) {
		s.serveGroups(w, r)
	})
	mux.HandleFunc("GET /groups/{groupID...}", func(w http.ResponseWriter, r *http.Request) {
		groupID := r.PathValue("groupID")
		s.serveGroup(w, r, groupID)
	})

	mux.HandleFunc("GET /graph", func(w http.ResponseWriter, r *http.Request) {
		s.serveGraph(w, r)
	})

	// Generic entities URLs
	mux.HandleFunc("GET /entities", func(w http.ResponseWriter, r *http.Request) {
		s.serveEntities(w, r)
	})
	mux.HandleFunc("POST /entities", func(w http.ResponseWriter, r *http.Request) {
		s.createEntity(w, r)
	})
	mux.Handle("/entities/", http.StripPrefix("/entities/", http.HandlerFunc(s.dispatchEntityRequest)))

	return mux
}

// dispatchEntityRequest dispatches /entities/<ref>/<method> requests.
// The reason we don't do this directly via handler patterns is that <ref>
// may include a slash (e.g., "external/entity"), and path escaping "/" is
// brittle in the presence of middleware handlers and reverse proxies.
func (s *Server) dispatchEntityRequest(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	if strings.HasSuffix(path, "/edit") {
		entityRef := strings.TrimSuffix(path, "/edit")
		if r.Method == http.MethodPost {
			s.updateEntity(w, r, entityRef)
		} else {
			s.serveEntityEdit(w, r, entityRef)
		}
		return
	}

	if strings.HasSuffix(path, "/clone") {
		entityRef := strings.TrimSuffix(path, "/clone")
		s.serveEntityClone(w, r, entityRef)
		return
	}

	if strings.HasSuffix(path, "/delete") {
		entityRef := strings.TrimSuffix(path, "/delete")
		if r.Method == http.MethodPost {
			s.deleteEntity(w, r, entityRef)
		} else {
			s.serveEntityDelete(w, r, entityRef)
		}
		return
	}

	if strings.HasSuffix(path, "/run-plugins") {
		entityRef := strings.TrimSuffix(path, "/run-plugins")
		if r.Method == http.MethodPost {
			s.runPlugins(w, r, entityRef)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	if strings.HasSuffix(path, "/comments") {
		entityRef := strings.TrimSuffix(path, "/comments")
		s.serveComments(w, r, entityRef)
		return
	}

	http.NotFound(w, r)
}

func (s *Server) getActiveComments(ref *catalog.Ref) ([]comments.Comment, error) {
	if !s.commentsEnabled() {
		return nil, nil
	}
	return s.commentsStore.GetOpenComments(ref.String())
}

func (s *Server) serveComments(w http.ResponseWriter, r *http.Request, entityRefStr string) {
	if !s.commentsEnabled() {
		http.Error(w, "Comments are disabled", http.StatusForbidden)
		return
	}
	ref, err := catalog.ParseRef(entityRefStr)
	if err != nil {
		http.Error(w, "Invalid entity reference", http.StatusBadRequest)
		return
	}

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Failed to parse form", http.StatusBadRequest)
			return
		}

		// Check if this is a resolve request
		if id := r.FormValue("resolve"); id != "" {
			if err := s.commentsStore.ResolveComment(ref.String(), id); err != nil {
				http.Error(w, fmt.Sprintf("Failed to resolve comment: %v", err), http.StatusInternalServerError)
				return
			}
			log.Printf("Comment %s resolved for %s", id, ref.String())
			// Fall through to render updated snippet
		} else {
			text := r.FormValue("text")
			author := r.FormValue("author")
			if text == "" {
				http.Error(w, "Comment text cannot be empty", http.StatusBadRequest)
				return
			}
			if len(text) > comments.MaxTextLength {
				http.Error(w, fmt.Sprintf("Comment text too long (max %d characters)", comments.MaxTextLength), http.StatusBadRequest)
				return
			}
			if len(author) > comments.MaxAuthorLength {
				http.Error(w, fmt.Sprintf("Author name too long (max %d characters)", comments.MaxAuthorLength), http.StatusBadRequest)
				return
			}
			if author == "" {
				author = "Anonymous"
			}

			// Generate a simple unique ID
			id := fmt.Sprintf("%d-%d", time.Now().UnixNano(), r.Context().Value(ctxRefData).(*storeData).repo.Size())

			comment := comments.Comment{
				ID:        id,
				Author:    author,
				Text:      text,
				CreatedAt: time.Now(),
			}

			if err := s.commentsStore.AddComment(ref.String(), comment); err != nil {
				http.Error(w, fmt.Sprintf("Failed to add comment: %v", err), http.StatusInternalServerError)
				return
			}
			log.Printf("Comment added to %s by %s", ref.String(), author)
			// Fall through to render updated snippet
		}
	}

	activeComments, err := s.getActiveComments(ref)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get comments: %v", err), http.StatusInternalServerError)
		return
	}

	s.serveHTMLPage(w, r, "comments.html", map[string]any{
		"Comments": activeComments,
		"Entity":   ref,
	})
}

// handleRefDispatch expects to handle a /ui/ref/<git-ref>/-/<rest> URL path,
// injects <git-ref> into the request context under the ctxRef key, and
// delegates to next with the request path updated to /<rest>.
func (s *Server) handleRefDispatch(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Expect incoming: /ui/ref/feature/my-branch/-/domains/123
		if !strings.HasPrefix(r.URL.Path, "/ui/ref/") {
			log.Fatalf("Called handleRefDispatch with wrong prefix: %s", r.URL.Path)
		}

		rest := strings.TrimPrefix(r.URL.Path, "/ui/ref/")

		// Split at /-/ delimiter that terminates the git branch/tag path segment.
		ref, targetPath, found := strings.Cut(rest, "/-/")

		if !found {
			http.NotFound(w, r)
			return
		}

		// Update URL.Path so the child handler considers the targetPath as the root.
		// Ensure it starts with "/".
		r.URL.Path = "/" + strings.TrimPrefix(targetPath, "/")

		// Inject ref into Context and dispatch to child handler.
		ctx := context.WithValue(r.Context(), ctxRef, ref)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// handleRefDataDispatch injects the relevant information for ref into the request context
// and delegates to the given next handler.
func (s *Server) handleRefDataDispatch(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ref := ""
		// See if reference has been stored in the context, else use default
		if v := r.Context().Value(ctxRef); v != nil {
			ref = v.(string)
		}
		// Retrieve data for ref.
		rd, err := s.loadStoreData(ref)
		if errors.Is(err, store.ErrNoSuchRef) {
			http.NotFound(w, r)
			return
		} else if err != nil {
			// Show detailed error in response to user as well: most likely, they selected
			// a branch whose catalog cannot be parsed.
			msg := fmt.Sprintf("Failed to get store for ref %q: %v", ref, err)
			log.Println(msg)
			http.Error(w, msg, http.StatusInternalServerError)
			return
		}

		// Inject refData into Context and dispatch to child handler.
		ctx := context.WithValue(r.Context(), ctxRefData, rd)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// A handler that adds the Cache-Control header for aggressive caching.
func cacheControlHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	refs := make([]string, 0, len(s.storeDataMap))
	for k := range s.storeDataMap {
		refs = append(refs, k)
	}
	slices.Sort(refs)

	var registeredPlugins []string
	if s.pluginRegistry != nil {
		registeredPlugins = s.pluginRegistry.Plugins()
		slices.Sort(registeredPlugins)
	}

	status := map[string]any{
		"options":           s.opts,
		"cachedRefs":        refs,
		"storeSourceType":   fmt.Sprintf("%T", s.source),
		"started":           s.started,
		"registeredPlugins": registeredPlugins,
	}

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(status); err != nil {
		log.Printf("Failed to encode status as JSON: %v", err)
		http.Error(w, "JSON marshalling error", http.StatusInternalServerError)
	}
}

func (s *Server) routes() *http.ServeMux {
	root := http.NewServeMux()

	// UI part: everything under /ui
	uiMux := s.uiMux()
	root.Handle("/ui/", http.StripPrefix("/ui", s.handleRefDataDispatch(uiMux)))
	root.Handle("/ui/ref/", s.handleRefDispatch(s.handleRefDataDispatch(uiMux)))

	// JSON API to query/modify entities.
	// For now, only works on "plain" URLs, not /ref/<git-branch ones.
	root.Handle("GET /catalog/entities", s.handleRefDataDispatch(http.HandlerFunc(s.serveEntitiesJSON)))

	root.Handle("POST /catalog/entities/{entityRef}/annotations/{annotationKey}", s.handleRefDataDispatch(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			entityRef := r.PathValue("entityRef")
			annotationKey := r.PathValue("annotationKey")
			s.updateAnnotationValue(w, r, entityRef, annotationKey)
		})))
	root.Handle("GET /catalog/autocomplete", s.handleRefDataDispatch(http.HandlerFunc(s.serveAutocomplete)))

	root.HandleFunc("POST /catalog/reload", s.reloadCatalog)

	// Health check. Useful for cloud deployments.
	root.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "OK")
	})

	// Status page. Useful for inspecting hosted server instances.
	root.Handle("/status", http.HandlerFunc(s.handleStatus))

	// Static resources (JavaScript, CSS, etc.)
	if s.opts.BaseDir == "" {
		root.Handle("GET /static/", cacheControlHandler(http.FileServer(http.FS(swcat.Files))))
	} else {
		staticFS := http.Dir(path.Join(s.opts.BaseDir, "static"))
		root.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(staticFS)))
	}

	// Default route (all other paths): redirect to the UI home page
	root.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "", http.StatusBadRequest)
			return
		}
		if r.Header.Get("Hx-Request") != "" {
			// Do not redirect htmx requests, those should only request valid paths.
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == "/" || r.URL.Path == "/ui" {
			// Redirect GET to the UI home page.
			http.Redirect(w, r, "/ui/components", http.StatusTemporaryRedirect)
			return
		}
		http.NotFound(w, r)
	})

	return root
}

// Serve starts the HTTP server on s.opts.Addr using the wrapped handler.
func (s *Server) Serve() error {
	handler := s.Handler()
	log.Printf("Go server listening on http://%s", s.opts.Addr)
	return http.ListenAndServe(s.opts.Addr, handler)
}

func (s *Server) Handler() http.Handler {
	return s.withRequestLogging(s.routes())
}
