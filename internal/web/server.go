package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dnswlt/swcat"
	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/config"
	"github.com/dnswlt/swcat/internal/dot"
	"github.com/dnswlt/swcat/internal/repo"
	"github.com/dnswlt/swcat/internal/store"
	"github.com/dnswlt/swcat/internal/svg"
	"gopkg.in/yaml.v3"
)

type ServerOptions struct {
	Addr     string        // E.g., "localhost:8080"
	BaseDir  string        // Directory from which resources (templates etc.) are read.
	DotPath  string        // E.g., "dot" (with dot on the PATH)
	ReadOnly bool          // If true, no Edit/Clone/Delete operations will be supported.
	Config   config.Bundle // Config parameters
	Version  string        // App version
}

// ServerState contains the state of the server:
// - Repository
// - SVG cache
// While the ServerState is not immutable due to the SVG cache,
// it contains all state that is swapped atomically on any updates
// to the repository (reloads, entity updates, etc.).
type ServerState struct {
	repo     *repo.Repository
	svgCache sync.Map // mutated during requests, hence sync'ed.
}

type Server struct {
	opts     ServerOptions
	template *template.Template
	// Mutex to be acquired by write requests that update more than
	// just the state (e.g. write to disk).
	writeMu sync.Mutex
	// Atomic pointer to this server's state.
	// We use the Snapshot Isolation pattern for atomic state updates.
	// Request processing code obtains the whole state pointer, works on it,
	// and calls SetState to update the state atomically, if needed.
	state     atomic.Pointer[ServerState]
	dotRunner dot.Runner
	// Server startup time. Used for cache busting JS/CSS resources.
	started time.Time
}

func NewServer(opts ServerOptions, repo *repo.Repository) (*Server, error) {
	if opts.Version == "" {
		opts.Version = "dev"
	}
	s := &Server{
		opts:      opts,
		dotRunner: dot.NewRunner(opts.DotPath),
		started:   time.Now(),
	}
	s.SetState(&ServerState{repo: repo})

	if err := s.reloadTemplates(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Server) State() *ServerState {
	return s.state.Load()
}

func (s *Server) SetState(state *ServerState) {
	s.state.Store(state)
}

func (s *ServerState) lookupSVG(cacheKey string) (*svg.Result, bool) {
	item, ok := s.svgCache.Load(cacheKey)
	if !ok {
		return nil, false
	}
	return item.(*svg.Result), true
}

func (s *ServerState) storeSVG(cacheKey string, svg *svg.Result) {
	s.svgCache.Store(cacheKey, svg)
}

type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

func (lrw *loggingResponseWriter) Write(b []byte) (int, error) {
	if lrw.statusCode == 0 { // no explicit status yet => implies 200
		lrw.WriteHeader(http.StatusOK)
	}
	return lrw.ResponseWriter.Write(b)
}

// svgRenderer returns a new Renderer that uses this server's SVG config
// and dotRunner. The repository has to be passed in; this is typically
// going to be the s.State() obtained at the beginning of request processing.
func (s *Server) svgRenderer(r *repo.Repository) *svg.Renderer {
	layouter := svg.NewStandardLayouter(s.opts.Config.SVG)
	return svg.NewRenderer(r, s.dotRunner, layouter)
}

// withRequestLogging wraps a handler and logs each request if in debug mode.
// Logs include method, path, remote address, and duration.
func (s *Server) withRequestLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Capture path as it might get updated by middleware handlers
		urlPath := r.URL.Path
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
		"markdown":      markdown,
		"formatTags":    formatTags,
		"formatLabels":  formatLabels,
		"isCloneable":   isCloneable,
		"entitySummary": entitySummary,
	})
	var err error
	if s.opts.BaseDir == "" {
		s.template, err = tmpl.ParseFS(swcat.Files, "templates/*.html")
	} else {
		s.template, err = tmpl.ParseGlob(path.Join(s.opts.BaseDir, "templates/*.html"))
	}
	return err
}

func (s *Server) serveComponents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := q.Get("q")
	components := s.State().repo.FindComponents(query)
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
	systems := s.State().repo.FindSystems(query)
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

