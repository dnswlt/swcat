package web

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/dnswlt/swcat"
	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/dot"
	"github.com/dnswlt/swcat/internal/repo"
	"github.com/dnswlt/swcat/internal/store"
	"github.com/dnswlt/swcat/internal/svg"
	"gopkg.in/yaml.v3"
)

type ServerOptions struct {
	Addr    string // E.g., "localhost:8080"
	BaseDir string // Directory from which resources (templates etc.) are read.
	DotPath string // E.g., "dot" (with dot on the PATH)
}

type Server struct {
	opts        ServerOptions
	template    *template.Template
	repo        *repo.Repository
	svgCache    map[string]*svg.Result
	svgCacheMut sync.Mutex
	dotRunner   dot.Runner
}

func NewServer(opts ServerOptions, repo *repo.Repository) (*Server, error) {
	s := &Server{
		opts:      opts,
		repo:      repo,
		svgCache:  make(map[string]*svg.Result),
		dotRunner: dot.NewRunner(opts.DotPath),
	}
	if err := s.reloadTemplates(); err != nil {
		return nil, err
	}
	return s, nil
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

func (s *Server) lookupSVG(cacheKey string) (*svg.Result, bool) {
	s.svgCacheMut.Lock()
	defer s.svgCacheMut.Unlock()
	svg, ok := s.svgCache[cacheKey]
	return svg, ok
}
func (s *Server) storeSVG(cacheKey string, svg *svg.Result) {
	s.svgCacheMut.Lock()
	defer s.svgCacheMut.Unlock()
	s.svgCache[cacheKey] = svg
}
func (s *Server) clearSVGCache() {
	s.svgCacheMut.Lock()
	defer s.svgCacheMut.Unlock()
	s.svgCache = make(map[string]*svg.Result)
}

// withRequestLogging wraps a handler and logs each request if in debug mode.
// Logs include method, path, remote address, and duration.
func (s *Server) withRequestLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap ResponseWriter to capture status code
		lrw := &loggingResponseWriter{ResponseWriter: w}

		next.ServeHTTP(lrw, r)

		duration := time.Since(start)
		log.Printf("%s %s %d %dms (remote=%s)",
			r.Method,
			r.URL.Path,
			lrw.statusCode,
			duration.Milliseconds(),
			r.RemoteAddr,
		)
	})
}

