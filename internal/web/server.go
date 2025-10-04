package web

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/dnswlt/swcat/internal/backstage"
)

type ServerOptions struct {
	Addr    string // E.g., "localhost:8080"
	BaseDir string
}

type Server struct {
	opts     ServerOptions
	template *template.Template
	repo     *backstage.Repository
	svgCache map[string]*backstage.SVGResult
}

func NewServer(opts ServerOptions, repo *backstage.Repository) (*Server, error) {
	s := &Server{
		opts:     opts,
		repo:     repo,
		svgCache: make(map[string]*backstage.SVGResult),
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
		"toURL": func(ref string) string {
			kind, name, found := strings.Cut(ref, ":")
			if !found {
				return "#"
			}
			switch kind {
			case "component":
				return "/ui/components/" + url.PathEscape(name)
			case "resource":
				return "/ui/resources/" + url.PathEscape(name)
			case "system":
				return "/ui/systems/" + url.PathEscape(name)
			case "group":
				return "/ui/groups/" + url.PathEscape(name)
			case "domain":
				return "/ui/domains/" + url.PathEscape(name)
			default:
				return "#"
			}
		},
		"urlencode": func(s string) string {
			return url.PathEscape(s)
		},
	})
	var err error
	s.template, err = tmpl.ParseGlob(path.Join(s.opts.BaseDir, "templates/*.html"))
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
	params := map[string]any{}
	system := s.repo.System(systemID)
	if system == nil {
		http.Error(w, "Invalid system", http.StatusBadRequest)
		return
	}
	params["System"] = system

	cacheKey := "system:" + systemID
	svg, ok := s.svgCache[cacheKey]
	if !ok {
		var err error
		svg, err = backstage.GenerateSystemSVG(s.repo, system)
		if err != nil {
			http.Error(w, "Failed to render SVG", http.StatusInternalServerError)
			log.Printf("Failed to render SVG: %v", err)
			return
		}
	}
	params["SVG"] = template.HTML(svg.SVG)
	params["SVGMetadataJSON"] = template.JS(svg.MetadataJSON())
	s.serveHTMLPage(w, r, "system_detail.html", params)
}

func (s *Server) serveComponent(w http.ResponseWriter, r *http.Request, componentID string) {
	params := map[string]any{}
	component := s.repo.Component(componentID)
	if component == nil {
		http.Error(w, "Invalid component", http.StatusBadRequest)
		return
	}
	params["Component"] = component

	cacheKey := "component:" + componentID
	svg, ok := s.svgCache[cacheKey]
	if !ok {
		var err error
		svg, err = backstage.GenerateComponentSVG(s.repo, component)
		if err != nil {
			http.Error(w, "Failed to render SVG", http.StatusInternalServerError)
			log.Printf("Failed to render SVG: %v", err)
			return
		}
		s.svgCache[cacheKey] = svg
	}
	params["SVG"] = template.HTML(svg.SVG)
	params["SVGMetadataJSON"] = template.JS(svg.MetadataJSON())

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
	params := map[string]any{}
	api := s.repo.API(apiID)
	if api == nil {
		http.Error(w, "Invalid API", http.StatusBadRequest)
		return
	}
	params["API"] = api

	cacheKey := "api:" + apiID
	svg, ok := s.svgCache[cacheKey]
	if !ok {
		var err error
		svg, err = backstage.GenerateAPISVG(s.repo, api)
		if err != nil {
			http.Error(w, "Failed to render SVG", http.StatusInternalServerError)
			log.Printf("Failed to render SVG: %v", err)
			return
		}
		s.svgCache[cacheKey] = svg
	}
	params["SVG"] = template.HTML(svg.SVG)
	params["SVGMetadataJSON"] = template.JS(svg.MetadataJSON())

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
	params := map[string]any{}
	resource := s.repo.Resource(resourceID)
	if resource == nil {
		http.Error(w, "Invalid resource", http.StatusBadRequest)
		return
	}
	params["Resource"] = resource

	cacheKey := "resource:" + resourceID
	svg, ok := s.svgCache[cacheKey]
	if !ok {
		var err error
		svg, err = backstage.GenerateResourceSVG(s.repo, resource)
		if err != nil {
			http.Error(w, "Failed to render SVG", http.StatusInternalServerError)
			log.Printf("Failed to render SVG: %v", err)
			return
		}
		s.svgCache[cacheKey] = svg
	}
	params["SVG"] = template.HTML(svg.SVG)
	params["SVGMetadataJSON"] = template.JS(svg.MetadataJSON())

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
	params := map[string]any{}
	domain := s.repo.Domain(domainID)
	if domain == nil {
		http.Error(w, "Invalid domain", http.StatusBadRequest)
		return
	}
	params["Domain"] = domain

	cacheKey := "domain:" + domainID
	svg, ok := s.svgCache[cacheKey]
	if !ok {
		var err error
		svg, err = backstage.GenerateDomainSVG(s.repo, domain)
		if err != nil {
			http.Error(w, "Failed to render SVG", http.StatusInternalServerError)
			log.Printf("Failed to render SVG: %v", err)
			return
		}
		s.svgCache[cacheKey] = svg
	}
	params["SVG"] = template.HTML(svg.SVG)
	params["SVGMetadataJSON"] = template.JS(svg.MetadataJSON())

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
	params := map[string]any{}
	group := s.repo.Group(groupID)
	if group == nil {
		http.Error(w, "Invalid group", http.StatusBadRequest)
		return
	}
	params["Group"] = group
	s.serveHTMLPage(w, r, "group_detail.html", params)
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

func (s *Server) Serve() error {
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

	// Health check. Useful for cloud deployments.
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "OK")
	})

	// Static resources (JavaScript, CSS, etc.)
	staticDir := path.Join(s.opts.BaseDir, "static")
	mux.Handle("GET /static/",
		http.StripPrefix("/static/",
			http.FileServer(http.Dir(staticDir)),
		),
	)

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

	var handler http.Handler = mux
	handler = s.withRequestLogging(handler)

	log.Printf("Go server listening on http://%s", s.opts.Addr)
	return http.ListenAndServe(s.opts.Addr, handler)
}