func (s *Server) serveSystem(w http.ResponseWriter, r *http.Request, systemID string) {
	systemRef, err := catalog.ParseRefAs(catalog.KindSystem, systemID)
	if err != nil {
		http.Error(w, "Invalid systemID", http.StatusBadRequest)
		return
	}
	params := map[string]any{}
	state := s.State()
	system := state.repo.System(systemRef)
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
		if c := state.repo.System(ref); c != nil {
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
	svgResult, ok := state.lookupSVG(cacheKey)
	if !ok {
		var err error
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		renderer := s.svgRenderer(state.repo)
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
		state.storeSVG(cacheKey, svgResult)
	}
	params["SVGTabs"] = []struct {
		Active bool
		Name   string
		Href   string
	}{
		{Active: !internalView, Name: "External", Href: setQueryParam(r.URL, "view", "external").RequestURI()},
		{Active: internalView, Name: "Internal", Href: setQueryParam(r.URL, "view", "internal").RequestURI()},
	}
	params["SVG"] = template.HTML(svgResult.SVG)
	params["SVGMetadataJSON"], err = s.svgMetadataJSON(r, svgResult.Metadata)
	if err != nil {
		http.Error(w, "Failed to create metadata JSON", http.StatusInternalServerError)
		log.Printf("Failed to create metadata JSON: %v", err)
		return
	}
	s.setCustomContent(system, params)

	s.serveHTMLPage(w, r, "system_detail.html", params)
}

func (s *Server) serveComponent(w http.ResponseWriter, r *http.Request, componentID string) {
	componentRef, err := catalog.ParseRefAs(catalog.KindComponent, componentID)
	if err != nil {
		http.Error(w, "Invalid componentID", http.StatusBadRequest)
		return
	}
	params := map[string]any{}
	state := s.State()
	component := state.repo.Component(componentRef)
	if component == nil {
		http.Error(w, "Invalid component", http.StatusNotFound)
		return
	}
	params["Component"] = component

	cacheKey := component.GetRef().String()
	svgResult, ok := state.lookupSVG(cacheKey)
	if !ok {
		var err error
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		svgResult, err = s.svgRenderer(state.repo).ComponentGraph(ctx, component)
		if err != nil {
			http.Error(w, "Failed to render SVG", http.StatusInternalServerError)
			log.Printf("Failed to render SVG: %v", err)
			return
		}
		state.storeSVG(cacheKey, svgResult)
	}
	params["SVG"] = template.HTML(svgResult.SVG)
	params["SVGMetadataJSON"], err = s.svgMetadataJSON(r, svgResult.Metadata)
	if err != nil {
		http.Error(w, "Failed to create metadata JSON", http.StatusInternalServerError)
		log.Printf("Failed to create metadata JSON: %v", err)
		return
	}
	s.setCustomContent(component, params)

	s.serveHTMLPage(w, r, "component_detail.html", params)
}

func (s *Server) serveAPIs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := q.Get("q")
	state := s.State()
	apis := state.repo.FindAPIs(query)
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

func (s *Server) setCustomContent(e catalog.Entity, params map[string]any) {
	customContent, err := customContentFromAnnotations(e.GetMetadata(), s.opts.Config.UI.AnnotationBasedContent)
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
	state := s.State()
	ap := state.repo.API(apiRef)
	if ap == nil {
		http.Error(w, "Invalid API", http.StatusNotFound)
		return
	}
	params["API"] = ap

	cacheKey := ap.GetRef().String()
	svgResult, ok := state.lookupSVG(cacheKey)
	if !ok {
		var err error
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		svgResult, err = s.svgRenderer(state.repo).APIGraph(ctx, ap)
		if err != nil {
			http.Error(w, "Failed to render SVG", http.StatusInternalServerError)
			log.Printf("Failed to render SVG: %v", err)
			return
		}
		state.storeSVG(cacheKey, svgResult)
	}
	params["SVG"] = template.HTML(svgResult.SVG)
	params["SVGMetadataJSON"], err = s.svgMetadataJSON(r, svgResult.Metadata)
	if err != nil {
		http.Error(w, "Failed to create metadata JSON", http.StatusInternalServerError)
		log.Printf("Failed to create metadata JSON: %v", err)
		return
	}

	s.setCustomContent(ap, params)
	s.serveHTMLPage(w, r, "api_detail.html", params)
}

func (s *Server) serveResources(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := q.Get("q")
	state := s.State()
	resources := state.repo.FindResources(query)
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
	state := s.State()
	params := map[string]any{}
	resource := state.repo.Resource(resourceRef)
	if resource == nil {
		http.Error(w, "Invalid resource", http.StatusNotFound)
		return
	}
	params["Resource"] = resource

	cacheKey := resource.GetRef().String()
	svgResult, ok := state.lookupSVG(cacheKey)
	if !ok {
		var err error
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		svgResult, err = s.svgRenderer(state.repo).ResourceGraph(ctx, resource)
		if err != nil {
			http.Error(w, "Failed to render SVG", http.StatusInternalServerError)
			log.Printf("Failed to render SVG: %v", err)
			return
		}
		state.storeSVG(cacheKey, svgResult)
	}
	params["SVG"] = template.HTML(svgResult.SVG)
	params["SVGMetadataJSON"], err = s.svgMetadataJSON(r, svgResult.Metadata)
	if err != nil {
		http.Error(w, "Failed to create metadata JSON", http.StatusInternalServerError)
		log.Printf("Failed to create metadata JSON: %v", err)
		return
	}
	s.setCustomContent(resource, params)

	s.serveHTMLPage(w, r, "resource_detail.html", params)
}

func (s *Server) serveDomains(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := q.Get("q")
	state := s.State()
	domains := state.repo.FindDomains(query)
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
	state := s.State()
	domain := state.repo.Domain(domainRef)
	if domain == nil {
		http.Error(w, "Invalid domain", http.StatusNotFound)
		return
	}
	params["Domain"] = domain

	cacheKey := domain.GetRef().String()
	svgResult, ok := state.lookupSVG(cacheKey)
	if !ok {
		var err error
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		svgResult, err = s.svgRenderer(state.repo).DomainGraph(ctx, domain)
		if err != nil {
			http.Error(w, "Failed to render SVG", http.StatusInternalServerError)
			log.Printf("Failed to render SVG: %v", err)
			return
		}
		state.storeSVG(cacheKey, svgResult)
	}
	params["SVG"] = template.HTML(svgResult.SVG)
	params["SVGMetadataJSON"], err = s.svgMetadataJSON(r, svgResult.Metadata)
	if err != nil {
		http.Error(w, "Failed to create metadata JSON", http.StatusInternalServerError)
		log.Printf("Failed to create metadata JSON: %v", err)
		return
	}
	s.setCustomContent(domain, params)

	s.serveHTMLPage(w, r, "domain_detail.html", params)
}

func (s *Server) serveGroups(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := q.Get("q")
	state := s.State()
	groups := state.repo.FindGroups(query)
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
	state := s.State()
	group := state.repo.Group(groupRef)
	if group == nil {
		http.Error(w, "Invalid group", http.StatusNotFound)
		return
	}
	params["Group"] = group
	s.setCustomContent(group, params)

	s.serveHTMLPage(w, r, "group_detail.html", params)
}

func (s *Server) serveEntities(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := q.Get("q")
	state := s.State()
	entities := state.repo.FindEntities(query)
	params := map[string]any{
		"Entities":      entities,
		"SearchPath":    toEntitiesListURL(r.Context()),
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
	state := s.State()
	entity := state.repo.Entity(ref)
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
	err := s.template.ExecuteTemplate(w, "error_message.html", map[string]any{
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

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

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
	state := s.State()
	clonedEntity := state.repo.Entity(clonedRef)
	if clonedEntity == nil {
		http.Error(w, "Invalid entity", http.StatusNotFound)
		return
	}

	newYAML := r.FormValue("yaml")
	newAPIEntity, err := store.NewEntityFromString(newYAML)
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

	if state.repo.Exists(newEntity) {
		s.renderErrorSnippet(w, fmt.Sprintf("Entity %s already exists", newEntity.GetRef()))
		return
	}

	newRepo, err := state.repo.InsertOrUpdateEntity(newEntity)
	if err != nil {
		s.renderErrorSnippet(w, fmt.Sprintf("Failed to insert entity into repo: %v", err))
		return
	}

	// Update the YAML file.
	if err := store.InsertOrReplaceEntity(newRepo.Store(), path, newAPIEntity); err != nil {
		http.Error(w, "Failed to update store", http.StatusInternalServerError)
		log.Printf("Error updating store: %v", err)
		return
	}

	// Update server's in-memory state
	s.SetState(&ServerState{repo: newRepo})

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

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	ref, err := catalog.ParseRef(entityRef)
	if err != nil {
		http.Error(w, "Invalid entity reference", http.StatusBadRequest)
		return
	}

	state := s.State()
	entity := state.repo.Entity(ref)
	if entity == nil {
		http.Error(w, "Invalid entity", http.StatusNotFound)
		return
	}

	// Update the repo
	newRepo, err := state.repo.DeleteEntity(entity.GetRef())
	if err != nil {
		s.renderErrorSnippet(w, fmt.Sprintf("Failed to delete entity from repo: %v", err))
		return
	}

	// Update the YAML file.
	apiRef := catalog.APIRef(ref)
	if err := store.DeleteEntity(newRepo.Store(), entity.GetSourceInfo().Path, apiRef); err != nil {
		http.Error(w, "Failed to update store", http.StatusInternalServerError)
		log.Printf("Error updating store: %v", err)
		return
	}

	s.SetState(&ServerState{repo: newRepo})

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

func (s *Server) updateEntity(w http.ResponseWriter, r *http.Request, entityRef string) {
	if !s.isHX(r) {
		http.Error(w, "Entity updates must be done via HTMX", http.StatusBadRequest)
		return
	}
	if s.opts.ReadOnly {
		http.Error(w, "Cannot update entities in read-only mode", http.StatusPreconditionFailed)
		return
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	ref, err := catalog.ParseRef(entityRef)
	if err != nil {
		http.Error(w, "Invalid entity reference", http.StatusBadRequest)
		return
	}

	state := s.State()
	originalEntity := state.repo.Entity(ref)
	if originalEntity == nil {
		http.Error(w, "Invalid entity", http.StatusNotFound)
		return
	}

	newYAML := r.FormValue("yaml")
	newAPIEntity, err := store.NewEntityFromString(newYAML)
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

	// Update the repo
	newRepo, err := state.repo.InsertOrUpdateEntity(newEntity)
	if err != nil {
		s.renderErrorSnippet(w, fmt.Sprintf("Failed to update entity in repo: %v", err))
		return
	}

	// Copy over path information for re-editing later.
	path := originalEntity.GetSourceInfo().Path
	newEntity.GetSourceInfo().Path = path

	// Update the YAML file.
	if err := store.InsertOrReplaceEntity(newRepo.Store(), path, newAPIEntity); err != nil {
		http.Error(w, "Failed to update store", http.StatusInternalServerError)
		log.Printf("Error updating store: %v", err)
		return
	}

	// Update server's in-memory state.
	s.SetState(&ServerState{repo: newRepo})

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

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

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
	state := s.State()
	originalEntity := state.repo.Entity(ref)
	if originalEntity == nil {
		http.Error(w, "Entity not found", http.StatusNotFound)
		return
	}

	// Copy and modify the source YAML node
	sourceNode, err := store.CopyNode(originalEntity.GetSourceInfo().Node)
	if sourceNode == nil || err != nil {
		http.Error(w, "Entity could not be copied for modification", http.StatusInternalServerError)
		return
	}
	if err := store.SetAnnotationInNode(sourceNode, annotationKey, newValue); err != nil {
		http.Error(w, fmt.Sprintf("Failed to set annotation in YAML node: %v", err), http.StatusInternalServerError)
		return
	}

	newAPIEntity, err := store.NewEntityFromNode(sourceNode, false)
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
	newRepo, err := state.repo.InsertOrUpdateEntity(newEntity)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to update entity in repo: %v", err), http.StatusInternalServerError)
		return
	}

	// Update the YAML file.
	if err := store.InsertOrReplaceEntity(newRepo.Store(), path, newAPIEntity); err != nil {
		http.Error(w, "Failed to update store", http.StatusInternalServerError)
		log.Printf("Error updating store: %v", err)
		return
	}

	// Update server's in-memory state.
	s.SetState(&ServerState{repo: newRepo})

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
	state := s.State()
	// Get completions for requested field.
	fieldType := "plain"
	var completions []string
	switch field {
	case "metadata.annotations":
		fieldType = "key"
		completions = state.repo.AnnotationKeys(ref.Kind)
	case "metadata.labels":
		fieldType = "key"
		completions = state.repo.LabelKeys(ref.Kind)
	case "spec.consumesApis", "spec.providesApis":
		fieldType = "item"
		apis := state.repo.FindAPIs("")
		completions = make([]string, len(apis))
		for i, a := range apis {
			completions[i] = a.GetRef().QName()
		}
	case "spec.dependsOn":
		fieldType = "item"
		entities := state.repo.FindEntities("kind:component OR kind:resource")
		completions = make([]string, len(entities))
		for i, a := range entities {
			// Use fully qualified refs including the kind for dependsOn.
			completions[i] = a.GetRef().String()
		}
	case "spec.owner":
		fieldType = "value"
		groups := state.repo.FindGroups("")
		completions = make([]string, len(groups))
		for i, g := range groups {
			completions[i] = g.GetRef().QName()
		}
	case "spec.system":
		fieldType = "value"
		systems := state.repo.FindSystems("")
		completions = make([]string, len(systems))
		for i, s := range systems {
			completions[i] = s.GetRef().QName()
		}
	case "spec.lifecycle", "spec.type":
		fieldType = "value"
		_, fieldName, _ := strings.Cut(field, ".")
		var err error
		completions, err = state.repo.SpecFieldValues(ref.Kind, fieldName)
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
		NavItem(toEntitiesListURL(r.Context()), "Search"),
	).SetActive(r.URL.Path).SetParams(r.URL.Query())

	templateParams := map[string]any{
		"Now":             time.Now().Format("2006-01-02 15:04:05"),
		"NavBar":          nav,
		"ReadOnly":        s.opts.ReadOnly,
		"CacheBustingKey": s.started.Format("20060102150405"),
		"Version":         s.opts.Version,
	}
	// Copy template params
	for k, v := range params {
		templateParams[k] = v
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
	state := s.State()
	entities := state.repo.FindEntities(query)

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
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	started := time.Now()
	state := s.State()
	newRepo, err := state.repo.Reload()
	if err != nil {
		log.Printf("Failed to reload catalog: %v", err)
		s.renderErrorSnippet(w, fmt.Sprintf("Failed to reload catalog: %v", err))
		return
	}
	log.Printf("Catalog reloaded in %d ms.", time.Since(started).Milliseconds())
	s.SetState(&ServerState{repo: newRepo})

	// Force HTMX to refresh the page
	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusOK)
}

// contextKey is the type used to store data in the request context.
type contextKey string

const (
	// ctxRef is the context key for the git reference (e.g., branch)
	// accessed by the current request.
	ctxRef contextKey = "gitRef"
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

	http.NotFound(w, r)
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

		// Inject Ref into Context and dispath to child handler.
		ctx := context.WithValue(r.Context(), ctxRef, ref)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) routes() *http.ServeMux {
	root := http.NewServeMux()

	// UI part: everything under /ui
	uiMux := s.uiMux()
	root.Handle("/ui/", http.StripPrefix("/ui", uiMux))
	root.Handle("/ui/ref/", s.handleRefDispatch(uiMux))

	// JSON API to query/modify entities.
	// For now, only works on "plain" URLs, not /ref/<git-branch ones.
	root.HandleFunc("GET /catalog/entities", func(w http.ResponseWriter, r *http.Request) {
		s.serveEntitiesJSON(w, r)
	})
	root.HandleFunc("POST /catalog/entities/{entityRef}/annotations/{annotationKey}", func(w http.ResponseWriter, r *http.Request) {
		entityRef := r.PathValue("entityRef")
		annotationKey := r.PathValue("annotationKey")
		s.updateAnnotationValue(w, r, entityRef, annotationKey)
	})
	root.HandleFunc("GET /catalog/autocomplete", func(w http.ResponseWriter, r *http.Request) {
		s.serveAutocomplete(w, r)
	})
	root.HandleFunc("POST /catalog/reload", s.reloadCatalog)

	// Health check. Useful for cloud deployments.
	root.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "OK")
	})

	// Static resources (JavaScript, CSS, etc.)
	if s.opts.BaseDir == "" {
		root.Handle("GET /static/", http.FileServer(http.FS(swcat.Files)))
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