func (s *Server) reloadTemplates() error {
	tmpl := template.New("root")
	tmpl = tmpl.Funcs(map[string]any{
		"toURL":     toURL,
		"refEncode": refEncode,
		"urlencode": urlencode,
		"markdown":  markdown,
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
	params := map[string]any{}
	q := r.URL.Query()
	components := s.repo.FindComponents(q.Get("q"))
	params["Components"] = components

	if r.Header.Get("HX-Request") == "true" {
		// htmx request: only render rows
		s.serveHTMLPage(w, r, "components_rows.html", params)
		return
	}
	// full page
	s.serveHTMLPage(w, r, "components.html", params)
}

func (s *Server) serveSystems(w http.ResponseWriter, r *http.Request) {
	params := map[string]any{}
	q := r.URL.Query()
	systems := s.repo.FindSystems(q.Get("q"))
	params["Systems"] = systems

	if r.Header.Get("HX-Request") == "true" {
		// htmx request: only render rows
		s.serveHTMLPage(w, r, "systems_rows.html", params)
		return
	}
	// full page
	s.serveHTMLPage(w, r, "systems.html", params)
}

func (s *Server) serveSystem(w http.ResponseWriter, r *http.Request, systemID string) {
	systemRef, err := catalog.ParseRefAs(catalog.KindSystem, systemID)
	if err != nil {
		http.Error(w, "Invalid systemID", http.StatusBadRequest)
		return
	}
	params := map[string]any{}
	system := s.repo.System(systemRef)
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
		if c := s.repo.System(ref); c != nil {
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
	svgResult, ok := s.lookupSVG(cacheKey)
	if !ok {
		var err error
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if internalView {
			svgResult, err = svg.SystemInternalGraph(ctx, s.dotRunner, s.repo, system)
		} else {
			svgResult, err = svg.SystemExternalGraph(ctx, s.dotRunner, s.repo, system, contextSystems)
		}
		if err != nil {
			http.Error(w, "Failed to render SVG", http.StatusInternalServerError)
			log.Printf("Failed to render SVG: %v", err)
			return
		}
		s.storeSVG(cacheKey, svgResult)
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
	params["SVGMetadataJSON"] = template.JS(svgResult.MetadataJSON())
	s.serveHTMLPage(w, r, "system_detail.html", params)
}

func (s *Server) serveComponent(w http.ResponseWriter, r *http.Request, componentID string) {
	componentRef, err := catalog.ParseRefAs(catalog.KindComponent, componentID)
	if err != nil {
		http.Error(w, "Invalid componentID", http.StatusBadRequest)
		return
	}
	params := map[string]any{}
	component := s.repo.Component(componentRef)
	if component == nil {
		http.Error(w, "Invalid component", http.StatusNotFound)
		return
	}
	params["Component"] = component

	cacheKey := component.GetRef().String()
	svgResult, ok := s.lookupSVG(cacheKey)
	if !ok {
		var err error
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		svgResult, err = svg.ComponentGraph(ctx, s.dotRunner, s.repo, component)
		if err != nil {
			http.Error(w, "Failed to render SVG", http.StatusInternalServerError)
			log.Printf("Failed to render SVG: %v", err)
			return
		}
		s.storeSVG(cacheKey, svgResult)
	}
	params["SVG"] = template.HTML(svgResult.SVG)
	params["SVGMetadataJSON"] = template.JS(svgResult.MetadataJSON())

	s.serveHTMLPage(w, r, "component_detail.html", params)
}

func (s *Server) serveAPIs(w http.ResponseWriter, r *http.Request) {
	params := map[string]any{}
	q := r.URL.Query()
	apis := s.repo.FindAPIs(q.Get("q"))
	params["APIs"] = apis

	if r.Header.Get("HX-Request") == "true" {
		// htmx request: only render rows
		s.serveHTMLPage(w, r, "apis_rows.html", params)
		return
	}
	// full page
	s.serveHTMLPage(w, r, "apis.html", params)
}

func (s *Server) serveAPI(w http.ResponseWriter, r *http.Request, apiID string) {
	apiRef, err := catalog.ParseRefAs(catalog.KindAPI, apiID)
	if err != nil {
		http.Error(w, "Invalid apiID", http.StatusBadRequest)
		return
	}
	params := map[string]any{}
	ap := s.repo.API(apiRef)
	if ap == nil {
		http.Error(w, "Invalid API", http.StatusNotFound)
		return
	}
	params["API"] = ap

	cacheKey := ap.GetRef().String()
	svgResult, ok := s.lookupSVG(cacheKey)
	if !ok {
		var err error
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		svgResult, err = svg.APIGraph(ctx, s.dotRunner, s.repo, ap)
		if err != nil {
			http.Error(w, "Failed to render SVG", http.StatusInternalServerError)
			log.Printf("Failed to render SVG: %v", err)
			return
		}
		s.storeSVG(cacheKey, svgResult)
	}
	params["SVG"] = template.HTML(svgResult.SVG)
	params["SVGMetadataJSON"] = template.JS(svgResult.MetadataJSON())

	s.serveHTMLPage(w, r, "api_detail.html", params)
}

func (s *Server) serveResources(w http.ResponseWriter, r *http.Request) {
	params := map[string]any{}
	q := r.URL.Query()
	resources := s.repo.FindResources(q.Get("q"))
	params["Resources"] = resources

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
	params := map[string]any{}
	resource := s.repo.Resource(resourceRef)
	if resource == nil {
		http.Error(w, "Invalid resource", http.StatusNotFound)
		return
	}
	params["Resource"] = resource

	cacheKey := resource.GetRef().String()
	svgResult, ok := s.lookupSVG(cacheKey)
	if !ok {
		var err error
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		svgResult, err = svg.ResourceGraph(ctx, s.dotRunner, s.repo, resource)
		if err != nil {
			http.Error(w, "Failed to render SVG", http.StatusInternalServerError)
			log.Printf("Failed to render SVG: %v", err)
			return
		}
		s.storeSVG(cacheKey, svgResult)
	}
	params["SVG"] = template.HTML(svgResult.SVG)
	params["SVGMetadataJSON"] = template.JS(svgResult.MetadataJSON())

	s.serveHTMLPage(w, r, "resource_detail.html", params)
}

func (s *Server) serveDomains(w http.ResponseWriter, r *http.Request) {
	params := map[string]any{}
	q := r.URL.Query()
	domains := s.repo.FindDomains(q.Get("q"))
	params["Domains"] = domains

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
	domain := s.repo.Domain(domainRef)
	if domain == nil {
		http.Error(w, "Invalid domain", http.StatusNotFound)
		return
	}
	params["Domain"] = domain

	cacheKey := domain.GetRef().String()
	svgResult, ok := s.lookupSVG(cacheKey)
	if !ok {
		var err error
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		svgResult, err = svg.DomainGraph(ctx, s.dotRunner, s.repo, domain)
		if err != nil {
			http.Error(w, "Failed to render SVG", http.StatusInternalServerError)
			log.Printf("Failed to render SVG: %v", err)
			return
		}
		s.storeSVG(cacheKey, svgResult)
	}
	params["SVG"] = template.HTML(svgResult.SVG)
	params["SVGMetadataJSON"] = template.JS(svgResult.MetadataJSON())

	s.serveHTMLPage(w, r, "domain_detail.html", params)
}

func (s *Server) serveGroups(w http.ResponseWriter, r *http.Request) {
	params := map[string]any{}
	q := r.URL.Query()
	groups := s.repo.FindGroups(q.Get("q"))
	params["Groups"] = groups

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
	group := s.repo.Group(groupRef)
	if group == nil {
		http.Error(w, "Invalid group", http.StatusNotFound)
		return
	}
	params["Group"] = group
	s.serveHTMLPage(w, r, "group_detail.html", params)
}

func (s *Server) serveEntityEdit(w http.ResponseWriter, r *http.Request, entityRef string) {
	ref, err := catalog.ParseRef(entityRef)
	if err != nil {
		http.Error(w, "Invalid domainID", http.StatusBadRequest)
		return
	}
	params := map[string]any{}
	entity := s.repo.Entity(ref)
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

	s.serveHTMLPage(w, r, "entity_edit.html", params)
}

func (s *Server) isHX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

func (s *Server) renderErrorSnippet(w http.ResponseWriter, errorMsg string) {
	w.Header().Set("Content-Type", "text/html; charset=UTF-8")
	s.template.ExecuteTemplate(w, "_error.html", map[string]any{
		"Error": errorMsg,
	})
}

func (s *Server) updateEntity(w http.ResponseWriter, r *http.Request, entityRef string) {
	if !s.isHX(r) {
		http.Error(w, "Entity updates must be done via HTMX", http.StatusBadRequest)
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

	originalEntity := s.repo.Entity(ref)
	if originalEntity == nil {
		http.Error(w, "Invalid entity", http.StatusNotFound)
		return
	}

	newYAML := r.FormValue("yaml")
	newAPIEntity, err := store.ReadEntityFromString(newYAML)
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
	if err := s.repo.UpdateEntity(newEntity); err != nil {
		s.renderErrorSnippet(w, fmt.Sprintf("Failed to update entity in repo: %v", err))
		return
	}
	// Invalidate the SVG cache
	s.clearSVGCache()

	// Copy over path information for re-editing later.
	path := originalEntity.GetSourceInfo().Path
	newEntity.GetSourceInfo().Path = path
	// Update the YAML file.
	entitiesInFile, err := store.ReadEntities(path)
	if err != nil {
		http.Error(w, "Failed to read entity file", http.StatusInternalServerError)
		log.Printf("Failed to read entities from %q: %v", path, err)
		return
	}

	// Find and replace the modified entity in the list of entities read from its path.
	var found bool
	originalRef := catalog.APIRef(originalEntity.GetRef())
	for i, e := range entitiesInFile {
		if e.GetRef().Equal(originalRef) {
			// Replace old with new for writing back to disk
			entitiesInFile[i] = newAPIEntity
			found = true
			break
		}
	}
	if !found {
		http.Error(w, "Could not find entity to update in file", http.StatusInternalServerError)
		log.Printf("Could not find entity to update in its alleged path %q", path)
		return
	}

	if err := store.WriteEntities(path, entitiesInFile); err != nil {
		http.Error(w, "Failed to write updated entity file", http.StatusInternalServerError)
		log.Printf("Failed to write entities to %q: %v", path, err)
		return
	}

	redirectURL, err := toURL(ref)
	if err != nil {
		// This must not happen: we must always be able to get a URL for our own entities.
		log.Fatalf("Failed to create entityURL for valid entity reference: %v", err)
	}

	w.Header().Set("HX-Redirect", redirectURL)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) serveHTMLPage(w http.ResponseWriter, r *http.Request, templateFile string, params map[string]any) {
	var output bytes.Buffer

	nav := NewNavBar(
		NavItem("/ui/domains", "Domains"),
		NavItem("/ui/systems", "Systems"),
		NavItem("/ui/components", "Components"),
		NavItem("/ui/resources", "Resources"),
		NavItem("/ui/apis", "APIs"),
		NavItem("/ui/groups", "Groups"),
	).SetActive(r.URL.Path).SetParams(r.URL.Query())

	templateParams := map[string]any{
		"Now":    time.Now().Format("2006-01-02 15:04:05"),
		"NavBar": nav,
	}
	// Copy template params
	for k, v := range params {
		templateParams[k] = v
	}

	err := s.template.ExecuteTemplate(&output, templateFile, templateParams)
	if err != nil {
		log.Printf("Failed to render template %q: %v", templateFile, err)
		http.Error(w, "Template rendering error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=UTF-8")
	w.Write(output.Bytes())
}

func (s *Server) routes() *http.ServeMux {
	mux := http.NewServeMux()

	// Domains / Systems / Components / Resources / APIs pages
	mux.HandleFunc("GET /ui/domains", func(w http.ResponseWriter, r *http.Request) {
		s.serveDomains(w, r)
	})
	mux.HandleFunc("GET /ui/domains/{domainID}", func(w http.ResponseWriter, r *http.Request) {
		domainID := r.PathValue("domainID")
		s.serveDomain(w, r, domainID)
	})
	mux.HandleFunc("GET /ui/systems", func(w http.ResponseWriter, r *http.Request) {
		s.serveSystems(w, r)
	})
	mux.HandleFunc("GET /ui/systems/{systemID}", func(w http.ResponseWriter, r *http.Request) {
		systemID := r.PathValue("systemID")
		s.serveSystem(w, r, systemID)
	})
	mux.HandleFunc("GET /ui/components", func(w http.ResponseWriter, r *http.Request) {
		s.serveComponents(w, r)
	})
	mux.HandleFunc("GET /ui/components/{componentID}", func(w http.ResponseWriter, r *http.Request) {
		componentID := r.PathValue("componentID")
		s.serveComponent(w, r, componentID)
	})
	mux.HandleFunc("GET /ui/resources", func(w http.ResponseWriter, r *http.Request) {
		s.serveResources(w, r)
	})
	mux.HandleFunc("GET /ui/resources/{resourceID}", func(w http.ResponseWriter, r *http.Request) {
		resourceID := r.PathValue("resourceID")
		s.serveResource(w, r, resourceID)
	})
	mux.HandleFunc("GET /ui/apis", func(w http.ResponseWriter, r *http.Request) {
		s.serveAPIs(w, r)
	})
	mux.HandleFunc("GET /ui/apis/{apiID}", func(w http.ResponseWriter, r *http.Request) {
		apiID := r.PathValue("apiID")
		s.serveAPI(w, r, apiID)
	})
	mux.HandleFunc("GET /ui/groups", func(w http.ResponseWriter, r *http.Request) {
		s.serveGroups(w, r)
	})
	mux.HandleFunc("GET /ui/groups/{groupID}", func(w http.ResponseWriter, r *http.Request) {
		groupID := r.PathValue("groupID")
		s.serveGroup(w, r, groupID)
	})

	mux.HandleFunc("GET /ui/entities/{entityRef}/edit", func(w http.ResponseWriter, r *http.Request) {
		entityRef := r.PathValue("entityRef")
		s.serveEntityEdit(w, r, entityRef)
	})
	mux.HandleFunc("POST /ui/entities/{entityRef}/edit", func(w http.ResponseWriter, r *http.Request) {
		entityRef := r.PathValue("entityRef")
		s.updateEntity(w, r, entityRef)
	})

	// Health check. Useful for cloud deployments.
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "OK")
	})

	// Static resources (JavaScript, CSS, etc.)
	if s.opts.BaseDir == "" {
		mux.Handle("GET /static/", http.FileServer(http.FS(swcat.Files)))
	} else {
		staticFS := http.Dir(path.Join(s.opts.BaseDir, "static"))
		mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(staticFS)))
	}

	// Default route (all other paths): redirect to the UI home page
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "", http.StatusBadRequest)
			return
		}
		if r.Header.Get("Hx-Request") != "" {
			// Do not redirect htmx requests, those should only request valid paths.
			http.Error(w, "", http.StatusNotFound)
			return
		}
		refererURL, err := url.Parse(r.Header.Get("Referer"))
		if err == nil && refererURL.Host == r.Host {
			// Request is coming from our own domain: this indicates an internal broken link.
			http.Error(w, "Broken link", http.StatusNotFound)
			return
		}
		// Redirect GET to the UI home page.
		http.Redirect(w, r, "/ui/components", http.StatusTemporaryRedirect)
	})

	return mux
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
