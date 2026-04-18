package catalog

import (
	"testing"

	catalog_pb "github.com/dnswlt/swcat/internal/catalog/pb"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestToPB_DomainField(t *testing.T) {
	domainRef := &Ref{Kind: KindDomain, Namespace: "ns", Name: "my-domain"}
	ownerRef := &Ref{Kind: KindGroup, Namespace: "ns", Name: "my-team"}
	systemRef := &Ref{Kind: KindSystem, Namespace: "ns", Name: "my-system"}

	tests := []struct {
		name     string
		entity   Entity
		wantSpec any
	}{
		{
			name: "Component",
			entity: &Component{
				Metadata: &Metadata{Name: "c", Namespace: "ns"},
				Spec: &ComponentSpec{
					Type:      "service",
					Lifecycle: "prod",
					Owner:     ownerRef,
					System:    systemRef,
					inv:       componentInvRel{domain: domainRef},
				},
			},
			wantSpec: &catalog_pb.ComponentSpec{
				Type:      "service",
				Lifecycle: "prod",
				Owner:     refToPB(ownerRef),
				System:    refToPB(systemRef),
				Domain:    refToPB(domainRef),
			},
		},
		{
			name: "API",
			entity: &API{
				Metadata: &Metadata{Name: "a", Namespace: "ns"},
				Spec: &APISpec{
					Type:      "openapi",
					Lifecycle: "stable",
					Owner:     ownerRef,
					System:    systemRef,
					inv:       apiInvRel{domain: domainRef},
				},
			},
			wantSpec: &catalog_pb.ApiSpec{
				Type:      "openapi",
				Lifecycle: "stable",
				Owner:     refToPB(ownerRef),
				System:    refToPB(systemRef),
				Domain:    refToPB(domainRef),
			},
		},
		{
			name: "Resource",
			entity: &Resource{
				Metadata: &Metadata{Name: "r", Namespace: "ns"},
				Spec: &ResourceSpec{
					Type:   "database",
					Owner:  ownerRef,
					System: systemRef,
					inv:    resourceInvRel{domain: domainRef},
				},
			},
			wantSpec: &catalog_pb.ResourceSpec{
				Type:   "database",
				Owner:  refToPB(ownerRef),
				System: refToPB(systemRef),
				Domain: refToPB(domainRef),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pb := ToPB(tt.entity)
			var gotSpec any
			switch tt.name {
			case "Component":
				gotSpec = pb.GetComponentSpec()
			case "API":
				gotSpec = pb.GetApiSpec()
			case "Resource":
				gotSpec = pb.GetResourceSpec()
			}

			if diff := cmp.Diff(tt.wantSpec, gotSpec, protocmp.Transform()); diff != "" {
				t.Errorf("ToPB() %s spec mismatch (-want +got):\n%s", tt.name, diff)
			}
		})
	}
}
