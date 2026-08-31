package web

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/dnswlt/swcat/internal/catalog"
	catalog_pb "github.com/dnswlt/swcat/internal/catalog/pb"
	"github.com/dnswlt/swcat/internal/query"
	"google.golang.org/protobuf/encoding/protojson"
)

// serveFindings handles POST /catalog/findings: entity lint findings by
// default, plus whichever catalog-wide scans the request named.
//
// The scans themselves live in scans.go, shared with the lint UI. This handler
// only decides what to run and converts the results to protobuf.
//
// A scan that cannot run does not fail the request. Each section reports its
// own status, so one unreachable external system still leaves the rest usable.
func (s *Server) serveFindings(w http.ResponseWriter, r *http.Request) {
	req, err := parseFindingsRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	data := s.getStoreData(r)
	resp := &catalog_pb.FindingsResponse{}

	if !req.GetLint().GetSkip() {
		resp.Lint = s.lintSection(data, req.GetLint())
	}

	for _, scan := range req.GetScans() {
		switch scan {
		case catalog_pb.Scan_SCAN_PROMETHEUS_WORKLOADS:
			res := s.scanPrometheusWorkloads(r.Context(), data, req.GetPrometheusWorkloads().GetUntrackedOnly())
			out := &catalog_pb.PrometheusWorkloadScan{Status: scanStatusToPB(res.Outcome)}
			for _, wl := range res.Workloads {
				out.Workloads = append(out.Workloads, &catalog_pb.PrometheusWorkload{
					Name:            wl.Name,
					MatchedEntities: refsToPB(wl.Matched),
				})
			}
			resp.PrometheusWorkloads = out

		case catalog_pb.Scan_SCAN_KUBE_WORKLOADS:
			res := s.scanKubeWorkloads(r.Context(), data, req.GetKubeWorkloads().GetUntrackedOnly())
			out := &catalog_pb.KubeWorkloadScan{Status: scanStatusToPB(res.Outcome)}
			for _, wl := range res.Workloads {
				out.Workloads = append(out.Workloads, &catalog_pb.KubeWorkload{
					Kind:            string(wl.Kind),
					Name:            wl.Name,
					Namespace:       wl.Namespace,
					MatchedEntities: refsToPB(wl.Matched),
				})
			}
			resp.KubeWorkloads = out

		case catalog_pb.Scan_SCAN_BITBUCKET_FILES:
			opts := req.GetBitbucketFiles()
			res := s.scanBitbucketFiles(r.Context(), data, opts.GetUntrackedOnly(), opts.GetRefresh())
			out := &catalog_pb.BitbucketFileScan{Status: scanStatusToPB(res.Outcome)}
			for _, f := range res.Results {
				file := &catalog_pb.BitbucketFile{
					ProjectKey: f.File.ProjectKey,
					RepoSlug:   f.File.RepoSlug,
					Path:       f.File.Path,
				}
				if f.Entity != nil {
					file.Entity = catalog.RefToPB(f.Entity.GetRef())
				}
				out.Files = append(out.Files, file)
			}
			resp.BitbucketFiles = out

		case catalog_pb.Scan_SCAN_LINK_CHECK:
			res := s.scanLinks(r.Context(), data, req.GetLinkCheck().GetBrokenOnly())
			out := &catalog_pb.LinkCheckScan{Status: scanStatusToPB(res.Outcome)}
			for _, row := range res.Rows {
				out.Links = append(out.Links, &catalog_pb.LinkCheck{
					Entity: catalog.RefToPB(row.Entity.GetRef()),
					Url:    row.URL,
					Title:  row.Title,
					Type:   row.Type,
					Status: row.Status,
					Reason: row.Reason,
				})
			}
			resp.LinkCheck = out
		}
	}

	output, err := protojson.Marshal(resp)
	if err != nil {
		log.Printf("Failed to marshal FindingsResponse to protojson: %v", err)
		http.Error(w, "JSON marshalling error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.Write(output)
}

// scanStatusToPB reports a scan outcome to API consumers. NOT_CONFIGURED stays
// distinct from an empty result: "this deployment has no Prometheus" is not the
// same answer as "Prometheus has no untracked workloads".
func scanStatusToPB(o scanOutcome) *catalog_pb.ScanStatus {
	switch o.state {
	case scanStateNotConfigured:
		return &catalog_pb.ScanStatus{
			State: catalog_pb.ScanStatus_STATE_NOT_CONFIGURED,
			Error: o.reason,
		}
	case scanStateFailed:
		return &catalog_pb.ScanStatus{
			State: catalog_pb.ScanStatus_STATE_FAILED,
			Error: o.reason,
		}
	default:
		return &catalog_pb.ScanStatus{State: catalog_pb.ScanStatus_STATE_OK}
	}
}

func refsToPB(refs []*catalog.Ref) []*catalog_pb.Ref {
	if len(refs) == 0 {
		return nil
	}
	out := make([]*catalog_pb.Ref, 0, len(refs))
	for _, r := range refs {
		out = append(out, catalog.RefToPB(r))
	}
	return out
}

// lintSection collects the entity-scoped lint findings.
func (s *Server) lintSection(data *storeData, opts *catalog_pb.LintOptions) *catalog_pb.LintFindings {
	if s.linter == nil {
		return &catalog_pb.LintFindings{
			Status: scanStatusToPB(scanNotConfigured("no linter is configured for this catalog")),
		}
	}

	out := &catalog_pb.LintFindings{Status: scanStatusToPB(scanSucceeded())}
	// FindEntities returns entities sorted by ref, so the output is stable.
	for _, e := range data.finder.FindEntities(data.repo, opts.GetQuery()) {
		ref := catalog.RefToPB(e.GetRef())
		for _, f := range s.getFindings(data, e) {
			out.Findings = append(out.Findings, &catalog_pb.Finding{
				Entity:   ref,
				RuleName: f.RuleName,
				Severity: string(f.Severity),
				Message:  f.Message,
			})
		}
	}
	return out
}

// parseFindingsRequest reads and validates the request body. An absent or empty
// body means the default request: entity lint findings and no scans.
func parseFindingsRequest(r *http.Request) (*catalog_pb.FindingsRequest, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body")
	}

	req := &catalog_pb.FindingsRequest{}
	if len(strings.TrimSpace(string(body))) > 0 {
		// Unknown fields are rejected, so a misspelled option is reported
		// rather than silently ignored.
		if err := protojson.Unmarshal(body, req); err != nil {
			return nil, fmt.Errorf("failed to parse request body as FindingsRequest: %v", err)
		}
	}

	requested := make(map[catalog_pb.Scan]bool, len(req.GetScans()))
	for _, scan := range req.GetScans() {
		// protojson accepts a numeric enum value it has never heard of, so an
		// undeclared number would otherwise fall through the handler's switch
		// and produce a response that looks as though the scan had run.
		if _, declared := catalog_pb.Scan_name[int32(scan)]; !declared {
			return nil, fmt.Errorf("unknown scan %d in 'scans'", int32(scan))
		}
		if scan == catalog_pb.Scan_SCAN_UNSPECIFIED {
			return nil, fmt.Errorf("'scans' must name a scan, not SCAN_UNSPECIFIED")
		}
		// The handler runs one scan per entry, so a repeat would call the
		// external system again and overwrite its own section: the work is done
		// twice for no extra result.
		if requested[scan] {
			return nil, fmt.Errorf("scan %s is listed more than once in 'scans'", scan)
		}
		requested[scan] = true
	}

	// Options for a scan that was not requested are rejected rather than
	// ignored: silently dropping them would let a caller believe it had
	// configured a scan that never ran.
	for _, o := range []struct {
		set  bool
		scan catalog_pb.Scan
		name string
	}{
		{req.GetPrometheusWorkloads() != nil, catalog_pb.Scan_SCAN_PROMETHEUS_WORKLOADS, "prometheusWorkloads"},
		{req.GetKubeWorkloads() != nil, catalog_pb.Scan_SCAN_KUBE_WORKLOADS, "kubeWorkloads"},
		{req.GetBitbucketFiles() != nil, catalog_pb.Scan_SCAN_BITBUCKET_FILES, "bitbucketFiles"},
		{req.GetLinkCheck() != nil, catalog_pb.Scan_SCAN_LINK_CHECK, "linkCheck"},
	} {
		if o.set && !requested[o.scan] {
			return nil, fmt.Errorf("'%s' options given, but %s is not listed in 'scans'", o.name, o.scan)
		}
	}

	// An unparsable query would otherwise match no entities, which reads as a
	// clean catalog rather than as a broken request.
	if q := req.GetLint().GetQuery(); q != "" {
		if _, err := query.Parse(q); err != nil {
			return nil, fmt.Errorf("invalid 'lint.query': %v", err)
		}
	}

	return req, nil
}
